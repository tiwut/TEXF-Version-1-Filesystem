package main

import (
	"flag"
	"fmt"
	"os"
	"texf/fs"

	"github.com/winfsp/cgofuse/fuse"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <device-or-image-file> <mountpoint> [fuse options...]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(1)
	}

	devicePath := flag.Arg(0)
	mountpoint := flag.Arg(1)

	fuseArgs := flag.Args()[2:]

	disk, err := fs.OpenDisk(devicePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open device/image %s: %v\n", devicePath, err)
		os.Exit(1)
	}
	defer disk.Close()

	driver := fs.NewDriver(disk)
	texfuse := fs.NewTEXFuse(driver)

	host := fuse.NewFileSystemHost(texfuse)
	fmt.Printf("Mounting %s at %s...\n", devicePath, mountpoint)

	success := host.Mount(mountpoint, fuseArgs)
	if !success {
		fmt.Fprintf(os.Stderr, "Failed to mount filesystem or mount terminated with error.\n")
		os.Exit(1)
	}
}
