package gopathfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"golang.org/x/sys/unix"
)

// Open overwrites the parent's Open method.
func (gpf *GoPathFs) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	if gpf.debug {
		fmt.Printf("Reqeusted to open file %s.\n", name)
	}

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

	if gpf.debug {
		fmt.Printf("Reqeusted to create file %s.\n", name)
	}

	prefix := gpf.cfg.GoPkgPrefix + "/"
	if strings.HasPrefix(name, prefix) {
		return gpf.createFirstPartyChildFile(name[len(prefix):], flags, mode, context)
	}

	return nil, fuse.ENOSYS
}

// Unlink overwrites the parent's Unlink method.
func (gpf *GoPathFs) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	if gpf.debug {
		fmt.Printf("Reqeusted to unlink file %s.\n", name)
	}

	prefix := gpf.cfg.GoPkgPrefix + "/"
	if strings.HasPrefix(name, prefix) {
		return gpf.unlinkFirstPartyChildFile(name[len(prefix):], context)
	}

	return fuse.ENOSYS
}

// Rename overwrites the parent's Rename method.
func (gpf *GoPathFs) Rename(oldName string, newName string, context *fuse.Context) (code fuse.Status) {
	if gpf.debug {
		fmt.Printf("Reqeusted to rename from %s to %s.\n", oldName, newName)
	}

	if strings.HasPrefix(oldName, "jingoal.com/") {
		oldName = filepath.Join(gpf.dirs.Workspace, oldName[len("jingoal.com/"):])
		newName = filepath.Join(gpf.dirs.Workspace, newName[len("jingoal.com/"):])
	} else {
		// Vendor directories.
		for _, vendor := range gpf.cfg.Vendors {
			oldName = filepath.Join(vendor, oldName)
			if _, err := os.Stat(oldName); err == nil {
				newName = filepath.Join(vendor, newName)
				break
			}
		}
		if newName == "" || oldName == "" {
			return fuse.ENOSYS
		}
	}

	if gpf.debug {
		fmt.Printf("Actual rename from %s to %s ... ", oldName, newName)
	}
	if err := os.Rename(oldName, newName); err != nil {
		if gpf.debug {
			fmt.Println("failed to rename file %s,", oldName, err)
		}
		return fuse.ENOSYS
	}
	if gpf.debug {
		fmt.Println("Succeeded to rename file %s.\n", oldName)
	}
	return fuse.OK
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

	if gpf.debug {
		fmt.Printf("Actually opening file %s.\n", name)
	}

	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("File not found: %s.\n", name)
			return nil, fuse.ENOENT
		}
	}

	if flags&fuse.O_ANYWRITE != 0 && unix.Access(name, unix.W_OK) != nil {
		fmt.Printf("File not writable: %s.\n", name)
		return nil, fuse.EPERM
	}

	f, err := os.OpenFile(name, int(flags), 0)
	if err != nil {
		fmt.Printf("Failed to open file: %s, %+v.\n", name, err)
		return nil, fuse.ENOENT
	}

	fmt.Printf("Succeeded to open file: %s.\n", name)
	return nodefs.NewLoopbackFile(f), fuse.OK
}

func (gpf *GoPathFs) createFirstPartyChildFile(name string, flags uint32, mode uint32,
	context *fuse.Context) (file nodefs.File, code fuse.Status) {

	name = filepath.Join(gpf.dirs.Workspace, name)

	if gpf.debug {
		fmt.Printf("Actually creating file %s.\n", name)
	}

	f, err := os.Create(name)
	if err != nil {
		if gpf.debug {
			fmt.Printf("Failed to create file %s.\n", name)
		}
		return nil, fuse.EIO
	}

	if gpf.debug {
		fmt.Printf("Succeeded to create file %s.\n", name)
	}
	return nodefs.NewLoopbackFile(f), fuse.OK
}

func (gpf *GoPathFs) unlinkFirstPartyChildFile(name string, context *fuse.Context) (code fuse.Status) {
	name = filepath.Join(gpf.dirs.Workspace, name)

	if gpf.debug {
		fmt.Printf("Actually unlinking file %s.\n", name)
	}

	if err := os.Remove(name); err != nil {
		if gpf.debug {
			fmt.Printf("Failed to unlink file %s.\n", name)
		}
		return fuse.EIO
	}

	if gpf.debug {
		fmt.Printf("Succeeded to unlink file %s.\n", name)
	}
	return fuse.OK
}
