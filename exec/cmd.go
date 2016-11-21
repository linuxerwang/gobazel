package exec

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/linuxerwang/gobazel/conf"
)

// RunGoInstall executes the "go install" command on the given Go package.
func RunGoInstall(cfg *conf.GobazelConf, goPkg string) {
	cmd := fmt.Sprintf("go install %s", goPkg)
	fmt.Print(cmd)
	if err := RunCommand(cfg, cmd); err != nil {
		fmt.Println(" (failed!)")
	} else {
		fmt.Println(" (done)")
	}
}

// RunGoWalkInstall walks the given proj directory and run "go install"
// for each go package.
func RunGoWalkInstall(cfg *conf.GobazelConf, workspace, proj string) {
	filepath.Walk(filepath.Join(workspace, proj), func(path string, info os.FileInfo, err error) error {
		if info.Name() == "BUILD" {
			if dir, err := filepath.Rel(workspace, path); err == nil {
				dir = filepath.Dir(dir)
				for _, v := range cfg.Vendors {
					if strings.HasPrefix(dir, v) {
						// Ignore third party Go vendor directories.
						return nil
					}
				}

				RunGoInstall(cfg, filepath.Join(cfg.GoPkgPrefix, dir))
			}
		}
		return nil
	})
}

// RunBazelQuery executes "bazel query" for the given folder and returns
// all bazel build targets under this sub tree.
func RunBazelQuery(workspace, folder string, command []string, targets map[string]struct{}) {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = workspace
	out, _ := cmd.Output()

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

// RunBazelBuild executes "bazel build" for the given bazel build target.
func RunBazelBuild(workspace, target string) {
	cmd := exec.Command("bazel", "build", target)
	cmd.Dir = workspace

	fmt.Printf("bazel build %s", target)
	if err := cmd.Run(); err != nil {
		fmt.Println(" (failed!)")
	} else {
		fmt.Println(" (done)")
	}
}

// RunCommand executes the given command.
func RunCommand(cfg *conf.GobazelConf, command string) error {
	parts := strings.Split(command, " ")
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Env = replaceGoPath(cfg)
	return cmd.Run()
}

func replaceGoPath(cfg *conf.GobazelConf) []string {
	environ := []string{fmt.Sprintf("GOPATH=%s", cfg.GoPath)}
	env := os.Environ()
	for _, e := range env {
		if strings.HasPrefix(e, "GOPATH=") {
			continue
		}
		environ = append(environ, e)
	}
	return environ
}
