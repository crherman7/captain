package cli

import (
	"github.com/spf13/cobra"
)

var (
	cfgFile   string
	stack     string
	stateFile string
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "captain",
		Short: "Deploy services to Kubernetes via Helm",
		Long:  "Captain orchestrates Docker builds and Helm deployments from a single YAML config.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "captain.yaml", "path to config file")
	cmd.PersistentFlags().StringVarP(&stack, "stack", "s", "", "target stack (e.g., local, production)")
	cmd.PersistentFlags().StringVar(&stateFile, "state", ".captain-state.json", "path to state file")

	cmd.AddCommand(newSetupCmd())
	cmd.AddCommand(newDeployCmd())
	cmd.AddCommand(newDestroyCmd())
	cmd.AddCommand(newBuildCmd())
	cmd.AddCommand(newDiffCmd())
	cmd.AddCommand(newOutputsCmd())

	return cmd
}

func Execute() error {
	return NewRootCmd().Execute()
}
