package core

import (
	"context"
	"fmt"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/go-viper/mapstructure/v2"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/rs/zerolog/log"
)

type ComposeFlags struct {
	Files            []string
	ProjectDirectory string
	ProjectName      string
}

func loadComposeProject(flags *ComposeFlags) (*types.Project, error) {
	// Same option set the compose CLI builds in its own toProjectOptions, so
	// project name, .env loading, COMPOSE_FILE and default config-file discovery
	// all resolve exactly the way `docker compose` resolves them.
	projectOpts, err := composecli.NewProjectOptions(
		flags.Files,
		composecli.WithWorkingDirectory(flags.ProjectDirectory),
		composecli.WithOsEnv,
		composecli.WithDotEnv,
		composecli.WithConfigFileEnv,
		composecli.WithDefaultConfigPath,
		composecli.WithName(flags.ProjectName),
	)
	if err != nil {
		return nil, err
	}

	project, err := projectOpts.LoadProject(context.Background())
	if err != nil {
		return nil, err
	}

	return project, nil
}

func loadMainExtensionConfig(
	project *types.Project,
) (*config.XMiniEnv, error) {
	ex, ok := project.Extensions[config.TOP_LEVEL_EXTENSION]
	if !ok {
		return nil, fmt.Errorf(
			"no minienv extension config found in docker compose configs",
		)
	}

	var extConfig config.XMiniEnv
	if err := mapstructure.Decode(ex, &extConfig); err != nil {
		return nil, fmt.Errorf(
			"failed to parse x-minienv top-level extension: %s",
			err,
		)
	}

	log.
		Info().
		Interface(config.TOP_LEVEL_EXTENSION, ex).
		Msg("loaded top-level extension")

	return &extConfig, nil
}
