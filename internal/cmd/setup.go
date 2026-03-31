package cmd

import (
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
				return nil
			}

			ui := NewUI()
			defer ui.Flush()
			ui.Header("Setup")
			if err := runSetup(cmd, cfg, ui); err != nil {
				ui.Error(err)
				return err
			}

			return nil
		},
	}
}
