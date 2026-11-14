package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/akamstar/durastate/internal/config"
	"github.com/spf13/cobra"
)

func newValidateCmd(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "validate [file]",
		Short: "Check a config file without applying it",
		Long: `Check a config file without applying it. Exits with code 2 if it is invalid.

Variables are interpolated with your environment. The daemon uses its own
environment when the config is applied, so results may differ.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := a.configPath(firstArg(args))
			cfg, err := config.Load(path, a.loadOpts)
			if asJSON {
				return writeValidateJSON(cmd.OutOrStdout(), path, cfg, err)
			}
			if err != nil {
				return reportConfigError(cmd.ErrOrStderr(), err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s is valid (%s)\n", path, plural(len(cfg.Services), "service"))
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

type validateResult struct {
	File     string        `json:"file"`
	Valid    bool          `json:"valid"`
	Services int           `json:"services,omitempty"`
	Errors   []configIssue `json:"errors,omitempty"`
}

type configIssue struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func writeValidateJSON(w io.Writer, path string, cfg *config.Config, loadErr error) error {
	res := validateResult{File: path, Valid: loadErr == nil}
	if cfg != nil {
		res.Services = len(cfg.Services)
	}
	var list config.ErrorList
	switch {
	case errors.As(loadErr, &list):
		for _, e := range list {
			res.Errors = append(res.Errors, configIssue{File: e.Pos.File, Line: e.Pos.Line, Column: e.Pos.Col, Path: e.Path, Message: e.Msg})
		}
	case loadErr != nil:
		res.Errors = []configIssue{{File: path, Message: loadErr.Error()}}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		return err
	}
	if loadErr != nil {
		return &exitError{code: ExitInvalidConfig, err: loadErr, silent: true}
	}
	return nil
}

// reportConfigError prints every config problem and returns a silent exit error.
func reportConfigError(w io.Writer, err error) error {
	fmt.Fprintln(w, err)
	n := 1
	var list config.ErrorList
	if errors.As(err, &list) {
		n = len(list)
	}
	fmt.Fprintf(w, "invalid configuration: %s\n", plural(n, "error"))
	return &exitError{code: ExitInvalidConfig, err: err, silent: true}
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}
