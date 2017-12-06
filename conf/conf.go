package conf

import (
	"fmt"
	"os"

	"github.com/linuxerwang/confish"
)

// GobazelConf represents the bazel build config.
type BuildConf struct {
	Rules   []string `cfg-attr:"rules"`
	Ignores []string `cfg-attr:"ignore-dirs"`
}

// GobazelConf represents the gobazel global config.
type GobazelConf struct {
	GoPath      string     `cfg-attr:"go-path"`
	GoPkgPrefix string     `cfg-attr:"go-pkg-prefix"`
	GoIdeCmd    string     `cfg-attr:"go-ide-cmd"`
	Ignores     []string   `cfg-attr:"ignore-dirs"`
	Vendors     []string   `cfg-attr:"vendor-dirs"`
	FallThrough []string   `cfg-attr:"fall-through-dirs"`
	Build       *BuildConf `cfg-attr:"build"`
}

type confWrapper struct {
	Conf *GobazelConf `cfg-attr:"gobazel"`
}

// LoadConfig loads gobazel config from the given file.
func LoadConfig(cfgPath string) *GobazelConf {
	cfg := confWrapper{}
	if err := confish.ParseFile(cfgPath, &cfg); err != nil {
		fmt.Printf("Failed to parse gobazel config file %s, %+v.\n", cfgPath, err)
		os.Exit(2)
	}
	return cfg.Conf
}
