//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// adminOnlyDir fails unless dir is owned by root and only root can write to it.
func adminOnlyDir(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s must be owned by root and writable only by root (mode now %v)", dir, fi.Mode().Perm())
	}
	return nil
}
