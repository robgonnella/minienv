package command

import (
	"github.com/robgonnella/minienv/internal/core"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "minienv",
	Short: `Effortless remote mini environments generated directly from docker
compose config`,
	Long: `Minienv is a tool for quickly deploying and accessing mini environments
based on your existing docker-compose config. It allows you to effortlessly
deploy any docker-compose configuration to various remote environments, and
make those environments accessible for review and testing.`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		log.Fatal().Err(err).Msg("command failed")
	}
}

func init() {
	rootCmd.PersistentFlags().StringArrayP("file", "f", []string{}, "Compose configuration files. Same as \"docker compose -f <file>\"")
	rootCmd.PersistentFlags().String("project-directory", "", "Specify an alternate working directory. Same as \"docker compose --project-directory <dir>\"")
	rootCmd.PersistentFlags().StringP("project-name", "p", "", "Project name. Same as \"docker compose -p <name>\"")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Executes in dry-run mode")
}

func getComposeFlags(cmd *cobra.Command) (*core.ComposeFlags, error) {
	projectDirectory, err := cmd.Flags().GetString("project-directory")
	if err != nil {
		return nil, err
	}

	files, err := cmd.Flags().GetStringArray("file")
	if err != nil {
		return nil, err
	}

	projectName, err := cmd.Flags().GetString("project-name")
	if err != nil {
		return nil, err
	}

	return &core.ComposeFlags{
		Files:            files,
		ProjectDirectory: projectDirectory,
		ProjectName:      projectName,
	}, nil
}
