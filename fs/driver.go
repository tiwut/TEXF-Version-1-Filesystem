package fs

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound      = errors.New("file not found")
	ErrAlreadyExists = errors.New("file already exists")
	ErrNotDirectory  = errors.New("not a directory")
	ErrIsDir         = errors.New("is a directory")
	ErrDirNotEmpty   = errors.New("directory not empty")
)

type Driver struct {
	Disk *Disk
}

func NewDriver(disk *Disk) *Driver {
	return &Driver{Disk: disk}
}

func (dr *Driver) TranslateBlock(inode *Inode, ino uint32, fileBlockNum uint32, alloc bool) (uint64, error) {
	const N = 1024

	if fileBlockNum < 12 {
		phys := uint64(inode.Direct[fileBlockNum])
		if phys == 0 && alloc {
			newBlock, err := dr.Disk.AllocBlock()
			if err != nil {
				return 0, err
			}
			inode.Direct[fileBlockNum] = uint32(newBlock)
			inode.Blocks += 8
			err = dr.Disk.WriteInode(ino, inode)
			if err != nil {
				dr.Disk.FreeBlock(newBlock)
				return 0, err
			}
			return newBlock, nil
		}
		return phys, nil
	}

	readPointerBlock := func(bNum uint64) ([]uint32, error) {
		data, err := dr.Disk.ReadBlock(bNum)
		if err != nil {
			return nil, err
		}
		ptrs := make([]uint32, N)
		for i := 0; i < N; i++ {
			ptrs[i] = binary.LittleEndian.Uint32(data[i*4 : i*4+4])
		}
		return ptrs, nil
	}

	writePointerBlock := func(bNum uint64, ptrs []uint32) error {
		data := make([]byte, BlockSize)
		for i := 0; i < N; i++ {
			binary.LittleEndian.PutUint32(data[i*4:i*4+4], ptrs[i])
		}
		return dr.Disk.WriteBlock(bNum, data)
	}

	if fileBlockNum < 12+N {
		idx := fileBlockNum - 12
		var ind1Block uint64
		if inode.Indirect1 == 0 {
			if !alloc {
				return 0, nil
			}
			newBlock, err := dr.Disk.AllocBlock()
			if err != nil {
				return 0, err
			}
			inode.Indirect1 = uint32(newBlock)
			inode.Blocks += 8
			err = dr.Disk.WriteInode(ino, inode)
			if err != nil {
				dr.Disk.FreeBlock(newBlock)
				return 0, err
			}
			ind1Block = newBlock
		} else {
			ind1Block = uint64(inode.Indirect1)
		}

		ptrs, err := readPointerBlock(ind1Block)
		if err != nil {
			return 0, err
		}

		phys := uint64(ptrs[idx])
		if phys == 0 && alloc {
			newBlock, err := dr.Disk.AllocBlock()
			if err != nil {
				return 0, err
			}
			ptrs[idx] = uint32(newBlock)
			err = writePointerBlock(ind1Block, ptrs)
			if err != nil {
				dr.Disk.FreeBlock(newBlock)
				return 0, err
			}
			inode.Blocks += 8
			err = dr.Disk.WriteInode(ino, inode)
			if err != nil {
				return 0, err
			}
			return newBlock, nil
		}
		return phys, nil
	}

	if fileBlockNum < 12+N+N*N {
		off := fileBlockNum - 12 - N
		idx1 := off / N
		idx2 := off % N

		var ind2Block uint64
		if inode.Indirect2 == 0 {
			if !alloc {
				return 0, nil
			}
			newBlock, err := dr.Disk.AllocBlock()
			if err != nil {
				return 0, err
			}
			inode.Indirect2 = uint32(newBlock)
			inode.Blocks += 8
			err = dr.Disk.WriteInode(ino, inode)
			if err != nil {
				dr.Disk.FreeBlock(newBlock)
				return 0, err
			}
			ind2Block = newBlock
		} else {
			ind2Block = uint64(inode.Indirect2)
		}

		ptrs1, err := readPointerBlock(ind2Block)
		if err != nil {
			return 0, err
		}

		var ind1Block uint64
		if ptrs1[idx1] == 0 {
			if !alloc {
				return 0, nil
			}
			newBlock, err := dr.Disk.AllocBlock()
			if err != nil {
				return 0, err
			}
			ptrs1[idx1] = uint32(newBlock)
			err = writePointerBlock(ind2Block, ptrs1)
			if err != nil {
				dr.Disk.FreeBlock(newBlock)
				return 0, err
			}
			inode.Blocks += 8
			err = dr.Disk.WriteInode(ino, inode)
			if err != nil {
				return 0, err
			}
			ind1Block = newBlock
		} else {
			ind1Block = uint64(ptrs1[idx1])
		}

		ptrs2, err := readPointerBlock(ind1Block)
		if err != nil {
			return 0, err
		}

		phys := uint64(ptrs2[idx2])
		if phys == 0 && alloc {
			newBlock, err := dr.Disk.AllocBlock()
			if err != nil {
				return 0, err
			}
			ptrs2[idx2] = uint32(newBlock)
			err = writePointerBlock(ind1Block, ptrs2)
			if err != nil {
				dr.Disk.FreeBlock(newBlock)
				return 0, err
			}
			inode.Blocks += 8
			err = dr.Disk.WriteInode(ino, inode)
			if err != nil {
				return 0, err
			}
			return newBlock, nil
		}
		return phys, nil
	}

	if fileBlockNum < 12+N+N*N+N*N*N {
		off := fileBlockNum - 12 - N - N*N
		idx1 := off / (N * N)
		idx2 := (off / N) % N
		idx3 := off % N

		var ind3Block uint64
		if inode.Indirect3 == 0 {
			if !alloc {
				return 0, nil
			}
			newBlock, err := dr.Disk.AllocBlock()
			if err != nil {
				return 0, err
			}
			inode.Indirect3 = uint32(newBlock)
			inode.Blocks += 8
			err = dr.Disk.WriteInode(ino, inode)
			if err != nil {
				dr.Disk.FreeBlock(newBlock)
				return 0, err
			}
			ind3Block = newBlock
		} else {
			ind3Block = uint64(inode.Indirect3)
		}

		ptrs1, err := readPointerBlock(ind3Block)
		if err != nil {
			return 0, err
		}

		var ind2Block uint64
		if ptrs1[idx1] == 0 {
			if !alloc {
				return 0, nil
			}
			newBlock, err := dr.Disk.AllocBlock()
			if err != nil {
				return 0, err
			}
			ptrs1[idx1] = uint32(newBlock)
			err = writePointerBlock(ind3Block, ptrs1)
			if err != nil {
				dr.Disk.FreeBlock(newBlock)
				return 0, err
			}
			inode.Blocks += 8
			err = dr.Disk.WriteInode(ino, inode)
			if err != nil {
				return 0, err
			}
			ind2Block = newBlock
		} else {
			ind2Block = uint64(ptrs1[idx1])
		}

		ptrs2, err := readPointerBlock(ind2Block)
		if err != nil {
			return 0, err
		}

		var ind1Block uint64
		if ptrs2[idx2] == 0 {
			if !alloc {
				return 0, nil
			}
			newBlock, err := dr.Disk.AllocBlock()
			if err != nil {
				return 0, err
			}
			ptrs2[idx2] = uint32(newBlock)
			err = writePointerBlock(ind2Block, ptrs2)
			if err != nil {
				dr.Disk.FreeBlock(newBlock)
				return 0, err
			}
			inode.Blocks += 8
			err = dr.Disk.WriteInode(ino, inode)
			if err != nil {
				return 0, err
			}
			ind1Block = newBlock
		} else {
			ind1Block = uint64(ptrs2[idx2])
		}

		ptrs3, err := readPointerBlock(ind1Block)
		if err != nil {
			return 0, err
		}

		phys := uint64(ptrs3[idx3])
		if phys == 0 && alloc {
			newBlock, err := dr.Disk.AllocBlock()
			if err != nil {
				return 0, err
			}
			ptrs3[idx3] = uint32(newBlock)
			err = writePointerBlock(ind1Block, ptrs3)
			if err != nil {
				dr.Disk.FreeBlock(newBlock)
				return 0, err
			}
			inode.Blocks += 8
			err = dr.Disk.WriteInode(ino, inode)
			if err != nil {
				return 0, err
			}
			return newBlock, nil
		}
		return phys, nil
	}

	return 0, errors.New("file block index too large")
}

func (dr *Driver) ClearBlockPointer(inode *Inode, ino uint32, fileBlockNum uint32) error {
	const N = 1024

	readPointerBlock := func(bNum uint64) ([]uint32, error) {
		data, err := dr.Disk.ReadBlock(bNum)
		if err != nil {
			return nil, err
		}
		ptrs := make([]uint32, N)
		for i := 0; i < N; i++ {
			ptrs[i] = binary.LittleEndian.Uint32(data[i*4 : i*4+4])
		}
		return ptrs, nil
	}

	writePointerBlock := func(bNum uint64, ptrs []uint32) error {
		data := make([]byte, BlockSize)
		for i := 0; i < N; i++ {
			binary.LittleEndian.PutUint32(data[i*4:i*4+4], ptrs[i])
		}
		return dr.Disk.WriteBlock(bNum, data)
	}

	if fileBlockNum < 12 {
		phys := uint64(inode.Direct[fileBlockNum])
		if phys != 0 {
			err := dr.Disk.FreeBlock(phys)
			if err != nil {
				return err
			}
			inode.Direct[fileBlockNum] = 0
			inode.Blocks -= 8
			return dr.Disk.WriteInode(ino, inode)
		}
		return nil
	}

	if fileBlockNum < 12+N {
		if inode.Indirect1 == 0 {
			return nil
		}
		idx := fileBlockNum - 12
		ptrs, err := readPointerBlock(uint64(inode.Indirect1))
		if err != nil {
			return err
		}
		phys := uint64(ptrs[idx])
		if phys != 0 {
			err = dr.Disk.FreeBlock(phys)
			if err != nil {
				return err
			}
			ptrs[idx] = 0
			err = writePointerBlock(uint64(inode.Indirect1), ptrs)
			if err != nil {
				return err
			}
			inode.Blocks -= 8

			allZero := true
			for _, p := range ptrs {
				if p != 0 {
					allZero = false
					break
				}
			}
			if allZero {
				err = dr.Disk.FreeBlock(uint64(inode.Indirect1))
				if err != nil {
					return err
				}
				inode.Indirect1 = 0
				inode.Blocks -= 8
			}

			return dr.Disk.WriteInode(ino, inode)
		}
		return nil
	}

	if fileBlockNum < 12+N+N*N {
		if inode.Indirect2 == 0 {
			return nil
		}
		off := fileBlockNum - 12 - N
		idx1 := off / N
		idx2 := off % N

		ptrs1, err := readPointerBlock(uint64(inode.Indirect2))
		if err != nil {
			return err
		}

		ind1Block := ptrs1[idx1]
		if ind1Block == 0 {
			return nil
		}

		ptrs2, err := readPointerBlock(uint64(ind1Block))
		if err != nil {
			return err
		}

		phys := uint64(ptrs2[idx2])
		if phys != 0 {
			err = dr.Disk.FreeBlock(phys)
			if err != nil {
				return err
			}
			ptrs2[idx2] = 0
			err = writePointerBlock(uint64(ind1Block), ptrs2)
			if err != nil {
				return err
			}
			inode.Blocks -= 8

			allZero2 := true
			for _, p := range ptrs2 {
				if p != 0 {
					allZero2 = false
					break
				}
			}
			if allZero2 {
				err = dr.Disk.FreeBlock(uint64(ind1Block))
				if err != nil {
					return err
				}
				ptrs1[idx1] = 0
				err = writePointerBlock(uint64(inode.Indirect2), ptrs1)
				if err != nil {
					return err
				}
				inode.Blocks -= 8

				allZero1 := true
				for _, p := range ptrs1 {
					if p != 0 {
						allZero1 = false
						break
					}
				}
				if allZero1 {
					err = dr.Disk.FreeBlock(uint64(inode.Indirect2))
					if err != nil {
						return err
					}
					inode.Indirect2 = 0
					inode.Blocks -= 8
				}
			}

			return dr.Disk.WriteInode(ino, inode)
		}
		return nil
	}

	if fileBlockNum < 12+N+N*N+N*N*N {
		if inode.Indirect3 == 0 {
			return nil
		}
		off := fileBlockNum - 12 - N - N*N
		idx1 := off / (N * N)
		idx2 := (off / N) % N
		idx3 := off % N

		ptrs1, err := readPointerBlock(uint64(inode.Indirect3))
		if err != nil {
			return err
		}

		ind2Block := ptrs1[idx1]
		if ind2Block == 0 {
			return nil
		}

		ptrs2, err := readPointerBlock(uint64(ind2Block))
		if err != nil {
			return err
		}

		ind1Block := ptrs2[idx2]
		if ind1Block == 0 {
			return nil
		}

		ptrs3, err := readPointerBlock(uint64(ind1Block))
		if err != nil {
			return err
		}

		phys := uint64(ptrs3[idx3])
		if phys != 0 {
			err = dr.Disk.FreeBlock(phys)
			if err != nil {
				return err
			}
			ptrs3[idx3] = 0
			err = writePointerBlock(uint64(ind1Block), ptrs3)
			if err != nil {
				return err
			}
			inode.Blocks -= 8

			allZero3 := true
			for _, p := range ptrs3 {
				if p != 0 {
					allZero3 = false
					break
				}
			}
			if allZero3 {
				err = dr.Disk.FreeBlock(uint64(ind1Block))
				if err != nil {
					return err
				}
				ptrs2[idx2] = 0
				err = writePointerBlock(uint64(ind2Block), ptrs2)
				if err != nil {
					return err
				}
				inode.Blocks -= 8

				allZero2 := true
				for _, p := range ptrs2 {
					if p != 0 {
						allZero2 = false
						break
					}
				}
				if allZero2 {
					err = dr.Disk.FreeBlock(uint64(ind2Block))
					if err != nil {
						return err
					}
					ptrs1[idx1] = 0
					err = writePointerBlock(uint64(inode.Indirect3), ptrs1)
					if err != nil {
						return err
					}
					inode.Blocks -= 8

					allZero1 := true
					for _, p := range ptrs1 {
						if p != 0 {
							allZero1 = false
							break
						}
					}
					if allZero1 {
						err = dr.Disk.FreeBlock(uint64(inode.Indirect3))
						if err != nil {
							return err
						}
						inode.Indirect3 = 0
						inode.Blocks -= 8
					}
				}
			}

			return dr.Disk.WriteInode(ino, inode)
		}
		return nil
	}

	return errors.New("file block index out of bounds for clearing")
}

func (dr *Driver) ReadInodeData(inode *Inode, offset int64, dest []byte) (int, error) {
	if offset >= int64(inode.Size) {
		return 0, nil
	}

	limit := int64(inode.Size) - offset
	if int64(len(dest)) > limit {
		dest = dest[:limit]
	}

	bytesRead := 0
	for bytesRead < len(dest) {
		currOffset := offset + int64(bytesRead)
		fileBlockNum := uint32(currOffset / BlockSize)
		blockOffset := currOffset % BlockSize

		physBlock, err := dr.TranslateBlock(inode, 0, fileBlockNum, false)
		if err != nil {
			return bytesRead, err
		}

		var blockData []byte
		if physBlock == 0 {

			blockData = make([]byte, BlockSize)
		} else {
			blockData, err = dr.Disk.ReadBlock(physBlock)
			if err != nil {
				return bytesRead, err
			}
		}

		toCopy := BlockSize - blockOffset
		if int64(toCopy) > int64(len(dest)-bytesRead) {
			toCopy = int64(len(dest) - bytesRead)
		}

		copy(dest[bytesRead:], blockData[blockOffset:blockOffset+toCopy])
		bytesRead += int(toCopy)
	}

	return bytesRead, nil
}

func (dr *Driver) WriteInodeData(inode *Inode, ino uint32, offset int64, src []byte) (int, error) {
	bytesWritten := 0

	for bytesWritten < len(src) {
		currOffset := offset + int64(bytesWritten)
		fileBlockNum := uint32(currOffset / BlockSize)
		blockOffset := currOffset % BlockSize

		physBlock, err := dr.TranslateBlock(inode, ino, fileBlockNum, true)
		if err != nil {
			return bytesWritten, err
		}

		var blockData []byte
		if blockOffset == 0 && len(src)-bytesWritten >= BlockSize {

			blockData = make([]byte, BlockSize)
		} else {

			blockData, err = dr.Disk.ReadBlock(physBlock)
			if err != nil {
				return bytesWritten, err
			}
		}

		toCopy := BlockSize - blockOffset
		if int64(toCopy) > int64(len(src)-bytesWritten) {
			toCopy = int64(len(src) - bytesWritten)
		}

		copy(blockData[blockOffset:blockOffset+toCopy], src[bytesWritten:])

		err = dr.Disk.WriteBlock(physBlock, blockData)
		if err != nil {
			return bytesWritten, err
		}

		bytesWritten += int(toCopy)
	}

	newSize := uint64(offset + int64(bytesWritten))
	if newSize > inode.Size {
		inode.Size = newSize
	}

	now := time.Now().Unix()
	inode.Mtime = now
	inode.Ctime = now

	err := dr.Disk.WriteInode(ino, inode)
	if err != nil {
		return bytesWritten, err
	}

	return bytesWritten, nil
}

func (dr *Driver) TruncateInode(inode *Inode, ino uint32, newSize uint64) error {
	if newSize == inode.Size {
		return nil
	}

	if newSize < inode.Size {

		newBlockCount := (newSize + BlockSize - 1) / BlockSize
		oldBlockCount := (inode.Size + BlockSize - 1) / BlockSize

		for b := oldBlockCount; b > newBlockCount; b-- {
			err := dr.ClearBlockPointer(inode, ino, uint32(b-1))
			if err != nil {
				return err
			}
		}
	} else {

	}

	inode.Size = newSize
	now := time.Now().Unix()
	inode.Mtime = now
	inode.Ctime = now

	return dr.Disk.WriteInode(ino, inode)
}

func (dr *Driver) ResolvePath(path string) (uint32, *Inode, error) {
	path = strings.TrimSpace(path)
	if path == "" || path[0] != '/' {
		return 0, nil, errors.New("absolute path must start with '/'")
	}

	parts := []string{}
	for _, p := range strings.Split(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}

	currIno := uint32(1)
	currInode, err := dr.Disk.ReadInode(currIno)
	if err != nil {
		return 0, nil, err
	}

	for _, part := range parts {
		if (currInode.Mode & S_IFMT) != S_IFDIR {
			return 0, nil, ErrNotDirectory
		}

		nextIno, err := dr.FindDirent(currIno, currInode, part)
		if err != nil {
			return 0, nil, err
		}

		currIno = nextIno
		currInode, err = dr.Disk.ReadInode(currIno)
		if err != nil {
			return 0, nil, err
		}
	}

	return currIno, currInode, nil
}

func (dr *Driver) FindDirent(dirIno uint32, dirInode *Inode, name string) (uint32, error) {
	if (dirInode.Mode & S_IFMT) != S_IFDIR {
		return 0, ErrNotDirectory
	}

	dirData := make([]byte, dirInode.Size)
	_, err := dr.ReadInodeData(dirInode, 0, dirData)
	if err != nil {
		return 0, err
	}

	offset := 0
	for offset < len(dirData) {
		if offset+8 > len(dirData) {
			break
		}
		de, err := DeserializeDirent(dirData[offset:])
		if err != nil {
			return 0, err
		}
		if de.RecLen == 0 {
			break
		}

		if de.Inode != 0 && de.Name == name {
			return de.Inode, nil
		}

		offset += int(de.RecLen)
	}

	return 0, ErrNotFound
}

func (dr *Driver) ListDirents(dirInode *Inode) ([]*Dirent, error) {
	if (dirInode.Mode & S_IFMT) != S_IFDIR {
		return nil, ErrNotDirectory
	}

	dirData := make([]byte, dirInode.Size)
	_, err := dr.ReadInodeData(dirInode, 0, dirData)
	if err != nil {
		return nil, err
	}

	var list []*Dirent
	offset := 0
	for offset < len(dirData) {
		if offset+8 > len(dirData) {
			break
		}
		de, err := DeserializeDirent(dirData[offset:])
		if err != nil {
			return nil, err
		}
		if de.RecLen == 0 {
			break
		}

		if de.Inode != 0 {
			list = append(list, de)
		}
		offset += int(de.RecLen)
	}

	return list, nil
}

func (dr *Driver) AddDirent(dirIno uint32, dirInode *Inode, name string, targetIno uint32, fileType uint8) error {
	if (dirInode.Mode & S_IFMT) != S_IFDIR {
		return ErrNotDirectory
	}

	newEntryMinLen := uint16((8 + len(name) + 7) &^ 7)

	blockCount := uint32((dirInode.Size + BlockSize - 1) / BlockSize)
	var foundSpace bool

	for fileBlockIdx := uint32(0); fileBlockIdx < blockCount; fileBlockIdx++ {
		physBlock, err := dr.TranslateBlock(dirInode, dirIno, fileBlockIdx, false)
		if err != nil {
			return err
		}
		if physBlock == 0 {
			continue
		}

		blockData, err := dr.Disk.ReadBlock(physBlock)
		if err != nil {
			return err
		}

		offset := 0
		for offset < BlockSize {
			de, err := DeserializeDirent(blockData[offset:])
			if err != nil {
				return err
			}
			if de.RecLen == 0 {
				break
			}

			deMinLen := uint16((8 + int(de.NameLen) + 7) &^ 7)

			isLastInBlock := (offset + int(de.RecLen)) >= BlockSize

			if isLastInBlock || de.RecLen > deMinLen {

				var slack uint16
				if isLastInBlock {
					slack = uint16(BlockSize - (offset + int(deMinLen)))
				} else {
					slack = de.RecLen - deMinLen
				}

				if slack >= newEntryMinLen {

					de.RecLen = deMinLen
					deBytes := SerializeDirent(de)
					copy(blockData[offset:offset+int(deMinLen)], deBytes)

					newOffset := offset + int(deMinLen)
					newRecLen := uint16(BlockSize - newOffset)
					if !isLastInBlock {

						newRecLen = slack
					}

					newDe := &Dirent{
						Inode:    targetIno,
						RecLen:   newRecLen,
						NameLen:  uint8(len(name)),
						FileType: fileType,
						Name:     name,
					}
					newDeBytes := SerializeDirent(newDe)
					copy(blockData[newOffset:newOffset+int(newRecLen)], newDeBytes)

					err = dr.Disk.WriteBlock(physBlock, blockData)
					if err != nil {
						return err
					}

					foundSpace = true
					break
				}
			}

			offset += int(de.RecLen)
		}

		if foundSpace {
			break
		}
	}

	if !foundSpace {

		newPhysBlock, err := dr.TranslateBlock(dirInode, dirIno, blockCount, true)
		if err != nil {
			return err
		}

		blockData := make([]byte, BlockSize)

		newDe := &Dirent{
			Inode:    targetIno,
			RecLen:   uint16(BlockSize),
			NameLen:  uint8(len(name)),
			FileType: fileType,
			Name:     name,
		}
		newDeBytes := SerializeDirent(newDe)
		copy(blockData[0:BlockSize], newDeBytes)

		err = dr.Disk.WriteBlock(newPhysBlock, blockData)
		if err != nil {
			return err
		}

		dirInode.Size += BlockSize
	}

	now := time.Now().Unix()
	dirInode.Mtime = now
	dirInode.Ctime = now
	return dr.Disk.WriteInode(dirIno, dirInode)
}

func (dr *Driver) RemoveDirent(dirIno uint32, dirInode *Inode, name string) error {
	if (dirInode.Mode & S_IFMT) != S_IFDIR {
		return ErrNotDirectory
	}

	blockCount := uint32((dirInode.Size + BlockSize - 1) / BlockSize)
	found := false

	for fileBlockIdx := uint32(0); fileBlockIdx < blockCount; fileBlockIdx++ {
		physBlock, err := dr.TranslateBlock(dirInode, dirIno, fileBlockIdx, false)
		if err != nil {
			return err
		}
		if physBlock == 0 {
			continue
		}

		blockData, err := dr.Disk.ReadBlock(physBlock)
		if err != nil {
			return err
		}

		var prevDe *Dirent
		var prevOffset int
		offset := 0

		for offset < BlockSize {
			de, err := DeserializeDirent(blockData[offset:])
			if err != nil {
				return err
			}
			if de.RecLen == 0 {
				break
			}

			if de.Inode != 0 && de.Name == name {

				if prevDe != nil {

					prevDe.RecLen += de.RecLen
					prevDeBytes := SerializeDirent(prevDe)
					copy(blockData[prevOffset:prevOffset+int(prevDe.RecLen)], prevDeBytes)
				} else {

					de.Inode = 0
					deBytes := SerializeDirent(de)
					copy(blockData[offset:offset+int(de.RecLen)], deBytes)
				}

				err = dr.Disk.WriteBlock(physBlock, blockData)
				if err != nil {
					return err
				}

				found = true
				break
			}

			prevDe = de
			prevOffset = offset
			offset += int(de.RecLen)
		}

		if found {
			break
		}
	}

	if !found {
		return ErrNotFound
	}

	now := time.Now().Unix()
	dirInode.Mtime = now
	dirInode.Ctime = now
	return dr.Disk.WriteInode(dirIno, dirInode)
}

func (dr *Driver) CreateEntry(parentIno uint32, name string, mode uint32, uid, gid uint32, linkTarget string) (uint32, *Inode, error) {

	parentInode, err := dr.Disk.ReadInode(parentIno)
	if err != nil {
		return 0, nil, err
	}

	_, err = dr.FindDirent(parentIno, parentInode, name)
	if err == nil {
		return 0, nil, ErrAlreadyExists
	}
	if !errors.Is(err, ErrNotFound) {
		return 0, nil, err
	}

	newIno, err := dr.Disk.AllocInode()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to allocate inode: %w", err)
	}

	now := time.Now().Unix()
	newInode := &Inode{
		Mode:  mode,
		UID:   uid,
		GID:   gid,
		Nlink: 1,
		Size:  0,
		Atime: now,
		Mtime: now,
		Ctime: now,
	}

	fileType := uint8(FileTypeRegular)
	if (mode & S_IFMT) == S_IFDIR {
		fileType = FileTypeDirectory
		newInode.Nlink = 2
	} else if (mode & S_IFMT) == S_IFLNK {
		fileType = FileTypeSymlink
	}

	err = dr.Disk.WriteInode(newIno, newInode)
	if err != nil {
		dr.Disk.FreeInode(newIno)
		return 0, nil, err
	}

	if fileType == FileTypeDirectory {

		physBlock, err := dr.TranslateBlock(newInode, newIno, 0, true)
		if err != nil {
			dr.Disk.FreeInode(newIno)
			return 0, nil, err
		}

		blockData := make([]byte, BlockSize)

		dotDe := &Dirent{
			Inode:    newIno,
			RecLen:   12,
			NameLen:  1,
			FileType: FileTypeDirectory,
			Name:     ".",
		}
		dotBytes := SerializeDirent(dotDe)
		copy(blockData[0:12], dotBytes)

		dotDotDe := &Dirent{
			Inode:    parentIno,
			RecLen:   uint16(BlockSize - 12),
			NameLen:  2,
			FileType: FileTypeDirectory,
			Name:     "..",
		}
		dotDotBytes := SerializeDirent(dotDotDe)
		copy(blockData[12:], dotDotBytes)

		err = dr.Disk.WriteBlock(physBlock, blockData)
		if err != nil {
			dr.Disk.FreeInode(newIno)
			return 0, nil, err
		}

		newInode.Size = BlockSize
		err = dr.Disk.WriteInode(newIno, newInode)
		if err != nil {
			dr.Disk.FreeInode(newIno)
			return 0, nil, err
		}

		parentInode.Nlink++
		err = dr.Disk.WriteInode(parentIno, parentInode)
		if err != nil {
			dr.Disk.FreeInode(newIno)
			return 0, nil, err
		}
	} else if fileType == FileTypeSymlink {

		_, err = dr.WriteInodeData(newInode, newIno, 0, []byte(linkTarget))
		if err != nil {
			dr.Disk.FreeInode(newIno)
			return 0, nil, err
		}
	}

	err = dr.AddDirent(parentIno, parentInode, name, newIno, fileType)
	if err != nil {

		dr.TruncateInode(newInode, newIno, 0)
		dr.Disk.FreeInode(newIno)
		if fileType == FileTypeDirectory {
			parentInode.Nlink--
			dr.Disk.WriteInode(parentIno, parentInode)
		}
		return 0, nil, err
	}

	return newIno, newInode, nil
}

func (dr *Driver) UnlinkEntry(parentIno uint32, name string) error {
	parentInode, err := dr.Disk.ReadInode(parentIno)
	if err != nil {
		return err
	}

	ino, err := dr.FindDirent(parentIno, parentInode, name)
	if err != nil {
		return err
	}

	inode, err := dr.Disk.ReadInode(ino)
	if err != nil {
		return err
	}

	if (inode.Mode & S_IFMT) == S_IFDIR {
		return ErrIsDir
	}

	err = dr.RemoveDirent(parentIno, parentInode, name)
	if err != nil {
		return err
	}

	inode.Nlink--
	if inode.Nlink == 0 {

		err = dr.TruncateInode(inode, ino, 0)
		if err != nil {
			return err
		}
		err = dr.Disk.FreeInode(ino)
		if err != nil {
			return err
		}
	} else {
		err = dr.Disk.WriteInode(ino, inode)
		if err != nil {
			return err
		}
	}

	return nil
}

func (dr *Driver) RmdirEntry(parentIno uint32, name string) error {
	parentInode, err := dr.Disk.ReadInode(parentIno)
	if err != nil {
		return err
	}

	ino, err := dr.FindDirent(parentIno, parentInode, name)
	if err != nil {
		return err
	}

	inode, err := dr.Disk.ReadInode(ino)
	if err != nil {
		return err
	}

	if (inode.Mode & S_IFMT) != S_IFDIR {
		return ErrNotDirectory
	}

	list, err := dr.ListDirents(inode)
	if err != nil {
		return err
	}

	for _, de := range list {
		if de.Name != "." && de.Name != ".." {
			return ErrDirNotEmpty
		}
	}

	err = dr.RemoveDirent(parentIno, parentInode, name)
	if err != nil {
		return err
	}

	err = dr.TruncateInode(inode, ino, 0)
	if err != nil {
		return err
	}

	err = dr.Disk.FreeInode(ino)
	if err != nil {
		return err
	}

	parentInode.Nlink--
	err = dr.Disk.WriteInode(parentIno, parentInode)
	if err != nil {
		return err
	}

	return nil
}

func (dr *Driver) RenameEntry(oldParentIno uint32, oldName string, newParentIno uint32, newName string) error {
	oldParentInode, err := dr.Disk.ReadInode(oldParentIno)
	if err != nil {
		return err
	}

	ino, err := dr.FindDirent(oldParentIno, oldParentInode, oldName)
	if err != nil {
		return err
	}

	inode, err := dr.Disk.ReadInode(ino)
	if err != nil {
		return err
	}

	newParentInode, err := dr.Disk.ReadInode(newParentIno)
	if err != nil {
		return err
	}

	targetIno, err := dr.FindDirent(newParentIno, newParentInode, newName)
	if err == nil {

		targetInode, err := dr.Disk.ReadInode(targetIno)
		if err != nil {
			return err
		}

		if (targetInode.Mode & S_IFMT) == S_IFDIR {

			err = dr.RmdirEntry(newParentIno, newName)
			if err != nil {
				return err
			}
		} else {
			err = dr.UnlinkEntry(newParentIno, newName)
			if err != nil {
				return err
			}
		}

		newParentInode, err = dr.Disk.ReadInode(newParentIno)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}

	fileType := uint8(FileTypeRegular)
	if (inode.Mode & S_IFMT) == S_IFDIR {
		fileType = FileTypeDirectory
	} else if (inode.Mode & S_IFMT) == S_IFLNK {
		fileType = FileTypeSymlink
	}

	err = dr.AddDirent(newParentIno, newParentInode, newName, ino, fileType)
	if err != nil {
		return err
	}

	if oldParentIno == newParentIno {
		oldParentInode = newParentInode
	} else {
		oldParentInode, err = dr.Disk.ReadInode(oldParentIno)
		if err != nil {
			return err
		}
	}

	err = dr.RemoveDirent(oldParentIno, oldParentInode, oldName)
	if err != nil {

		_ = dr.RemoveDirent(newParentIno, newParentInode, newName)
		return err
	}

	if fileType == FileTypeDirectory && oldParentIno != newParentIno {

		physBlock, err := dr.TranslateBlock(inode, ino, 0, false)
		if err != nil {
			return err
		}
		blockData, err := dr.Disk.ReadBlock(physBlock)
		if err != nil {
			return err
		}

		deDotDot, err := DeserializeDirent(blockData[12:])
		if err != nil {
			return err
		}
		deDotDot.Inode = newParentIno
		deBytes := SerializeDirent(deDotDot)
		copy(blockData[12:12+int(deDotDot.RecLen)], deBytes)

		err = dr.Disk.WriteBlock(physBlock, blockData)
		if err != nil {
			return err
		}

		newParentInode, err = dr.Disk.ReadInode(newParentIno)
		if err == nil {
			newParentInode.Nlink++
			_ = dr.Disk.WriteInode(newParentIno, newParentInode)
		}

		oldParentInode, err = dr.Disk.ReadInode(oldParentIno)
		if err == nil {
			oldParentInode.Nlink--
			_ = dr.Disk.WriteInode(oldParentIno, oldParentInode)
		}
	}

	return nil
}
