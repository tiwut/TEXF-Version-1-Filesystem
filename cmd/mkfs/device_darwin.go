//go:build darwin
package main

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	DKIOCGETBLOCKSIZE  = 0x40046418
	DKIOCGETBLOCKCOUNT = 0x40086419
)

func getDeviceSizePlatform(file *os.File) (int64, bool) {
	fd := file.Fd()

	var blockSize uint32
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, DKIOCGETBLOCKSIZE, uintptr(unsafe.Pointer(&blockSize)))
	if err != 0 {
		return 0, false
	}

	var blockCount uint64
	_, _, err = syscall.Syscall(syscall.SYS_IOCTL, fd, DKIOCGETBLOCKCOUNT, uintptr(unsafe.Pointer(&blockCount)))
	if err != 0 {
		return 0, false
	}

	return int64(blockSize) * int64(blockCount), true
}
