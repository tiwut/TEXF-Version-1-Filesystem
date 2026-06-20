.PHONY: all clean

all: mkfs.texf texf-mount

mkfs.texf: cmd/mkfs/main.go fs/types.go
	go build -o mkfs.texf cmd/mkfs/main.go

texf-mount: cmd/mount/main.go fs/types.go fs/disk.go fs/driver.go fs/fuse/fuse.go
	go build -o texf-mount cmd/mount/main.go

clean:
	rm -f mkfs.texf texf-mount
