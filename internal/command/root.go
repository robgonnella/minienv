package command

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/robgonnella/minienv/internal/core"
	"github.com/robgonnella/minienv/internal/git"
	"github.com/robgonnella/minienv/internal/image"
	"github.com/robgonnella/minienv/internal/loader"
	"github.com/robgonnella/minienv/internal/publishing"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// Built per call rather than as a package-level var so the flag set starts
// clean: a shared command accumulates parsed values between invocations.
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "minienv",
		Short: `Effortless remote mini environments generated directly from docker
compose config`,
		Long: `Minienv is a tool for quickly deploying and accessing mini environments
based on your existing docker-compose config. It allows you to effortlessly
deploy any docker-compose configuration to various remote environments, and
make those environments accessible for review and testing.`,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			cmd.SilenceUsage = true
		},
	}

	flags := cmd.PersistentFlags()

	flags.StringArrayP("file", "f", []string{},
		`Compose configuration files. Same as "docker compose -f <file>"`)
	flags.String("project-directory", "",
		`Specify an alternate working directory. `+
			`Same as "docker compose --project-directory <dir>"`)
	flags.StringP("project-name", "p", "",
		`Project name. Same as "docker compose -p <name>"`)
	flags.Bool("dry-run", false, "Executes in dry-run mode")

	cmd.AddCommand(newDeployCmd(), newDestroyCmd(), newVersionCmd())

	return cmd
}

// Execute runs the CLI. Every layer below should take its context from the one
// created here, so cancellation has a single origin.
func Execute() int {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		log.Error().Err(err).Msg("command failed")
		return 1
	}

	return 0
}

func getLoaderOptions(cmd *cobra.Command) (*loader.LoaderOpts, error) {
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

	ngrokAPIKey := os.Getenv("NGROK_API_KEY")
	if ngrokAPIKey == "" {
		log.
			Warn().
			Msg("NGROK_API_KEY environment variable is not set. " +
				"Published service URLs will not be printed",
			)
	}

	imageClient := image.NewDocker(dryRun)
	gitClient := git.NewGitClient()
	publishClient := publishing.NewNgrokClient(ngrokAPIKey)

	// This is the composition root: the one place that reads runtime
	// environment values. Reading them here, lazily, keeps credentials out of
	// every package below — and out of their test binaries.
	return &loader.LoaderOpts{
		Files:            files,
		ProjectDirectory: projectDirectory,
		ProjectName:      projectName,
		DryRun:           dryRun,
		ImageClient:      imageClient,
		GitClient:        gitClient,
		PublishClient:    publishClient,
		NgrokAuthToken:   os.Getenv("NGROK_AUTHTOKEN"),
		HelmDriver:       os.Getenv("HELM_DRIVER"),
	}, nil
}

func loadProject(
	ctx context.Context,
	cmd *cobra.Command,
) (*core.Core, error) {
	loaderOpts, err := getLoaderOptions(cmd)
	if err != nil {
		return nil, err
	}

	return loader.New(loaderOpts).LoadCore(ctx)
}
