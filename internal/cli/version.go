package cli

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

type versionOutput struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func newVersionCmd(info BuildInfo) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := versionOutput{
				Version: orDefault(info.Version, "dev"),
				Commit:  orDefault(info.Commit, "none"),
				Date:    orDefault(info.Date, "unknown"),
				Go:      runtime.Version(),
				OS:      runtime.GOOS,
				Arch:    runtime.GOARCH,
			}
			w := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			_, err := fmt.Fprintf(w, "dstate %s (commit %s, built %s, %s, %s/%s)\n",
				out.Version, out.Commit, out.Date, out.Go, out.OS, out.Arch)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
