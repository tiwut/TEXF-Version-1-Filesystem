# TEXF (Version 1) Filesystem

TEXF (Version 1) is a custom Unix-like filesystem designed and built entirely from scratch. Using Go and the cross-platform FUSE library `cgofuse`, it can be compiled and mounted natively on Linux, macOS (using macFUSE or FUSE-T), and Windows (using WinFSP).

---

## Features

- **Built from Scratch**: Custom disk layout and logical block manager.
- **Cross-Platform**: Support for Linux, macOS, and Windows.
- **Robust Layout**: Superblock, allocation bitmaps, inode table, and data blocks.
- **Complete Path Support**: Supports all standard path characters, using standard Unix `/` delimiters.
- **Symlinks & Directory Nesting**: Fully supports nested directories, symbolic links, hard links count, and file modes.
- **Large File Support**: Single, double, and triple indirect block mapping to support massive files.

---

## On-Disk Structure Layout

The filesystem uses 4096-byte blocks organized in sequential order:

1. **Block 0 (Superblock)**: Global volume attributes and configuration mapping.
2. **Blocks 1 to B_bb (Block Bitmap)**: Allocation status tracking for data blocks.
3. **Blocks B_bb+1 to B_ib (Inode Bitmap)**: Allocation status tracking for inodes.
4. **Blocks B_ib+1 to B_it (Inode Table)**: Contains 128-byte `Inode` records (32 inodes per 4KB block).
5. **Blocks B_it+1 to N-1 (Data Blocks)**: Direct/indirect indices, file contents, and directory entries.

---

## Compilation

Ensure you have Go installed on your system. Run `make` to compile the filesystem formatter and mount utilities:

```bash
make
```

This builds two executables in your workspace:
- `mkfs.texf`: Filesystem format utility.
- `texf-mount`: FUSE mounting daemon.

---

## Formatting

To format a disk image or raw partition with the TEXF filesystem:

```bash
# To format a virtual disk image:
dd if=/dev/zero of=verify_disk.img bs=4096 count=4096
./mkfs.texf verify_disk.img

# To format a physical USB stick (e.g., /dev/sda):
sudo ./mkfs.texf /dev/sda
```

---

## Mounting

To mount a formatted partition or disk image:

```bash
# Create mountpoint
mkdir -p ./mnt

# Mount in the foreground with user permissions enabled:
sudo ./texf-mount /dev/sda ./mnt -f -o allow_other
```

### Mount Options:
- `-f`: Runs FUSE in the foreground (prevents runtime environments from reaping the daemon process).
- `-o allow_other`: Grants access permissions to regular users (such as your logged-in user profile) when mounted via `sudo`.

---

## Verification

To run the automated verification script that creates an image, formats it, mounts it, reads/writes file data, verifies directory creation, check symlinks, and performs cleanup:

```bash
./verify_mount.sh
```
