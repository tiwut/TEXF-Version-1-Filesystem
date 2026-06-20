package fuse

import (
	"path"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"texf/fs"
)

type TEXFuse struct {
	fuse.FileSystemBase
	Driver *fs.Driver
}

func NewTEXFuse(driver *fs.Driver) *TEXFuse {
	return &TEXFuse{Driver: driver}
}

func mapError(err error) int {
	if err == nil {
		return 0
	}
	switch err {
	case fs.ErrNotFound:
		return -fuse.ENOENT
	case fs.ErrAlreadyExists:
		return -fuse.EEXIST
	case fs.ErrNotDirectory:
		return -fuse.ENOTDIR
	case fs.ErrIsDir:
		return -fuse.EISDIR
	case fs.ErrDirNotEmpty:
		return -fuse.ENOTEMPTY
	default:
		return -fuse.EIO
	}
}

func (f *TEXFuse) Init() {
}

func (f *TEXFuse) Destroy() {
}

func (f *TEXFuse) Statfs(p string, stat *fuse.Statfs_t) int {
	sb := f.Driver.Disk.GetSuperblock()
	stat.Bsize = fs.BlockSize
	stat.Frsize = fs.BlockSize
	stat.Blocks = sb.BlockCount
	stat.Bfree = sb.FreeBlocksCount
	stat.Bavail = sb.FreeBlocksCount
	stat.Files = sb.InodeCount
	stat.Ffree = sb.FreeInodesCount
	stat.Favail = sb.FreeInodesCount
	stat.Namemax = 255
	return 0
}

func (f *TEXFuse) Getattr(p string, stat *fuse.Stat_t, fh uint64) int {
	ino, inode, err := f.Driver.ResolvePath(p)
	if err != nil {
		return mapError(err)
	}

	stat.Mode = inode.Mode
	stat.Uid = inode.UID
	stat.Gid = inode.GID
	stat.Nlink = inode.Nlink
	stat.Size = int64(inode.Size)
	stat.Atim = fuse.Timespec{Sec: inode.Atime}
	stat.Mtim = fuse.Timespec{Sec: inode.Mtime}
	stat.Ctim = fuse.Timespec{Sec: inode.Ctime}
	stat.Birthtim = fuse.Timespec{Sec: inode.Ctime}
	stat.Blksize = fs.BlockSize
	stat.Blocks = int64(inode.Blocks)
	stat.Ino = uint64(ino)
	return 0
}

func (f *TEXFuse) Chmod(p string, mode uint32) int {
	ino, inode, err := f.Driver.ResolvePath(p)
	if err != nil {
		return mapError(err)
	}

	inode.Mode = (inode.Mode & fs.S_IFMT) | (mode & fs.S_IRWXUGO)
	inode.Ctime = time.Now().Unix()

	err = f.Driver.Disk.WriteInode(ino, inode)
	if err != nil {
		return -fuse.EIO
	}
	return 0
}

func (f *TEXFuse) Chown(p string, uid uint32, gid uint32) int {
	ino, inode, err := f.Driver.ResolvePath(p)
	if err != nil {
		return mapError(err)
	}

	if uid != ^uint32(0) {
		inode.UID = uid
	}
	if gid != ^uint32(0) {
		inode.GID = gid
	}
	inode.Ctime = time.Now().Unix()

	err = f.Driver.Disk.WriteInode(ino, inode)
	if err != nil {
		return -fuse.EIO
	}
	return 0
}

func (f *TEXFuse) Utimens(p string, tmsp []fuse.Timespec) int {
	ino, inode, err := f.Driver.ResolvePath(p)
	if err != nil {
		return mapError(err)
	}

	now := time.Now().Unix()
	if tmsp != nil && len(tmsp) >= 2 {
		inode.Atime = tmsp[0].Sec
		inode.Mtime = tmsp[1].Sec
	} else {
		inode.Atime = now
		inode.Mtime = now
	}
	inode.Ctime = now

	err = f.Driver.Disk.WriteInode(ino, inode)
	if err != nil {
		return -fuse.EIO
	}
	return 0
}

func (f *TEXFuse) Access(p string, mask uint32) int {
	return 0
}

func (f *TEXFuse) Create(p string, flags int, mode uint32) (int, uint64) {
	dir := path.Dir(p)
	name := path.Base(p)

	parentIno, _, err := f.Driver.ResolvePath(dir)
	if err != nil {
		return mapError(err), 0
	}

	uid, gid, _ := fuse.Getcontext()
	_, _, err = f.Driver.CreateEntry(parentIno, name, fs.S_IFREG|(mode&fs.S_IRWXUGO), uid, gid, "")
	if err != nil {
		return mapError(err), 0
	}

	return 0, 0
}

func (f *TEXFuse) Open(p string, flags int) (int, uint64) {
	_, inode, err := f.Driver.ResolvePath(p)
	if err != nil {
		return mapError(err), 0
	}
	if (inode.Mode & fs.S_IFMT) == fs.S_IFDIR {
		return -fuse.EISDIR, 0
	}
	return 0, 0
}

func (f *TEXFuse) Read(p string, buff []byte, ofst int64, fh uint64) int {
	_, inode, err := f.Driver.ResolvePath(p)
	if err != nil {
		return mapError(err)
	}

	n, err := f.Driver.ReadInodeData(inode, ofst, buff)
	if err != nil {
		return -fuse.EIO
	}
	return n
}

func (f *TEXFuse) Write(p string, buff []byte, ofst int64, fh uint64) int {
	ino, inode, err := f.Driver.ResolvePath(p)
	if err != nil {
		return mapError(err)
	}

	n, err := f.Driver.WriteInodeData(inode, ino, ofst, buff)
	if err != nil {
		return -fuse.EIO
	}
	return n
}

func (f *TEXFuse) Truncate(p string, size int64, fh uint64) int {
	ino, inode, err := f.Driver.ResolvePath(p)
	if err != nil {
		return mapError(err)
	}

	err = f.Driver.TruncateInode(inode, ino, uint64(size))
	if err != nil {
		return mapError(err)
	}
	return 0
}

func (f *TEXFuse) Unlink(p string) int {
	dir := path.Dir(p)
	name := path.Base(p)

	parentIno, _, err := f.Driver.ResolvePath(dir)
	if err != nil {
		return mapError(err)
	}

	err = f.Driver.UnlinkEntry(parentIno, name)
	return mapError(err)
}

func (f *TEXFuse) Mkdir(p string, mode uint32) int {
	dir := path.Dir(p)
	name := path.Base(p)

	parentIno, _, err := f.Driver.ResolvePath(dir)
	if err != nil {
		return mapError(err)
	}

	uid, gid, _ := fuse.Getcontext()
	_, _, err = f.Driver.CreateEntry(parentIno, name, fs.S_IFDIR|(mode&fs.S_IRWXUGO), uid, gid, "")
	return mapError(err)
}

func (f *TEXFuse) Rmdir(p string) int {
	dir := path.Dir(p)
	name := path.Base(p)

	parentIno, _, err := f.Driver.ResolvePath(dir)
	if err != nil {
		return mapError(err)
	}

	err = f.Driver.RmdirEntry(parentIno, name)
	return mapError(err)
}

func (f *TEXFuse) Rename(oldpath string, newpath string) int {
	oldDir := path.Dir(oldpath)
	oldName := path.Base(oldpath)
	newDir := path.Dir(newpath)
	newName := path.Base(newpath)

	oldParentIno, _, err := f.Driver.ResolvePath(oldDir)
	if err != nil {
		return mapError(err)
	}

	newParentIno, _, err := f.Driver.ResolvePath(newDir)
	if err != nil {
		return mapError(err)
	}

	err = f.Driver.RenameEntry(oldParentIno, oldName, newParentIno, newName)
	return mapError(err)
}

func (f *TEXFuse) Mknod(p string, mode uint32, dev uint64) int {
	dir := path.Dir(p)
	name := path.Base(p)

	parentIno, _, err := f.Driver.ResolvePath(dir)
	if err != nil {
		return mapError(err)
	}

	uid, gid, _ := fuse.Getcontext()
	_, _, err = f.Driver.CreateEntry(parentIno, name, mode, uid, gid, "")
	return mapError(err)
}

func (f *TEXFuse) Readdir(p string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) int {
	_, inode, err := f.Driver.ResolvePath(p)
	if err != nil {
		return mapError(err)
	}

	list, err := f.Driver.ListDirents(inode)
	if err != nil {
		return mapError(err)
	}

	for _, de := range list {
		stat := &fuse.Stat_t{
			Ino:  uint64(de.Inode),
			Mode: uint32(de.FileType) << 12,
		}
		if de.FileType == fs.FileTypeDirectory {
			stat.Mode = fs.S_IFDIR
		} else if de.FileType == fs.FileTypeSymlink {
			stat.Mode = fs.S_IFLNK
		} else {
			stat.Mode = fs.S_IFREG
		}

		if !fill(de.Name, stat, 0) {
			break
		}
	}

	return 0
}

func (f *TEXFuse) Symlink(target string, newpath string) int {
	dir := path.Dir(newpath)
	name := path.Base(newpath)

	parentIno, _, err := f.Driver.ResolvePath(dir)
	if err != nil {
		return mapError(err)
	}

	uid, gid, _ := fuse.Getcontext()
	_, _, err = f.Driver.CreateEntry(parentIno, name, fs.S_IFLNK|0777, uid, gid, target)
	return mapError(err)
}

func (f *TEXFuse) Readlink(p string) (int, string) {
	_, inode, err := f.Driver.ResolvePath(p)
	if err != nil {
		return mapError(err), ""
	}

	if (inode.Mode & fs.S_IFMT) != fs.S_IFLNK {
		return -fuse.EINVAL, ""
	}

	buf := make([]byte, inode.Size)
	_, err = f.Driver.ReadInodeData(inode, 0, buf)
	if err != nil {
		return -fuse.EIO, ""
	}

	return 0, string(buf)
}
