//go:build !windows

package fileio

import (
	"os"
	"syscall"
)

// ownLikeDir gives path the owner of its directory when root made it: the
// history appended by --fill-auth-config, which the plan runs with sudo (A5.1),
// stays the operator's, who saves and appends to it next.
func ownLikeDir(path, dir string) {
	if os.Geteuid() != 0 {
		return
	}
	st, err := os.Stat(dir)
	if err != nil {
		return
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		_ = os.Chown(path, int(sys.Uid), int(sys.Gid))
	}
}
