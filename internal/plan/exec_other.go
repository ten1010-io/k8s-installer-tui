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
