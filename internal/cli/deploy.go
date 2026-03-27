package cli

import (
	"fmt"
	"os"

	"github.com/christopherherman/captain/internal/build"
	"github.com/christopherherman/captain/internal/config"
	"github.com/christopherherman/captain/internal/deploy"
	"github.com/christopherherman/captain/internal/setup"
	"github.com/christopherherman/captain/internal/state"
	"github.com/spf13/cobra"
)

func newDeployCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deploy",
		Short: "Build and deploy all services",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			cluster := cfg.GetCluster(stack)
			spin := NewSpinner(os.Stderr)

			// Run setup steps before anything else
			if len(cfg.Setup) > 0 {
				printHeader("Setup")
				if err := runSetup(cmd, cfg, spin); err != nil {
					return err
				}
			}

			st, err := state.Load(stateFilePath())
			if err != nil {
				return err
			}

			kubeCtx := ""
			if cluster != nil {
				kubeCtx = cluster.Context
			}

			builder := build.NewBuildxBuilder(&build.ExecRunner{})
			deployer := deploy.NewHelmDeployer(kubeCtx)

			p := NewPipeline(cfg, cluster, st, builder, deployer, stack)

			actions, err := p.Plan()
			if err != nil {
				return err
			}

			changed := 0
			for _, a := range actions {
				if a.Status != StatusUnchanged {
					changed++
				}
			}

			if changed == 0 {
				printHeader("Deploy")
				for _, a := range actions {
					spin.Skip(a.ServiceName, "unchanged")
				}
				return nil
			}

			printHeader("Deploy")
			if err := p.Execute(cmd.Context(), actions, spin); err != nil {
				printError(err)
				return err
			}

			if err := st.Save(stateFilePath()); err != nil {
				return fmt.Errorf("saving state: %w", err)
			}

			fmt.Fprintln(os.Stderr) //nolint:errcheck
			return nil
		},
	}
}

func runSetup(cmd *cobra.Command, cfg *config.Config, spin *Spinner) error {
	r := setup.NewRunner(stack)
	return r.Run(cmd.Context(), cfg.Setup,
		func(name string) { spin.Skip(name, "ready") },
		func(name string) { spin.Start(name, "running...") },
		func(name string) { spin.Stop("✔", "done") },
	)
}

// stateFilePath returns the state file path, scoped by stack if set.
func stateFilePath() string {
	if stack != "" {
		return fmt.Sprintf(".captain-state.%s.json", stack)
	}
	return stateFile
}
