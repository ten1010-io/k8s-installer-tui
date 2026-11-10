//go:build unix

package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A second run is refused while the lock is held. Only flock holds it, so this
// is a unix test: elsewhere lockFile opens the file and holds nothing.
func TestRunnerRefusesWhenLocked(t *testing.T) {
	r, f, _ := newTestRunner(t, aipubStages())
	os.MkdirAll(r.Dir, 0755)
	held, err := lockFile(filepath.Join(r.Dir, "lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	_, err = r.Run(context.Background(), false)
	var locked *LockedError
	if !errors.As(err, &locked) || locked.PID != fmt.Sprint(os.Getpid()) || f.ranIDs() != "" {
		t.Errorf("err %v, ran %q", err, f.ranIDs())
	}
	if _, err := os.Stat(filepath.Join(r.Dir, stateFile)); err == nil {
		t.Error("state.json written by a run that did not start")
	}
}

// A step left running by a program killed outright (its process group still
// alive) keeps a run from starting, resume or not; once it has ended, the run
// starts.
func TestRunnerRefusesWhileAStepIsLeftRunning(t *testing.T) {
	r, f, inputs := newTestRunner(t, aipubStages())
	left := exec.Command("sleep", "30")
	left.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := left.Start(); err != nil {
		t.Fatal(err)
	}
	pid := left.Process.Pid
	defer left.Process.Kill()
	os.MkdirAll(r.Dir, 0700)
	prev := RunState{InputHash: InputHash(inputs), Steps: []StepState{{ID: "K1", Status: StatusRunning, PGID: pid}}}
	data, _ := json.Marshal(prev)
	os.WriteFile(filepath.Join(r.Dir, stateFile), data, 0600)
	for _, resume := range []bool{true, false} {
		_, err := r.Run(context.Background(), resume)
		var still *StillRunningError
		if !errors.As(err, &still) || still.PGID != pid || !strings.Contains(err.Error(), fmt.Sprintf("pid %d", pid)) || f.ranIDs() != "" {
			t.Errorf("resume %v: err %v, ran %q", resume, err, f.ranIDs())
		}
	}
	if got, _ := os.ReadFile(filepath.Join(r.Dir, stateFile)); string(got) != string(data) {
		t.Errorf("state.json changed: %s", got)
	}
	left.Process.Kill()
	left.Wait()
	f.classes["nfs-client"] = true
	if o, err := r.Run(context.Background(), true); o != OutcomeOK || err != nil {
		t.Errorf("after it ended: %s %v", o, err)
	}
}

// ExecStream gives the pid of the command, the leader of its session and so of
// its process group, which state.json records while the step runs.
func TestExecStreamTellsTheGroup(t *testing.T) {
	got := make(chan int, 1)
	ctx, cancel := context.WithCancel(context.Background())
	ctx = withStarted(ctx, func(pid int) { got <- pid })
	done := make(chan error, 1)
	go func() { done <- ExecStream(ctx, t.TempDir(), io.Discard, "sleep", "30") }()
	pid := <-got
	if !groupAlive(pid) {
		t.Errorf("group %d not alive while it runs", pid)
	}
	cancel()
	<-done
	if groupAlive(pid) {
		t.Errorf("group %d alive after it ended", pid)
	}
}
