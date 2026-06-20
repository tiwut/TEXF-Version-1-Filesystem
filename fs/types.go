package fs

import (
	"encoding/binary"
	"errors"
)

const (
	BlockSize = 4096
	Magic     = 0x54455846

	InodeSize = 128

	FileTypeRegular   = 1
	FileTypeDirectory = 2
	FileTypeSymlink   = 3

	S_IFMT    = 0170000
	S_IFREG   = 0100000
	S_IFDIR   = 0040000
	S_IFLNK   = 0120000
	S_IRWXUGO = 0000777
)

type Superblock struct {
	Magic            uint32
	Version          uint32
	BlockSize        uint32
	BlockCount       uint64
	InodeCount       uint64
	FreeBlocksCount  uint64
	FreeInodesCount  uint64
	BlockBitmapBlock uint64
	InodeBitmapBlock uint64
	InodeTableBlock  uint64
	DataBlocksStart  uint64
	InodeTableSize   uint64
	Label            [16]byte
}

func (sb *Superblock) Serialize() []byte {
	buf := make([]byte, BlockSize)
	binary.LittleEndian.PutUint32(buf[0:4], sb.Magic)
	binary.LittleEndian.PutUint32(buf[4:8], sb.Version)
	binary.LittleEndian.PutUint32(buf[8:12], sb.BlockSize)
	binary.LittleEndian.PutUint64(buf[12:20], sb.BlockCount)
	binary.LittleEndian.PutUint64(buf[20:28], sb.InodeCount)
	binary.LittleEndian.PutUint64(buf[28:36], sb.FreeBlocksCount)
	binary.LittleEndian.PutUint64(buf[36:44], sb.FreeInodesCount)
	binary.LittleEndian.PutUint64(buf[44:52], sb.BlockBitmapBlock)
	binary.LittleEndian.PutUint64(buf[52:60], sb.InodeBitmapBlock)
	binary.LittleEndian.PutUint64(buf[60:68], sb.InodeTableBlock)
	binary.LittleEndian.PutUint64(buf[68:76], sb.DataBlocksStart)
	binary.LittleEndian.PutUint64(buf[76:84], sb.InodeTableSize)
	copy(buf[84:100], sb.Label[:])
	return buf
}

func DeserializeSuperblock(buf []byte) (*Superblock, error) {
	if len(buf) < BlockSize {
		return nil, errors.New("buffer too small for superblock")
	}
	sb := &Superblock{
		Magic:            binary.LittleEndian.Uint32(buf[0:4]),
		Version:          binary.LittleEndian.Uint32(buf[4:8]),
		BlockSize:        binary.LittleEndian.Uint32(buf[8:12]),
		BlockCount:       binary.LittleEndian.Uint64(buf[12:20]),
		InodeCount:       binary.LittleEndian.Uint64(buf[20:28]),
		FreeBlocksCount:  binary.LittleEndian.Uint64(buf[28:36]),
		FreeInodesCount:  binary.LittleEndian.Uint64(buf[36:44]),
		BlockBitmapBlock: binary.LittleEndian.Uint64(buf[44:52]),
		InodeBitmapBlock: binary.LittleEndian.Uint64(buf[52:60]),
		InodeTableBlock:  binary.LittleEndian.Uint64(buf[60:68]),
		DataBlocksStart:  binary.LittleEndian.Uint64(buf[68:76]),
		InodeTableSize:   binary.LittleEndian.Uint64(buf[76:84]),
	}
	copy(sb.Label[:], buf[84:100])
	if sb.Magic != Magic {
		return nil, errors.New("invalid magic number")
	}
	return sb, nil
}

type Inode struct {
	Mode      uint32
	UID       uint32
	GID       uint32
	Nlink     uint32
	Size      uint64
	Atime     int64
	Mtime     int64
	Ctime     int64
	Blocks    uint64
	Direct    [12]uint32
	Indirect1 uint32
	Indirect2 uint32
	Indirect3 uint32
}

func (in *Inode) Serialize() []byte {
	buf := make([]byte, InodeSize)
	binary.LittleEndian.PutUint32(buf[0:4], in.Mode)
	binary.LittleEndian.PutUint32(buf[4:8], in.UID)
	binary.LittleEndian.PutUint32(buf[8:12], in.GID)
	binary.LittleEndian.PutUint32(buf[12:16], in.Nlink)
	binary.LittleEndian.PutUint64(buf[16:24], in.Size)
	binary.LittleEndian.PutUint64(buf[24:32], uint64(in.Atime))
	binary.LittleEndian.PutUint64(buf[32:40], uint64(in.Mtime))
	binary.LittleEndian.PutUint64(buf[40:48], uint64(in.Ctime))
	binary.LittleEndian.PutUint64(buf[48:56], in.Blocks)
	for i := 0; i < 12; i++ {
		binary.LittleEndian.PutUint32(buf[56+i*4:60+i*4], in.Direct[i])
	}
	binary.LittleEndian.PutUint32(buf[104:108], in.Indirect1)
	binary.LittleEndian.PutUint32(buf[108:112], in.Indirect2)
	binary.LittleEndian.PutUint32(buf[112:116], in.Indirect3)

	return buf
}

func DeserializeInode(buf []byte) (*Inode, error) {
	if len(buf) < InodeSize {
		return nil, errors.New("buffer too small for inode")
	}
	in := &Inode{
		Mode:      binary.LittleEndian.Uint32(buf[0:4]),
		UID:       binary.LittleEndian.Uint32(buf[4:8]),
		GID:       binary.LittleEndian.Uint32(buf[8:12]),
		Nlink:     binary.LittleEndian.Uint32(buf[12:16]),
		Size:      binary.LittleEndian.Uint64(buf[16:24]),
		Atime:     int64(binary.LittleEndian.Uint64(buf[24:32])),
		Mtime:     int64(binary.LittleEndian.Uint64(buf[32:40])),
		Ctime:     int64(binary.LittleEndian.Uint64(buf[40:48])),
		Blocks:    binary.LittleEndian.Uint64(buf[48:56]),
		Indirect1: binary.LittleEndian.Uint32(buf[104:108]),
		Indirect2: binary.LittleEndian.Uint32(buf[108:112]),
		Indirect3: binary.LittleEndian.Uint32(buf[112:116]),
	}
	for i := 0; i < 12; i++ {
		in.Direct[i] = binary.LittleEndian.Uint32(buf[56+i*4 : 60+i*4])
	}
	return in, nil
}

type Dirent struct {
	Inode    uint32
	RecLen   uint16
	NameLen  uint8
	FileType uint8
	Name     string
}

func SerializeDirent(d *Dirent) []byte {
	buf := make([]byte, d.RecLen)
	binary.LittleEndian.PutUint32(buf[0:4], d.Inode)
	binary.LittleEndian.PutUint16(buf[4:6], d.RecLen)
	buf[6] = d.NameLen
	buf[7] = d.FileType
	copy(buf[8:8+int(d.NameLen)], d.Name)
	return buf
}

func DeserializeDirent(buf []byte) (*Dirent, error) {
	if len(buf) < 8 {
		return nil, errors.New("buffer too small for dirent")
	}
	d := &Dirent{
		Inode:    binary.LittleEndian.Uint32(buf[0:4]),
		RecLen:   binary.LittleEndian.Uint16(buf[4:6]),
		NameLen:  buf[6],
		FileType: buf[7],
	}
	if int(d.RecLen) > len(buf) {
		return nil, errors.New("dirent record length exceeds buffer size")
	}
	if 8+int(d.NameLen) > int(d.RecLen) {
		return nil, errors.New("dirent name length exceeds record length")
	}
	d.Name = string(buf[8 : 8+int(d.NameLen)])
	return d, nil
}
