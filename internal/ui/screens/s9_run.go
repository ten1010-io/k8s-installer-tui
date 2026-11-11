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
		s.message = "지난 실행 기록을 읽지 못했습니다: " + err.Error()
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
		s.message = "아카이브를 " + filepath.Dir(found[0]) + " 에서 찾아 아카이브 경로에 넣었습니다: " + found[0]
	default:
		s.message = plan.ArchiveName + " 가 여러 곳에 있습니다: " + strings.Join(found, ", ") + " — 쓸 것을 아카이브 경로에 넣으세요"
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
				s.message = "실행하지 않았습니다: " + err.Error() + ". 아카이브 경로를 고친 뒤 실행하세요"
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
		s.message = "사전 확인이 끝나면 실행합니다"
		return nil
	}
	if s.checksFor != s.checkKey() {
		s.checks, s.ssh = map[string]plan.CheckResult{}, nil
	}
	notRun, failed := plan.CheckVerdict(s.steps, s.checks)
	switch {
	case len(failed) > 0:
		s.message = "사전 확인 ✗: " + strings.Join(failed, " ") + ". 고친 뒤 c 로 다시 확인하고 실행하세요"
		return nil
	case len(notRun) > 0 && after:
		// they ran and still gave nothing: never loop on them
		s.message = "사전 확인 결과가 없습니다 (" + strings.Join(notRun, " ") + "). c 로 다시 확인하고 실행하세요"
		return nil
	case len(notRun) > 0:
		s.runAfterChecks = true
		s.message = "사전 확인을 먼저 실행합니다 (" + strings.Join(notRun, " ") + "). 통과하면 바로 실행합니다"
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
		return "reset-cluster.yml 로 인벤토리의 모든 노드에서 클러스터를 지웁니다 (kubeadm reset, ki cp 서비스, containerd·패키지, 인증서). " +
			"되돌릴 수 없습니다. 초기화하려면 reset 을 입력하고 Enter, 그 밖에는 취소: " + s.resetTyped
	case "no-archive":
		return "새 사이트인데 아카이브 경로가 비어 있습니다. K2 가 만드는 사용자 레지스트리는 비어 있어, K4 없이는 A1 setup-network 부터 " +
			"이미지를 받지 못해 실패합니다. 아카이브 경로 칸에 " + plan.ArchiveName + " 의 경로를 넣으세요. 그래도 K4 없이 실행할까요? (y 예, n·Enter 아니오)"
	}
	if s.confirm == "resume" {
		return "지난 실행 뒤에 사이트 파일이 바뀌었습니다. 끝난 단계는 다시 실행하지 않고 이어서 실행할까요? (y 예, n 아니오)"
	}
	if s.last != nil && len(s.last.Remaining()) == 0 {
		return "지난 실행이 모두 끝났습니다. 처음부터 모든 단계를 다시 실행할까요? (y 예, n 아니오)"
	}
	return "끝난 단계도 모두 처음부터 다시 실행할까요? (y 예, n 아니오)"
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
	s.message = "초기화 전 확인 중... (다른 실행, 실행 기록, 모든 노드의 클러스터 기록)"
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
		s.message = "✗ 초기화하지 않았습니다: " + msg.err.Error()
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
		s.message = "✗ 클러스터 초기화 실패: " + err.Error() + " (로그 " + log + ")"
		return
	}
	if err := plan.SetAsideRun(plan.StateDir(s.opts)); err != nil {
		s.message = "✓ 클러스터를 초기화했습니다. 지난 실행 기록을 치우지 못했습니다 (" + err.Error() + "): f 로 처음부터 실행하세요"
		return
	}
	s.ex, s.selMoved, s.last = nil, false, nil
	s.detectSite()
	s.message = "✓ 클러스터를 초기화했습니다 (로그 " + log + "). 실행을 누르면 처음부터 (K1 부터) 실행합니다"
}

func (s *S9Install) answer(yes bool) tea.Cmd {
	c := s.confirm
	s.confirm, s.resetTyped = "", ""
	if !yes {
		switch c {
		case "reset-cluster":
			s.message = "클러스터 초기화를 취소했습니다"
		case "no-archive":
			s.message = "실행하지 않았습니다. 아카이브 경로 칸에 " + plan.ArchiveName + " 의 경로를 넣고 실행하세요"
		default:
			s.message = "실행하지 않았습니다"
		}
		return nil
	}
	switch c {
	case "reset-cluster":
		// what was true when x was pressed may not be now
		if !s.resetOffered() {
			s.message = "초기화하지 않았습니다: 지금은 초기화할 수 있는 상태가 아닙니다"
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
			mon := "설치 안 함"
			if s.last.Monitoring {
				mon = "설치"
			}
			kept = append(kept, "모니터링 "+mon)
		}
		s.opts.Monitoring = s.last.Monitoring
		if k4 := s.last.Step("K4"); k4 != nil && k4.Status == plan.StatusOK {
			if plan.AbsArchive(s.archive.Value()) != s.last.ArchivePath {
				kept = append(kept, "아카이브 경로 "+s.last.ArchivePath+" (K4 끝남)")
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
	s.ex, s.message, s.showLog = ex, "", false
	s.active.Store(ex)
	if len(kept) > 0 {
		s.message = "이어서 실행은 지난 실행의 설정으로 합니다: " + strings.Join(kept, ", ") + " (바꾸려면 f 처음부터)"
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
	if _, waiting := s.waitingStep(); !waiting && s.message == recheckMessage {
		s.message = ""
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
// no longer waits.
const recheckMessage = "StorageClass 를 다시 확인합니다..."

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
			s.message = recheckMessage
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
			s.ex, s.selMoved, s.message = nil, false, ""
		}
	}
	return nil
}

func (s *S9Install) openLog(st plan.StepState) {
	if st.Log == "" {
		s.message = st.ID + " 는 아직 로그가 없습니다"
		return
	}
	s.logStep = st
	s.logVP = viewport.New(s.width, max(5, s.height-3))
	lines := plan.TailLines(st.Log, 5000)
	s.logVP.SetContent(lipgloss.NewStyle().Width(max(1, s.width)).Render(strings.Join(lines, "\n")))
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
		s.message = "실행 중인 단계가 끝나면 멈춥니다. 한 번 더 Ctrl+C 면 바로 멈춤"
		return true
	case 2:
		s.ex.runner.Kill()
		s.message = "실행 중인 단계를 멈추는 중... (한 번 더 Ctrl+C 면 종료)"
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
		head := styles.StyleTitle.Render("로그 "+s.logStep.ID+" "+s.logStep.Title) + styles.StyleMuted.Render("   "+s.logStep.Log+"   ↑/↓ 스크롤   l·Esc 닫기")
		return head + "\n\n" + s.logVP.View()
	}
	ex, now := s.ex, time.Now()
	var b strings.Builder
	took := now.Sub(ex.started)
	if ex.done {
		took = ex.snap.Updated.Sub(ex.started)
	}
	b.WriteString(styles.StyleTitle.Render("설치 실행") + styles.StyleMuted.Render("   경과 "+elapsed(took)+"   상태·로그: "+plan.StateDir(s.opts)) + "\n")
	b.WriteString(styles.StyleMuted.Render("  · 대기  ▶ 실행 중  ✓ 성공  ✗ 실패  ⏸ 기다림  - 건너뜀. 다른 터미널에서 플레이북을 손으로 실행하지 마세요") + "\n\n")
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
			b.WriteString(styles.StylePrimary.Render("        설치했으면 Enter (설치함, 계속) — 다시 확인합니다") + "\n")
		case st.Status == plan.StatusFailed && st.Note != "":
			b.WriteString(styles.StyleWarning.Render("        "+st.Note) + "\n")
		case st.Note != "" && st.Status != plan.StatusOK:
			b.WriteString(styles.StyleMuted.Render("        "+st.Note) + "\n")
		}
	}
	for _, st := range ex.snap.Steps {
		if st.Status == plan.StatusRunning && ex.byID[st.ID].Kind != plan.Manual {
			b.WriteString("\n" + styles.StyleMuted.Render(fmt.Sprintf("── 로그 %s (마지막 %d줄) ──", st.ID, tailLines)) + "\n")
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
	keys := "  ↑/↓ 단계   l 로그"
	switch {
	case ex.done:
		if ex.outcome == plan.OutcomeFailed {
			keys += "   r 실패 단계 재시도"
		}
		if s.resetOffered() {
			keys += "   x 클러스터 초기화(reset)"
		}
		keys += "   Esc 계획 화면으로 (실행을 누르면 이어서)"
	default:
		if _, ok := s.waitingStep(); ok {
			keys += "   Enter 설치함, 계속"
		}
		keys += "   Ctrl+C 멈춤 (한 번: 실행 중인 단계 뒤, 두 번: 바로)"
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
		b.WriteString(styles.StyleError.Render("✗ 시작하지 못했습니다: "+ex.err.Error()) + "\n")
		return b.String()
	}
	if err := ex.runner.SaveErr(); err != nil {
		b.WriteString(styles.StyleWarning.Render("! "+plan.StateDir(s.opts)+"/state.json 을 쓰지 못했습니다 ("+err.Error()+
			"). 기록이 실행보다 뒤처졌을 수 있습니다") + "\n")
	}
	switch {
	case ex.outcome == plan.OutcomeOK:
		b.WriteString(styles.StyleSuccess.Render("✓ 설치를 마쳤습니다") + "\n")
		return b.String()
	case ex.outcome == plan.OutcomeStopped:
		b.WriteString(styles.StyleWarning.Render("■ 멈췄습니다. 실행을 누르면 끝나지 않은 단계부터 이어서 실행합니다") + "\n")
	case ex.snap.WentOn():
		// to its end, past a step that stays failed (plan.informerOnly)
		b.WriteString(styles.StyleWarning.Render("■ 끝까지 실행했지만 아래 단계는 실패로 남았습니다. 원인을 푼 뒤 실행을 누르면 그 단계만 다시 실행합니다") + "\n")
	default:
		b.WriteString(styles.StyleError.Render("✗ 실패했습니다. 고친 뒤 실행을 누르면 실패한 단계부터 이어서 실행합니다") + "\n")
	}
	for _, st := range ex.snap.Steps {
		if st.Status != plan.StatusFailed {
			continue
		}
		code := ""
		if st.ExitCode != nil {
			code = fmt.Sprintf(" (종료 코드 %d)", *st.ExitCode)
		}
		b.WriteString(styles.StyleError.Render("✗ "+st.ID+" "+st.Title+code+"  로그: "+st.Log) + "\n")
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
	b.WriteString(styles.StyleTitle.Render("지난 실행") + styles.StyleMuted.Render("  "+s.last.Started.Format("2006-01-02 15:04")+" 시작, "+plan.StateDir(s.opts)) + "\n")
	if len(done) > 0 {
		b.WriteString(styles.StyleSuccess.Render("  ✓ 끝남: "+strings.Join(done, " ")) + "\n")
	}
	if len(left) == 0 {
		b.WriteString(styles.StyleMuted.Render("  모두 끝났습니다. f 처음부터 다시 실행") + "\n")
	} else {
		b.WriteString(styles.StyleWarning.Render("  남음: "+strings.Join(left, " ")+" — 실행 = 이어서 실행, f 처음부터") + "\n")
		if s.last.InputHash != s.inputHash {
			b.WriteString(styles.StyleWarning.Render("  ! 지난 실행 뒤에 사이트 파일이 바뀌었습니다 (이어서 실행할 때 묻습니다)") + "\n")
		}
	}
	return b.String()
}
