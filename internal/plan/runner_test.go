package plan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func TestStages(t *testing.T) {
	o := testOptions()
	o.ArchivePath = "/media/usb/a.tgz"
	in := o.AnsibleDir()
	// every playbook alone; the StorageClass stop beside setup-cert
	if got, want := StageIDs(Stages(Build(aipubState(), o), o, in)), "K1 | K2 | K3 | K4 | A1 | A2+M1 | A3 | A4 | A5 | A5.1 | A5.2"; got != want {
		t.Errorf("new site: %s, want %s", got, want)
	}
	if got, want := StageIDs(Stages(Build(state.DefaultState(), o), o, in)), "K1 | K2 | K3 | K4"; got != want {
		t.Errorf("AIPub off: %s, want %s", got, want)
	}
	mon := o
	mon.Monitoring = false
	if got, want := StageIDs(Stages(Build(aipubState(), mon), mon, in)), "K1 | K2 | K3 | K4 | A1 | A2+M1 | A3 | (A4) | A5 | A5.1 | A5.2"; got != want {
		t.Errorf("monitoring off: %s, want %s", got, want)
	}
	// an existing site whose AIPub is there: K1..K3 kept, skipped (nothing set
	// on the control plane, so no update-cluster either), no JWKS steps
	s := aipubState()
	s.ControlPlane.JWKS = false
	ex := o
	ex.Existing, ex.AipubInstalled = true, true
	if got, want := StageIDs(Stages(Build(s, ex), ex, in)), "(K1) | (K2) | (K3) | K4 | A1 | A2+M1 | A3 | A4 | A5"; got != want {
		t.Errorf("existing site: %s, want %s", got, want)
	}
	// an existing cluster on which AIPub is installed now: K3 is the
	// update-cluster of the cluster stage, the rest as on a new site
	ex.AipubInstalled = false
	s = aipubState()
	s.ControlPlane.Audit = true
	if got, want := StageIDs(Stages(Build(s, ex), ex, in)), "(K1) | (K2) | K3 | K4 | A1 | A2+M1 | A3 | A4 | A5 | A5.1 | A5.2"; got != want {
		t.Errorf("existing cluster, AIPub new: %s, want %s", got, want)
	}
	// saved elsewhere: the copy goes first
	stages := Stages(Build(aipubState(), o), o, "/home/ops/site")
	if got := StageIDs(stages); !strings.HasPrefix(got, "C1 | K1 |") {
		t.Errorf("saved elsewhere: %s", got)
	}
	// no terminal to ask a password on: sudo -n, in the runner only
	if !strings.Contains(Command(stages[0][0]), "sudo -n cp /home/ops/site/inventory.yml") ||
		!strings.Contains(Command(stages[0][0]), "&& sudo -n install -D") {
		t.Errorf("copy step: %s", Command(stages[0][0]))
	}
	if a51 := stages[len(stages)-2][0]; a51.ID != "A5.1" || !strings.HasPrefix(Command(a51), "sudo -n ") {
		t.Errorf("A5.1: %s", Command(a51))
	}
	if script := Script(Build(aipubState(), o), o, "/home/ops/site"); strings.Contains(script, "sudo -n cp") {
		t.Errorf("the script asks no password:\n%s", script)
	}
	// no check runs in the stages
	for _, stage := range stages {
		for _, st := range stage {
			if st.Kind == Check {
				t.Errorf("check %s in the stages", st.ID)
			}
		}
	}
}

func TestStorageNeeds(t *testing.T) {
	o := testOptions()
	m1 := func(s *state.AppState, o Options) Step {
		for _, st := range Build(s, o) {
			if st.ID == "M1" {
				return st
			}
		}
		t.Fatal("no M1")
		return Step{}
	}
	// HA: harbor, monitoring and the shared volume of the backend on the HA class
	if st := m1(aipubState(), o); st.Skip || strings.Join(st.Classes, ",") != "nfs-client" ||
		!strings.Contains(st.Note, "nfs-client ← A3 setup-base, A4 setup-monitoring, A5 setup-aipub") {
		t.Errorf("HA: %+v", st)
	}
	// without HA nothing claims a class unless the site names one
	s := aipubState()
	s.AipubHaMode = false
	if st := m1(s, o); !st.Skip || !strings.Contains(st.SkipWhy, "StorageClass 를 쓰는 단계가 없음") {
		t.Errorf("non-HA: %+v", st)
	}
	o.StorageVars = map[string]string{"aipub_app_file_storage_storage_class_name": "local-path"}
	if st := m1(s, o); st.Skip || strings.Join(st.Classes, ",") != "local-path" || !strings.Contains(st.Note, "local-path ← A5 setup-aipub") {
		t.Errorf("non-HA with a class for the backend: %+v", st)
	}
	// HA, with the monitoring and the backend on classes of their own
	o.StorageVars = map[string]string{"aipub_monitoring_storage_class_name": "fast", "aipub_app_file_storage_type": "static"}
	if st := m1(aipubState(), o); strings.Join(st.Classes, ",") != "fast,nfs-client" || strings.Contains(st.Note, "A5") {
		t.Errorf("HA with classes of their own: %+v", st)
	}
	// monitoring on a dynamic volume with no class name: the default class of the cluster
	s = aipubState()
	s.AipubHaMode = false
	o.StorageVars = map[string]string{"aipub_monitoring_storage_type": "dynamic"}
	if st := m1(s, o); st.Skip || strings.Join(st.Classes, ",") != DefaultClass || !strings.Contains(st.Note, DefaultClass+" ← A4 setup-monitoring") {
		t.Errorf("monitoring on the default class: %+v", st)
	}
	// AIPub off: no stop at all
	for _, st := range Build(state.DefaultState(), testOptions()) {
		if st.Kind == Manual {
			t.Errorf("M1 without AIPub")
		}
	}
	// the script stops for a person too
	script := Script(Build(aipubState(), testOptions()), testOptions(), "/opt/k8s-installer/ansible")
	if !strings.Contains(script, "read -r -p '── M1 스토리지클래스 드라이버를 설치하세요 — 위저드가 쓰는 클래스: nfs-client. 설치했으면 Enter: ' _ || true") {
		t.Errorf("script:\n%s", script)
	}
}

// The default class is there when a class is marked default, whatever its name.
func TestCheckClassesDefault(t *testing.T) {
	marked := ""
	var asked string
	run := func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		asked = args[len(args)-1]
		return []byte("node1 | CHANGED | rc=0 | (stdout) " + marked + "\n"), nil
	}
	missing, lines := CheckClasses(context.Background(), testOptions(), aipubState(), run, []string{DefaultClass})
	if strings.Join(missing, ",") != DefaultClass || !strings.Contains(strings.Join(lines, " "), "기본 StorageClass 가 없습니다") {
		t.Errorf("none marked: %v %v", missing, lines)
	}
	if !strings.Contains(asked, `storageclass\.kubernetes\.io/is-default-class=="true"`) {
		t.Errorf("asked %s", asked)
	}
	marked = "local-path"
	if missing, lines := CheckClasses(context.Background(), testOptions(), aipubState(), run, []string{DefaultClass}); len(missing) != 0 ||
		!strings.Contains(strings.Join(lines, " "), "기본 StorageClass local-path 있음") {
		t.Errorf("one marked: %v %v", missing, lines)
	}
}

func TestReadStorageVars(t *testing.T) {
	p := filepath.Join(t.TempDir(), "vars.yml")
	os.WriteFile(p, []byte("aipub_ha_mode: true\naipub_app_file_storage_type: static\naipub_monitoring_storage_class_name: \"{{ aipub_ha_mode_storage_class }}\"\n"), 0644)
	got := ReadStorageVars(p)
	if len(got) != 1 || got["aipub_app_file_storage_type"] != "static" {
		t.Errorf("%v", got)
	}
	if ReadStorageVars(filepath.Join(t.TempDir(), "none.yml")) != nil {
		t.Error("a missing file gives values")
	}
}

// fakeSite runs the steps of a runner: each script by the playbook or shell
// command in it, writing a line to the log; fail makes a step fail, block holds
// a step until ctx ends or release is closed.
type fakeSite struct {
	mu      sync.Mutex
	ran     []string
	fail    map[string]int    // by ID: exit code
	out     map[string]string // by ID: what the step prints
	block   map[string]chan struct{}
	started map[string]chan struct{}
	classes map[string]bool // the StorageClasses there
	checks  int
	sudo    bool // sudo -n true fails: sudo wants a password
	steps   map[string]Step
	running int
	overlap bool
}

func newFakeSite(stages [][]Step) *fakeSite {
	f := &fakeSite{fail: map[string]int{}, out: map[string]string{}, block: map[string]chan struct{}{},
		started: map[string]chan struct{}{}, classes: map[string]bool{}, steps: map[string]Step{}}
	for _, stage := range stages {
		for _, st := range stage {
			f.steps[st.ID] = st
			f.started[st.ID] = make(chan struct{})
		}
	}
	return f
}

func (f *fakeSite) idOf(script string) string {
	for id, st := range f.steps {
		if st.Kind != Manual && strings.HasSuffix(strings.TrimSpace(script), Command(st)) {
			return id
		}
	}
	return "?"
}

func (f *fakeSite) exec(ctx context.Context, dir string, out io.Writer, name string, args ...string) error {
	id := f.idOf(args[len(args)-1])
	f.mu.Lock()
	f.ran = append(f.ran, id)
	f.running++
	if f.running > 1 {
		f.overlap = true
	}
	block, code, text := f.block[id], f.fail[id], f.out[id]
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.running--
		f.mu.Unlock()
	}()
	close(f.started[id])
	fmt.Fprintf(out, "PLAY [%s]\n%s", id, text)
	if block != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-block:
		}
	}
	if code != 0 {
		return exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	}
	return nil
}

// cmd answers the kubectl of the StorageClass check.
func (f *fakeSite) cmd(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	script := args[len(args)-1]
	f.mu.Lock()
	defer f.mu.Unlock()
	if name == "sudo" {
		if f.sudo {
			return []byte("sudo: a password is required\n"), errors.New("exit status 1")
		}
		return nil, nil
	}
	f.checks++
	for c := range f.steps["M1"].classesSet() {
		if strings.Contains(script, "get storageclass "+c+" ") {
			if f.classes[c] {
				return []byte("node1 | CHANGED | rc=0 | (stdout) storageclass.storage.k8s.io/" + c + "\n"), nil
			}
			return []byte(`node1 | FAILED | rc=1 | (stdout)  (stderr) Error from server (NotFound): storageclasses.storage.k8s.io "` + c + `" not found` + "\n"), nil
		}
	}
	return nil, errors.New("unexpected " + script)
}

func (st Step) classesSet() map[string]bool {
	m := map[string]bool{}
	for _, c := range st.Classes {
		m[c] = true
	}
	return m
}

func (f *fakeSite) ranIDs() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.ran, " ")
}

// newTestRunner is a runner of the plan of aipubState in a root of its own,
// with the site files given as its inputs.
func newTestRunner(t *testing.T, stages [][]Step) (*Runner, *fakeSite, []string) {
	o := testOptions()
	o.Root = t.TempDir()
	site := t.TempDir()
	inputs := []string{filepath.Join(site, "inventory.yml"), filepath.Join(site, "vars.yml")}
	for _, p := range inputs {
		os.WriteFile(p, []byte("a: 1\n"), 0644)
	}
	r := NewRunner(o, aipubState(), stages, inputs)
	f := newFakeSite(stages)
	r.Exec, r.Cmd = f.exec, f.cmd
	return r, f, inputs
}

func aipubStages() [][]Step {
	o := testOptions()
	o.ArchivePath = "/media/usb/a.tgz"
	return Stages(Build(aipubState(), o), o, o.AnsibleDir())
}

func statuses(st RunState) string {
	var out []string
	for _, s := range st.Steps {
		out = append(out, s.ID+"="+string(s.Status))
	}
	return strings.Join(out, " ")
}

func TestRunnerRunsTheStagesInOrder(t *testing.T) {
	r, f, _ := newTestRunner(t, aipubStages())
	f.classes["nfs-client"] = true // there already: M1 does not wait
	f.out["K3"] = "TASK [kubeadm init]\nok: [node1]\n"
	outcome, err := r.Run(context.Background(), false)
	if err != nil || outcome != OutcomeOK {
		t.Fatalf("outcome %q, %v", outcome, err)
	}
	if got, want := f.ranIDs(), "K1 K2 K3 K4 A1 A2 A3 A4 A5 A5.1 A5.2"; got != want {
		t.Errorf("ran %s, want %s", got, want)
	}
	st, err := LoadRunState(r.Dir)
	if err != nil || st == nil || st.Outcome != OutcomeOK {
		t.Fatalf("state.json: %+v %v", st, err)
	}
	for _, s := range st.Steps {
		if s.Status != StatusOK || s.Start == nil || s.End == nil || s.Log == "" {
			t.Errorf("%s: %+v", s.ID, s)
		}
	}
	if m1 := st.Step("M1"); m1.Stage != st.Step("A2").Stage || !strings.Contains(m1.Note, "nfs-client 있음") {
		t.Errorf("M1: %+v", m1)
	}
	log, _ := os.ReadFile(filepath.Join(r.Dir, "logs", "K3.log"))
	if !strings.Contains(string(log), "ansible-playbook -i inventory.yml playbooks/setup-cluster.yml") ||
		!strings.Contains(string(log), "TASK [kubeadm init]") || !strings.Contains(string(log), "K3 끝: 종료 코드 0") {
		t.Errorf("K3.log:\n%s", log)
	}
}

func TestRunnerStopsAtAFailedStep(t *testing.T) {
	r, f, _ := newTestRunner(t, aipubStages())
	f.fail["K3"] = 2
	f.out["K3"] = "fatal: [node1]: FAILED! => {\"msg\": \"kubeadm init failed\"}\n"
	outcome, err := r.Run(context.Background(), false)
	if err != nil || outcome != OutcomeFailed {
		t.Fatalf("outcome %q, %v", outcome, err)
	}
	if got := f.ranIDs(); got != "K1 K2 K3" {
		t.Errorf("ran %s", got)
	}
	st, _ := LoadRunState(r.Dir)
	k3 := st.Step("K3")
	if k3.Status != StatusFailed || k3.ExitCode == nil || *k3.ExitCode != 2 {
		t.Errorf("K3: %+v", k3)
	}
	if tail := strings.Join(TailLines(k3.Log, 10), "\n"); !strings.Contains(tail, "kubeadm init failed") {
		t.Errorf("log tail: %s", tail)
	}
	if s := st.Step("K4"); s.Status != StatusPending {
		t.Errorf("K4: %+v", s)
	}
}

// Two steps of a stage run at the same time; one failing lets the other finish.
func TestRunnerRunsAStageTogether(t *testing.T) {
	stages := [][]Step{
		{{ID: "X1", Kind: Shell, Shell: []string{"echo x1"}}, {ID: "X2", Kind: Shell, Shell: []string{"echo x2"}, WithPrev: true}},
		{{ID: "X3", Kind: Shell, Shell: []string{"echo x3"}}},
	}
	r, f, _ := newTestRunner(t, stages)
	release := make(chan struct{})
	f.block["X1"], f.block["X2"] = release, release
	f.fail["X1"] = 1
	go func() {
		<-f.started["X1"]
		<-f.started["X2"] // both started before either ended
		close(release)
	}()
	outcome, _ := r.Run(context.Background(), false)
	st := r.Snapshot()
	if outcome != OutcomeFailed || !f.overlap || statuses(st) != "X1=failed X2=ok X3=pending" {
		t.Errorf("outcome %s, overlap %v: %s", outcome, f.overlap, statuses(st))
	}
}

func TestRunnerResumesFromTheFailedStep(t *testing.T) {
	stages := aipubStages()
	r, f, inputs := newTestRunner(t, stages)
	f.classes["nfs-client"] = true
	f.fail["A3"] = 1
	if outcome, _ := r.Run(context.Background(), false); outcome != OutcomeFailed {
		t.Fatalf("first run: %s", outcome)
	}
	prev, _ := LoadRunState(r.Dir)
	if prev.InputHash != InputHash(inputs) {
		t.Error("the hash of the inputs is not that of the files")
	}

	r2 := NewRunner(r.Opts, r.State, stages, inputs)
	f2 := newFakeSite(stages)
	f2.classes["nfs-client"] = true
	r2.Exec, r2.Cmd = f2.exec, f2.cmd
	if outcome, err := r2.Run(context.Background(), true); outcome != OutcomeOK || err != nil {
		t.Fatalf("resume: %s %v", outcome, err)
	}
	if got := f2.ranIDs(); got != "A3 A4 A5 A5.1 A5.2" {
		t.Errorf("resume ran %s", got)
	}
	st, _ := LoadRunState(r.Dir)
	if k1 := st.Step("K1"); k1.Start == nil || !k1.Start.Equal(*prev.Step("K1").Start) {
		t.Errorf("K1 kept: %+v", k1)
	}
	// a file of the site changed since
	os.WriteFile(inputs[1], []byte("a: 2\n"), 0644)
	if st.InputHash == InputHash(inputs) {
		t.Error("a change of the site files is not seen")
	}
	// without resume every step runs again
	r3 := NewRunner(r.Opts, r.State, stages, inputs)
	f3 := newFakeSite(stages)
	f3.classes["nfs-client"] = true
	r3.Exec, r3.Cmd = f3.exec, f3.cmd
	r3.Run(context.Background(), false)
	if got := f3.ranIDs(); !strings.HasPrefix(got, "K1 K2") {
		t.Errorf("from the start ran %s", got)
	}
}

// The copy of a site saved elsewhere runs on every run: a resume after the
// site was saved again copies it before the playbooks read it.
func TestRunnerResumeCopiesTheSiteAgain(t *testing.T) {
	o := testOptions()
	o.ArchivePath = "/media/usb/a.tgz"
	stages := Stages(Build(aipubState(), o), o, "/home/ops/site")
	r, f, inputs := newTestRunner(t, stages)
	f.classes["nfs-client"] = true
	f.fail["A3"] = 1
	if outcome, _ := r.Run(context.Background(), false); outcome != OutcomeFailed || !strings.HasPrefix(f.ranIDs(), "C1 K1") {
		t.Fatalf("first run: %s, ran %s", outcome, f.ranIDs())
	}
	r2 := NewRunner(r.Opts, r.State, stages, inputs)
	f2 := newFakeSite(stages)
	f2.classes["nfs-client"] = true
	r2.Exec, r2.Cmd = f2.exec, f2.cmd
	if outcome, err := r2.Run(context.Background(), true); outcome != OutcomeOK || err != nil {
		t.Fatalf("resume: %s %v", outcome, err)
	}
	if got := f2.ranIDs(); got != "C1 A3 A4 A5 A5.1 A5.2" {
		t.Errorf("resume ran %s, want the copy first", got)
	}
}

// C1 keeps what it replaces in the history of the site, and stops once on a
// file edited on the installer side since the last copy.
func TestRunnerCopyStopsOnceOnAnInstallerEdit(t *testing.T) {
	o := testOptions()
	o.Root = t.TempDir()
	site := t.TempDir()
	o.SiteDir = site
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(site, "inventory.yml"), []byte("a: 1\n"), 0644))
	dst := filepath.Join(filepath.FromSlash(o.AnsibleDir()), "inventory.yml")
	must(os.MkdirAll(filepath.Dir(dst), 0755))
	stages := Stages(nil, o, site)
	// run runs C1 and says whether its copy command ran
	run := func() (Outcome, StepState, bool) {
		r := NewRunner(o, aipubState(), stages, nil)
		f := newFakeSite(stages)
		r.Exec, r.Cmd = f.exec, f.cmd
		outcome, err := r.Run(context.Background(), false)
		if err != nil {
			t.Fatal(err)
		}
		st := r.Snapshot()
		return outcome, *st.Step("C1"), f.ranIDs() == "C1"
	}
	if outcome, c1, ran := run(); outcome != OutcomeOK || !ran {
		t.Fatalf("first copy: %s %+v, ran %v", outcome, c1, ran)
	}
	// what the copy did, then an edit by hand on the installer side
	must(os.WriteFile(dst, []byte("a: 1\nb: by hand\n"), 0644))
	outcome, c1, ran := run()
	if outcome != OutcomeFailed || ran || !strings.Contains(c1.Note, "inventory.yml") || !strings.Contains(c1.Note, "history.jsonl") {
		t.Fatalf("installer edit: %s %+v, copy ran %v", outcome, c1, ran)
	}
	if outcome, c1, ran := run(); outcome != OutcomeOK || !ran {
		t.Errorf("after it was kept: %s %+v, ran %v", outcome, c1, ran)
	}
}

// A shell step that runs sudo here (C1, A5.1) has no terminal for a password:
// the run does not start when sudo would ask one.
func TestRunnerNeedsSudoWithoutAPassword(t *testing.T) {
	r, f, _ := newTestRunner(t, aipubStages()) // A5.1
	f.sudo = true
	_, err := r.Run(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "A5.1 는 이 제어 노드에서 sudo 로 실행하는데") ||
		!strings.Contains(err.Error(), "a password is required") || f.ranIDs() != "" {
		t.Errorf("err %v, ran %q", err, f.ranIDs())
	}
	if _, err := os.Stat(filepath.Join(r.Dir, stateFile)); err == nil {
		t.Error("state.json written by a run that did not start")
	}
	// none of them to run: not asked
	r, f, _ = newTestRunner(t, [][]Step{{{ID: "X1", Kind: Shell, Shell: []string{"echo x1"}}}})
	f.sudo = true
	if o, err := r.Run(context.Background(), false); o != OutcomeOK || err != nil {
		t.Errorf("%s %v", o, err)
	}
}

// Stop lets the running step finish and starts no other; Kill ends it now.
func TestRunnerStopAndKill(t *testing.T) {
	stages := [][]Step{{{ID: "X1", Kind: Shell, Shell: []string{"x1"}}}, {{ID: "X2", Kind: Shell, Shell: []string{"x2"}}}}
	r, f, _ := newTestRunner(t, stages)
	release := make(chan struct{})
	f.block["X1"] = release
	done := make(chan Outcome)
	go func() { o, _ := r.Run(context.Background(), false); done <- o }()
	<-f.started["X1"]
	r.Stop()
	close(release)
	if o := <-done; o != OutcomeStopped || statuses(r.Snapshot()) != "X1=ok X2=pending" || f.ranIDs() != "X1" {
		t.Errorf("stop: %s %s", o, statuses(r.Snapshot()))
	}

	r, f, _ = newTestRunner(t, stages)
	f.block["X1"] = make(chan struct{}) // never released
	go func() { o, _ := r.Run(context.Background(), false); done <- o }()
	<-f.started["X1"]
	r.Kill()
	select {
	case o := <-done:
		if o != OutcomeStopped || statuses(r.Snapshot()) != "X1=cancelled X2=pending" {
			t.Errorf("kill: %s %s", o, statuses(r.Snapshot()))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Kill does not end the step")
	}
}

func waitStatus(t *testing.T, r *Runner, id string, want StepStatus) {
	t.Helper()
	for i := 0; i < 500; i++ {
		snap := r.Snapshot()
		if s := snap.Step(id); s != nil && s.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	snap := r.Snapshot()
	t.Fatalf("%s never %s: %s", id, want, statuses(snap))
}

// M1 waits until the operator says the driver is in and every class is there.
func TestRunnerWaitsForTheStorageClass(t *testing.T) {
	r, f, _ := newTestRunner(t, aipubStages())
	var seen []StepState
	var mu sync.Mutex
	r.Notify = func(s StepState) {
		mu.Lock()
		seen = append(seen, s)
		mu.Unlock()
	}
	done := make(chan Outcome)
	go func() { o, _ := r.Run(context.Background(), false); done <- o }()
	waitStatus(t, r, "M1", StatusWaiting)
	snap := r.Snapshot()
	if m1 := snap.Step("M1"); !strings.Contains(m1.Note, "없음: nfs-client") {
		t.Errorf("M1: %+v", m1)
	}
	waitStatus(t, r, "A2", StatusOK) // setup-cert ran beside it
	if s := snap.Step("A3"); s.Status != StatusPending {
		t.Errorf("A3 started before the class: %+v", s)
	}
	r.Continue() // not there yet: waits again
	for i := 0; i < 500 && f.checksDone() < 2; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	waitStatus(t, r, "M1", StatusWaiting)
	f.mu.Lock()
	f.classes["nfs-client"] = true
	f.mu.Unlock()
	r.Continue()
	if o := <-done; o != OutcomeOK {
		t.Fatalf("outcome %s: %s", o, statuses(r.Snapshot()))
	}
	if f.checksDone() != 3 {
		t.Errorf("%d checks", f.checksDone())
	}
	log, _ := os.ReadFile(filepath.Join(r.Dir, "logs", "M1.log"))
	if !strings.Contains(string(log), "설치함 (운영자), 다시 확인") || !strings.Contains(string(log), "nfs-client 있음") {
		t.Errorf("M1.log:\n%s", log)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 {
		t.Error("no notification")
	}
}

func (f *fakeSite) checksDone() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checks
}

// setup-cert failing beside the stop ends the wait: the run stops there.
func TestRunnerStopsWaitingWhenTheStageFails(t *testing.T) {
	r, f, _ := newTestRunner(t, aipubStages())
	f.fail["A2"] = 1
	outcome, _ := r.Run(context.Background(), false)
	st := r.Snapshot()
	if outcome != OutcomeFailed || st.Step("M1").Status != StatusCancelled || st.Step("A3").Status != StatusPending {
		t.Errorf("%s: %s", outcome, statuses(st))
	}
}

func TestRunnerExplainsTheBackendInformer(t *testing.T) {
	r, f, _ := newTestRunner(t, aipubStages())
	f.classes["nfs-client"] = true
	f.fail["A5"] = 2
	f.out["A5"] = "[ERROR] CRD 'workspaces.aipub.ten1010.io' does not exist. The informers cannot sync\n"
	outcome, _ := r.Run(context.Background(), false)
	snap := r.Snapshot()
	a5 := snap.Step("A5")
	if outcome != OutcomeFailed || !strings.Contains(a5.Note, "Python 서비스를 설치한 뒤 이어서 실행") {
		t.Errorf("%s %+v", outcome, a5)
	}
	// nor is it known that the informer was all that failed
	if strings.Contains(a5.Note, "그 앞의 서비스는 설치됐습니다") || !strings.Contains(a5.Note, "확인되지 않아") {
		t.Errorf("the error alone taken for the informer alone: %q", a5.Note)
	}
	if got := f.ranIDs(); strings.Count(got, "A5") != 1 {
		t.Errorf("setup-aipub retried: %s", got)
	}
	// the error alone, with no result line of install.sh, is not known to be
	// the informer only: the run stops there
	if a5.Continued || snap.Step("A5.1").Status != StatusPending {
		t.Errorf("went on without the result lines: %s", statuses(snap))
	}
}

// What setup-aipub of aipub-installer prints when backend-informer is the one
// component that failed, for want of the Workspace CRD (helmfile mode, from a
// real run: the colour codes, both result lines, the failure of install.sh
// that ansible ignores and the one task that ends the play).
const informerEmit = "TASK [[aipub] install.sh db backend-api aipub-nav backend-informer --emit-values] ***\n" +
	"fatal: [node1]: FAILED! => \n" +
	"    changed: true\n" +
	"        \x1b[0;31m[ERROR]\x1b[0m CRD 'workspaces.aipub.ten1010.io' does not exist. The informers cannot sync and the pod would restart in a loop.\n" +
	"        \x1b[0;31m[ERROR]\x1b[0m install backend-informer failed (exit 1); see the log above.\n" +
	"        AIPUB_RESULT module=aipub action=install component=backend-informer result=FAIL exit=1\n" +
	"        AIPUB_JAVA_RESULT module=aipub action=install component=backend-informer result=FAIL exit=1\n" +
	"...ignoring\n" +
	"\n" +
	"TASK [[aipub] Result of db backend-api aipub-nav backend-informer] ***\n" +
	"ok: [node1] => \n" +
	"    - AIPUB_RESULT module=aipub action=install component=aipub-nav result=OK\n" +
	"    - AIPUB_RESULT module=aipub action=install component=backend-informer result=FAIL\n"

const informerStop = informerEmit +
	"TASK [[aipub] Result of helmfile sync] ***\n" +
	"ok: [node1]\n" +
	"TASK [[aipub] Stop: install.sh --emit-values failed] ***************************\n" +
	"fatal: [node1]: FAILED! => \n" +
	"    changed: false\n"

// The same stop with the installer (or dag) as the deploy tool: the call that
// has backend-informer in it is ignored, and its result task ends the play.
const informerStopInstaller = "TASK [[aipub] install.sh aipub-nav backend-informer] ***\n" +
	"fatal: [node1]: FAILED! => \n" +
	"        \x1b[0;31m[ERROR]\x1b[0m CRD 'workspaces.aipub.ten1010.io' does not exist. The informers cannot sync and the pod would restart in a loop.\n" +
	"        AIPUB_RESULT module=aipub action=install component=backend-informer result=FAIL exit=1\n" +
	"...ignoring\n" +
	"TASK [[aipub] Result of aipub-nav backend-informer] ****\n" +
	"fatal: [node1]: FAILED! => \n" +
	"    - AIPUB_RESULT module=aipub action=install component=backend-informer result=FAIL\n"

// The run goes on past setup-aipub when only backend-informer failed for want
// of the Workspace CRD: the JWKS steps after it need the backend and the CA,
// not the informer. The step stays failed, and a resume runs it alone.
func TestRunnerGoesOnPastTheInformerStop(t *testing.T) {
	stages := aipubStages()
	r, f, inputs := newTestRunner(t, stages)
	f.classes["nfs-client"] = true
	f.fail["A5"] = 2
	f.out["A5"] = informerStop
	outcome, err := r.Run(context.Background(), false)
	snap := r.Snapshot()
	a5 := snap.Step("A5")
	if err != nil || outcome != OutcomeFailed || a5.Status != StatusFailed || !a5.Continued ||
		!strings.Contains(a5.Note, "Python 서비스를 설치한 뒤 이어서 실행") || !strings.Contains(a5.Note, "계속합니다") {
		t.Fatalf("%s %v %+v", outcome, err, a5)
	}
	if snap.Step("A5.1").Status != StatusOK || snap.Step("A5.2").Status != StatusOK || !strings.HasSuffix(f.ranIDs(), "A5 A5.1 A5.2") {
		t.Fatalf("the JWKS steps did not run after it: %s, ran %s", statuses(snap), f.ranIDs())
	}
	st, _ := LoadRunState(r.Dir)
	if st == nil || st.Outcome != OutcomeFailed || !st.WentOn() {
		t.Fatalf("state.json: %+v", st)
	}

	// the python services are there: a resume runs setup-aipub, and the JWKS
	// steps again after it (its k8s-jwks has written the config on the nodes
	// by hand once more), nothing before it
	r2 := NewRunner(r.Opts, r.State, stages, inputs)
	f2 := newFakeSite(stages)
	f2.classes["nfs-client"] = true
	r2.Exec, r2.Cmd = f2.exec, f2.cmd
	if outcome, err := r2.Run(context.Background(), true); outcome != OutcomeOK || err != nil || f2.ranIDs() != "A5 A5.1 A5.2" {
		t.Fatalf("resume: %s %v, ran %s", outcome, err, f2.ranIDs())
	}
	if snap := r2.Snapshot(); snap.Step("A5").Continued || snap.WentOn() {
		t.Errorf("after it succeeded: %+v", snap.Step("A5"))
	}

	// with the installer as the deploy tool the same stop looks different
	r3, f3, _ := newTestRunner(t, stages)
	f3.classes["nfs-client"] = true
	f3.fail["A5"] = 2
	f3.out["A5"] = informerStopInstaller
	r3.Run(context.Background(), false)
	if snap := r3.Snapshot(); !snap.Step("A5").Continued || snap.Step("A5.2").Status != StatusOK {
		t.Errorf("installer mode: %s", statuses(snap))
	}
}

func TestFailedTasks(t *testing.T) {
	got := failedTasks(strings.Split(informerStop, "\n"))
	if len(got) != 1 || got[0] != "[aipub] Stop: install.sh --emit-values failed" {
		t.Errorf("%q", got)
	}
	if got := failedTasks(strings.Split(informerEmit, "\n")); len(got) != 0 {
		t.Errorf("an ignored failure counted: %q", got)
	}
}

// Anything else failing in setup-aipub stops the run there, as any failure: a
// second component, a node out of reach, and what an earlier attempt said.
func TestRunnerStopsWhenMoreThanTheInformerFailed(t *testing.T) {
	for name, out := range map[string]string{
		"another component failed too": informerStop + "    - AIPUB_RESULT module=aipub action=install component=backend-api result=FAIL\n",
		"a node out of reach":          informerStop + "fatal: [node2]: UNREACHABLE! => {\"msg\": \"Failed to connect\"}\n",
		"the CRD's name, no error":     "  can-i list workspaces.aipub.ten1010.io: yes\n    - AIPUB_RESULT module=aipub action=install component=backend-informer result=FAIL\n",
		// the informer's error is in the log of every new cluster: what ended
		// the play is what counts. Here helmfile sync did, before k8s-jwks and
		// the rest, and printed no line of a component
		"helmfile sync failed":                    informerEmit + "TASK [[aipub] Result of helmfile sync] ***\nfatal: [node1]: FAILED! => \n    msg: helmfile sync failed\n",
		"a task failed after the stop was passed": informerStop + "TASK [[aipub] Something later] ***\nfatal: [node1]: FAILED! => \n",
		"install.sh died without a result line":   informerEmit + "TASK [[aipub] Result of db backend-api aipub-nav --after-helmfile] ***\nfatal: [node1]: FAILED! => \n",
	} {
		r, f, _ := newTestRunner(t, aipubStages())
		f.classes["nfs-client"] = true
		f.fail["A5"] = 2
		f.out["A5"] = out
		outcome, _ := r.Run(context.Background(), false)
		snap := r.Snapshot()
		if outcome != OutcomeFailed || snap.Step("A5").Continued || snap.Step("A5.1").Status != StatusPending || snap.WentOn() {
			t.Errorf("%s: went on: %s", name, statuses(snap))
		}
		// nor does its note say the informer was all that failed
		if note := snap.Step("A5").Note; strings.Contains(note, "알려진 일") || strings.Contains(note, "그 앞의 서비스는 설치됐습니다") {
			t.Errorf("%s: the informer's note: %q", name, note)
		}
	}

	// an earlier attempt stopped on the informer; this one fails on something else
	stages := aipubStages()
	r, f, inputs := newTestRunner(t, stages)
	f.classes["nfs-client"] = true
	f.fail["A5"] = 2
	f.out["A5"] = informerStop
	r.Run(context.Background(), false)
	r2 := NewRunner(r.Opts, r.State, stages, inputs)
	f2 := newFakeSite(stages)
	f2.classes["nfs-client"] = true
	f2.fail["A5"] = 2
	f2.out["A5"] = "fatal: [node1]: FAILED! => {\"msg\": \"helm upgrade failed\"}\n"
	r2.Exec, r2.Cmd = f2.exec, f2.cmd
	outcome, _ := r2.Run(context.Background(), true)
	snap2 := r2.Snapshot()
	a5 := snap2.Step("A5")
	if outcome != OutcomeFailed || a5.Continued || strings.Contains(a5.Note, "Workspace CRD") {
		t.Errorf("the earlier attempt decided this one: %s %+v", outcome, a5)
	}
}

func TestRunnerNeverRunsAReset(t *testing.T) {
	stages := [][]Step{{{ID: "X1", Kind: Playbook, Playbook: "aipub-playbooks/reset-base.yml"}},
		{{ID: "X2", Kind: Playbook, Playbook: "aipub-playbooks/setup-base.yml", Args: []string{"-e", "delete_data="}}}}
	for _, st := range [][]Step{stages[0], stages[1]} {
		r, f, _ := newTestRunner(t, [][]Step{st})
		outcome, _ := r.Run(context.Background(), false)
		if outcome != OutcomeFailed || f.ranIDs() != "" || !strings.Contains(r.Snapshot().Steps[0].Note, "실행하지 않습니다") {
			t.Errorf("%s ran: %s %+v", st[0].ID, f.ranIDs(), r.Snapshot().Steps[0])
		}
	}
}

// A reset or remove playbook named in a shell line is refused like a playbook step.
func TestRunnerNeverRunsAResetInAShellLine(t *testing.T) {
	for _, line := range []string{"ansible-playbook -i inventory.yml playbooks/reset-cluster.yml",
		"cd x && ansible-playbook 'aipub-playbooks/remove-aipub.yaml'"} {
		r, f, _ := newTestRunner(t, [][]Step{{{ID: "X1", Kind: Shell, Shell: []string{line}}}})
		if outcome, _ := r.Run(context.Background(), false); outcome != OutcomeFailed || f.ranIDs() != "" {
			t.Errorf("%q ran: %s %s", line, outcome, f.ranIDs())
		}
	}
	// the copy of a site in a directory that happens to be called so is no reset
	if forbidden(Step{Kind: Shell, Shell: []string{"sudo -n cp /home/ops/reset-site/inventory.yml /opt/k8s-installer/ansible/inventory.yml"}}) {
		t.Error("a path is taken for a reset playbook")
	}
}

// Kill before Run starts: nothing runs and state.json stays as the last run left it.
func TestRunnerKilledBeforeItStarts(t *testing.T) {
	r, f, _ := newTestRunner(t, aipubStages())
	r.Kill()
	if o, err := r.Run(context.Background(), false); o != OutcomeStopped || err != nil || f.ranIDs() != "" {
		t.Errorf("%s %v, ran %q", o, err, f.ranIDs())
	}
	if _, err := os.Stat(filepath.Join(r.Dir, stateFile)); err == nil {
		t.Error("state.json written by a run killed before it started")
	}
}

// An Enter pressed before the StorageClass stop asks does not answer it.
func TestRunnerStopIgnoresAnEarlierEnter(t *testing.T) {
	r, f, _ := newTestRunner(t, aipubStages())
	r.Continue() // pressed early
	done := make(chan Outcome)
	go func() { o, _ := r.Run(context.Background(), false); done <- o }()
	waitStatus(t, r, "M1", StatusWaiting)
	time.Sleep(200 * time.Millisecond)
	snap := r.Snapshot()
	if n := f.checksDone(); n != 1 || snap.Step("M1").Status != StatusWaiting {
		t.Errorf("%d checks, M1 %s: the early Enter answered", n, snap.Step("M1").Status)
	}
	r.Kill()
	<-done
}

// A state.json that cannot be written once the run is under way stops nothing,
// and the first error is kept for the end of the run.
func TestRunnerKeepsTheFirstSaveError(t *testing.T) {
	stages := [][]Step{{{ID: "X1", Kind: Shell, Shell: []string{"x1"}}}, {{ID: "X2", Kind: Shell, Shell: []string{"x2"}}}}
	r, f, _ := newTestRunner(t, stages)
	release := make(chan struct{})
	f.block["X1"] = release
	done := make(chan Outcome)
	go func() { o, _ := r.Run(context.Background(), false); done <- o }()
	<-f.started["X1"]
	// a directory where state.json was: the rename onto it fails, for root too
	p := filepath.Join(r.Dir, stateFile)
	os.Remove(p)
	os.MkdirAll(filepath.Join(p, "x"), 0700)
	close(release)
	if o := <-done; o != OutcomeOK || f.ranIDs() != "X1 X2" {
		t.Errorf("%s, ran %s", o, f.ranIDs())
	}
	if err := r.SaveErr(); err == nil {
		t.Error("no save error kept")
	}
}

func TestStepScript(t *testing.T) {
	o := testOptions()
	o.ShellExecutable = "/usr/bin/dash"
	var k3 Step
	for _, st := range Build(aipubState(), o) {
		if st.ID == "K3" {
			k3 = st
		}
	}
	script := StepScript(o, k3)
	for _, want := range []string{"source /opt/k8s-installer/venv/bin/activate", "cd /opt/k8s-installer/ansible",
		"ansible-playbook -i inventory.yml playbooks/setup-cluster.yml", "ansible_shell_executable=/usr/bin/dash"} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
}

func TestTailLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.log")
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	os.WriteFile(p, []byte(b.String()), 0644)
	if got := TailLines(p, 3); strings.Join(got, ",") != "line 19997,line 19998,line 19999" {
		t.Errorf("%v", got)
	}
	if TailLines(filepath.Join(t.TempDir(), "none"), 3) != nil {
		t.Error("lines of a missing file")
	}
}

func TestCheckVerdict(t *testing.T) {
	steps := Build(state.DefaultState(), testOptions()) // P1 P2 P6
	notRun, failed := CheckVerdict(steps, map[string]CheckResult{"P1": {OK: true}, "P2": {OK: true, Warn: true}})
	if strings.Join(notRun, ",") != "P6" || len(failed) != 0 {
		t.Errorf("%v %v", notRun, failed)
	}
	_, failed = CheckVerdict(steps, map[string]CheckResult{"P1": {OK: true}, "P2": {}, "P6": {OK: true}})
	if strings.Join(failed, ",") != "P2" {
		t.Errorf("%v", failed)
	}
}
