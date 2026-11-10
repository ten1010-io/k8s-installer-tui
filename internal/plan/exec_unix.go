//go:build unix

package plan

import (
	"os"
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

// fileOwner is the uid that owns the file of fi.
func fileOwner(fi os.FileInfo) (int, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}

// TerminateJob ends the commands that share this process's terminal job (the
// ssh-copy-id of --send-ssh-key without --ask-pass-once) when this process
// leads that job, as it does run from a shell; this process ignores the signal
// (it listens for it).
func TerminateJob() {
	if syscall.Getpgrp() == os.Getpid() {
		_ = syscall.Kill(-os.Getpid(), syscall.SIGTERM)
	}
}
