//go:build !unix

package plan

import (
	"os"
	"os/exec"
)

// killTree kills the command; there are no process groups to reach here.
func killTree(cmd *exec.Cmd) error { return cmd.Process.Kill() }

// fileOwner: no owner to check here.
func fileOwner(fi os.FileInfo) (int, bool) { return 0, false }

// TerminateJob: no process groups here.
func TerminateJob() {}

// setSession: no sessions here.
func setSession(cmd *exec.Cmd) {}

// groupAlive: no process groups to ask about here.
func groupAlive(pgid int) bool { return false }

// interruptTree: no signals here; the command is killed.
func interruptTree(cmd *exec.Cmd) error { return cmd.Process.Kill() }

// lockFile: no flock here; the file is opened, nothing is held.
func lockFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
}
