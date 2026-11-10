//go:build unix

package plan

import (
	"context"
	"testing"
	"time"
)

// Cancelling a command that runs in a session of its own (setsid, as the send
// does) kills what it started too, which holds the output pipe: ExecRun comes
// back at once, not when the grandchild ends.
func TestExecRunCancelKillsTheSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := ExecRun(ctx, "", "setsid", "-w", "sh", "-c", "sleep 30 | cat")
	if err == nil {
		t.Error("no error after the cancel")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v", d)
	}
}
