.PHONY: all clean

all: mkfs.texf texf-mount texf-gui

mkfs.texf: cmd/mkfs/main.go cmd/mkfs/device_darwin.go cmd/mkfs/device_other.go fs/types.go
	go build -o mkfs.texf ./cmd/mkfs

texf-mount: cmd/mount/main.go fs/types.go fs/disk.go fs/driver.go fs/fuse/fuse.go
	go build -o texf-mount ./cmd/mount

texf-gui: cmd/gui/main.go
	go build -o texf-gui ./cmd/gui

clean:
	rm -f mkfs.texf texf-mount texf-gui
