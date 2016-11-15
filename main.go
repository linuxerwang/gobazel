package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"github.com/linuxerwang/gobazel/gopathfs"
)

const (
	initialConf = `gobazel {
    go-path: ""
    go-pkg-prefix: "test.com"
    go-ide-cmd: ""

    vendor-dirs: [
        "third-party-go/vendor",
    ]

    ignore-dirs: [
        "bazel-.*",
        "third-party.*",
    ]
}
`
)

var (
	debug = flag.Bool("debug", false, "Enable debug output.")

	dirs gopathfs.Dirs
)

func init() {
	dirs = gopathfs.Dirs{}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("Failed to get the current working directory,", err)
		os.Exit(2)
	}

	// The command has to be executed in a bazel workspace.
	dirs.Workspace = wd
	dirs.GobzlConf = filepath.Join(wd, ".gobazelrc")
}

func usage() {
	fmt.Println(`gobazel: A fuse mount tool for bazel to support Golang.

Usage:
	gobazel [options]

Note:
	This command has to be executed in a bazel workspace (where your WORKSPACE file reside).

Options:`)

	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	// The command has to be executed in a bazel workspace.
	if _, err := os.Stat(filepath.Join(dirs.Workspace, "WORKSPACE")); err != nil {
		fmt.Println("Error, the command has to be run in a bazel workspace,", err)
		os.Exit(2)
	}

	// File gobazel.cfg holds configurations for gobazel.
	if _, err := os.Stat(dirs.GobzlConf); err != nil {
		if os.IsNotExist(err) {
			if err = ioutil.WriteFile(dirs.GobzlConf, []byte(initialConf), 0644); err != nil {
				fmt.Printf("Failed to create file %s, %+v.\n", dirs.GobzlConf, err)
				os.Exit(2)
			}

			fmt.Printf("Created gobazel config file %s, please customize it and run the command again.\n", dirs.GobzlConf)
			os.Exit(0)
		} else {
			fmt.Println(err)
		}
	}

	cfg := gopathfs.LoadConfig(dirs.GobzlConf)
	if cfg.GoPath == "" {
		fmt.Println("Error, go-path has to be set in your .gobazelrc file.")
		os.Exit(2)
	}
	if cfg.GoPkgPrefix == "" {
		fmt.Println("Error, go-pkg-prefix has to be set in your .gobazelrc file.")
		os.Exit(2)
	}

	dirs.BinDir = filepath.Join(cfg.GoPath, "bin")
	os.Mkdir(dirs.BinDir, 0755)
	dirs.PkgDir = filepath.Join(cfg.GoPath, "pkg")
	os.Mkdir(dirs.PkgDir, 0755)
	dirs.SrcDir = filepath.Join(cfg.GoPath, "src")
	os.Mkdir(dirs.SrcDir, 0755)

	// Create a FUSE virtual file system on dirs.SrcDir.
	nfs := pathfs.NewPathNodeFs(gopathfs.NewGoPathFs(*debug, cfg, &dirs), nil)
	server, _, err := nodefs.MountRoot(dirs.SrcDir, nfs.Root(), nil)
	if err != nil {
		fmt.Printf("Mount fail: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("Mounted bazel source folder to %s. You need to set %s as your GOPATH. \n\n Ctrl+C to exit.\n", dirs.SrcDir, cfg.GoPath)

	// Handle ctl+c.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for {
			<-c
			fmt.Printf("\nUnmount %s.\n", dirs.SrcDir)
			if err := server.Unmount(); err != nil {
				fmt.Println("Error to unmount,", err)
				continue
			}
			os.Exit(0)
		}
	}()

	// If a Go IDE is specified, start it with the proper GOPATH.
	if cfg.GoIdeCmd != "" {
		go func() {
			time.Sleep(time.Second)
			cmd := exec.Command(cfg.GoIdeCmd)
			env := os.Environ()
			env = append(env, fmt.Sprintf("GOPATH=%s", cfg.GoPath))
			cmd.Env = env
			if err := cmd.Run(); err != nil {
				fmt.Println("Error to run IDE, ", err)
			}
		}()
	}

	server.Serve()
}
