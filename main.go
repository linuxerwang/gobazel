package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
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
}
`
	bzlQuery = "kind(%s, deps(%s/...))"
)

var (
	debug = flag.Bool("debug", false, "Enable debug output.")
	build = flag.Bool("build", false, "Build all packages.")

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
			if err := runCommand(cfg, cfg.GoIdeCmd); err != nil {
				fmt.Println("Error to run IDE, ", err)
			}
		}
	}()

	server.Serve()
}

func bazelBuild(cfg *gopathfs.GobazelConf, dirs *gopathfs.Dirs) {
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
		cmd := [3]string{"bazel", "query", ""}
		for _, rule := range cfg.Build.Rules {
			cmd[2] = fmt.Sprintf(bzlQuery, rule, fi.Name())
			runBazelQuery(dirs.Workspace, fi.Name(), cmd[:], targets)
		}

		fmt.Println("done.")
	}

	// Execute bazel build.
	for target, _ := range targets {
		fmt.Printf("Build bazel target %.s", target)
		runBazelBuild(dirs.Workspace, target)
	}

	// First party projects.
	for _, proj := range projects {
		cmd := fmt.Sprintf("go install %s/%s/...", cfg.GoPkgPrefix, proj)
		fmt.Println(cmd)
		runCommand(cfg, cmd)
	}
}

func runBazelQuery(workspace, folder string, command []string, targets map[string]struct{}) {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//"+folder+"/") {
			targets[line] = struct{}{}
		}
	}
}

func runBazelBuild(workspace, target string) {
	cmd := exec.Command("bazel", "build", target)
	cmd.Dir = workspace
	cmd.Run()
}

func runCommand(cfg *gopathfs.GobazelConf, command string) error {
	parts := strings.Split(command, " ")
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Env = replaceGoPath(cfg)
	return cmd.Run()
}

func replaceGoPath(cfg *gopathfs.GobazelConf) []string {
	environ := []string{}
	env := os.Environ()
	for _, e := range env {
		if strings.HasPrefix(e, "GOPATH=") {
			e = fmt.Sprintf("GOPATH=%s", cfg.GoPath)
		}
		environ = append(environ, e)
	}
	return environ
}
