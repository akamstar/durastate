package cli

import (
	"github.com/akamstar/durastate/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect the configuration",
	}
	cmd.AddCommand(newConfigShowCmd(a))
	return cmd
}

func newConfigShowCmd(a *app) *cobra.Command {
	var asJSON, showSecrets bool
	cmd := &cobra.Command{
		Use:   "show [file]",
		Short: "Print the effective configuration, with every default applied",
		Long: `Print the effective configuration, with every default applied and
variables interpolated. Secrets are masked unless --show-secrets is set.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.configPath(firstArg(args)), a.loadOpts)
			if err != nil {
				return reportConfigError(cmd.ErrOrStderr(), err)
			}
			var out []byte
			if asJSON {
				out, err = cfg.JSON(showSecrets)
				out = append(out, '\n')
			} else {
				out, err = cfg.YAML(showSecrets)
			}
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "print secret values instead of "+config.Masked)
	return cmd
}
