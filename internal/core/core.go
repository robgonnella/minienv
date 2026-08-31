package core

import (
	"fmt"
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
	if err := c.validate(); err != nil {
		return err
	}
	for _, d := range c.deployers {
		if !d.Active() {
			continue
		}
		log.Info().Str("deployer", d.String()).Msg("executing deploy")
		if err := d.Init(c.project); err != nil {
			return err
		}
		if err := d.Deploy(c.project); err != nil {
			return err
		}
	}

	return nil
}

func (c *Core) Destroy() error {
	if err := c.validate(); err != nil {
		return err
	}
	for _, d := range c.deployers {
		if !d.Active() {
			continue
		}
		log.Info().Str("deployer", d.String()).Msg("executing destroy")
		if err := d.Init(c.project); err != nil {
			return err
		}
		if err := d.Destroy(c.project); err != nil {
			return err
		}
	}

	return nil
}

func (c *Core) validate() error {
	activeDeployers := []string{}
	for _, d := range c.deployers {
		if d.Active() {
			activeDeployers = append(activeDeployers, d.ConfigField())
		}
	}

	if len(activeDeployers) > 1 {
		return fmt.Errorf(
			"Detected multiple active configurations for deployment. "+
				"Only one of [%s] can be configured",
			strings.Join(activeDeployers, ", "),
		)
	}

	return nil
}
