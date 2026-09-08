package command

import (
	"context"

	"github.com/spf13/cobra"
)

func newDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "destroy",
		Aliases: []string{"down"},
		Short:   "Destroys your remote minienv according to compose configuration",
		Long: `Uses your docker compose configuration, including the minienv
extension fields, to destroy your minienv in the targeted remote environment.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executeDown(cmd.Context(), cmd)
		},
	}
}

func executeDown(ctx context.Context, cmd *cobra.Command) error {
	minienv, err := loadProject(ctx, cmd)
	if err != nil {
		return err
	}

	return minienv.Destroy(ctx)
}
