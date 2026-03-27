package cli

import (
	"fmt"
	"os"
	"slices"

	"github.com/christopherherman/captain/internal/config"
	"github.com/christopherherman/captain/internal/deploy"
	"github.com/christopherherman/captain/internal/graph"
	"github.com/spf13/cobra"
)

func newDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Short: "Tear down all deployed services",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			cluster := cfg.GetCluster(stack)

			g := graph.New()
			for name, svc := range cfg.Services {
				if svc.Disabled {
					continue
				}
				g.AddNode(name)
			}
			for name, svc := range cfg.Services {
				if svc.Disabled {
					continue
				}
				for _, ref := range svc.References {
					if err := g.AddEdge(name, ref); err != nil {
						return fmt.Errorf("service %q: %w", name, err)
					}
				}
			}

			sorted, err := g.Sort()
			if err != nil {
				return err
			}

			// Reverse: destroy dependents before their dependencies
			slices.Reverse(sorted)

			kubeCtx := ""
			if cluster != nil {
				kubeCtx = cluster.Context
			}
			deployer := deploy.NewHelmDeployer(kubeCtx)
			spin := NewSpinner(os.Stderr)

			printHeader("Destroy")

			for _, name := range sorted {
				spin.Start(name, "uninstalling...")
				if err := deployer.Uninstall(cmd.Context(), name, cfg.Namespace); err != nil {
					spin.Stop("✗", "failed")
					printError(err)
					continue
				}
				spin.Stop("✔", "destroyed")
			}

			if err := os.Remove(stateFilePath()); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing state file: %w", err)
			}

			fmt.Fprintln(os.Stderr) //nolint:errcheck
			return nil
		},
	}
}
