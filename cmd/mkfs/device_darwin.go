//go:build darwin
package main

import (
	"bytes"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"syscall"
	"unsafe"
)

const (
	DKIOCGETBLOCKSIZE  = 0x40046418
	DKIOCGETBLOCKCOUNT = 0x40086419
)

func getDeviceSizePlatform(file *os.File) (int64, bool) {
	// Method 1: Try diskutil (highly reliable on macOS, works even when disk is busy)
	path := file.Name()
	cmd := exec.Command("diskutil", "info", "-plist", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		re := regexp.MustCompile(`<key>TotalSize</key>\s*<integer>(\d+)</integer>`)
		matches := re.FindSubmatch(out.Bytes())
		if len(matches) >= 2 {
			if size, err := strconv.ParseInt(string(matches[1]), 10, 64); err == nil && size > 0 {
				return size, true
			}
		}
	}

	// Method 2: Fallback to DKIOCGETBLOCKSIZE and DKIOCGETBLOCKCOUNT ioctls
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
