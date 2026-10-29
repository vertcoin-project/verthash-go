//go:build unix

package verthash

import (
	"os"

	"golang.org/x/sys/unix"
)

// mapFile maps the first size bytes of f read-write.
func mapFile(f *os.File, size int64) ([]byte, error) {
	data, err := unix.Mmap(int(f.Fd()), 0, int(size), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return nil, os.NewSyscallError("mmap", err)
	}
	return data, nil
}

// mapFileReadOnly maps the first size bytes of f read-only, for hashing.
func mapFileReadOnly(f *os.File, size int64) ([]byte, error) {
	data, err := unix.Mmap(int(f.Fd()), 0, int(size), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, os.NewSyscallError("mmap", err)
	}
	// Each hash reads 32 bytes at 4096 random offsets. Without this, every page fault also
	// reads in the pages around it, which evicts the pages hashes need when the file doesn't
	// fit in RAM. It's only a hint, so a failure doesn't matter.
	_ = unix.Madvise(data, unix.MADV_RANDOM)
	return data, nil
}

// flushMap writes the mapped pages to disk.
func flushMap(f *os.File, data []byte) error {
	return os.NewSyscallError("msync", unix.Msync(data, unix.MS_SYNC))
}

func unmapFile(data []byte) error {
	return os.NewSyscallError("munmap", unix.Munmap(data))
}
