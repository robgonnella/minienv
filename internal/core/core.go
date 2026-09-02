package core

import (
	"fmt"
	"slices"
	"strings"

	"github.com/robgonnella/minienv/internal/config"
	"github.com/rs/zerolog/log"
)

type Core struct {
	project   *config.ComposeProject
	ext       *config.XMiniEnv
	deployers []Deployer
	dryRun    bool
}

func New(
	ext *config.XMiniEnv,
	project *config.ComposeProject,
	dryRun bool,
) *Core {
	deployers := []Deployer{
		NewHelm(ext, dryRun),
	}

	return &Core{
		project,
		ext,
		deployers,
		dryRun,
	}
}

func (c *Core) Deploy() error {
	deployer, err := c.getActiveDeployer()
	if err != nil {
		return err
	}

	log.Info().Str("deployer", deployer.String()).Msg("initializing")
	if err := deployer.Init(c.project); err != nil {
		return err
	}

	log.Info().Str("deployer", deployer.String()).Msg("executing deploy")
	if err := deployer.Deploy(c.project); err != nil {
		return err
	}

	return nil
}

func (c *Core) Destroy() error {
	deployer, err := c.getActiveDeployer()
	if err != nil {
		return err
	}

	log.Info().Str("deployer", deployer.String()).Msg("initializing")
	if err := deployer.Init(c.project); err != nil {
		return err
	}

	log.Info().Str("deployer", deployer.String()).Msg("executing destroy")
	if err := deployer.Destroy(c.project); err != nil {
		return err
	}

	return nil
}

func (c *Core) getActiveDeployer() (Deployer, error) {
	var targetDeployer Deployer
	activeDeployers := []string{}
	for _, d := range c.deployers {
		if d.Active() {
			activeDeployers = append(activeDeployers, d.ConfigField())
			targetDeployer = d
		}
	}

	if len(activeDeployers) > 1 {
		return nil, fmt.Errorf(
			"Detected multiple active configurations for deployment. "+
				"Only one of [%s] can be configured",
			strings.Join(activeDeployers, ", "),
		)
	}

	if targetDeployer == nil {
		return nil, fmt.Errorf(
			"failed to find an active configuration for deployment. "+
				"Configure one of [%s] in x-minienv extension field.",
			slices.Collect(func(yield func(s string) bool) {
				for _, d := range c.deployers {
					if !yield(d.ConfigField()) {
						return
					}
				}
			}),
		)
	}

	return targetDeployer, nil
}
