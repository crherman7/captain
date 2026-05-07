package cmd

import (
	"github.com/crherman7/captain/internal/build"
	"github.com/crherman7/captain/internal/config"
	"github.com/crherman7/captain/internal/deploy"
	"github.com/crherman7/captain/internal/setup"
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
			ui := NewUI()
			defer ui.Flush()

			if len(cfg.Setup) > 0 {
				ui.Header("Setup")
				if err := runSetup(cmd, cfg, ui); err != nil {
					return err
				}
			}

			kubeCtx := ""
			if cluster != nil {
				kubeCtx = cluster.Context
			}

			builder := build.NewBuildxBuilder(&build.ExecRunner{})
			deployer := deploy.NewHelmDeployer(kubeCtx)

			p := NewPipeline(cfg, cluster, builder, deployer, stack)

			actions, err := p.Plan()
			if err != nil {
				return err
			}

			if err := p.Execute(cmd.Context(), actions, ui); err != nil {
				ui.Error(err)
				return err
			}

			return nil
		},
	}
}

func runSetup(cmd *cobra.Command, cfg *config.Config, ui UI) error {
	r := setup.NewRunner(stack)
	return r.Run(cmd.Context(), cfg.Setup,
		func(name string) { ui.ServiceSkip(name, "skipped") },
		func(name string) { ui.ServiceStart(name, "running...") },
		func(name string) { ui.ServiceDone(name, "✔", "done") },
	)
}
