//go:build darwin

package verthash

import (
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

// reserveFile allocates size bytes for f, contiguous if it can, then sets its
// size. Truncate alone would leave a sparse file, and a write through the
// mapping that finds the disk full is a SIGBUS.
func reserveFile(f *os.File, size int64) error {
	fst := unix.Fstore_t{Flags: unix.F_ALLOCATECONTIG, Posmode: unix.F_PEOFPOSMODE, Length: size}
	if err := unix.FcntlFstore(f.Fd(), unix.F_PREALLOCATE, &fst); err != nil {
		fst.Flags = unix.F_ALLOCATEALL
		if err := unix.FcntlFstore(f.Fd(), unix.F_PREALLOCATE, &fst); err != nil {
			return err
		}
	}
	return f.Truncate(size)
}
