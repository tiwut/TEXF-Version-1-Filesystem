.PHONY: all clean

CGO_CFLAGS := -I/usr/local/include -I/usr/local/include/macfuse -I/usr/local/include/osxfuse -I/opt/homebrew/include -I/opt/homebrew/include/macfuse
CGO_LDFLAGS := -L/usr/local/lib -L/opt/homebrew/lib
export CGO_CFLAGS
export CGO_LDFLAGS

FUSE_EXISTS := $(shell (command -v pkg-config >/dev/null 2>&1 && (pkg-config --exists fuse3 2>/dev/null || pkg-config --exists fuse 2>/dev/null)) || ls /usr/include/fuse.h /usr/local/include/fuse.h /usr/local/include/macfuse/fuse.h /usr/local/include/osxfuse/fuse.h /opt/homebrew/include/fuse.h /opt/homebrew/include/macfuse/fuse.h >/dev/null 2>&1 && echo "yes" || echo "no")

TARGETS := mkfs.texf texf-gui

ifeq ($(FUSE_EXISTS),yes)
TARGETS += texf-mount
endif

all: $(TARGETS)
	@if [ "$(FUSE_EXISTS)" = "no" ]; then \
		echo "========================================================================="; \
		echo " WARNING: FUSE headers not found (fuse.h). Skipping build of texf-mount."; \
		echo " Install libfuse (Linux) or macFUSE/FUSE-T (macOS) to build mount support."; \
		echo "========================================================================="; \
	fi

mkfs.texf: cmd/mkfs/main.go cmd/mkfs/device_darwin.go cmd/mkfs/device_other.go fs/types.go
	go build -o mkfs.texf ./cmd/mkfs

texf-mount: cmd/mount/main.go fs/types.go fs/disk.go fs/driver.go fs/fuse/fuse.go
	go build -o texf-mount ./cmd/mount

texf-gui: cmd/gui/main.go
	go build -o texf-gui ./cmd/gui

clean:
	rm -f mkfs.texf texf-mount texf-gui
