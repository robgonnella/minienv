package deployer

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"text/template"

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

	hclString, err := b.hcl(filtered)
	if err != nil {
		return err
	}

	log.Debug().Msgf("building images: \n%s", hclString)

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

func (b *Buildx) hcl(services []BakeService) (string, error) {
	tplStr := b.template()

	tplFuncs := template.FuncMap{
		"joinServiceNamesQuoted": func(services []BakeService) string {
			names := []string{}
			for _, s := range services {
				names = append(names, fmt.Sprintf("%q", s.Name))
			}
			return strings.Join(names, ", ")
		},
		"joinQuoted": func(elems []string) string {
			for i, s := range elems {
				elems[i] = fmt.Sprintf("%q", s)
			}
			return strings.Join(elems, ", ")
		},
	}

	tmpl, err := template.New("tmpl").Funcs(tplFuncs).Parse(tplStr)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	err = tmpl.Execute(&out, map[string]any{"Services": services})
	if err != nil {
		return "", err
	}

	return out.String(), nil
}

func (b *Buildx) template() string {
	return `
group "default" {
	targets = [{{ .Services | joinServiceNamesQuoted }}]
}

{{ range .Services -}}
target "{{ .Name }}" {
	context = "{{ .Context }}"
	dockerfile = "{{ .Dockerfile }}"
	tags = ["{{ .Registry }}:{{ .Tag }}"]
	platforms = [{{ .Platforms | joinQuoted }}]
}
{{- end }}
`
}
