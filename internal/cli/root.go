// Package cli defines the dstate command line interface.
package cli

import (
	"github.com/akamstar/durastate/internal/config"
	"github.com/akamstar/durastate/internal/paths"
	"github.com/spf13/cobra"
)

// BuildInfo describes the running binary. Values are set at build time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// app holds what commands depend on, so tests can replace it.
type app struct {
	info       BuildInfo
	paths      paths.Resolver
	loadOpts   config.Options
	configFlag string
}

// NewRootCmd returns the top-level dstate command.
func NewRootCmd(info BuildInfo) *cobra.Command {
	return newRootCmd(&app{info: info, paths: paths.Host()})
}

func newRootCmd(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:           "dstate",
		Short:         "Keep your processes running, the declarative way",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVarP(&a.configFlag, "config", "c", "",
		"config file (default: $"+paths.EnvConfig+", $"+paths.EnvHome+"/dstate.yaml or the OS location)")
	root.AddCommand(
		newVersionCmd(a.info),
		newValidateCmd(a),
		newConfigCmd(a),
		newInitCmd(),
	)
	return root
}

// configPath returns the config file to use: an explicit argument, then
// --config, then the environment and OS defaults.
func (a *app) configPath(arg string) string {
	switch {
	case arg != "":
		return arg
	case a.configFlag != "":
		return a.configFlag
	default:
		return a.paths.ConfigFile()
	}
}
