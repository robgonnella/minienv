// Package image builds and pushes the container images a deploy needs.
package image

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/robgonnella/minienv/internal/errs"
	"github.com/rs/zerolog/log"
)

type Docker struct {
	dryRun     bool
	hclBuilder *HclBuilder
}

func NewDocker(dryRun bool) *Docker {
	return &Docker{
		dryRun,
		NewHclBuilder(),
	}
}

func (d *Docker) BuildAndPush(
	ctx context.Context,
	services []ServiceProperties,
) error {
	filtered := d.getFilteredList(services)

	if len(filtered) == 0 {
		log.Warn().Msg("no services to build")

		return nil
	}

	hcl, err := d.hclBuilder.Build(filtered)
	if err != nil {
		return err
	}

	log.Debug().Msgf("building images: \n%s", hcl)

	// #nosec G204 -- argv comes from bakeArgs, which is a fixed list; the
	// project-supplied part of the build arrives on stdin as hcl.
	cmd := exec.CommandContext(ctx, "docker", d.bakeArgs()...)
	cmd.Stdin = strings.NewReader(hcl)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return errs.Errorf(ErrBuild, "failed to build or push images: %w", err)
	}

	return nil
}

// bakeArgs builds the argv for buildx. The hcl arrives on stdin, hence the
// "-f -". On a dry run --push is left off so bake plans the build without
// publishing anything.
func (d *Docker) bakeArgs() []string {
	args := []string{"buildx", "bake"}

	if !d.dryRun {
		args = append(args, "--push")
	}

	return append(args, "-f", "-")
}

func (d *Docker) getFilteredList(
	services []ServiceProperties,
) []ServiceProperties {
	return slices.Collect(func(yield func(service ServiceProperties) bool) {
		for _, s := range services {
			if s.Registry != "" &&
				s.Tag != "" &&
				s.Context != "" &&
				s.Dockerfile != "" {
				if !yield(s) {
					return
				}
			} else {
				log.
					Warn().
					Fields(s.LogFields()).
					Msg("missing required fields: skipping build and push")
			}
		}
	})
}
