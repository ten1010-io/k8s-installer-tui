package screens

import (
	"context"
	"encoding/base64"
	"errors"
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
	nodeBuilt  bool // the node holds the record of a built cluster (the probe of a reset)
	nodeSilent bool // the node does not answer that probe
	kubectl    int
	p2         int                      // the ansible runs of P2, which every round of the checks makes
	ran        []string                 // playbooks (or "shell"), in order
	block      map[string]chan struct{} // by playbook name: held until closed or cancelled
	fail       map[string]int
	say        map[string]string // by playbook name: lines of its log before it fails
	started    chan string
}

func newRunSite() *runSite {
	return &runSite{block: map[string]chan struct{}{}, fail: map[string]int{}, say: map[string]string{}, started: make(chan string, 64)}
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
	case strings.Contains(script, "ki-none"):
		// the probe before a reset: whether the node holds a cluster's record
		switch {
		case f.nodeSilent:
			return []byte("node1 | UNREACHABLE! => {\"changed\": false, \"msg\": \"Failed to connect\", \"unreachable\": true}\n"), errors.New("exit status 4")
		case f.nodeBuilt:
			return []byte("node1 | CHANGED | rc=0 | (stdout) ki-built\n"), nil
		}
		return []byte("node1 | CHANGED | rc=0 | (stdout) Last login: today\\r\\nki-none\\r\\n\n"), nil
	case strings.Contains(script, "kube-apiserver.yaml"):
		// P3 of an existing site: the manifest with nothing put in by hand
		manifest := base64.StdEncoding.EncodeToString([]byte("apiVersion: v1\nkind: Pod\nspec:\n  containers:\n  - name: kube-apiserver\n    command:\n    - kube-apiserver\n"))
		return []byte(`node1 | SUCCESS => {"changed": false, "content": "` + manifest + `", "encoding": "base64"}` + "\n"), nil
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
	block, code, say := f.block[what], f.fail[what], f.say[what]
	f.mu.Unlock()
	f.started <- what
	fmt.Fprintf(out, "PLAY [%s]\nTASK [fake task of %s]\n%s", what, what, say)
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
	// the archive of the package: a new AIPub site does not start without it
	opts.ArchivePath = filepath.Join(t.TempDir(), plan.ArchiveName)
	os.WriteFile(opts.ArchivePath, []byte("fake"), 0644)
	s9 := NewS9Install(s, paths, opts)
	f := newRunSite()
	s9.SetRunner(f.run)
	s9.SetExec(f.exec)
	s9.SetArchiveDirs(func() []string { return nil })
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
	if got, want := f.playbooks(), "shell setup-k8s-installer setup-control-node setup-cluster push-user-registry-artifacts setup-network setup-cert setup-base setup-monitoring setup-aipub shell update-cluster"; got != want {
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
	for _, want := range []string{"지난 실행", "✓ 끝남: C1 K1 K2 K3 K4 A1 A2 M1", "남음: ✗A3 ·A4 ·A5", "< 이어서 실행 >", "f 처음부터"} {
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

// r on a failed run goes on from the failed step, from the run view.
func TestRetryKeyGoesOnFromTheFailedStep(t *testing.T) {
	s9, f := newRunScreen(t)
	f.classReady = true
	f.fail["setup-base"] = 2
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })
	if v := plainView(s9); !strings.Contains(v, "r 실패 단계 재시도") {
		t.Fatalf("no r:\n%s", v)
	}
	f.mu.Lock()
	f.fail["setup-base"], f.ran = 0, nil
	f.mu.Unlock()
	p.key(runeKey("r"))
	p.until(t, "the retry ends", func() bool { return s9.ex != nil && s9.ex.done })
	if got := f.playbooks(); s9.ex.outcome != plan.OutcomeOK || !strings.HasPrefix(got, "shell setup-base") {
		t.Fatalf("retry: %s, ran %s", s9.ex.outcome, got)
	}
}

// resetRecorder is a reset-cluster.yml that only says it ran, on a control
// node whose record of a cluster was looked at and is not there.
func resetRecorder(s9 *S9Install) *int {
	var ran int
	s9.base.Detect = func() plan.Site { return plan.Site{} }
	s9.SetResetCmd(func() tea.Cmd {
		ran++
		return func() tea.Msg { return resetDoneMsg{} }
	})
	return &ran
}

// x is never offered after K3: A1 failed, the cluster stands.
func TestResetNotOfferedAfterK3(t *testing.T) {
	s9, f := newRunScreen(t)
	f.fail["setup-network"] = 2
	ran := resetRecorder(s9)
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })
	if v := plainView(s9); strings.Contains(v, "x 클러스터 초기화") {
		t.Errorf("x offered after A1 failed:\n%s", v)
	}
	p.key(runeKey("x"))
	p.key(tea.KeyMsg{Type: tea.KeyEsc}) // the plan view
	p.key(tea.KeyMsg{Type: tea.KeyTab}) // off the archive path
	p.key(runeKey("x"))
	if s9.confirm != "" || *ran != 0 || strings.Contains(plainView(s9), "x 클러스터 초기화") {
		t.Errorf("x after A1: confirm %q, ran %d:\n%s", s9.confirm, *ran, plainView(s9))
	}
}

// x is never offered on an existing site, whatever step failed.
func TestResetNotOfferedOnAnExistingSite(t *testing.T) {
	s9, _ := newRunScreen(t)
	ran := resetRecorder(s9)
	s9.last = &plan.RunState{Existing: true, Outcome: plan.OutcomeFailed,
		Steps: []plan.StepState{{ID: "K1", Status: plan.StatusSkipped}, {ID: "K3", Status: plan.StatusFailed}}}
	p := newPump(s9)
	p.key(tea.KeyMsg{Type: tea.KeyTab})
	p.key(runeKey("x"))
	if s9.resetOffered() || s9.confirm != "" || *ran != 0 || strings.Contains(plainView(s9), "x 클러스터 초기화") {
		t.Errorf("x on an existing site: confirm %q:\n%s", s9.confirm, plainView(s9))
	}
	s9.last.Existing = false
	if !s9.resetOffered() {
		t.Error("the same failure on a new site is where x helps")
	}
	// a run planned new on a node where a cluster is, or may be: K3 fails
	// there because the cluster runs, and a reset would destroy it
	for name, site := range map[string]plan.Site{
		"the record is there now":        {Existing: true},
		"the record could not be read":   {Unknown: true},
		"--existing given at this start": {Existing: true},
	} {
		site := site
		s9.base.Detect = func() plan.Site { return site }
		p.key(runeKey("x"))
		if s9.resetOffered() || s9.confirm != "" || *ran != 0 || strings.Contains(plainView(s9), "x 클러스터 초기화") {
			t.Errorf("%s: x offered (confirm %q)", name, s9.confirm)
		}
	}
	s9.base.Detect = func() plan.Site { return plan.Site{} }
	if !s9.resetOffered() {
		t.Error("the record looked at and absent: x helps")
	}
}

// After K3 failed on a new site x is offered, only the typed word runs
// reset-cluster.yml, and afterwards 실행 is a run from K1 of a new site. While
// a run is under way x does nothing.
func TestResetAfterK3FailedOnANewSite(t *testing.T) {
	s9, f := newRunScreen(t)
	f.fail["setup-cluster"] = 2
	ran := resetRecorder(s9)
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })
	if v := plainView(s9); !strings.Contains(v, "x 클러스터 초기화(reset)") {
		t.Fatalf("no x after K3 failed:\n%s", v)
	}

	// y is no answer: only the word is
	p.key(runeKey("x"))
	if s9.confirm != "reset-cluster" || !strings.Contains(plainView(s9), "되돌릴 수 없습니다. 초기화하려면 reset 을 입력하고 Enter") {
		t.Fatalf("no question:\n%s", plainView(s9))
	}
	p.key(runeKey("y"))
	p.key(tea.KeyMsg{Type: tea.KeyEnter})
	if s9.confirm != "" || *ran != 0 || !strings.Contains(s9.message, "취소") {
		t.Fatalf("y ran the reset (%d), confirm %q, %q", *ran, s9.confirm, s9.message)
	}
	p.key(runeKey("x"))
	p.key(runeKey("rese"))
	p.key(tea.KeyMsg{Type: tea.KeyEsc})
	if s9.confirm != "" || *ran != 0 {
		t.Fatalf("Esc ran the reset")
	}
	p.key(runeKey("x"))
	p.key(runeKey("resett"))
	p.key(tea.KeyMsg{Type: tea.KeyBackspace})
	p.key(tea.KeyMsg{Type: tea.KeyEnter})
	p.until(t, "the reset ends", func() bool { return *ran == 1 && s9.ex == nil })
	if _, err := os.Stat(filepath.Join(plan.StateDir(s9.opts), "state.json.reset")); err != nil || s9.last != nil {
		t.Errorf("the record of the run is not set aside: %v %+v", err, s9.last)
	}
	if !strings.Contains(s9.message, "처음부터 (K1 부터) 실행합니다") {
		t.Errorf("message %q", s9.message)
	}

	// 실행 now: from K1, a new site
	f.mu.Lock()
	f.fail, f.ran = map[string]int{}, nil
	f.block["setup-k8s-installer"] = make(chan struct{})
	f.mu.Unlock()
	p.pressRun()
	p.until(t, "K1 runs", func() bool { return s9.stepStatus("K1") == plan.StatusRunning })
	// during the run x does nothing (and the last run is not the one shown)
	p.key(runeKey("x"))
	if s9.confirm != "" || s9.resetOffered() {
		t.Errorf("x during a run: %q", s9.confirm)
	}
	s9.ex.runner.Kill()
	p.until(t, "the run stops", func() bool { return s9.ex.done })
	if got := f.playbooks(); !strings.HasPrefix(got, "shell setup-k8s-installer") || s9.opts.Existing {
		t.Errorf("after the reset ran %s (existing %t)", got, s9.opts.Existing)
	}
}

// The word is reset exactly, and what was true when x was pressed is looked at
// again when the word is typed: the record of this node, every node of the
// inventory, and the screen being free.
func TestResetNeedsTheWordAndAFreshLook(t *testing.T) {
	s9, f := newRunScreen(t)
	f.fail["setup-cluster"] = 2
	ran := resetRecorder(s9)
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })
	settle := func() {
		t.Helper()
		p.until(t, "the answer is taken", func() bool { return s9.confirm == "" && !s9.resetting })
	}
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	for _, word := range []string{"", "rese", "resetx", "Reset", "RESET", "reset "} {
		p.key(runeKey("x"))
		if s9.confirm != "reset-cluster" {
			t.Fatalf("x not offered before %q", word)
		}
		if word != "" {
			p.key(runeKey(word))
		}
		p.key(enter)
		settle()
		if *ran != 0 {
			t.Fatalf("%q ran the reset", word)
		}
	}

	// the record appeared between x and the word (another run built the cluster)
	p.key(runeKey("x"))
	s9.base.Detect = func() plan.Site { return plan.Site{Existing: true} }
	p.key(runeKey("reset"))
	p.key(enter)
	settle()
	if *ran != 0 || !strings.Contains(s9.message, "초기화하지 않았습니다") {
		t.Fatalf("ran %d on a site that is existing by now: %q", *ran, s9.message)
	}
	s9.base.Detect = func() plan.Site { return plan.Site{} }

	// a node of the inventory holds the record of a cluster built elsewhere
	f.mu.Lock()
	f.nodeBuilt = true
	f.mu.Unlock()
	p.key(runeKey("x"))
	p.key(runeKey("reset"))
	p.key(enter)
	settle()
	if *ran != 0 || !strings.Contains(s9.message, "node1") || !strings.Contains(s9.message, "초기화하지 않습니다") {
		t.Fatalf("ran %d though node1 holds a cluster: %q", *ran, s9.message)
	}
	// or does not answer: not known to be clean
	f.mu.Lock()
	f.nodeBuilt, f.nodeSilent = false, true
	f.mu.Unlock()
	p.key(runeKey("x"))
	p.key(runeKey("reset"))
	p.key(enter)
	settle()
	if *ran != 0 || !strings.Contains(s9.message, "확인하지 못") {
		t.Fatalf("ran %d though node1 did not answer: %q", *ran, s9.message)
	}
	f.mu.Lock()
	f.nodeSilent = false
	f.mu.Unlock()

	// not offered while the checks run or a run waits for them, nor without a
	// way to look at the record
	s9.runAfterChecks = true
	p.key(runeKey("x"))
	if s9.resetOffered() || s9.confirm != "" {
		t.Error("x while a run waits for the checks")
	}
	s9.runAfterChecks = false
	detect := s9.base.Detect
	s9.base.Detect = nil
	if s9.resetOffered() {
		t.Error("x with no way to look at the record")
	}
	s9.base.Detect = detect

	// and with everything as it should be, the word runs it
	p.key(runeKey("x"))
	p.key(runeKey("reset"))
	p.key(enter)
	p.until(t, "the reset ends", func() bool { return *ran == 1 && !s9.resetting })
	if s9.resetRelease != nil {
		t.Error("the lock of the runs is still held after the reset")
	}
}

// A new AIPub site with no archive does not start: asked, no by default, and
// only y starts it (K4 skipped).
func TestNewSiteWithoutArchiveAsks(t *testing.T) {
	s9, f := newRunScreen(t)
	f.classReady = true
	s9.archive.SetValue("")
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the question", func() bool { return s9.confirm == "no-archive" })
	v := plainView(s9)
	if s9.ex != nil || !strings.Contains(v, "새 사이트인데 아카이브 경로가 비어 있습니다") || !strings.Contains(v, "A1 setup-network 부터") ||
		!strings.Contains(v, plan.ArchiveName) || !strings.Contains(v, "n·Enter 아니오") {
		t.Fatalf("question:\n%s", v)
	}
	p.key(tea.KeyMsg{Type: tea.KeyEnter}) // the default: no
	if s9.confirm != "" || s9.ex != nil || f.playbooks() != "" || !strings.Contains(s9.message, "아카이브 경로 칸에") {
		t.Fatalf("Enter started it (%q): %q", f.playbooks(), s9.message)
	}
	p.pressRun()
	p.until(t, "the question again", func() bool { return s9.confirm == "no-archive" })
	p.key(runeKey("y"))
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })
	if got := f.playbooks(); strings.Contains(got, "push-user-registry-artifacts") || s9.stepStatus("K4") != plan.StatusSkipped {
		t.Errorf("ran %s, K4 %s", got, s9.stepStatus("K4"))
	}
}

// An archive path that is not a readable file is refused before anything runs.
func TestUnreadableArchiveRefused(t *testing.T) {
	s9, f := newRunScreen(t)
	f.classReady = true
	for _, bad := range []string{t.TempDir(), filepath.Join(t.TempDir(), "missing.tgz")} {
		s9.archive.SetValue(bad)
		p := newPump(s9)
		p.pressRun()
		p.until(t, "the checks end", func() bool { return !s9.checking && !s9.runAfterChecks })
		if s9.ex != nil || s9.confirm != "" || f.playbooks() != "" || !strings.Contains(s9.message, "실행하지 않았습니다: 아카이브 "+bad) {
			t.Errorf("%s: ran %q, %q", bad, f.playbooks(), s9.message)
		}
	}
}

// An empty 아카이브 경로 of a new site is filled with the one archive found;
// several are listed and none is chosen; none, nothing is said.
func TestArchiveFoundForTheField(t *testing.T) {
	s9, _ := newRunScreen(t)
	one, two, none := t.TempDir(), t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(one, plan.ArchiveName), []byte("x"), 0644)
	os.WriteFile(filepath.Join(two, plan.ArchiveName), []byte("x"), 0644)
	s9.opts.ArchivePath = ""

	s9.SetArchiveDirs(func() []string { return []string{none, one} })
	s9.SyncFromState(s9.appState)
	if got := s9.archive.Value(); got != filepath.Join(one, plan.ArchiveName) || !strings.Contains(s9.message, one+" 에서 찾아") {
		t.Errorf("one: %q, %q", got, s9.message)
	}

	s9.opts.ArchivePath = ""
	s9.SetArchiveDirs(func() []string { return []string{one, two} })
	s9.SyncFromState(s9.appState)
	if got := s9.archive.Value(); got != "" || !strings.Contains(s9.message, "여러 곳에 있습니다") ||
		!strings.Contains(s9.message, filepath.Join(two, plan.ArchiveName)) {
		t.Errorf("several: %q, %q", got, s9.message)
	}

	s9.SetArchiveDirs(func() []string { return []string{none} })
	s9.SyncFromState(s9.appState)
	if got := s9.archive.Value(); got != "" || s9.message != "" {
		t.Errorf("none: %q, %q", got, s9.message)
	}

	// an existing site may leave it out: nothing is filled
	s9.opts.Existing, s9.base.Existing = true, true
	s9.SetArchiveDirs(func() []string { return []string{one} })
	s9.SyncFromState(s9.appState)
	if got := s9.archive.Value(); got != "" {
		t.Errorf("existing site: %q", got)
	}
}

// A run that skipped K4 and failed at A1 on the images: the note says so, and
// 이어서 실행 with an archive given now runs K4 first, then A1, recording it.
func TestResumeAfterK4SkippedRunsK4(t *testing.T) {
	s9, f := newRunScreen(t)
	f.classReady = true
	archive := s9.base.ArchivePath
	s9.archive.SetValue("")
	f.fail["setup-network"] = 1
	f.say["setup-network"] = "Error: INSTALLATION FAILED: context deadline exceeded\ncilium-x 0/1 ImagePullBackOff\n"
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the question", func() bool { return s9.confirm == "no-archive" })
	p.key(runeKey("y"))
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })
	a1 := s9.ex.snap.Step("A1")
	if a1 == nil || !strings.Contains(a1.Note, "kubectl -n kube-system get pods") || !strings.Contains(a1.Note, "K4 push-user-registry-artifacts 를 건너뛰었습니다") {
		t.Fatalf("A1: %+v", a1)
	}

	base := s9.base
	base.ArchivePath = ""
	s9.base = base
	s9 = reopen(s9, f) // the program started again: the last run's empty path
	if s9.archive.Value() != "" {
		t.Fatalf("archive %q", s9.archive.Value())
	}
	s9.archive.SetValue(archive)
	f.mu.Lock()
	f.ran, f.fail = nil, map[string]int{}
	f.mu.Unlock()
	p = newPump(s9)
	p.pressRun()
	p.until(t, "the resume ends", func() bool { return s9.ex != nil && s9.ex.done })
	if got := f.playbooks(); !strings.HasPrefix(got, "shell push-user-registry-artifacts setup-network setup-cert") || s9.ex.outcome != plan.OutcomeOK {
		t.Errorf("resumed %s: %s", got, s9.ex.outcome)
	}
	if v := plainView(s9); strings.Contains(v, "이어서 실행은 지난 실행의 설정으로") {
		t.Errorf("said the archive is kept:\n%s", v)
	}
	if st, _ := plan.LoadRunState(plan.StateDir(s9.opts)); st == nil || st.ArchivePath != archive || st.Step("K4").Status != plan.StatusOK {
		t.Errorf("state.json: %+v", st)
	}
}

// f after K3 built the cluster (the record of k8s-installer appeared since the
// unfinished run started) plans an existing site: K1 K2 skipped, K3
// update-cluster, never setup-cluster. 이어서 실행 keeps the plan of the run.
func TestFreshAfterTheRecordAppearedIsAnExistingSite(t *testing.T) {
	record := filepath.Join(t.TempDir(), "applied-release")
	saved := plan.ClusterRecord
	plan.ClusterRecord = record
	t.Cleanup(func() { plan.ClusterRecord = saved })

	s9, f := newRunScreen(t)
	f.classReady = true
	f.fail["setup-network"] = 1
	base := s9.base
	base.Detect = func() plan.Site { return plan.DetectSite(false, false) }
	s9.base = base
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })
	os.WriteFile(record, []byte("1.36.0\n"), 0644) // what K3 leaves on a real site

	s9 = reopen(s9, f)
	if s9.opts.Existing || !strings.Contains(plainView(s9), "새 사이트: 지난 실행") {
		t.Fatalf("the resume is not planned as the run was:\n%s", plainView(s9))
	}
	f.mu.Lock()
	f.ran, f.fail = nil, map[string]int{}
	f.mu.Unlock()
	p = newPump(s9)
	p.key(tea.KeyMsg{Type: tea.KeyTab})
	p.key(runeKey("f"))
	if !s9.opts.Existing || !strings.Contains(plainView(s9), "사이트: 기존 사이트: "+record+" 있음") {
		t.Fatalf("f planned the run's site:\n%s", plainView(s9))
	}
	for _, st := range s9.steps {
		if (st.ID == "K1" || st.ID == "K2") && !st.Skip || st.Playbook == "playbooks/setup-cluster.yml" {
			t.Errorf("%s %s skip=%t", st.ID, st.Playbook, st.Skip)
		}
	}
	p.until(t, "the question after the checks", func() bool { return s9.confirm == "fresh" })
	p.key(runeKey("y"))
	p.until(t, "the fresh run ends", func() bool { return s9.ex != nil && s9.ex.done })
	if got := f.playbooks(); strings.Contains(got, "setup-cluster") || strings.Contains(got, "setup-k8s-installer") || s9.ex.outcome != plan.OutcomeOK {
		t.Errorf("fresh ran %s: %s", got, s9.ex.outcome)
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
	n.SetArchiveDirs(func() []string { return nil })
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
