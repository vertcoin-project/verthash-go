//go:build linux

package verthash

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// freeSpace returns the bytes available to this user on dir's file system.
func freeSpace(dir string) (uint64, bool) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, false
	}
	return st.Bavail * uint64(st.Bsize), true
}

// reserveFile allocates size bytes for f. Truncate alone would leave a sparse
// file, and a write through the mapping that finds the disk full is a SIGBUS.
func reserveFile(f *os.File, size int64) error {
	err := unix.Fallocate(int(f.Fd()), 0, 0, size)
	if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOSYS) {
		// the file system can't preallocate; freeSpace was checked before
		return f.Truncate(size)
	}
	return err
}
