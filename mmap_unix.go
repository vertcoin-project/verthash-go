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

// mapFileReadOnly maps the first size bytes of f read-only.
func mapFileReadOnly(f *os.File, size int64) ([]byte, error) {
	data, err := unix.Mmap(int(f.Fd()), 0, int(size), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, os.NewSyscallError("mmap", err)
	}
	return data, nil
}

// flushMap writes the mapped pages to disk.
func flushMap(f *os.File, data []byte) error {
	return os.NewSyscallError("msync", unix.Msync(data, unix.MS_SYNC))
}

func unmapFile(data []byte) error {
	return os.NewSyscallError("munmap", unix.Munmap(data))
}
