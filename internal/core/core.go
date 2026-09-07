package core

import (
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

	c.printPublishedUrls()

	return nil
}

// A failed lookup only warns: the environment is already up by this point.
func (c *Core) printPublishedUrls() {
	serviceUrlMap, err := c.deployer.PublishedServiceUrls()
	if err != nil {
		log.Warn().Err(err).Msg("failed to get published service URLs")
		return
	}

	if len(serviceUrlMap) == 0 {
		return
	}

	rows := []string{"Service Name\tURL", "--\t----"}

	for _, name := range slices.Sorted(maps.Keys(serviceUrlMap)) {
		url := serviceUrlMap[name]
		rows = append(rows, name+"\t"+url.String())
	}

	var table strings.Builder
	w := tabwriter.NewWriter(&table, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.Join(rows, "\n"))
	_ = w.Flush()

	_, _ = fmt.Fprint(
		os.Stdout,
		"\n====== Published Service URLs ======\n"+table.String(),
	)
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
