package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	osexec "os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"github.com/linuxerwang/gobazel/conf"
	"github.com/linuxerwang/gobazel/exec"
	"github.com/linuxerwang/gobazel/gopathfs"
)

const (
	initialConf = `gobazel {
    go-path: ""
    go-pkg-prefix: "test.com"
    # go-ide-cmd: "/usr/bin/atom"
    go-ide-cmd: "/usr/bin/code"
    # go-ide-cmd: "/usr/bin/liteide"

    build {
        rules: [
        ]
        ignore-dirs: [
            "bazel-.*",
            "third-party.*",
        ]
    }

    vendor-dirs: [
        "third-party-go/vendor",
    ]

    ignore-dirs: [
        "bazel-.*",
        "third-party.*",
    ]

    fall-through-dirs: [
        ".vscode",
    ]
}
`
	bzlQuery = "kind(%s, deps(%s/...))"
)

var (
	debug    = flag.Bool("debug", false, "Enable debug output.")
	build    = flag.Bool("build", false, "Build all packages.")
	daemon   = flag.Bool("daemon", true, "To detach from parent process.")
	detached = flag.Bool("detached", false, "The current process has been detached from parent process. Do not set it manually, it's only used by gobazel to detach itself.")

	dirs    gopathfs.Dirs
	version string
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
	OR to show its version:
	gobazel version

Note:
	This command has to be executed in a bazel workspace (where your WORKSPACE file reside).

Options:`)

	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	for _, arg := range flag.Args() {
		if strings.ToLower(arg) == "version" {
			fmt.Println("Version:", version)
			return
		}
	}

	if *daemon && !*detached {
		pid, err := detach()
		if err != nil {
			fmt.Println(err)
			os.Exit(2)
		}
		fmt.Printf("gobazel is running detached. To stop it, run \"kill -SIGQUIT %d\".\n", pid)
		return
	}

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

	cfg := conf.LoadConfig(dirs.GobzlConf)
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
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGABRT, syscall.SIGQUIT, syscall.SIGINT)
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

	go func() {
		time.Sleep(time.Second)

		// If set to build all packages.
		if *build {
			fmt.Println("\nBuilding all package, it may take seconds to a few minutes, depending on how many pakcages you have ...")

			if cfg.Build == nil {
				fmt.Println("No build config found in .gobazelrc, ignored.")
				return
			}

			// Best effort run, errors are ignored.
			bazelBuild(cfg, &dirs)
		}

		// If a Go IDE is specified, start it with the proper GOPATH.
		if cfg.GoIdeCmd != "" {
			fmt.Println("\nStarting IDE ...")
			if err := exec.RunCommand(cfg, cfg.GoIdeCmd+" "+dirs.SrcDir); err != nil {
				fmt.Println("Error to run IDE, ", err)
			}
		}
	}()

	server.Serve()
}

func bazelBuild(cfg *conf.GobazelConf, dirs *gopathfs.Dirs) {
	ignoreRegexes := make([]*regexp.Regexp, len(cfg.Build.Ignores))
	for i, ign := range cfg.Build.Ignores {
		ignoreRegexes[i] = regexp.MustCompile(ign)
	}

	f, err := os.Open(dirs.Workspace)
	if err != nil {
		fmt.Println("Failed to read workspace,", err)
		os.Exit(2)
	}
	defer f.Close()

	fis, err := f.Readdir(-1)
	if err != nil {
		fmt.Println("Failed to read workspace,", err)
		os.Exit(2)
	}

	targets := map[string]struct{}{}
	projects := []string{}

outterLoop:
	for _, fi := range fis {
		fmt.Printf("Folder %s ... ", fi.Name())
		if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			fmt.Println("ignored.")
			continue
		}

		for _, re := range ignoreRegexes {
			if re.MatchString(fi.Name()) {
				fmt.Println("ignored.")
				continue outterLoop
			}
		}

		projects = append(projects, fi.Name())

		// Check if there are given bazel build targets in this directory.
		cmd := [4]string{"bazel", "query", "--keep_going", ""}
		for _, rule := range cfg.Build.Rules {
			cmd[3] = fmt.Sprintf(bzlQuery, rule, fi.Name())
			exec.RunBazelQuery(dirs.Workspace, fi.Name(), cmd[:], targets)
		}

		fmt.Println("done.")
	}

	// Execute bazel build.
	for target := range targets {
		exec.RunBazelBuild(dirs.Workspace, target)
	}

	// Run go install for all first party projects.
	for _, proj := range projects {
		exec.RunGoWalkInstall(cfg, dirs.Workspace, proj)
	}
}

func detach() (int, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	args := append(os.Args, "--detached")
	cmd := osexec.Command(args[0], args[1:]...)
	cmd.Dir = cwd
	err = cmd.Start()
	if err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	cmd.Process.Release()
	return pid, nil
}
