package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	//go:embed templates/dstate.yaml
	initConfig []byte
	//go:embed templates/env.example
	initEnvExample []byte
)

func newInitCmd() *cobra.Command {
	var dir string
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a commented dstate.yaml and a .env.example",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			files := []struct {
				name string
				data []byte
			}{
				{"dstate.yaml", initConfig},
				{".env.example", initEnvExample},
			}
			if !force {
				for _, f := range files {
					p := filepath.Join(dir, f.name)
					if _, err := os.Stat(p); err == nil {
						return fmt.Errorf("%s already exists (use --force to overwrite)", p)
					} else if !errors.Is(err, fs.ErrNotExist) {
						return err
					}
				}
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			for _, f := range files {
				p := filepath.Join(dir, f.name)
				if err := os.WriteFile(p, f.data, 0o644); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "created", p)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "next: edit dstate.yaml, then run: dstate validate", filepath.Join(dir, "dstate.yaml"))
			return nil
		},
	}
	cmd.Flags().StringVarP(&dir, "dir", "d", ".", "directory to create the files in")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite existing files")
	return cmd
}
