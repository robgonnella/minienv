package command

import (
	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:     "destroy",
	Aliases: []string{"down"},
	Short:   "Destroys your remote minienv according to compose configuration",
	Long: `Uses your docker compose configuration, including the the minienv
extension fields, to destroy your minienv in the targeted remote environment.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeDown(cmd)
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
}

func executeDown(cmd *cobra.Command) error {
	ctx, err := loadProject(cmd)
	if err != nil {
		return err
	}
	return ctx.Core.Destroy()
}
