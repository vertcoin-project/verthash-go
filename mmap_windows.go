//go:build windows

package verthash

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// mapFile maps the first size bytes of f read-write.
func mapFile(f *os.File, size int64) ([]byte, error) {
	h, err := windows.CreateFileMapping(windows.Handle(f.Fd()), nil, windows.PAGE_READWRITE,
		uint32(size>>32), uint32(size), nil)
	if err != nil {
		return nil, os.NewSyscallError("CreateFileMapping", err)
	}
	// the view keeps the mapping object alive
	defer windows.CloseHandle(h)

	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_WRITE, 0, 0, uintptr(size))
	if err != nil {
		return nil, os.NewSyscallError("MapViewOfFile", err)
	}
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&addr)), size), nil
}

// mapFileReadOnly maps the first size bytes of f read-only.
func mapFileReadOnly(f *os.File, size int64) ([]byte, error) {
	h, err := windows.CreateFileMapping(windows.Handle(f.Fd()), nil, windows.PAGE_READONLY,
		uint32(size>>32), uint32(size), nil)
	if err != nil {
		return nil, os.NewSyscallError("CreateFileMapping", err)
	}
	defer windows.CloseHandle(h)

	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_READ, 0, 0, uintptr(size))
	if err != nil {
		return nil, os.NewSyscallError("MapViewOfFile", err)
	}
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&addr)), size), nil
}

// flushMap writes the mapped pages and the file's metadata to disk.
func flushMap(f *os.File, data []byte) error {
	err := windows.FlushViewOfFile(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	if err != nil {
		return os.NewSyscallError("FlushViewOfFile", err)
	}
	return f.Sync()
}

func unmapFile(data []byte) error {
	return os.NewSyscallError("UnmapViewOfFile", windows.UnmapViewOfFile(uintptr(unsafe.Pointer(&data[0]))))
}
