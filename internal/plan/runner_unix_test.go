//go:build unix

package plan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
