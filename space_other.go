//go:build !windows && !linux && !darwin

package verthash

import "os"

// freeSpace isn't implemented here, so the free space check is skipped.
func freeSpace(dir string) (uint64, bool) {
	return 0, false
}

func reserveFile(f *os.File, size int64) error {
	return f.Truncate(size)
}
