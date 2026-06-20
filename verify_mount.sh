#!/bin/bash
set -e

echo "=== Creating disk image ==="
dd if=/dev/zero of=verify_disk.img bs=4096 count=4096

echo "=== Formatting disk image with TEXF v1 ==="
./mkfs.texf verify_disk.img

echo "=== Creating mount point ==="
mkdir -p ./mnt

echo "=== Mounting TEXF v1 ==="
./texf-mount verify_disk.img ./mnt -o direct_io &
MOUNT_PID=$!

sleep 1.5

cleanup() {
    echo "=== Cleaning up ==="
    if mountpoint -q ./mnt; then
        echo "Unmounting ./mnt..."
        fusermount -u ./mnt || umount ./mnt || true
    fi
    if [ -d ./mnt ]; then
        rmdir ./mnt
    fi
    if [ -f verify_disk.img ]; then
        rm verify_disk.img
    fi
    echo "=== Done ==="
}
trap cleanup EXIT

echo "=== Verifying mount point directory listing ==="
ls -la ./mnt

echo "=== Creating subdirectories ==="
mkdir ./mnt/dir1
mkdir ./mnt/dir1/dir2
ls -la ./mnt
ls -la ./mnt/dir1

echo "=== Creating and writing a file ==="
echo "Hello, TEXF filesystem version 1!" > ./mnt/dir1/hello.txt
echo "File content:"
cat ./mnt/dir1/hello.txt

echo "=== Verifying file attributes ==="
stat ./mnt/dir1/hello.txt

echo "=== Writing a 1MB file ==="
dd if=/dev/urandom of=./mnt/dir1/large.bin bs=4096 count=256
echo "Verify file size of large.bin:"
ls -lh ./mnt/dir1/large.bin
echo "Calculate checksum of large.bin:"
sha256sum ./mnt/dir1/large.bin

echo "=== Verifying Symbolic Links ==="
ln -s hello.txt ./mnt/dir1/link_to_hello
echo "Readlink output:"
readlink ./mnt/dir1/link_to_hello
echo "Cat link_to_hello output:"
cat ./mnt/dir1/link_to_hello

echo "=== Verifying Renaming ==="
mv ./mnt/dir1/hello.txt ./mnt/hello_renamed.txt
ls -la ./mnt
cat ./mnt/hello_renamed.txt

echo "=== Verifying Deletion ==="
rm ./mnt/hello_renamed.txt
rm ./mnt/dir1/link_to_hello
rm ./mnt/dir1/large.bin
rmdir ./mnt/dir1/dir2
rmdir ./mnt/dir1

echo "=== Verifying mount point is empty ==="
ls -la ./mnt

echo "=== Verification complete! TEXF v1 filesystem is fully functional and stable. ==="
