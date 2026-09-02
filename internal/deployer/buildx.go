package deployer

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
)

type BakeService struct {
	Name       string
	Registry   string
	Tag        string
	Context    string
	Dockerfile string
	Platforms  []string
	Args       map[string]string
}

func (bs *BakeService) LogFields() map[string]any {
	return map[string]any{
		"name":       bs.Name,
		"registry":   bs.Registry,
		"tag":        bs.Tag,
		"context":    bs.Context,
		"dockerfile": bs.Dockerfile,
		"platforms":  bs.Platforms,
	}
}

type Buildx struct{ dryRun bool }

func NewBuildx(dryRun bool) *Buildx {
	return &Buildx{dryRun}
}

func (b *Buildx) BuildAndPush(services []BakeService) error {
	filtered := b.getFilteredList(services)
	hclString := b.hcl(filtered)
	args := []string{"buildx", "bake"}
	if !b.dryRun {
		args = append(args, "--push")
	}
	args = append(args, "-f", "-")
	cmd := exec.Command("docker", args...)
	cmd.Stdin = strings.NewReader(hclString)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (b *Buildx) getFilteredList(services []BakeService) []BakeService {
	return slices.Collect(func(yield func(service BakeService) bool) {
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

func (b *Buildx) hcl(services []BakeService) string {
	groupTargets := "["
	serviceTargets := []string{}

	for i, s := range services {
		log.Info().Fields(s.LogFields()).Msg("baking service")

		comma := ","
		if i == len(services)-1 {
			comma = ""
		}

		groupTargets = fmt.Sprintf("%s\"%s\"%s", groupTargets, s.Name, comma)

		platforms := "["
		for i, p := range s.Platforms {
			platformComma := ","
			if i == len(s.Platforms)-1 {
				platformComma = ""
			}
			platforms = fmt.Sprintf("%s\"%s\"%s", platforms, p, platformComma)
		}
		platforms += "]"

		tag := strings.TrimSpace(fmt.Sprintf("%s:%s", s.Registry, s.Tag))

		serviceTarget := fmt.Sprintf(`
target "%s" {
  context = "%s"
  dockerfile = "%s"
  tags = ["%s"]
  platforms = %s`, s.Name, s.Context, s.Dockerfile, tag, platforms)

		if len(s.Args) > 0 {
			serviceTarget += "\n  args = {"
			for k, v := range s.Args {
				serviceTarget = fmt.Sprintf("%s\n    %s = %s", serviceTarget, k, v)
			}
			serviceTarget += "\n  }"
		}

		serviceTarget += "\n}"
		serviceTargets = append(serviceTargets, serviceTarget)
	}

	groupTargets += "]"

	group := fmt.Sprintf(`
group "default" {
  targets = %s
}
`, groupTargets)

	return fmt.Sprintf("%s%s", group, strings.Join(serviceTargets, "\n\n"))
}
