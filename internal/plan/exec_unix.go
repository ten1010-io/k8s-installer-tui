//go:build unix

package plan

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

// setSession makes the command the leader of a session of its own, so of a
// process group whose id is its pid, which interruptTree and killTree reach
// whole, and which Ctrl-C on the terminal does not (this program decides what
// it stops). The session has no controlling terminal: a sudo or ssh that
// would ask for a password fails instead of stopping on SIGTTIN for ever.
func setSession(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// groupAlive reports whether a process of group pgid is still there (one of
// another user counts: kill says EPERM).
func groupAlive(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// interruptTree sends SIGINT to the process group the command leads: ansible
// stops as on Ctrl-C.
func interruptTree(cmd *exec.Cmd) error {
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err == nil {
		return nil
	}
	return cmd.Process.Signal(syscall.SIGINT)
}

// lockFile takes the lock of a run (flock, released when the file is closed or
// the program ends) and writes this pid into it; a LockedError with the pid of
// the holder when another has it.
func lockFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		pid, _ := io.ReadAll(f)
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, &LockedError{Path: path, PID: strings.TrimSpace(string(pid))}
		}
		return nil, err
	}
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return f, nil
}
