package cli

import (
	"fmt"
	"os"

	"github.com/christopherherman/captain/internal/build"
	"github.com/christopherherman/captain/internal/config"
	"github.com/christopherherman/captain/internal/state"
	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Build Docker images for services with build configs",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			cluster := cfg.GetCluster(stack)
			st, err := state.Load(stateFilePath())
			if err != nil {
				return err
			}

			builder := build.NewBuildxBuilder(&build.ExecRunner{})
			p := NewPipeline(cfg, cluster, st, builder, nil, stack)

			actions, err := p.Plan()
			if err != nil {
				return err
			}

			spin := NewSpinner(os.Stderr)
			printHeader("Build")

			built := 0
			for _, a := range actions {
				if !a.HasBuild {
					continue
				}
				spin.Start(a.ServiceName, "building...")
				if err := builder.Build(cmd.Context(), a.BuildTarget); err != nil {
					spin.Stop("✗", "build failed")
					printError(err)
					return err
				}
				spin.Stop("✔", "built")
				built++
			}

			if built == 0 {
				fmt.Fprintln(os.Stderr, "  No services have build configs") //nolint:errcheck
			}

			fmt.Fprintln(os.Stderr) //nolint:errcheck
			return nil
		},
	}
}
