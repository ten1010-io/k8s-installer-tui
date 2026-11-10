//go:build unix

package plan

import (
	"os/exec"
	"syscall"
)

// killTree kills the process group the command leads, when it leads one (setsid
// makes ssh-copy-id the leader of a session of its own, with the ssh it runs),
// else the command alone.
func killTree(cmd *exec.Cmd) error {
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}
