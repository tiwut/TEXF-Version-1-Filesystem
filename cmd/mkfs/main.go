package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"

	"texf/fs"
)

func main() {
	labelFlag := flag.String("label", "TEXF_VOLUME", "Volume label (up to 16 bytes)")
	inodesFlag := flag.Int("inodes", 0, "Number of inodes to allocate (defaults to total blocks / 4)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <device-or-image-file>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	target := flag.Arg(0)

	err := formatDevice(target, *labelFlag, *inodesFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting %s: %v\n", target, err)
		os.Exit(1)
	}

	fmt.Println("Formatting complete. TEXF v1 filesystem created successfully.")
}

func formatDevice(path string, label string, numInodes int) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("failed to open device/file: %w", err)
	}
	defer file.Close()

	size, err := file.Seek(0, 2)
	if err != nil || size == 0 {

		fi, errStat := file.Stat()
		if errStat != nil {
			return fmt.Errorf("failed to stat device/file: %w", errStat)
		}
		size = fi.Size()
	} else {

		_, err = file.Seek(0, 0)
		if err != nil {
			return fmt.Errorf("failed to seek back to start of device/file: %w", err)
		}
	}

	if size < 65536 {

		return fmt.Errorf("file size %d too small (minimum 64KB)", size)
	}

	totalBlocks := uint64(size / fs.BlockSize)
	if totalBlocks < 16 {
		return fmt.Errorf("device too small: only %d blocks", totalBlocks)
	}

	if numInodes <= 0 {
		numInodes = int(totalBlocks / 4)
	}
	if numInodes < 32 {
		numInodes = 32
	}

	numInodes = (numInodes + 31) &^ 31

	inodeBitmapBlocks := uint64((numInodes + (fs.BlockSize*8 - 1)) / (fs.BlockSize * 8))

	blockBitmapBlocks := uint64((totalBlocks + (fs.BlockSize*8 - 1)) / (fs.BlockSize * 8))

	inodeTableBlocks := uint64(numInodes / 32)

	blockBitmapStart := uint64(1)
	inodeBitmapStart := blockBitmapStart + blockBitmapBlocks
	inodeTableStart := inodeBitmapStart + inodeBitmapBlocks
	dataBlocksStart := inodeTableStart + inodeTableBlocks

	if dataBlocksStart+1 >= totalBlocks {
		return fmt.Errorf("device too small to contain metadata structures")
	}

	sb := &fs.Superblock{
		Magic:            fs.Magic,
		Version:          1,
		BlockSize:        fs.BlockSize,
		BlockCount:       totalBlocks,
		InodeCount:       uint64(numInodes),
		FreeBlocksCount:  totalBlocks - dataBlocksStart - 1,
		FreeInodesCount:  uint64(numInodes - 1),
		BlockBitmapBlock: blockBitmapStart,
		InodeBitmapBlock: inodeBitmapStart,
		InodeTableBlock:  inodeTableStart,
		DataBlocksStart:  dataBlocksStart,
		InodeTableSize:   inodeTableBlocks,
	}
	copy(sb.Label[:], label)

	fmt.Printf("Formatting: %s\n", path)
	fmt.Printf("Volume Label: %s\n", label)
	fmt.Printf("Total Blocks: %d (Block Size: %d)\n", totalBlocks, fs.BlockSize)
	fmt.Printf("Total Inodes: %d\n", numInodes)
	fmt.Printf("Block Bitmap Start Block: %d (Blocks: %d)\n", blockBitmapStart, blockBitmapBlocks)
	fmt.Printf("Inode Bitmap Start Block: %d (Blocks: %d)\n", inodeBitmapStart, inodeBitmapBlocks)
	fmt.Printf("Inode Table Start Block: %d (Blocks: %d)\n", inodeTableStart, inodeTableBlocks)
	fmt.Printf("Data Blocks Start Block: %d (Data Blocks: %d)\n", dataBlocksStart, totalBlocks-dataBlocksStart)

	_, err = file.WriteAt(sb.Serialize(), 0)
	if err != nil {
		return fmt.Errorf("failed to write superblock: %w", err)
	}

	for b := uint64(0); b < blockBitmapBlocks; b++ {
		buf := make([]byte, fs.BlockSize)
		blockOffset := b * fs.BlockSize * 8

		for i := 0; i < fs.BlockSize; i++ {
			var val byte = 0
			for bit := 0; bit < 8; bit++ {
				globalBitIdx := blockOffset + uint64(i)*8 + uint64(bit)
				if globalBitIdx <= dataBlocksStart {
					val |= (1 << bit)
				}
			}
			buf[i] = val
		}

		_, err = file.WriteAt(buf, int64(blockBitmapStart+b)*fs.BlockSize)
		if err != nil {
			return fmt.Errorf("failed to write block bitmap: %w", err)
		}
	}

	for b := uint64(0); b < inodeBitmapBlocks; b++ {
		buf := make([]byte, fs.BlockSize)
		if b == 0 {
			buf[0] = 0x01
		}

		_, err = file.WriteAt(buf, int64(inodeBitmapStart+b)*fs.BlockSize)
		if err != nil {
			return fmt.Errorf("failed to write inode bitmap: %w", err)
		}
	}

	now := time.Now().Unix()
	rootInode := &fs.Inode{
		Mode:   fs.S_IFDIR | 0755,
		UID:    0,
		GID:    0,
		Nlink:  2,
		Size:   fs.BlockSize,
		Atime:  now,
		Mtime:  now,
		Ctime:  now,
		Blocks: 8,
	}
	rootInode.Direct[0] = uint32(dataBlocksStart)

	inodeTableBytes := make([]byte, inodeTableBlocks*fs.BlockSize)
	copy(inodeTableBytes[0:fs.InodeSize], rootInode.Serialize())

	_, err = file.WriteAt(inodeTableBytes, int64(inodeTableStart)*fs.BlockSize)
	if err != nil {
		return fmt.Errorf("failed to write inode table: %w", err)
	}

	rootDataBlock := make([]byte, fs.BlockSize)

	dotDe := &fs.Dirent{
		Inode:    1,
		RecLen:   12,
		NameLen:  1,
		FileType: fs.FileTypeDirectory,
		Name:     ".",
	}
	binary.LittleEndian.PutUint32(rootDataBlock[0:4], dotDe.Inode)
	binary.LittleEndian.PutUint16(rootDataBlock[4:6], dotDe.RecLen)
	rootDataBlock[6] = dotDe.NameLen
	rootDataBlock[7] = dotDe.FileType
	copy(rootDataBlock[8:9], dotDe.Name)

	dotDotDe := &fs.Dirent{
		Inode:    1,
		RecLen:   uint16(fs.BlockSize - 12),
		NameLen:  2,
		FileType: fs.FileTypeDirectory,
		Name:     "..",
	}
	binary.LittleEndian.PutUint32(rootDataBlock[12:16], dotDotDe.Inode)
	binary.LittleEndian.PutUint16(rootDataBlock[16:18], dotDotDe.RecLen)
	rootDataBlock[18] = dotDotDe.NameLen
	rootDataBlock[19] = dotDotDe.FileType
	copy(rootDataBlock[20:22], dotDotDe.Name)

	_, err = file.WriteAt(rootDataBlock, int64(dataBlocksStart)*fs.BlockSize)
	if err != nil {
		return fmt.Errorf("failed to write root directory block: %w", err)
	}

	err = file.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync device file: %w", err)
	}

	return nil
}
