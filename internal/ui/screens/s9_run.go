package screens

import (
	"context"
	"fmt"
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
	s.opts.Existing, s.opts.AipubInstalled = s.base.Existing, s.base.AipubInstalled
	s.last = nil
	prev, err := plan.LoadRunState(plan.StateDir(s.opts))
	if err != nil {
		s.message = "지난 실행 기록을 읽지 못했습니다: " + err.Error()
		return
	}
	s.last = prev
	if prev != nil && prev.Outcome != plan.OutcomeOK {
		s.opts.Existing, s.opts.AipubInstalled = prev.Existing, prev.AipubInstalled
		// and what the operator chose for it: 이어서 실행 runs that plan
		s.opts.Monitoring, s.opts.ArchivePath = prev.Monitoring, prev.ArchivePath
		s.archive.SetValue(prev.ArchivePath)
	}
}

// pressRun is 실행: through the checks (gate), then the run, from where the
// last one stopped.
func (s *S9Install) pressRun() tea.Cmd {
	s.fresh = false
	return s.gate(false)
}

// pressFresh is f: through the checks too, then asked, the run from the first step.
func (s *S9Install) pressFresh() tea.Cmd {
	if s.last == nil {
		return nil
	}
	s.fresh = true
	return s.gate(false)
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
	if s.fresh {
		s.confirm = "fresh"
		return nil
	}
	return s.offerRun()
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
	if s.confirm == "resume" {
		return "지난 실행 뒤에 사이트 파일이 바뀌었습니다. 끝난 단계는 다시 실행하지 않고 이어서 실행할까요? (y 예, n 아니오)"
	}
	if s.last != nil && len(s.last.Remaining()) == 0 {
		return "지난 실행이 모두 끝났습니다. 처음부터 모든 단계를 다시 실행할까요? (y 예, n 아니오)"
	}
	return "끝난 단계도 모두 처음부터 다시 실행할까요? (y 예, n 아니오)"
}

func (s *S9Install) answer(yes bool) tea.Cmd {
	c := s.confirm
	s.confirm = ""
	if !yes {
		s.message = "실행하지 않았습니다"
		return nil
	}
	return s.startRun(c == "resume")
}

// startRun runs the stages of the plan as it is shown, with resume from the
// last run, whose 모니터링 and 아카이브 경로 a resume keeps (said when they were
// changed on the screen since).
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
		if strings.TrimSpace(s.archive.Value()) != s.last.ArchivePath {
			archive := s.last.ArchivePath
			if archive == "" {
				archive = "없음"
			}
			kept = append(kept, "아카이브 경로 "+archive)
		}
		s.opts.Monitoring = s.last.Monitoring
		s.archive.SetValue(s.last.ArchivePath)
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
	if s.message != "" {
		b.WriteString("\n" + styles.StylePrimary.Render(s.message) + "\n")
	}
	keys := "  ↑/↓ 단계   l 로그"
	switch {
	case ex.done:
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
