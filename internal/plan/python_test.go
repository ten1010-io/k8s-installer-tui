package plan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// pythonOptions are the options of a run whose installer has the python module:
// an install root with aipub-playbooks/setup-python.yml in it.
func pythonOptions(t *testing.T) Options {
	t.Helper()
	o := testOptions()
	o.Root = t.TempDir()
	o.ArchivePath = "/media/usb/a.tgz"
	path := filepath.Join(o.AnsibleDir(), filepath.FromSlash(PythonPlaybook))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# setup-python.yml\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return o
}

func stepOf(steps []Step, id string) Step {
	for _, s := range steps {
		if s.ID == id {
			return s
		}
	}
	return Step{}
}

// With an installer that has the python module, the plan ends with
// setup-python and setup-aipub once more, after every step that runs
// update-cluster.yml. Beside an installer without it, the plan is as it was.
func TestPythonSteps(t *testing.T) {
	o := pythonOptions(t)
	steps := Build(aipubState(), o)
	if got, want := ids(steps), "P1 P2 P5 P6 P7 P8 K1 K2 K3 K4 A1 A2 M1 A3 A4 A5 A5.1 A5.2 A6 A7"; got != want {
		t.Errorf("with python: %s, want %s", got, want)
	}
	a6, a7 := stepOf(steps, "A6"), stepOf(steps, "A7")
	if a6.Playbook != "aipub-playbooks/setup-python.yml" || a7.Playbook != "aipub-playbooks/setup-aipub.yml" || a6.Kind != Playbook {
		t.Errorf("A6 %+v, A7 %+v", a6, a7)
	}
	if strings.Contains(stepOf(steps, "A5").Note, "포함하지 않습니다") {
		t.Errorf("A5 still says python is not included: %q", stepOf(steps, "A5").Note)
	}

	// left out by the operator (--no-python, the install screen)
	o.Python = false
	if got := ids(Build(aipubState(), o)); !strings.HasSuffix(got, "A5.2 (A6) (A7)") {
		t.Errorf("python left out: %s", got)
	}

	// an installer from before the module
	old := testOptions()
	old.Root = t.TempDir()
	if got := ids(Build(aipubState(), old)); strings.Contains(got, "A6") {
		t.Errorf("an installer without setup-python.yml: %s", got)
	}
	// and a site without AIPub
	if got := ids(Build(state.DefaultState(), pythonOptions(t))); strings.Contains(got, "A6") {
		t.Errorf("no AIPub: %s", got)
	}
}

// setup-python.yml puts the audit log usage-manager reads into the manifest by
// hand unless the audit preset has it in the kubeadm configuration, and an
// update-cluster.yml after it takes it out. The step says so while the preset
// is off, and says nothing once it is on.
func TestPythonStepSaysWhenTheAuditPresetIsOff(t *testing.T) {
	o := pythonOptions(t)
	s := aipubState()
	if note := stepOf(Build(s, o), "A6").Note; !strings.Contains(note, "감사 로그 프리셋") {
		t.Errorf("preset off, and the step does not say so: %q", note)
	}
	s.ControlPlane.Audit = true
	if note := stepOf(Build(s, o), "A6").Note; strings.Contains(note, "감사 로그 프리셋") {
		t.Errorf("preset on, and the step still warns: %q", note)
	}
}

// The stop of setup-aipub on a new cluster (only backend-informer, for want of
// the Workspace CRD) is what the python services cure. With them in the plan
// the run goes on through A6 and A7, the A5 it went on past is good once A7 has
// succeeded, and the run ends ok with nothing left for a resume.
func TestRunnerClearsTheInformerStopAfterPython(t *testing.T) {
	o := pythonOptions(t)
	stages := Stages(Build(aipubState(), o), o, o.AnsibleDir())
	newRunner := func() (*Runner, *fakeSite) {
		site := t.TempDir()
		inputs := []string{filepath.Join(site, "inventory.yml"), filepath.Join(site, "vars.yml")}
		for _, p := range inputs {
			os.WriteFile(p, []byte("a: 1\n"), 0644)
		}
		ro := o
		ro.Root = t.TempDir()
		// the runner asks the root whether the installer has python
		path := filepath.Join(ro.AnsibleDir(), filepath.FromSlash(PythonPlaybook))
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte("# setup-python.yml\n"), 0644)
		r := NewRunner(ro, aipubState(), stages, inputs)
		f := newFakeSite(stages)
		f.classes["nfs-client"] = true
		r.Exec, r.Cmd = f.exec, f.cmd
		return r, f
	}

	r, f := newRunner()
	f.fail["A5"] = 2
	f.out["A5"] = informerStop
	outcome, err := r.Run(context.Background(), false)
	snap := r.Snapshot()
	if err != nil || outcome != OutcomeOK {
		t.Fatalf("outcome %s %v: %s", outcome, err, statuses(snap))
	}
	if !strings.HasSuffix(f.ranIDs(), "A5 A5.1 A5.2 A6 A7") {
		t.Fatalf("ran %s", f.ranIDs())
	}
	a5 := snap.Step("A5")
	if a5.Status != StatusOK || a5.Continued || !strings.Contains(a5.Note, "A7") {
		t.Errorf("A5 after A7 succeeded: %+v", a5)
	}
	if st, _ := LoadRunState(r.Dir); st == nil || st.Outcome != OutcomeOK || st.WentOn() || !st.Python {
		t.Errorf("state.json: %+v", st)
	}

	// The informer still failing in A7 is a failure like any other: the run
	// stops there, failed, and A5 stays the step it went on past.
	r, f = newRunner()
	f.fail["A5"], f.out["A5"] = 2, informerStop
	f.fail["A7"], f.out["A7"] = 2, informerStop
	outcome, _ = r.Run(context.Background(), false)
	snap = r.Snapshot()
	if outcome != OutcomeFailed || snap.Step("A7").Status != StatusFailed || snap.Step("A7").Continued || !snap.Step("A5").Continued {
		t.Errorf("A7 failed on the informer: %s, %s", outcome, statuses(snap))
	}

	// A6 failing stops the run before A7, and A5 is still to be made good.
	r, f = newRunner()
	f.fail["A5"], f.out["A5"] = 2, informerStop
	f.fail["A6"] = 2
	outcome, _ = r.Run(context.Background(), false)
	snap = r.Snapshot()
	if outcome != OutcomeFailed || snap.Step("A7").Status == StatusOK || !snap.Step("A5").Continued || strings.Contains(f.ranIDs(), "A7") {
		t.Errorf("A6 failed: %s, %s, ran %s", outcome, statuses(snap), f.ranIDs())
	}

	// While the run goes on, A5 says what will clear it rather than "install
	// the python services, then resume"
	r, f = newRunner()
	f.fail["A5"], f.out["A5"] = 2, informerStop
	f.fail["A6"] = 2
	r.Run(context.Background(), false)
	snap = r.Snapshot()
	if note := snap.Step("A5").Note; !strings.Contains(note, "A6") || strings.Contains(note, "설치한 뒤 이어서 실행") {
		t.Errorf("A5 while python is in the plan: %q", note)
	}
}
