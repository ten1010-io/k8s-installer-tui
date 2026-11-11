package screens

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

var ansiCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// runSite fakes the control node of a run: the checks and the StorageClass
// kubectl (run), and the steps (exec), each by the playbook it runs.
type runSite struct {
	mu         sync.Mutex
	sudoFails  bool
	classReady bool
	kubectl    int
	p2         int                      // the ansible runs of P2, which every round of the checks makes
	ran        []string                 // playbooks (or "shell"), in order
	block      map[string]chan struct{} // by playbook name: held until closed or cancelled
	fail       map[string]int
	started    chan string
}

func newRunSite() *runSite {
	return &runSite{block: map[string]chan struct{}{}, fail: map[string]int{}, started: make(chan string, 64)}
}

func (f *runSite) run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if name != "bash" {
		return nil, nil // getent of P7: no answer, a warning
	}
	script := args[len(args)-1]
	if strings.Contains(script, "'sudo -n true'") {
		f.p2++
	}
	switch {
	case strings.Contains(script, "get storageclass"):
		f.kubectl++
		if f.classReady {
			return []byte("node1 | CHANGED | rc=0 | (stdout) storageclass.storage.k8s.io/nfs-client\n"), nil
		}
		return []byte(`node1 | FAILED | rc=1 | (stdout)  (stderr) Error from server (NotFound): storageclasses.storage.k8s.io "nfs-client" not found` + "\n"), nil
	case strings.Contains(script, "'sudo -n true'") && f.sudoFails:
		return []byte("node1 | FAILED | rc=1 | (stdout)  (stderr) sudo: a password is required\n"), nil
	case strings.Contains(script, "sudo -n"):
		return []byte("node1 | CHANGED | rc=0 | (stdout) \n"), nil
	}
	return nil, nil
}

var playbookName = regexp.MustCompile(`/([a-z0-9-]+)\.yml`)

func (f *runSite) exec(ctx context.Context, dir string, out io.Writer, name string, args ...string) error {
	lines := strings.Split(strings.TrimSpace(args[len(args)-1]), "\n")
	what := "shell"
	if m := playbookName.FindStringSubmatch(lines[len(lines)-1]); m != nil && strings.Contains(lines[len(lines)-1], "ansible-playbook") {
		what = m[1]
	}
	f.mu.Lock()
	f.ran = append(f.ran, what)
	block, code := f.block[what], f.fail[what]
	f.mu.Unlock()
	f.started <- what
	fmt.Fprintf(out, "PLAY [%s]\nTASK [fake task of %s]\n", what, what)
	if block != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-block:
		}
	}
	if code != 0 {
		fmt.Fprintf(out, "fatal: [node1]: FAILED! => {\"msg\": \"fake failure of %s\"}\n", what)
		return exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	}
	return nil
}

func (f *runSite) playbooks() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.ran, " ")
}

// newRunScreen is the install screen of a new AIPub site with HA on the class
// nfs-client, in an installer root of its own that setup.sh has made (P1).
func newRunScreen(t *testing.T) (*S9Install, *runSite) {
	t.Setenv("HOME", t.TempDir())
	s := state.DefaultState()
	s.AipubEnabled = true
	s.AipubHaModeStorageClass = "nfs-client"
	s.K8sIngressVIP = "192.168.0.100"
	s.K8sCpNodes = []string{"node1"}
	site := t.TempDir()
	paths := fileio.DefaultPaths(site)
	os.MkdirAll(filepath.Dir(paths.Vars), 0755)
	os.WriteFile(paths.Inventory, []byte("all: {}\n"), 0644)
	os.WriteFile(paths.Vars, []byte("a: 1\n"), 0644)
	opts := plan.DefaultOptions()
	opts.Root = t.TempDir()
	os.WriteFile(filepath.Join(opts.Root, "release"), []byte("1.36.0\n"), 0644)
	s9 := NewS9Install(s, paths, opts)
	f := newRunSite()
	s9.SetRunner(f.run)
	s9.SetExec(f.exec)
	s9.SetSize(120, 300)
	s9.SyncFromState(s)
	return s9, f
}

// pump is the loop of bubbletea for a screen: commands run in goroutines, their
// messages go to Update on the test's goroutine.
type pump struct {
	s  *S9Install
	ch chan tea.Msg
}

func newPump(s *S9Install) *pump { return &pump{s: s, ch: make(chan tea.Msg, 64)} }

func (p *pump) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		msg := cmd()
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, c := range b {
				p.run(c)
			}
			return
		}
		if msg != nil {
			p.ch <- msg
		}
	}()
}

func (p *pump) key(k tea.KeyMsg) {
	_, cmd := p.s.Update(k)
	p.run(cmd)
}

// until feeds messages until cond holds, refreshing the run as a tick would.
func (p *pump) until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		if p.s.ex != nil && !p.s.ex.done {
			p.s.refresh()
		}
		if cond() {
			return
		}
		select {
		case msg := <-p.ch:
			_, cmd := p.s.Update(msg)
			p.run(cmd)
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			t.Fatalf("never %s:\n%s", what, plainView(p.s))
		}
	}
}

func plainView(s *S9Install) string { return ansiCodes.ReplaceAllString(s.View(), "") }

// pressRun moves to 실행 and presses it.
func (p *pump) pressRun() {
	for i := 0; i < 5; i++ {
		p.key(tea.KeyMsg{Type: tea.KeyTab})
	}
	p.key(tea.KeyMsg{Type: tea.KeyRight})
	p.key(tea.KeyMsg{Type: tea.KeyEnter})
}

func (s *S9Install) stepStatus(id string) plan.StepStatus {
	if s.ex == nil {
		return ""
	}
	if st := s.ex.snap.Step(id); st != nil {
		return st.Status
	}
	return ""
}

func runeKey(r string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)} }

// 실행 runs the checks first; one ✗ and nothing runs.
func TestRunNeedsTheChecks(t *testing.T) {
	s9, f := newRunScreen(t)
	f.sudoFails = true
	p := newPump(s9)
	p.pressRun()
	if !strings.Contains(plainView(s9), "사전 확인을 먼저 실행합니다") {
		t.Errorf("no word of the checks:\n%s", plainView(s9))
	}
	p.until(t, "the checks end", func() bool { return !s9.checking && !s9.runAfterChecks })
	if v := plainView(s9); !strings.Contains(v, "사전 확인 ✗: P2") || s9.ex != nil || f.playbooks() != "" {
		t.Errorf("ran with P2 ✗ (%q):\n%s", f.playbooks(), v)
	}
}

// The run shows each step and the end of the log of the one that runs, stops at
// the StorageClass until Enter, and holds the wizard meanwhile.
func TestRunShowsProgressAndWaitsForTheStorageClass(t *testing.T) {
	s9, f := newRunScreen(t)
	release := make(chan struct{})
	f.block["setup-k8s-installer"] = release
	p := newPump(s9)
	p.pressRun()
	p.until(t, "K1 runs", func() bool { return s9.stepStatus("K1") == plan.StatusRunning })
	p.until(t, "the log of K1 is shown", func() bool { return strings.Contains(plainView(s9), "TASK [fake task of setup-k8s-installer]") })
	v := plainView(s9)
	for _, want := range []string{"설치 실행", "✓ C1", "▶ K1", "· K2", "── 로그 K1 (마지막 10줄)", "Ctrl+C 멈춤"} {
		if !strings.Contains(v, want) {
			t.Errorf("no %q:\n%s", want, v)
		}
	}
	if !s9.Busy() || !strings.Contains(s9.BusyReason(), "설치를 실행하는 중") {
		t.Error("the wizard may leave during the run")
	}
	close(release)

	p.until(t, "M1 waits", func() bool { return s9.stepStatus("M1") == plan.StatusWaiting })
	p.until(t, "setup-cert ends beside it", func() bool { return s9.stepStatus("A2") == plan.StatusOK })
	v = plainView(s9)
	for _, want := range []string{"⏸ M1", "스토리지클래스 드라이버를 설치하세요 — 위저드가 쓰는 클래스: nfs-client",
		"없음: nfs-client", "설치했으면 Enter", "· A3"} {
		if !strings.Contains(v, want) {
			t.Errorf("no %q:\n%s", want, v)
		}
	}
	if hints := strings.Join(s9.KeyHints(), " "); !strings.Contains(hints, "Enter 설치함, 계속") {
		t.Errorf("hints: %s", hints)
	}
	f.mu.Lock()
	checks := f.kubectl
	f.classReady = true
	f.mu.Unlock()
	p.key(tea.KeyMsg{Type: tea.KeyEnter})
	p.until(t, "the run ends", func() bool { return s9.ex.done })
	f.mu.Lock()
	after := f.kubectl
	f.mu.Unlock()
	if v := plainView(s9); s9.ex.outcome != plan.OutcomeOK || !strings.Contains(v, "✓ 설치를 마쳤습니다") || after != checks+1 {
		t.Errorf("outcome %s, %d checks after Enter:\n%s", s9.ex.outcome, after-checks, v)
	}
	if v := plainView(s9); strings.Contains(v, "다시 확인합니다") {
		t.Errorf("the word of Enter at the stop outlived it:\n%s", v)
	}
	if got, want := f.playbooks(), "shell setup-k8s-installer setup-control-node setup-cluster setup-network setup-cert setup-base setup-monitoring setup-aipub shell update-cluster"; got != want {
		t.Errorf("ran %s\nwant %s", got, want)
	}
	if s9.Busy() {
		t.Error("busy after the run")
	}
}

// A failed run says where and why; on coming back, 실행 goes on from the failed
// step, asking first when the site files changed since.
func TestRunResumesFromTheFailedStep(t *testing.T) {
	s9, f := newRunScreen(t)
	f.classReady = true
	f.fail["setup-base"] = 2
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })
	v := plainView(s9)
	for _, want := range []string{"✗ 실패했습니다", "✗ A3 setup-base (종료 코드 2)  로그: " + plan.StateDir(s9.opts) + "/logs/A3.log",
		"fake failure of setup-base", "Esc 계획 화면으로"} {
		if !strings.Contains(v, want) {
			t.Errorf("no %q:\n%s", want, v)
		}
	}
	p.key(runeKey("l"))
	if v := plainView(s9); !strings.Contains(v, "로그 A3 setup-base") || !strings.Contains(v, "TASK [fake task of setup-base]") {
		t.Errorf("log view:\n%s", v)
	}
	p.key(tea.KeyMsg{Type: tea.KeyEsc}) // the log
	p.key(tea.KeyMsg{Type: tea.KeyEsc}) // the run
	s9.SyncFromState(s9.appState)       // the screen entered again
	v = plainView(s9)
	for _, want := range []string{"지난 실행", "✓ 끝남: C1 K1 K2 K3 A1 A2 M1", "남음: ✗A3 ·A4 ·A5", "< 이어서 실행 >", "f 처음부터"} {
		if !strings.Contains(v, want) {
			t.Errorf("no %q:\n%s", want, v)
		}
	}

	os.WriteFile(s9.paths.Vars, []byte("a: 2\n"), 0644)
	f.mu.Lock()
	f.ran, f.fail = nil, map[string]int{}
	f.mu.Unlock()
	p.pressRun()
	// the checks again first (of the site as it is now), then the question
	p.until(t, "the question", func() bool { return s9.confirm != "" })
	if v := plainView(s9); !strings.Contains(v, "사이트 파일이 바뀌었습니다") || s9.ex != nil || !s9.Busy() {
		t.Fatalf("no question:\n%s", v)
	}
	p.key(runeKey("y"))
	p.until(t, "the resume ends", func() bool { return s9.ex != nil && s9.ex.done })
	// the copy first: the playbooks read the site as it was saved again
	if got := f.playbooks(); !strings.HasPrefix(got, "shell setup-base setup-monitoring setup-aipub") || s9.ex.outcome != plan.OutcomeOK {
		t.Errorf("resumed %s: %s", got, s9.ex.outcome)
	}
}

func TestFailureRecoveryKeysRAndX(t *testing.T) {
	s9, f := newRunScreen(t)
	f.classReady = true
	f.fail["setup-base"] = 2
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })

	v := plainView(s9)
	if !strings.Contains(v, "r 실패 단계 재시도") || !strings.Contains(v, "x 클러스터 초기화(reset)") {
		t.Fatalf("expected recovery keys in view:\n%s", v)
	}

	// 1. Press x to trigger cluster reset
	p.key(runeKey("x"))
	if s9.confirm != "reset-cluster" {
		t.Fatalf("expected confirm reset-cluster, got %q", s9.confirm)
	}
	if v := plainView(s9); !strings.Contains(v, "모든 노드의 클러스터를 초기화(reset-cluster.yml)하시겠습니까?") {
		t.Fatalf("expected reset confirm question:\n%s", v)
	}

	// Mock resetCmd
	resetRan := false
	s9.SetResetCmd(func() tea.Cmd {
		resetRan = true
		return func() tea.Msg { return resetDoneMsg{err: nil} }
	})

	// 2. Answer y to execute reset
	p.key(runeKey("y"))
	p.until(t, "reset to finish", func() bool { return strings.Contains(s9.message, "클러스터 초기화") })
	if !resetRan {
		t.Fatal("resetCmd was not executed")
	}

	// 3. Clear failure and press r to retry directly from the run screen
	f.mu.Lock()
	f.fail["setup-base"] = 0
	f.mu.Unlock()

	p.key(runeKey("r"))
	p.until(t, "the retry ends", func() bool { return s9.ex != nil && s9.ex.done })
	if s9.ex.outcome != plan.OutcomeOK {
		t.Fatalf("expected OK after retry, got %s", s9.ex.outcome)
	}
}

func (f *runSite) checksRun() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.p2
}

// reopen is the install screen of s9's site and installer root as the program
// shows it when started again: the last run read from state.json, no checks.
func reopen(s9 *S9Install, f *runSite) *S9Install {
	n := NewS9Install(s9.appState, s9.paths, s9.base)
	n.SetRunner(f.run)
	n.SetExec(f.exec)
	n.SetSize(120, 300)
	n.SyncFromState(s9.appState)
	return n
}

// f (from the first step) goes through the checks as 실행 does: run first when
// they have not, waited for when they run, and no run on a ✗.
func TestFreshNeedsTheChecks(t *testing.T) {
	s9, f := newRunScreen(t)
	f.classReady = true
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the first run ends", func() bool { return s9.ex != nil && s9.ex.done })
	if s9.ex.outcome != plan.OutcomeOK {
		t.Fatalf("first run: %s", s9.ex.outcome)
	}

	// not run yet (the program started again): they run, then the question
	s9 = reopen(s9, f)
	p = newPump(s9)
	before := f.checksRun()             // one round, of the first run
	p.key(tea.KeyMsg{Type: tea.KeyTab}) // off the archive path, where f is text
	p.key(runeKey("f"))
	if !strings.Contains(plainView(s9), "사전 확인을 먼저 실행합니다") || s9.confirm != "" {
		t.Fatalf("f asks before the checks:\n%s", plainView(s9))
	}
	p.until(t, "the question after the checks", func() bool { return s9.confirm == "fresh" })
	if f.checksRun() != 2*before {
		t.Errorf("%d P2 runs, want %d", f.checksRun()-before, before)
	}
	f.mu.Lock()
	f.ran = nil
	f.mu.Unlock()
	p.key(runeKey("y"))
	p.until(t, "the fresh run ends", func() bool { return s9.ex != nil && s9.ex.done })
	if got := f.playbooks(); !strings.HasPrefix(got, "shell setup-k8s-installer setup-control-node") {
		t.Errorf("from the start ran %s", got)
	}

	// a check ✗: no question, nothing runs
	s9 = reopen(s9, f)
	p = newPump(s9)
	f.mu.Lock()
	f.sudoFails, f.ran = true, nil
	f.mu.Unlock()
	p.key(tea.KeyMsg{Type: tea.KeyTab})
	p.key(runeKey("c"))
	p.until(t, "the checks end", func() bool { return !s9.checking })
	p.key(runeKey("f"))
	p.key(runeKey("y"))
	if v := plainView(s9); !strings.Contains(v, "사전 확인 ✗: P2") || s9.confirm != "" || s9.ex != nil || f.playbooks() != "" {
		t.Errorf("f ran past P2 ✗ (%q):\n%s", f.playbooks(), v)
	}

	// checks under way: waited for, then the question
	s9 = reopen(s9, f)
	p = newPump(s9)
	f.mu.Lock()
	f.sudoFails = false
	f.mu.Unlock()
	p.key(tea.KeyMsg{Type: tea.KeyTab})
	p.key(runeKey("c"))
	p.key(runeKey("f"))
	if !strings.Contains(plainView(s9), "사전 확인이 끝나면 실행합니다") || s9.confirm != "" || s9.ex != nil {
		t.Errorf("f during the checks:\n%s", plainView(s9))
	}
	p.until(t, "the question after the checks", func() bool { return s9.confirm == "fresh" })
}

// Checks of a site saved again since do not let a run start: 실행 runs them
// again for the site as it is now.
func TestRunAfterTheSiteChangedRunsTheChecksAgain(t *testing.T) {
	s9, f := newRunScreen(t)
	f.classReady = true
	p := newPump(s9)
	p.key(tea.KeyMsg{Type: tea.KeyTab})
	p.key(runeKey("c"))
	p.until(t, "the checks end", func() bool { return !s9.checking })
	once := f.checksRun()
	if _, failed := plan.CheckVerdict(s9.steps, s9.checks); len(failed) > 0 || once == 0 {
		t.Fatalf("checks: %v, ran %d", failed, once)
	}
	os.WriteFile(s9.paths.Inventory, []byte("all: {children: {}}\n"), 0644)
	p.pressRun()
	if !strings.Contains(plainView(s9), "사전 확인을 먼저 실행합니다") || s9.ex != nil {
		t.Fatalf("ran on the checks of the old site:\n%s", plainView(s9))
	}
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })
	if f.checksRun() != 2*once || s9.ex.outcome != plan.OutcomeOK {
		t.Errorf("%d P2 runs, want %d; %s", f.checksRun(), 2*once, s9.ex.outcome)
	}
}

// A resume runs the plan of the run it goes on with: its 모니터링 (and 아카이브
// 경로), shown when the screen comes back and kept, said so, when changed since.
func TestResumeKeepsTheOptionsOfTheRun(t *testing.T) {
	s9, f := newRunScreen(t)
	f.classReady = true
	f.fail["setup-base"] = 2
	s9.opts.Monitoring = false
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })
	if st, _ := plan.LoadRunState(plan.StateDir(s9.opts)); st == nil || st.Monitoring {
		t.Fatalf("state.json: %+v", st)
	}

	s9 = reopen(s9, f) // the program started again, 모니터링 설치 by default
	if s9.opts.Monitoring || !strings.Contains(plainView(s9), "설치 안 함") {
		t.Errorf("the screen does not show the run's 모니터링:\n%s", plainView(s9))
	}
	s9.opts.Monitoring = true // turned on on the screen
	f.mu.Lock()
	f.ran, f.fail = nil, map[string]int{}
	f.mu.Unlock()
	p = newPump(s9)
	p.pressRun()
	p.until(t, "the resume ends", func() bool { return s9.ex != nil && s9.ex.done })
	if got := f.playbooks(); strings.Contains(got, "setup-monitoring") || s9.ex.outcome != plan.OutcomeOK ||
		!strings.Contains(plainView(s9), "이어서 실행은 지난 실행의 설정으로 합니다: 모니터링 설치 안 함") {
		t.Errorf("resumed %s, %s:\n%s", got, s9.ex.outcome, plainView(s9))
	}
}

// The Ctrl+C that quits stops the run off the update loop (a tea.Cmd) while
// the loop takes the end of the run: no data race (go test -race).
func TestStopRunOffTheUpdateLoop(t *testing.T) {
	s9, f := newRunScreen(t)
	f.block["setup-k8s-installer"] = make(chan struct{})
	p := newPump(s9)
	p.pressRun()
	p.until(t, "K1 runs", func() bool { return s9.stepStatus("K1") == plan.StatusRunning })
	// the run has ended (the second Ctrl+C), its end not yet taken by the loop
	s9.ex.runner.Kill()
	<-s9.ex.ended
	stopped := make(chan struct{})
	go func() {
		s9.Stop()
		close(stopped)
	}()
	p.until(t, "the run ends", func() bool { return s9.ex.done })
	<-stopped
	if s9.stepStatus("K1") != plan.StatusCancelled {
		t.Errorf("K1 %s", s9.stepStatus("K1"))
	}
}

// Ctrl+C once lets the step finish and stops there; twice stops it now; the
// third quits.
func TestRunCtrlC(t *testing.T) {
	s9, f := newRunScreen(t)
	release := make(chan struct{})
	f.block["setup-k8s-installer"] = release
	p := newPump(s9)
	p.pressRun()
	p.until(t, "K1 runs", func() bool { return s9.stepStatus("K1") == plan.StatusRunning })
	if !s9.Interrupt() || !strings.Contains(plainView(s9), "실행 중인 단계가 끝나면 멈춥니다") {
		t.Errorf("first Ctrl+C:\n%s", plainView(s9))
	}
	close(release)
	p.until(t, "the run stops", func() bool { return s9.ex.done })
	if s9.ex.outcome != plan.OutcomeStopped || s9.stepStatus("K1") != plan.StatusOK || f.playbooks() != "shell setup-k8s-installer" {
		t.Errorf("%s, ran %s", s9.ex.outcome, f.playbooks())
	}

	s9, f = newRunScreen(t)
	f.block["setup-k8s-installer"] = make(chan struct{})
	p = newPump(s9)
	p.pressRun()
	p.until(t, "K1 runs", func() bool { return s9.stepStatus("K1") == plan.StatusRunning })
	s9.Interrupt()
	if !s9.Interrupt() {
		t.Error("second Ctrl+C quits")
	}
	p.until(t, "the run stops", func() bool { return s9.ex.done })
	if s9.stepStatus("K1") != plan.StatusCancelled {
		t.Errorf("K1 %s", s9.stepStatus("K1"))
	}
	if s9.Interrupt() {
		t.Error("Ctrl+C after the run does not quit")
	}
}
