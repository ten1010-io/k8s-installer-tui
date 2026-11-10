//go:build !unix

package plan

import "os/exec"

// killTree kills the command; there are no process groups to reach here.
func killTree(cmd *exec.Cmd) error { return cmd.Process.Kill() }
