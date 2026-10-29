//go:build windows

package verthash

import (
	"os"

	"golang.org/x/sys/windows"
)

// freeSpace returns the bytes available to this user on dir's volume.
func freeSpace(dir string) (uint64, bool) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, false
	}
	var avail uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, nil, nil); err != nil {
		return 0, false
	}
	return avail, true
}

// reserveFile sets f's size to size. On NTFS this allocates the clusters, so a
// full disk fails here instead of while writing through the mapping.
func reserveFile(f *os.File, size int64) error {
	return f.Truncate(size)
}
