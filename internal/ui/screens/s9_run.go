package screens

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// The run of the plan (실행): plan.Runner in a goroutine, its state read every
// half second (state.json and the logs are the record; the screen shows a
// snapshot of the runner). The run lives as long as the program: quitting stops
// it (Stop), and state.json lets the next start go on from where it stopped.

// runTickMsg refreshes the view of a run.
type runTickMsg struct{ run *plan.Runner }

// runDoneMsg ends a run: its outcome, or why it did not start.
type runDoneMsg struct {
	run     *plan.Runner
	outcome plan.Outcome
	err     error
}

// tailLines is how much of the log of a running step is shown.
const tailLines = 10

// execRun is a run under way, or the last one.
type execRun struct {
	runner  *plan.Runner
	stages  [][]plan.Step
	byID    map[string]plan.Step
	started time.Time
	ended   chan struct{} // closed when Run has returned
	done    bool
	outcome plan.Outcome
	err     error
	snap    plan.RunState
	presses int // Ctrl+C during the run
	sel     int // the step l shows, an index of snap.Steps
}

// SetExec replaces how the steps of a run are run (tests).
func (s *S9Install) SetExec(e plan.StreamFunc) { s.exec = e }

// inputs are the site files a run is made from (plan.InputHash).
func (s *S9Install) inputs() []string {
	return []string{s.paths.Inventory, s.paths.Vars, s.paths.AipubVars, s.paths.WebhookConfig()}
}

func (s *S9Install) running() bool { return s.ex != nil && !s.ex.done }

// loadPrev reads state.json of the installer: the last run, which 실행 goes on
// with. A run that did not finish is planned as it started: whether the cluster
// and AIPub were there then, not what the files it saved say now.
func (s *S9Install) loadPrev() {
	s.opts.Existing, s.opts.AipubInstalled, s.opts.SiteWhy = s.base.Existing, s.base.AipubInstalled, s.base.SiteWhy
	s.last = nil
	prev, err := plan.LoadRunState(plan.StateDir(s.opts))
	if err != nil {
		s.message = i18n.T("Could not read the record of the last run: %s", err.Error())
		return
	}
	s.last = prev
	if prev != nil && prev.Outcome != plan.OutcomeOK {
		s.opts.Existing, s.opts.AipubInstalled = prev.Existing, prev.AipubInstalled
		s.opts.SiteWhy = plan.ResumedSite(prev, s.base.SiteWhy)
		// and what the operator chose for it: 이어서 실행 runs that plan
		s.opts.Monitoring, s.opts.ArchivePath = prev.Monitoring, prev.ArchivePath
		s.archive.SetValue(prev.ArchivePath)
	}
}

// detectSite decides the site again as it is now (plan.Options.Detect), for a
// run from the first step: the record of k8s-installer may have appeared since
// the program started or since the unfinished run (K3 built the cluster), or
// gone (reset-cluster.yml). A plan from what the last run saw would run
// setup-cluster on the live cluster. Without Detect the site is the one the
// program was started with, never the last run's.
func (s *S9Install) detectSite() {
	if s.base.Detect != nil {
		site := s.base.Detect()
		// --aipub-installed means nothing without a cluster (main refuses it)
		s.base.Existing, s.base.AipubInstalled, s.base.SiteWhy = site.Existing, site.AipubInstalled && site.Existing, site.Why
	}
	s.opts.Existing, s.opts.AipubInstalled, s.opts.SiteWhy = s.base.Existing, s.base.AipubInstalled, s.base.SiteWhy
	s.rebuild()
}

// findArchive fills an empty 아카이브 경로 of a site that needs it
// (plan.NeedsArchive) with the one archive found where a package is usually
// unpacked (plan.ArchiveDirs); several are listed and none is chosen.
func (s *S9Install) findArchive() {
	if strings.TrimSpace(s.archive.Value()) != "" || !plan.NeedsArchive(s.appState.AipubEnabled, s.opts) || s.archiveDirs == nil {
		return
	}
	found := plan.FindArchive(s.archiveDirs())
	switch len(found) {
	case 0:
	case 1:
		s.archive.SetValue(found[0])
		s.message = i18n.T("Found the archive in %s and put it in the archive path: %s", filepath.Dir(found[0]), found[0])
	default:
		s.message = i18n.T("%s is in several places: %s — put the one to use in the archive path", plan.ArchiveName, strings.Join(found, ", "))
	}
}

// pressRun is 실행: through the checks (gate), then the run, from where the
// last one stopped; from the first step when there is nothing to go on with,
// for the site as it is now.
func (s *S9Install) pressRun() tea.Cmd {
	s.fresh = false
	if s.last == nil || len(s.last.Remaining()) == 0 {
		s.detectSite()
	}
	return s.gate(false)
}

// pressFresh is f: through the checks too, then asked, the run from the first
// step, planned for the site as it is now.
func (s *S9Install) pressFresh() tea.Cmd {
	if s.last == nil || s.running() {
		return nil
	}
	s.fresh = true
	s.detectSite()
	return s.gate(false)
}

// resuming is whether the run 실행 starts now goes on with the last one.
func (s *S9Install) resuming() bool {
	return !s.fresh && s.last != nil && len(s.last.Remaining()) > 0
}

// k4Kept is whether the run that starts now keeps a K4 of the last run that
// succeeded, and with it the archive path of that run.
func (s *S9Install) k4Kept() bool {
	if !s.resuming() {
		return false
	}
	k4 := s.last.Step("K4")
	return k4 != nil && k4.Status == plan.StatusOK
}

// archiveGate is the last stop before a run: an archive path that is given
// must be a file this user reads, and a site that needs one (plan.NeedsArchive)
// does not start without it unless the operator says so (waived, the
// no-archive question, no by default). A run that keeps a K4 that succeeded
// needs nothing.
func (s *S9Install) archiveGate(waived bool) tea.Cmd {
	if !s.k4Kept() {
		path := strings.TrimSpace(s.archive.Value())
		switch {
		case path != "":
			if err := plan.CheckArchive(path); err != nil {
				s.message = i18n.T("Not run: %s. Fix the archive path, then run", err.Error())
				return nil
			}
		case !waived && plan.NeedsArchive(s.appState.AipubEnabled, s.opts):
			s.confirm = "no-archive"
			return nil
		}
	}
	if s.fresh {
		s.confirm = "fresh"
		return nil
	}
	return s.offerRun()
}

// gate lets a run start only on checks that passed (none ✗) for the site as it
// is now: P3 keeps update-cluster from wiping what was set by hand, P6 gives the
// shell every playbook becomes with. Checks of another site (checksFor) are
// dropped; checks not run yet run first, and checks under way are waited for,
// then the run goes on (runAfterChecks). after: the checks it waited for have
// ended.
func (s *S9Install) gate(after bool) tea.Cmd {
	s.runAfterChecks = false
	if s.running() {
		return nil
	}
	if s.checking {
		s.runAfterChecks = true
		s.message = i18n.T("The run starts once the checks end")
		return nil
	}
	if s.checksFor != s.checkKey() {
		s.checks, s.ssh = map[string]plan.CheckResult{}, nil
	}
	notRun, failed := plan.CheckVerdict(s.steps, s.checks)
	switch {
	case len(failed) > 0:
		s.message = i18n.T("Checks ✗: %s. Fix them, check again with c, then run", strings.Join(failed, " "))
		return nil
	case len(notRun) > 0 && after:
		// they ran and still gave nothing: never loop on them
		s.message = i18n.T("No check results (%s). Check again with c, then run", strings.Join(notRun, " "))
		return nil
	case len(notRun) > 0:
		s.runAfterChecks = true
		s.message = i18n.T("Running the checks first (%s). The run starts as soon as they pass", strings.Join(notRun, " "))
		return s.runChecks()
	}
	return s.archiveGate(false)
}

// checkKey is what the checks were made for: the site files and the options
// that change them (P3 only on an existing site, the installer root).
func (s *S9Install) checkKey() string {
	return fmt.Sprintf("%s|%t|%s", plan.InputHash(s.inputs()), s.opts.Existing, s.opts.Root)
}

// offerRun starts the run, or asks first: when the last one finished (start
// again from the first step?), or the site files changed since it stopped.
func (s *S9Install) offerRun() tea.Cmd {
	switch {
	case s.last == nil:
		return s.startRun(false)
	case len(s.last.Remaining()) == 0:
		s.confirm = "fresh"
		return nil
	case s.last.InputHash != plan.InputHash(s.inputs()):
		s.confirm = "resume"
		return nil
	}
	return s.startRun(true)
}

// confirmQuestion is asked after the checks passed (gate): y starts the run.
func (s *S9Install) confirmQuestion() string {
	switch s.confirm {
	case "reset-cluster":
		return i18n.T("reset-cluster.yml removes the cluster from every node of the inventory (kubeadm reset, ki cp services, containerd·packages, certificates). "+
			"This cannot be undone. To reset, type reset and press Enter; anything else cancels: %s", s.resetTyped)
	case "no-archive":
		return i18n.T("A new site with an empty archive path. The user registry K2 makes is empty, so without K4 the steps from A1 setup-network "+
			"cannot pull their images and fail. Put the path of %s in the archive path. Run without K4 anyway? (y yes, n·Enter no)", plan.ArchiveName)
	}
	if s.confirm == "resume" {
		return i18n.T("The site files changed since the last run. Resume, without running the finished steps again? (y yes, n no)")
	}
	if s.last != nil && len(s.last.Remaining()) == 0 {
		return i18n.T("The last run finished every step. Run all the steps again from the start? (y yes, n no)")
	}
	return i18n.T("Run all the steps again from the start, the finished ones too? (y yes, n no)")
}

type resetDoneMsg struct {
	err error
}

// resetOffered is whether x may reset the cluster now: no run under way, and
// the last run failed in K1..K3 of a new site (plan.ResetHelps), where the
// cluster is still being built. Never after K3, never on an existing site.
//
// Nor when the site as it is now is not plainly new: the run may have been
// planned new on a control node whose record could not be read, or whose
// cluster was built by something else, and K3 then fails on "Kubelet already
// enabled" exactly because a cluster is running there. The record of
// k8s-installer has to be looked at and absent for a reset to be offered.
//
// Not while the checks run or a run waits for them (the gate would ask its own
// question over this one), nor while a reset is being prepared or runs. Without
// a way to look at the record (no Detect) it is not offered at all.
func (s *S9Install) resetOffered() bool {
	if s.running() || s.resetting || s.checking || s.runAfterChecks || s.base.Detect == nil ||
		s.last == nil || s.last.Outcome != plan.OutcomeFailed {
		return false
	}
	for _, st := range s.last.Steps {
		if st.Status != plan.StatusFailed {
			continue
		}
		if !plan.ResetHelps(s.last.Existing, st.ID) {
			return false
		}
		site := s.base.Detect()
		return !site.Existing && !site.Unknown
	}
	return false
}

// resetReadyMsg is the end of what is checked between the typed word and the
// reset (plan.PrepareReset): release gives back the lock of the runs, held
// until the reset has ended.
type resetReadyMsg struct {
	release func()
	err     error
}

// prepareReset checks, at the moment the word was typed, that nothing else
// runs this installation, that the record of the run is still the one shown,
// and that no node holds a built cluster (plan.PrepareReset). It asks every
// node, so it is a command, not a call.
func (s *S9Install) prepareReset() tea.Cmd {
	s.resetting = true
	s.message = i18n.T("Checking before the reset... (other runs, the run record, the cluster record of every node)")
	opts, shown, run := s.opts, s.last, s.run
	opts.Detect = s.base.Detect
	return func() tea.Msg {
		release, err := plan.PrepareReset(context.Background(), opts, shown, run)
		return resetReadyMsg{release: release, err: err}
	}
}

// resetReady takes the answer of prepareReset: the reset runs only when
// everything held, with the lock of the runs kept until it ends.
func (s *S9Install) resetReady(msg resetReadyMsg) tea.Cmd {
	if msg.err != nil {
		s.resetting = false
		s.message = i18n.T("✗ Not reset: %s", msg.err.Error())
		return nil
	}
	s.resetRelease = msg.release
	return s.runResetCluster()
}

// runResetCluster runs reset-cluster.yml (plan.ResetScript) on the terminal,
// as this user like the steps: not under sudo, whose root has neither the key
// the nodes take nor this user's known_hosts.
func (s *S9Install) runResetCluster() tea.Cmd {
	if s.resetCmd != nil {
		return s.resetCmd()
	}
	cmd := exec.Command("bash", "-c", plan.ResetScript(s.opts))
	cmd.Dir = s.opts.AnsibleDir()
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return resetDoneMsg{err: err}
	})
}

// resetDone takes the end of reset-cluster.yml. After it the steps the last
// run records as ok are undone (K2's ki cp services among them), so its record
// is set aside: 실행 is then a run from the first step, planned for the site
// as it is now, which is a new site again (the record of the cluster is gone).
func (s *S9Install) resetDone(err error) {
	if s.resetRelease != nil {
		s.resetRelease() // the lock of the runs, held through the reset
		s.resetRelease = nil
	}
	s.resetting = false
	log := plan.StateDir(s.opts) + "/logs/reset-cluster.log"
	if err != nil {
		s.message = i18n.T("✗ Resetting the cluster failed: %s (log %s)", err.Error(), log)
		return
	}
	if err := plan.SetAsideRun(plan.StateDir(s.opts)); err != nil {
		s.message = i18n.T("✓ The cluster was reset. The record of the last run could not be set aside (%s): run from the start with f", err.Error())
		return
	}
	s.ex, s.selMoved, s.last = nil, false, nil
	s.detectSite()
	s.message = i18n.T("✓ The cluster was reset (log %s). Run now starts from K1", log)
}

func (s *S9Install) answer(yes bool) tea.Cmd {
	c := s.confirm
	s.confirm, s.resetTyped = "", ""
	if !yes {
		switch c {
		case "reset-cluster":
			s.message = i18n.T("The cluster reset was cancelled")
		case "no-archive":
			s.message = i18n.T("Not run. Put the path of %s in the archive path, then run", plan.ArchiveName)
		default:
			s.message = i18n.T("Not run")
		}
		return nil
	}
	switch c {
	case "reset-cluster":
		// what was true when x was pressed may not be now
		if !s.resetOffered() {
			s.message = i18n.T("Not reset: the cluster cannot be reset now")
			return nil
		}
		return s.prepareReset()
	case "no-archive":
		return s.archiveGate(true)
	}
	return s.startRun(c == "resume")
}

// startRun runs the stages of the plan as it is shown, with resume from the
// last run, whose 모니터링 a resume keeps, and its 아카이브 경로 when its K4
// succeeded (said when they were changed on the screen since). A K4 that did not
// succeed (skipped without an archive, failed) runs with the path given now,
// before the step the last run stopped at.
func (s *S9Install) startRun(resume bool) tea.Cmd {
	var kept []string
	if resume && s.last != nil {
		if s.opts.Monitoring != s.last.Monitoring {
			mon := i18n.T("monitoring: don't install")
			if s.last.Monitoring {
				mon = i18n.T("monitoring: install")
			}
			kept = append(kept, mon)
		}
		s.opts.Monitoring = s.last.Monitoring
		if k4 := s.last.Step("K4"); k4 != nil && k4.Status == plan.StatusOK {
			if plan.AbsArchive(s.archive.Value()) != s.last.ArchivePath {
				kept = append(kept, i18n.T("archive path %s (K4 done)", s.last.ArchivePath))
			}
			s.archive.SetValue(s.last.ArchivePath)
		}
	}
	s.rebuild()
	stages := plan.Stages(s.steps, s.opts, s.savedIn())
	r := plan.NewRunner(s.opts, s.appState, stages, s.inputs())
	r.Cmd = s.run
	if s.exec != nil {
		r.Exec = s.exec
	}
	ex := &execRun{runner: r, stages: stages, byID: map[string]plan.Step{}, started: time.Now(), ended: make(chan struct{})}
	for _, stage := range stages {
		for _, st := range stage {
			ex.byID[st.ID] = st
		}
	}
	s.ex, s.message, s.showLog, s.rechecking = ex, "", false, false
	s.active.Store(ex)
	if len(kept) > 0 {
		s.message = i18n.T("A resume runs with the settings of the last run: %s (to change them, f from the start)", strings.Join(kept, ", "))
	}
	done := make(chan runDoneMsg, 1)
	go func() {
		defer close(ex.ended)
		o, err := r.Run(context.Background(), resume)
		done <- runDoneMsg{run: r, outcome: o, err: err}
	}()
	return tea.Batch(func() tea.Msg { return <-done }, s.tick(r))
}

func (s *S9Install) tick(r *plan.Runner) tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return runTickMsg{run: r} })
}

// updateRun takes the messages of a run; false when msg is none of them.
func (s *S9Install) updateRun(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case runTickMsg:
		if s.ex == nil || msg.run != s.ex.runner || s.ex.done {
			return nil, true
		}
		s.refresh()
		return s.tick(msg.run), true
	case runDoneMsg:
		if s.ex == nil || msg.run != s.ex.runner {
			return nil, true
		}
		s.ex.done, s.ex.outcome, s.ex.err = true, msg.outcome, msg.err
		s.refresh()
		// a run stopped before it began left state.json as it was
		if msg.err == nil && len(s.ex.snap.Steps) > 0 {
			snap := s.ex.snap
			s.last = &snap
		}
		s.inputHash = plan.InputHash(s.inputs())
		return nil, true
	}
	return nil, false
}

// refresh takes the state of the runner; the selection follows the step that
// runs, waits or failed, until the operator moves it.
func (s *S9Install) refresh() {
	s.ex.snap = s.ex.runner.Snapshot()
	// the word of Enter at the StorageClass stop, gone once nothing waits
	if _, waiting := s.waitingStep(); !waiting && s.rechecking {
		s.message, s.rechecking = "", false
	}
	for i, st := range s.ex.snap.Steps {
		if st.Status == plan.StatusRunning || st.Status == plan.StatusWaiting || st.Status == plan.StatusFailed {
			if !s.selMoved {
				s.ex.sel = i
			}
			break
		}
	}
}

// recheckMessage is shown from Enter at the StorageClass stop until the step
// no longer waits (S9Install.rechecking: the message is this one, whatever its
// language).
const recheckMessage = "Checking the StorageClass again..."

// waitingStep is the Manual step that waits for the operator, if any.
func (s *S9Install) waitingStep() (plan.StepState, bool) {
	if s.ex == nil {
		return plan.StepState{}, false
	}
	for _, st := range s.ex.snap.Steps {
		if st.Status == plan.StatusWaiting {
			return st, true
		}
	}
	return plan.StepState{}, false
}

// runKey is a key on the run view.
func (s *S9Install) runKey(msg tea.KeyMsg) tea.Cmd {
	if s.showLog {
		switch msg.String() {
		case "l", "esc", "q":
			s.showLog = false
			return nil
		}
		var cmd tea.Cmd
		s.logVP, cmd = s.logVP.Update(msg)
		return cmd
	}
	n := len(s.ex.snap.Steps)
	switch msg.String() {
	case "up":
		if s.ex.sel > 0 {
			s.ex.sel--
			s.selMoved = true
		}
	case "down":
		if s.ex.sel < n-1 {
			s.ex.sel++
			s.selMoved = true
		}
	case "l":
		if s.ex.sel < n {
			s.openLog(s.ex.snap.Steps[s.ex.sel])
		}
	case "enter":
		if _, ok := s.waitingStep(); ok && !s.ex.done {
			s.ex.runner.Continue()
			s.message, s.rechecking = i18n.T(recheckMessage), true
		}
	case "r":
		if s.ex.done && s.ex.outcome == plan.OutcomeFailed && s.confirm == "" {
			return s.pressRun()
		}
	case "x":
		if s.ex.done && s.confirm == "" && s.resetOffered() {
			s.confirm, s.resetTyped, s.message = "reset-cluster", "", ""
			return nil
		}
	case "esc":
		if s.ex.done {
			s.ex, s.selMoved, s.message, s.rechecking = nil, false, "", false
		}
	}
	return nil
}

func (s *S9Install) openLog(st plan.StepState) {
	if st.Log == "" {
		s.message, s.rechecking = i18n.T("%s has no log yet", st.ID), false
		return
	}
	s.logStep = st
	s.logVP = viewport.New(s.width, max(5, s.height-3))
	lines := plan.TailLines(st.Log, 5000)
	s.logVP.SetContent(Wrap(strings.Join(lines, "\n"), max(1, s.width)))
	s.logVP.GotoBottom()
	s.showLog = true
}

// interruptRun is Ctrl+C during a run: the first stops after the running steps,
// the second stops them now; the third quits (false).
func (s *S9Install) interruptRun() bool {
	s.ex.presses++
	switch s.ex.presses {
	case 1:
		s.ex.runner.Stop()
		s.message, s.rechecking = i18n.T("Stops once the running steps end. Ctrl+C again stops now"), false
		return true
	case 2:
		s.ex.runner.Kill()
		s.message, s.rechecking = i18n.T("Stopping the running steps... (Ctrl+C again quits)"), false
		return true
	}
	return false
}

// stopRun ends a run before the program quits, in a bounded time. It runs off
// the update loop (a tea.Cmd, or after the program ended), so it reads only
// active and what of the run does not change (runner, ended), never s.ex.
func (s *S9Install) stopRun() {
	ex := s.active.Load()
	if ex == nil {
		return
	}
	select {
	case <-ex.ended:
		return
	default:
	}
	ex.runner.Kill()
	select {
	case <-ex.ended:
	case <-time.After(20 * time.Second):
	}
}

func elapsed(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

func statusStyle(st plan.StepStatus) lipgloss.Style {
	switch st {
	case plan.StatusOK:
		return styles.StyleSuccess
	case plan.StatusFailed, plan.StatusCancelled:
		return styles.StyleError
	case plan.StatusWaiting:
		return styles.StyleWarning
	case plan.StatusRunning:
		return styles.StylePrimary
	case plan.StatusSkipped:
		return styles.StyleMuted
	}
	return lipgloss.NewStyle()
}

// viewRun is the run: every step with its state and time, the instructions of
// the StorageClass stop, the end of the log of what runs, and at the end what
// came of it.
func (s *S9Install) viewRun() string {
	if s.showLog {
		head := styles.StyleTitle.Render(i18n.T("Log %s %s", s.logStep.ID, s.logStep.Title)) +
			styles.StyleMuted.Render("   "+s.logStep.Log+"   "+i18n.T("↑/↓ scroll   l·Esc close"))
		return head + "\n\n" + s.logVP.View()
	}
	ex, now := s.ex, time.Now()
	var b strings.Builder
	took := now.Sub(ex.started)
	if ex.done {
		took = ex.snap.Updated.Sub(ex.started)
	}
	b.WriteString(styles.StyleTitle.Render(i18n.T("Install run")) + styles.StyleMuted.Render("   "+i18n.T("elapsed %s   state·logs: %s", elapsed(took), plan.StateDir(s.opts))) + "\n")
	b.WriteString(styles.StyleMuted.Render("  "+i18n.T("· pending  ▶ running  ✓ ok  ✗ failed  ⏸ waiting  - skipped. Do not run playbooks by hand in another terminal")) + "\n\n")
	focus := 0
	for i, st := range ex.snap.Steps {
		mark := "  "
		if i == ex.sel {
			mark = "› "
			focus = strings.Count(b.String(), "\n")
		}
		took := ""
		if d := st.Elapsed(now); st.Start != nil {
			took = elapsed(d)
		}
		line := fmt.Sprintf("%s%s %-4s %-34s %6s", mark, st.Status.Icon(), st.ID, st.Title, took)
		b.WriteString(statusStyle(st.Status).Render(line) + "\n")
		step := ex.byID[st.ID]
		switch {
		case st.Status == plan.StatusWaiting:
			b.WriteString(styles.StyleWarning.Render("        "+plan.ManualPrompt(step)) + "\n")
			b.WriteString(styles.StyleMuted.Render("        "+step.Note) + "\n")
			b.WriteString(styles.StyleWarning.Render("        "+st.Note) + "\n")
			b.WriteString(styles.StylePrimary.Render("        "+i18n.T("Once installed, Enter (installed, continue) — it is checked again")) + "\n")
		case st.Status == plan.StatusFailed && st.Note != "":
			b.WriteString(styles.StyleWarning.Render("        "+st.Note) + "\n")
		case st.Note != "" && st.Status != plan.StatusOK:
			b.WriteString(styles.StyleMuted.Render("        "+st.Note) + "\n")
		}
	}
	for _, st := range ex.snap.Steps {
		if st.Status == plan.StatusRunning && ex.byID[st.ID].Kind != plan.Manual {
			b.WriteString("\n" + styles.StyleMuted.Render(i18n.T("── log %s (last %d lines) ──", st.ID, tailLines)) + "\n")
			for _, l := range plan.TailLines(st.Log, tailLines) {
				b.WriteString("  " + l + "\n")
			}
		}
	}
	if ex.done {
		b.WriteString("\n" + s.summary())
	}
	if s.confirm != "" {
		b.WriteString("\n" + styles.StyleWarning.Render("! "+s.confirmQuestion()) + "\n")
	}
	if s.message != "" {
		b.WriteString("\n" + styles.StylePrimary.Render(s.message) + "\n")
	}
	keys := "  " + i18n.T("↑/↓ step   l log")
	switch {
	case ex.done:
		if ex.outcome == plan.OutcomeFailed {
			keys += "   " + i18n.T("r retry the failed step")
		}
		if s.resetOffered() {
			keys += "   " + i18n.T("x reset the cluster")
		}
		keys += "   " + i18n.T("Esc to the plan (Run resumes)")
	default:
		if _, ok := s.waitingStep(); ok {
			keys += "   " + i18n.T("Enter installed, continue")
		}
		keys += "   " + i18n.T("Ctrl+C stop (once: after the running steps, twice: now)")
	}
	b.WriteString("\n" + styles.StyleMuted.Render(keys))
	return FitView(b.String(), focus, focus+5, s.width, s.height)
}

// summary is what came of the run.
func (s *S9Install) summary() string {
	ex := s.ex
	var b strings.Builder
	switch {
	case ex.err != nil:
		b.WriteString(styles.StyleError.Render(i18n.T("✗ Could not start: %s", ex.err.Error())) + "\n")
		return b.String()
	}
	if err := ex.runner.SaveErr(); err != nil {
		b.WriteString(styles.StyleWarning.Render(i18n.T("! Could not write %s/state.json (%s). The record may be behind the run",
			plan.StateDir(s.opts), err.Error())) + "\n")
	}
	switch {
	case ex.outcome == plan.OutcomeOK:
		b.WriteString(styles.StyleSuccess.Render(i18n.T("✓ The install finished")) + "\n")
		return b.String()
	case ex.outcome == plan.OutcomeStopped:
		b.WriteString(styles.StyleWarning.Render(i18n.T("■ Stopped. Run resumes from the steps not finished")) + "\n")
	case ex.snap.WentOn():
		// to its end, past a step that stays failed (plan.informerOnly)
		b.WriteString(styles.StyleWarning.Render(i18n.T("■ Ran to the end, but the steps below stay failed. Once the cause is fixed, Run runs only those steps again")) + "\n")
	default:
		b.WriteString(styles.StyleError.Render(i18n.T("✗ Failed. Once fixed, Run resumes from the failed step")) + "\n")
	}
	for _, st := range ex.snap.Steps {
		if st.Status != plan.StatusFailed {
			continue
		}
		code := ""
		if st.ExitCode != nil {
			code = " " + i18n.T("(exit code %d)", *st.ExitCode)
		}
		b.WriteString(styles.StyleError.Render("✗ "+st.ID+" "+st.Title+code+"  "+i18n.T("log: %s", st.Log)) + "\n")
		if st.Note != "" {
			b.WriteString(styles.StyleWarning.Render("  │ "+st.Note) + "\n")
		}
		for _, l := range plan.TailLines(st.Log, 5) {
			b.WriteString(styles.StyleMuted.Render("  │ "+l) + "\n")
		}
	}
	return b.String()
}

// viewPrev is the last run on the plan view: what is done, what 실행 does next.
func (s *S9Install) viewPrev() string {
	if s.last == nil {
		return ""
	}
	var done, left []string
	for _, st := range s.last.Steps {
		switch st.Status {
		case plan.StatusOK:
			done = append(done, st.ID)
		case plan.StatusSkipped:
		default:
			left = append(left, st.Status.Icon()+st.ID)
		}
	}
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render(i18n.T("Last run")) +
		styles.StyleMuted.Render("  "+i18n.T("started %s, %s", s.last.Started.Format("2006-01-02 15:04"), plan.StateDir(s.opts))) + "\n")
	if len(done) > 0 {
		b.WriteString(styles.StyleSuccess.Render("  "+i18n.T("✓ done: %s", strings.Join(done, " "))) + "\n")
	}
	if len(left) == 0 {
		b.WriteString(styles.StyleMuted.Render("  "+i18n.T("All done. f runs it again from the start")) + "\n")
	} else {
		b.WriteString(styles.StyleWarning.Render("  "+i18n.T("left: %s — Run = resume, f from the start", strings.Join(left, " "))) + "\n")
		if s.last.InputHash != s.inputHash {
			b.WriteString(styles.StyleWarning.Render("  "+i18n.T("! The site files changed since the last run (asked on resume)")) + "\n")
		}
	}
	return b.String()
}
