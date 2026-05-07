package cmd

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var (
	cfgFile string
	stack   string
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "captain",
		Short:         "Deploy services to Kubernetes via Helm",
		Long:          "Captain orchestrates Docker builds and Helm deployments from a single YAML config.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "captain.yaml", "path to config file")
	cmd.PersistentFlags().StringVarP(&stack, "stack", "s", "", "target stack (e.g., local, production)")

	cmd.AddCommand(newSetupCmd())
	cmd.AddCommand(newDeployCmd())
	cmd.AddCommand(newDestroyCmd())
	cmd.AddCommand(newBuildCmd())
	cmd.AddCommand(newOutputsCmd())

	return cmd
}

func Execute() error {
	return NewRootCmd().Execute()
}
