package fs

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

type Disk struct {
	file *os.File
	sb   *Superblock
	mu   sync.Mutex
}

func OpenDisk(path string) (*Disk, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open disk file: %w", err)
	}

	disk := &Disk{file: file}

	sbData, err := disk.ReadBlock(0)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to read superblock: %w", err)
	}

	sb, err := DeserializeSuperblock(sbData)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to deserialize superblock: %w", err)
	}

	disk.sb = sb
	return disk, nil
}

func (d *Disk) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.file != nil {
		err := d.file.Sync()
		errClose := d.file.Close()
		d.file = nil
		if err != nil {
			return err
		}
		return errClose
	}
	return nil
}

func (d *Disk) GetSuperblock() *Superblock {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sb
}

func (d *Disk) Sync() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.file == nil {
		return errors.New("disk closed")
	}
	return d.file.Sync()
}

func (d *Disk) ReadBlock(blockNum uint64) ([]byte, error) {
	buf := make([]byte, BlockSize)
	offset := int64(blockNum) * BlockSize

	_, err := d.file.ReadAt(buf, offset)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func (d *Disk) WriteBlock(blockNum uint64, data []byte) error {
	if len(data) != BlockSize {
		return fmt.Errorf("invalid data size for WriteBlock: %d", len(data))
	}
	offset := int64(blockNum) * BlockSize

	_, err := d.file.WriteAt(data, offset)
	return err
}

func (d *Disk) ReadInode(ino uint32) (*Inode, error) {
	if ino == 0 {
		return nil, errors.New("invalid inode number 0")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if ino > uint32(d.sb.InodeCount) {
		return nil, fmt.Errorf("inode number %d out of range (max %d)", ino, d.sb.InodeCount)
	}

	idx := ino - 1
	blockNum := d.sb.InodeTableBlock + uint64(idx/32)
	offset := (idx % 32) * InodeSize

	blockData, err := d.ReadBlock(blockNum)
	if err != nil {
		return nil, fmt.Errorf("failed to read inode table block %d: %w", blockNum, err)
	}

	inode, err := DeserializeInode(blockData[offset : offset+InodeSize])
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize inode %d: %w", ino, err)
	}

	return inode, nil
}

func (d *Disk) WriteInode(ino uint32, inode *Inode) error {
	if ino == 0 {
		return errors.New("invalid inode number 0")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if ino > uint32(d.sb.InodeCount) {
		return fmt.Errorf("inode number %d out of range (max %d)", ino, d.sb.InodeCount)
	}

	idx := ino - 1
	blockNum := d.sb.InodeTableBlock + uint64(idx/32)
	offset := (idx % 32) * InodeSize

	blockData, err := d.ReadBlock(blockNum)
	if err != nil {
		return fmt.Errorf("failed to read inode table block %d: %w", blockNum, err)
	}

	inodeBytes := inode.Serialize()
	copy(blockData[offset:offset+InodeSize], inodeBytes)

	err = d.WriteBlock(blockNum, blockData)
	if err != nil {
		return fmt.Errorf("failed to write inode table block %d: %w", blockNum, err)
	}

	return nil
}

func (d *Disk) writeSuperblockLocked() error {
	sbBytes := d.sb.Serialize()
	err := d.WriteBlock(0, sbBytes)
	if err != nil {
		return fmt.Errorf("failed to write superblock: %w", err)
	}
	return nil
}

func (d *Disk) AllocBlock() (uint64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.sb.FreeBlocksCount == 0 {
		return 0, errors.New("no free blocks available")
	}

	bitmapBlocks := d.sb.InodeTableBlock - d.sb.BlockBitmapBlock

	for b := uint64(0); b < bitmapBlocks; b++ {
		blockNum := d.sb.BlockBitmapBlock + b
		bitmapData, err := d.ReadBlock(blockNum)
		if err != nil {
			return 0, fmt.Errorf("failed to read block bitmap: %w", err)
		}

		for i := 0; i < BlockSize; i++ {
			if bitmapData[i] != 0xFF {

				for bit := 0; bit < 8; bit++ {
					if (bitmapData[i] & (1 << bit)) == 0 {

						globalBitIdx := b*BlockSize*8 + uint64(i)*8 + uint64(bit)

						if globalBitIdx >= d.sb.BlockCount {
							return 0, errors.New("block bitmap index exceeds block count")
						}

						bitmapData[i] |= (1 << bit)
						err = d.WriteBlock(blockNum, bitmapData)
						if err != nil {
							return 0, fmt.Errorf("failed to write block bitmap: %w", err)
						}

						d.sb.FreeBlocksCount--
						err = d.writeSuperblockLocked()
						if err != nil {
							return 0, err
						}

						return globalBitIdx, nil
					}
				}
			}
		}
	}

	return 0, errors.New("block bitmap indicates free blocks, but none found")
}

func (d *Disk) FreeBlock(blockNum uint64) error {
	if blockNum >= d.sb.BlockCount {
		return fmt.Errorf("block number %d out of range", blockNum)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	bitIdx := blockNum
	bitmapBlockIdx := d.sb.BlockBitmapBlock + (bitIdx / (BlockSize * 8))
	byteIdx := (bitIdx / 8) % BlockSize
	bitPos := bitIdx % 8

	bitmapData, err := d.ReadBlock(bitmapBlockIdx)
	if err != nil {
		return fmt.Errorf("failed to read block bitmap: %w", err)
	}

	if (bitmapData[byteIdx] & (1 << bitPos)) == 0 {

		return nil
	}

	bitmapData[byteIdx] &= ^(1 << bitPos)

	err = d.WriteBlock(bitmapBlockIdx, bitmapData)
	if err != nil {
		return fmt.Errorf("failed to write block bitmap: %w", err)
	}

	d.sb.FreeBlocksCount++
	err = d.writeSuperblockLocked()
	if err != nil {
		return err
	}

	return nil
}

func (d *Disk) AllocInode() (uint32, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.sb.FreeInodesCount == 0 {
		return 0, errors.New("no free inodes available")
	}

	bitmapBlocks := d.sb.BlockBitmapBlock - d.sb.InodeBitmapBlock

	for b := uint64(0); b < bitmapBlocks; b++ {
		blockNum := d.sb.InodeBitmapBlock + b
		bitmapData, err := d.ReadBlock(blockNum)
		if err != nil {
			return 0, fmt.Errorf("failed to read inode bitmap: %w", err)
		}

		for i := 0; i < BlockSize; i++ {
			if bitmapData[i] != 0xFF {
				for bit := 0; bit < 8; bit++ {
					if (bitmapData[i] & (1 << bit)) == 0 {
						globalBitIdx := b*BlockSize*8 + uint64(i)*8 + uint64(bit)
						ino := uint32(globalBitIdx + 1)

						if ino > uint32(d.sb.InodeCount) {
							return 0, errors.New("inode bitmap index exceeds inode count")
						}

						bitmapData[i] |= (1 << bit)
						err = d.WriteBlock(blockNum, bitmapData)
						if err != nil {
							return 0, fmt.Errorf("failed to write inode bitmap: %w", err)
						}

						d.sb.FreeInodesCount--
						err = d.writeSuperblockLocked()
						if err != nil {
							return 0, err
						}

						return ino, nil
					}
				}
			}
		}
	}

	return 0, errors.New("inode bitmap indicates free inodes, but none found")
}

func (d *Disk) FreeInode(ino uint32) error {
	if ino == 0 || ino > uint32(d.sb.InodeCount) {
		return fmt.Errorf("invalid inode number %d to free", ino)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	bitIdx := uint64(ino - 1)
	bitmapBlockIdx := d.sb.InodeBitmapBlock + (bitIdx / (BlockSize * 8))
	byteIdx := (bitIdx / 8) % BlockSize
	bitPos := bitIdx % 8

	bitmapData, err := d.ReadBlock(bitmapBlockIdx)
	if err != nil {
		return fmt.Errorf("failed to read inode bitmap: %w", err)
	}

	if (bitmapData[byteIdx] & (1 << bitPos)) == 0 {

		return nil
	}

	bitmapData[byteIdx] &= ^(1 << bitPos)

	err = d.WriteBlock(bitmapBlockIdx, bitmapData)
	if err != nil {
		return fmt.Errorf("failed to write inode bitmap: %w", err)
	}

	d.sb.FreeInodesCount++
	err = d.writeSuperblockLocked()
	if err != nil {
		return err
	}

	return nil
}
