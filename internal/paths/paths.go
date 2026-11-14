// Package paths returns the default locations used by dstate on each OS.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

// ConfigFileName is the name of the config file in a dstate home directory.
const ConfigFileName = "dstate.yaml"

// Env variables that override the default locations.
const (
	EnvConfig = "DSTATE_CONFIG" // config file path
	EnvHome   = "DSTATE_HOME"   // directory holding dstate.yaml
)

// Resolver computes paths from an environment and a target OS, so that
// every OS can be tested on any machine.
type Resolver struct {
	Getenv func(string) string
	GOOS   string
}

// Host returns a Resolver for the running machine.
func Host() Resolver {
	return Resolver{Getenv: os.Getenv, GOOS: runtime.GOOS}
}

// ConfigFile returns the config file path, in order of precedence:
// DSTATE_CONFIG, DSTATE_HOME/dstate.yaml, then the OS default.
func (r Resolver) ConfigFile() string {
	if p := r.Getenv(EnvConfig); p != "" {
		return p
	}
	return r.join(r.Home(), ConfigFileName)
}

// Home returns the dstate home directory: DSTATE_HOME or the OS default.
func (r Resolver) Home() string {
	if h := r.Getenv(EnvHome); h != "" {
		return h
	}
	switch r.GOOS {
	case "windows":
		base := r.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return base + `\dstate`
	case "darwin":
		return "/Library/Application Support/dstate"
	default:
		return "/etc/dstate"
	}
}

// join uses the separator of the target OS, not of the host.
func (r Resolver) join(dir, name string) string {
	if r.GOOS == runtime.GOOS {
		return filepath.Join(dir, name)
	}
	if r.GOOS == "windows" {
		return dir + `\` + name
	}
	return dir + "/" + name
}
