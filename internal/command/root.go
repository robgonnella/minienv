package command

import (
	"github.com/robgonnella/minienv/internal/core"
	"github.com/robgonnella/minienv/internal/loader"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type GlobalCliFlags struct {
	Files            []string
	ProjectDirectory string
	ProjectName      string
	DryRun           bool
}

type RunContext struct {
	Core *core.Core
}

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

func getGlobalCliFlags(cmd *cobra.Command) (*GlobalCliFlags, error) {
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

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return nil, err
	}

	return &GlobalCliFlags{
		Files:            files,
		ProjectDirectory: projectDirectory,
		ProjectName:      projectName,
		DryRun:           dryRun,
	}, nil
}

func loadProject(
	cmd *cobra.Command,
) (*RunContext, error) {
	flags, err := getGlobalCliFlags(cmd)
	if err != nil {
		return nil, err
	}

	loaderOpts := loader.LoaderOpts(*flags)
	projectLoader := loader.New(&loaderOpts)

	core, err := projectLoader.LoadCore()
	if err != nil {
		return nil, err
	}

	return &RunContext{Core: core}, nil
}
