.PHONY: all clean

ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))
CGO_CFLAGS := -I$(ROOT_DIR)include -DFUSE_USE_VERSION=28 -D_FILE_OFFSET_BITS=64 -I/usr/local/include -I/usr/local/include/macfuse -I/usr/local/include/osxfuse -I/opt/homebrew/include -I/opt/homebrew/include/macfuse
CGO_LDFLAGS := -L/usr/local/lib -L/opt/homebrew/lib
export CGO_CFLAGS
export CGO_LDFLAGS

TARGETS := mkfs.texf texf-mount texf-gui

all: $(TARGETS)

mkfs.texf: cmd/mkfs/main.go cmd/mkfs/device_darwin.go cmd/mkfs/device_other.go fs/types.go
	go build -o mkfs.texf ./cmd/mkfs

texf-mount: cmd/mount/main.go fs/types.go fs/disk.go fs/driver.go fs/fuse/fuse.go
	go build -o texf-mount ./cmd/mount

texf-gui: cmd/gui/main.go
	go build -o texf-gui ./cmd/gui

clean:
	rm -f mkfs.texf texf-mount texf-gui

