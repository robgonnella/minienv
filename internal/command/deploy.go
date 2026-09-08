// Package command is the composition root. It wires the CLI to the loader and
// is the only place that reads runtime environment values.
package command

import (
	"context"

	"github.com/spf13/cobra"
)

func newDeployCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "deploy",
		Aliases: []string{"up"},
		Short:   "Brings up your remote minienv according to compose configuration",
		Long: `Uses your docker compose configuration, including the minienv
extension fields, to deploy your minienv to the targeted remote environment
and make your minienv accessible for review and testing.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executeDeploy(cmd.Context(), cmd)
		},
	}
}

func executeDeploy(ctx context.Context, cmd *cobra.Command) error {
	minienv, err := loadProject(ctx, cmd)
	if err != nil {
		return err
	}

	return minienv.Deploy(ctx)
}
