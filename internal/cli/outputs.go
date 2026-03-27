package cli

import (
	"os"

	"github.com/christopherherman/captain/internal/config"
	"github.com/christopherherman/captain/internal/resolver"
	"github.com/spf13/cobra"
)

func newOutputsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "outputs",
		Short: "Show resolved service outputs (exposes)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			r := resolver.New(os.LookupEnv)

			printHeader("Service Outputs")

			for name, svc := range cfg.Services {
				if len(svc.Exposes) == 0 {
					continue
				}

				ctx := resolver.ResolveContext{
					ServiceName: name,
					Namespace:   cfg.Namespace,
				}

				resolved, err := r.ResolveStringMap(svc.Exposes, ctx)
				if err != nil {
					printError(err)
					continue
				}

				for key, val := range resolved {
					printOutput(name, key, val)
				}
			}

			return nil
		},
	}
}
