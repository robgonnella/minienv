// Package core drives a deploy or destroy against whichever deployer the
// project activated.
package core

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/rs/zerolog/log"
)

// Padding between the service name and url columns of the published table.
const tablePadding = 3

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

func (c *Core) Deploy(ctx context.Context) error {
	log.Info().Str("deployer", c.deployer.String()).Msg("initializing")

	if err := c.deployer.Init(ctx, c.project); err != nil {
		return errs.Errorf(
			ErrDeployerInit,
			"failed to initialize deployer: %w",
			err,
		)
	}

	log.Info().Str("deployer", c.deployer.String()).Msg("executing deploy")

	if err := c.deployer.Deploy(ctx); err != nil {
		return errs.Errorf(ErrDeploy, "deploy failed: %w", err)
	}

	c.printPublishedUrls(ctx)

	return nil
}

func (c *Core) Destroy(ctx context.Context) error {
	log.Info().Str("deployer", c.deployer.String()).Msg("initializing")

	if err := c.deployer.Init(ctx, c.project); err != nil {
		return errs.Errorf(
			ErrDeployerInit,
			"failed to initialize deployer: %w",
			err,
		)
	}

	log.Info().Str("deployer", c.deployer.String()).Msg("executing destroy")

	if err := c.deployer.Destroy(ctx); err != nil {
		return errs.Errorf(ErrDestroy, "destroy failed: %w", err)
	}

	return nil
}

// A failed lookup only warns: the environment is already up by this point.
func (c *Core) printPublishedUrls(ctx context.Context) {
	serviceURLMap, err := c.deployer.PublishedServiceUrls(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get published service URLs")

		return
	}

	if len(serviceURLMap) == 0 {
		return
	}

	rows := []string{"Service Name\tURL", "--\t----"}

	for _, name := range slices.Sorted(maps.Keys(serviceURLMap)) {
		url := serviceURLMap[name]
		rows = append(rows, name+"\t"+url.String())
	}

	var table strings.Builder

	w := tabwriter.NewWriter(&table, 0, 0, tablePadding, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.Join(rows, "\n"))
	_ = w.Flush()

	_, _ = fmt.Fprint(
		os.Stdout,
		"\n====== Published Service URLs ======\n"+table.String(),
	)
}
