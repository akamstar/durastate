// Package cli defines the dstate command line interface.
package cli

import (
	"github.com/spf13/cobra"
)

// BuildInfo describes the running binary. Values are set at build time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// NewRootCmd returns the top-level dstate command.
func NewRootCmd(info BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:           "dstate",
		Short:         "Keep your processes running, the declarative way",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVersionCmd(info))
	return root
}
