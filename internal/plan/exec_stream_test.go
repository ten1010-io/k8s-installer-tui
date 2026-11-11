//go:build unix

package plan

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// A cancel reaches the whole process group: a shell that ignores SIGINT and the
// command it waits for are killed after the grace.
func TestExecStreamStopsTheGroup(t *testing.T) {
	defer func(g time.Duration) { killGrace = g }(killGrace)
	killGrace = 300 * time.Millisecond
	var out bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := ExecStream(ctx, t.TempDir(), &out, "bash", "-c", "trap '' INT; echo started; sleep 30 & wait")
	if err == nil || time.Since(start) > 5*time.Second {
		t.Errorf("err %v after %s", err, time.Since(start))
	}
	if !strings.Contains(out.String(), "started") {
		t.Errorf("output: %q", out.String())
	}

	// SIGINT alone ends what does not ignore it, at once
	ctx, cancel = context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start = time.Now()
	killGrace = 20 * time.Second
	_ = ExecStream(ctx, t.TempDir(), &out, "bash", "-c", "sleep 30")
	if time.Since(start) > 5*time.Second {
		t.Errorf("SIGINT did not end it: %s", time.Since(start))
	}
}

// The command leads a session of its own: no controlling terminal, so a sudo
// that wants a password fails instead of stopping on SIGTTIN.
func TestExecStreamRunsInASessionOfItsOwn(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc")
	}
	var out bytes.Buffer
	err := ExecStream(context.Background(), t.TempDir(), &out, "bash", "-c", `read -r _ _ _ _ _ sid _ < /proc/$$/stat; echo "sid=$sid pid=$$"`)
	var sid, pid int
	if _, serr := fmt.Sscanf(strings.TrimSpace(out.String()), "sid=%d pid=%d", &sid, &pid); err != nil || serr != nil || sid != pid {
		t.Errorf("err %v, %q", err, out.String())
	}
}
