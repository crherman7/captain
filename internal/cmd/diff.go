package cmd

import (
	"fmt"
	"os"

	"github.com/christopherherman/captain/internal/build"
	"github.com/christopherherman/captain/internal/config"
	"github.com/christopherherman/captain/internal/state"
	"github.com/spf13/cobra"
)

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "Show what would change on deploy",
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

			p := NewPipeline(cfg, cluster, st, build.NewBuildxBuilder(&build.ExecRunner{}), nil, stack)

			actions, err := p.Plan()
			if err != nil {
				return err
			}

			var rows [][]string
			for _, a := range actions {
				rows = append(rows, []string{a.ServiceName, a.Status.String()})
			}

			fmt.Fprintln(os.Stderr) //nolint:errcheck
			printTable(os.Stdout, []string{"Service", "Status"}, rows)

			return nil
		},
	}
}
