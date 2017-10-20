package gopathfs

import(
	"github.com/hanwen/go-fuse/fuse"
	"golang.org/x/sys/unix"
)


func unixAttrToFuseAttr(from unix.Stat_t) (result fuse.Attr) {
	result.Ino = from.Ino
	result.Size = uint64(from.Size)
	result.Blocks = uint64(from.Blocks)
	result.Mode = from.Mode

	sec, nsec := from.Atim.Unix()
	result.Atime = uint64(sec)
	result.Atimensec = uint32(nsec)

	sec, nsec = from.Ctim.Unix()
	result.Ctime = uint64(sec)
	result.Ctimensec = uint32(nsec)

	sec, nsec = from.Mtim.Unix()
	result.Mtime = uint64(sec)
	result.Mtimensec = uint32(nsec)

	return
}
