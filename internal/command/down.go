package command

import (
	"github.com/robgonnella/minienv/internal/core"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Destroys your remote minienv according to compose configuration",
	Long: `Uses your docker compose configuration, including the the minienv
extension fields, to destroy your minienv in the targeted remote environment.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeDown(cmd)
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}

func executeDown(cmd *cobra.Command) error {
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

	return core.K8ServicesDown(project, ext, dryRun)
}
