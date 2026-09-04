package core

import (
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/rs/zerolog/log"
)

type Core struct {
	project  *config.ComposeProject
	ext      *config.XMiniEnv
	deployer deployer.Deployer
	dryRun   bool
}

func New(
	ext *config.XMiniEnv,
	project *config.ComposeProject,
	deployer deployer.Deployer,
	dryRun bool,
) *Core {
	return &Core{
		project,
		ext,
		deployer,
		dryRun,
	}
}

func (c *Core) Deploy() error {
	log.Info().Str("deployer", c.deployer.String()).Msg("initializing")
	if err := c.deployer.Init(c.project); err != nil {
		return errs.Errorf(KindDeployerInit, "failed to initialize deployer: %w", err)
	}

	log.Info().Str("deployer", c.deployer.String()).Msg("executing deploy")
	if err := c.deployer.Deploy(); err != nil {
		return errs.Errorf(KindDeploy, "deploy failed: %w", err)
	}

	return nil
}

func (c *Core) Destroy() error {
	log.Info().Str("deployer", c.deployer.String()).Msg("initializing")
	if err := c.deployer.Init(c.project); err != nil {
		return errs.Errorf(KindDeployerInit, "failed to initialize deployer: %w", err)
	}

	log.Info().Str("deployer", c.deployer.String()).Msg("executing destroy")
	if err := c.deployer.Destroy(); err != nil {
		return errs.Errorf(KindDestroy, "destroy failed: %w", err)
	}

	return nil
}
