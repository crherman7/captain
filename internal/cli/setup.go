package cli

import (
	"fmt"
	"os"

	"github.com/christopherherman/captain/internal/config"
	"github.com/spf13/cobra"
)

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Run setup steps only (without deploying)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			if len(cfg.Setup) == 0 {
				printHeader("No setup steps defined")
				return nil
			}

			spin := NewSpinner(os.Stderr)
			printHeader("Setup")
			if err := runSetup(cmd, cfg, spin); err != nil {
				printError(err)
				return err
			}

			fmt.Fprintln(os.Stderr) //nolint:errcheck
			return nil
		},
	}
}
