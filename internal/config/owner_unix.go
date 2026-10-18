//go:build unix

package config

import (
	"os"
	"syscall"
)

// chownLike copies the uid/gid of the file at path onto target, so replacing
// the config file does not silently change its owner. Failures are ignored: a
// non-root process cannot chown, and in that case it already owns both files.
func chownLike(target, path string) {
	fi, err := os.Stat(path)
	if err != nil {
		return
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	_ = os.Chown(target, int(st.Uid), int(st.Gid))
}
