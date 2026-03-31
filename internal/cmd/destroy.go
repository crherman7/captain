package cmd

import (
	"fmt"
	"os"
	"slices"

	"github.com/christopherherman/captain/internal/config"
	"github.com/christopherherman/captain/internal/deploy"
	"github.com/spf13/cobra"
)

func includeService(svc config.ServiceConfig, activeStack string) bool {
	if svc.Disabled {
		return false
	}
	if activeStack == "" || len(svc.Stacks) == 0 {
		return true
	}
	for _, s := range svc.Stacks {
		if s == activeStack {
			return true
		}
	}
	return false
}

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
			ui := NewUI()
			defer ui.Flush()

			p := &Pipeline{Config: cfg, Cluster: cluster, Stack: stack}
			g, err := p.buildGraph()
			if err != nil {
				return err
			}

			sorted, err := g.Sort()
			if err != nil {
				return err
			}

			slices.Reverse(sorted)

			kubeCtx := ""
			if cluster != nil {
				kubeCtx = cluster.Context
			}
			deployer := deploy.NewHelmDeployer(kubeCtx)

			ui.Header("Destroy")

			var errs []error
			for _, name := range sorted {
				ui.ServiceStart(name, "uninstalling...")
				if err := deployer.Uninstall(cmd.Context(), name, cfg.Namespace); err != nil {
					ui.ServiceDone(name, "✗", "failed")
					ui.Error(err)
					errs = append(errs, fmt.Errorf("%s: %w", name, err))
					continue
				}
				ui.ServiceDone(name, "✔", "destroyed")
			}

			if len(errs) > 0 {
				return fmt.Errorf("failed to destroy %d service(s)", len(errs))
			}

			if err := os.Remove(stateFilePath()); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing state file: %w", err)
			}

			return nil
		},
	}
}
