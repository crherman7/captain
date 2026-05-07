package cmd

import (
	"github.com/crherman7/captain/internal/build"
	"github.com/crherman7/captain/internal/config"
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

			builder := build.NewBuildxBuilder(&build.ExecRunner{})
			p := NewPipeline(cfg, cluster, builder, nil, stack)

			actions, err := p.Plan()
			if err != nil {
				return err
			}

			ui := NewUI()
			defer ui.Flush()
			ui.Header("Build")

			var targets []build.Target
			for _, a := range actions {
				if a.HasBuild {
					targets = append(targets, a.BuildTarget)
				}
			}

			if len(targets) == 0 {
				ui.ServiceSkip("build", "no services have build configs")
				return nil
			}

			for _, t := range targets {
				ui.ServiceStart(t.Name, "building...")
			}

			builder.OnOutput = func(msg string) {
				target, step := build.ParseTargetMessage(msg)
				if target != "" {
					ui.ServiceUpdate(target, "building "+step)
				}
			}

			if err := builder.Bake(cmd.Context(), targets); err != nil {
				reportBuildFailures(ui, targets, err, "")
				ui.Error(err)
				return err
			}

			for _, t := range targets {
				ui.ServiceDone(t.Name, "✔", "built")
			}

			return nil
		},
	}
}
