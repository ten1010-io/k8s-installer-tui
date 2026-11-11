//go:build unix

package plan

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// What stands between the typed word and reset-cluster.yml. The lock is flock,
// so this is a unix test, as TestRunnerRefusesWhenLocked.
func TestPrepareReset(t *testing.T) {
	o := testOptions()
	o.Root = t.TempDir()
	o.Detect = func() Site { return Site{} }
	dir := StateDir(o)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	failedAt := func(id string, existing bool, started time.Time) *RunState {
		return &RunState{Existing: existing, Started: started, Outcome: OutcomeFailed,
			Steps: []StepState{{ID: "K1", Status: StatusOK}, {ID: id, Status: StatusFailed}}}
	}
	write := func(st *RunState) {
		t.Helper()
		data, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, stateFile), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	nodes := "node1 | CHANGED | rc=0 | (stdout) ki-none\nnode2 | CHANGED | rc=0 | (stdout) ki-none\n"
	asked := 0
	var runErr error
	run := func(ctx context.Context, d, name string, args ...string) ([]byte, error) {
		asked++
		if script := args[len(args)-1]; !strings.Contains(script, "-m raw") || !strings.Contains(script, "sudo -n test ! -e "+ClusterRecord+" && echo ki-none || echo ki-built") ||
			!strings.Contains(script, "all:!broken_node") {
			t.Errorf("probe: %s", script)
		}
		return []byte(nodes), runErr
	}
	started := time.Now().Truncate(time.Second)
	shown := failedAt("K3", false, started)
	write(shown)
	refused := func(name, want string) {
		t.Helper()
		release, err := PrepareReset(context.Background(), o, shown, run)
		if err == nil {
			release()
			t.Fatalf("%s: not refused", name)
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", name, err, want)
		}
		// a refusal holds nothing: the next look can take the lock
		l, lerr := lockFile(filepath.Join(dir, "lock"))
		if lerr != nil {
			t.Fatalf("%s: the lock is still held after the refusal: %v", name, lerr)
		}
		l.Close()
	}

	// another program runs this installation
	held, err := lockFile(filepath.Join(dir, "lock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareReset(context.Background(), o, shown, run); err == nil {
		t.Fatal("reset while another run holds the lock")
	} else {
		var locked *LockedError
		if !errors.As(err, &locked) {
			t.Errorf("held lock: %v", err)
		}
	}
	held.Close()
	if asked != 0 {
		t.Error("the nodes were asked before the lock was had")
	}

	// the record of the run is not the one the screen showed
	write(failedAt("K3", false, started.Add(time.Minute)))
	refused("another run since", "실행 기록이 화면에 보인 것과 다릅니다")
	write(failedAt("A1", false, started))
	refused("failed after K3 by now", "실행 기록이 화면에 보인 것과 다릅니다")
	write(failedAt("K3", true, started))
	refused("an existing site's run", "실행 기록이 화면에 보인 것과 다릅니다")
	os.Remove(filepath.Join(dir, stateFile))
	refused("no record of a run", "실행 기록이 화면에 보인 것과 다릅니다")
	write(shown)

	// this control node is not plainly a new site
	o.Detect = func() Site { return Site{Existing: true, Why: "기존 사이트: 기록 있음"} }
	refused("the record is here", "새 사이트가 아닙니다")
	o.Detect = func() Site { return Site{Unknown: true, Why: "확인하지 못함"} }
	refused("the record could not be read", "새 사이트가 아닙니다")
	o.Detect = nil
	refused("no way to look", "확인할 수 없습니다")
	o.Detect = func() Site { return Site{} }

	// a node holds a cluster built from another control node, or says nothing
	nodes = "node1 | CHANGED | rc=0 | (stdout) ki-none\nnode2 | CHANGED | rc=0 | (stdout) ki-built\n"
	refused("node2 is built", "node2")
	nodes = "node1 | CHANGED | rc=0 | (stdout) ki-none\nnode2 | CHANGED | rc=0 | (stdout) \n"
	refused("node2 said nothing", "확인하지 못한 노드가 있습니다: node2")
	nodes = ""
	refused("no node answered", "노드를 확인하지 못했습니다")
	// ansible ended non-zero (a node where sudo is refused, or out of reach, is
	// not in what the others printed): the clean answers of the rest are not enough
	nodes = "node1 | CHANGED | rc=0 | (stdout) ki-none\n"
	runErr = errors.New("exit status 2")
	refused("a node failed the probe", "노드를 확인하지 못했습니다")
	runErr = nil
	nodes = "node1 | CHANGED | rc=0 | (stdout) ki-none\nnode2 | CHANGED | rc=0 | (stdout) ki-none\n"

	// everything holds: the lock is the caller's until release
	release, err := PrepareReset(context.Background(), o, shown, run)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if _, err := lockFile(filepath.Join(dir, "lock")); err == nil {
		t.Error("the lock is not held through the reset")
	}
	release()
	l, err := lockFile(filepath.Join(dir, "lock"))
	if err != nil {
		t.Fatalf("the lock is not given back: %v", err)
	}
	l.Close()
}
