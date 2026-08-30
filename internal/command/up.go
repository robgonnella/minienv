package command

import (
	"github.com/robgonnella/minienv/internal/core"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Brings up your remote minienv according to compose configuration",
	Long: `Uses your docker compose configuration, including the the minienv
extension fields, to deploy your minienv to the targeted remote environment
and make your minienv accessible for review and testing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeUp(cmd)
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}

func executeUp(cmd *cobra.Command) error {
	flags, err := getComposeFlags(cmd)
	if err != nil {
		return err
	}

	project, err := core.LoadComposeProject(flags)
	if err != nil {
		return err
	}

	ext, err := core.LoadMainExtensionConfig(project)
	if err != nil {
		return err
	}

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return err
	}

	return core.K8sServicesUp(project, ext, dryRun)
}
