package gopathfs

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"github.com/linuxerwang/gobazel/conf"
	"github.com/linuxerwang/gobazel/exec"
	"github.com/rjeczalik/notify"
)

var (
	pathSeparator = string(os.PathSeparator)
)

// Dirs contains directory paths for GoPathFs.
type Dirs struct {
	Workspace string
	GobzlConf string
	GobzlPid  string
	BinDir    string
	PkgDir    string
	SrcDir    string
	GoSDKDir  string
}

// GoPathFs implements a virtual tree for src folder of GOPATH.
type GoPathFs struct {
	pathfs.FileSystem
	debug         bool
	dirs          *Dirs
	cfg           *conf.GobazelConf
	ignoreRegexes []*regexp.Regexp
	notifyCh      chan notify.EventInfo
}

// Access overwrites the parent's Access method.
func (gpf *GoPathFs) Access(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.OK
}

// OnMount overwrites the parent's OnMount method.
func (gpf *GoPathFs) OnMount(nodeFs *pathfs.PathNodeFs) {
	if err := notify.Watch(filepath.Join(gpf.dirs.Workspace, "..."), gpf.notifyCh, notify.All); err != nil {
		log.Fatal(err)
	}

	go func() {
		for ei := range gpf.notifyCh {
			path := ei.Path()[len(gpf.dirs.Workspace+pathSeparator):]
			gpf.notifyFileChange(nodeFs, path)
		}
	}()
}

// OnUnmount overwrites the parent's OnUnmount method.
func (gpf *GoPathFs) OnUnmount() {
	notify.Stop(gpf.notifyCh)
}

func (gpf *GoPathFs) notifyFileChange(nodeFs *pathfs.PathNodeFs, path string) {
	if gpf.isIgnored(path) {
		return
	}

	if strings.HasSuffix(path, pathSeparator+".git") || strings.Contains(path, pathSeparator+".git"+pathSeparator) {
		return
	}

	go nodeFs.Notify(filepath.Join(gpf.cfg.GoPkgPrefix, path))

	isVendor := false
	for _, vendor := range gpf.cfg.Vendors {
		if strings.HasPrefix(path, vendor+pathSeparator) {
			isVendor = true
			nodeFs.FileNotify(path[len(vendor+pathSeparator):], 0, 0)
			break
		}
	}

	// If it's a proto file, run bazel build.
	if strings.HasSuffix(path, ".proto") {
		bzlPkg := filepath.Dir(path) + ":*"
		exec.RunBazelBuild(gpf.dirs.Workspace, bzlPkg)
	}

	// Run go install.
	if strings.HasSuffix(path, ".proto") || strings.HasSuffix(path, ".go") {
		goPkg := filepath.Dir(path)
		if !isVendor {
			goPkg = filepath.Join(gpf.cfg.GoPkgPrefix, goPkg)
		}
		exec.RunGoInstall(gpf.cfg, goPkg)
	}
}

func (gpf *GoPathFs) isIgnored(dir string) bool {
	if strings.HasPrefix(dir, ".") {
		return true
	}

	for _, re := range gpf.ignoreRegexes {
		if re.MatchString(dir) {
			return true
		}
	}
	return false
}

func (gpf *GoPathFs) isVendorDir(dir string) bool {
	for _, vendor := range gpf.cfg.Vendors {
		if dir == vendor {
			return true
		}
		if strings.HasPrefix(dir, vendor+pathSeparator) {
			return true
		}
	}
	return false
}

// NewGoPathFs returns a new GoPathFs.
func NewGoPathFs(debug bool, cfg *conf.GobazelConf, dirs *Dirs) *GoPathFs {
	ignoreRegexes := make([]*regexp.Regexp, len(cfg.Ignores))
	for i, ign := range cfg.Ignores {
		ignoreRegexes[i] = regexp.MustCompile(ign)
	}

	gpfs := GoPathFs{
		FileSystem:    pathfs.NewDefaultFileSystem(),
		debug:         debug,
		dirs:          dirs,
		cfg:           cfg,
		ignoreRegexes: ignoreRegexes,
		notifyCh:      make(chan notify.EventInfo, 10),
	}

	// Find the go-sdk in bazel external folder. The debugger can use the same
	// go-sdk source code for debugging.
	found := false
	if fi, err := os.Lstat("bazel-out"); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink("bazel-out"); err == nil {
				target = filepath.ToSlash(target)
				suffix := filepath.Join("execroot", "__main__", "bazel-out")
				if strings.HasSuffix(target, suffix) {
					gpfs.dirs.GoSDKDir = filepath.Join(target[:len(target)-len(suffix)], "external", "go_sdk")
					found = true
				}
			}
		}
	}
	if !found {
		fmt.Println("Could not find symbolic link \"bazel-out\", debugger will not find Go SDK source codes.")
	}

	return &gpfs
}
