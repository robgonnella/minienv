package command

import (
	"github.com/robgonnella/minienv/internal/core"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:     "deploy",
	Aliases: []string{"up"},
	Short:   "Brings up your remote minienv according to compose configuration",
	Long: `Uses your docker compose configuration, including the the minienv
extension fields, to deploy your minienv to the targeted remote environment
and make your minienv accessible for review and testing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeDeploy(cmd)
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)
}

func executeDeploy(cmd *cobra.Command) error {
	project, ext, err := loadProject(cmd)
	if err != nil {
		return err
	}

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return err
	}

	c := core.New(ext, project, dryRun)

	return c.Deploy()
}
