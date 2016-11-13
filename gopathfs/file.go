package gopathfs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"golang.org/x/sys/unix"
)

// Open overwrites the parent's Open method.
func (gpf *GoPathFs) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	if strings.HasPrefix(name, gpf.cfg.GoPkgPrefix+"/") {
		return gpf.openFirstPartyChildFile(name, flags, context)
	}

	// Search in vendor directories.
	for _, vendor := range gpf.cfg.Vendors {
		f, status := gpf.openUnderlyingFile(filepath.Join(gpf.dirs.Workspace, vendor, name), flags, context)
		if status == fuse.OK {
			return f, status
		}
	}

	return nil, fuse.ENOENT
}

// Create overwrites the parent's Create method.
func (gpf *GoPathFs) Create(name string, flags uint32, mode uint32,
	context *fuse.Context) (file nodefs.File, code fuse.Status) {

	prefix := gpf.cfg.GoPkgPrefix + "/"
	if strings.HasPrefix(name, prefix) {
		return gpf.createFirstPartyChildFile(name[len(prefix):], flags, mode, context)
	}

	return nil, fuse.ENOSYS
}

// Unlink overwrites the parent's Unlink method.
func (gpf *GoPathFs) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	prefix := gpf.cfg.GoPkgPrefix + "/"
	if strings.HasPrefix(name, prefix) {
		return gpf.unlinkFirstPartyChildFile(name[len(prefix):], context)
	}

	return fuse.ENOSYS
}

func (gpf *GoPathFs) openFirstPartyChildFile(name string, flags uint32,
	context *fuse.Context) (file nodefs.File, code fuse.Status) {

	name = name[len(gpf.cfg.GoPkgPrefix+"/"):]

	f, status := gpf.openUnderlyingFile(filepath.Join(gpf.dirs.Workspace, name), flags, context)
	if status == fuse.OK {
		return f, status
	}

	// Also search in bazel-genfiles.
	return gpf.openUnderlyingFile(filepath.Join(gpf.dirs.Workspace, "bazel-genfiles", name), flags, context)
}

func (gpf *GoPathFs) openUnderlyingFile(name string, flags uint32,
	context *fuse.Context) (file nodefs.File, code fuse.Status) {

	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return nil, fuse.ENOENT
		}
	}

	if flags&fuse.O_ANYWRITE != 0 && unix.Access(name, unix.W_OK) != nil {
		return nil, fuse.EPERM
	}

	f, err := os.OpenFile(name, int(flags), 0)
	if err != nil {
		return nil, fuse.ENOENT
	}

	return nodefs.NewLoopbackFile(f), fuse.OK
}

func (gpf *GoPathFs) createFirstPartyChildFile(name string, flags uint32, mode uint32,
	context *fuse.Context) (file nodefs.File, code fuse.Status) {

	name = filepath.Join(gpf.dirs.Workspace, name)

	f, err := os.Create(name)
	if err != nil {
		return nil, fuse.EIO
	}

	return nodefs.NewLoopbackFile(f), fuse.OK
}

func (gpf *GoPathFs) unlinkFirstPartyChildFile(name string, context *fuse.Context) (code fuse.Status) {
	name = filepath.Join(gpf.dirs.Workspace, name)

	if err := os.Remove(name); err != nil {
		return fuse.EIO
	}

	return fuse.OK
}
