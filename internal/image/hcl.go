package image

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/robgonnella/minienv/internal/errs"
)

type HclBuilder struct{}

func NewHclBuilder() *HclBuilder {
	return &HclBuilder{}
}

func (b *HclBuilder) Build(services []ServiceProperties) (string, error) {
	tplStr := b.template()

	tplFuncs := template.FuncMap{
		"joinServiceNamesQuoted": func(services []ServiceProperties) string {
			names := make([]string, 0, len(services))
			for _, s := range services {
				names = append(names, fmt.Sprintf("%q", s.Name))
			}

			return strings.Join(names, ", ")
		},
		"joinQuoted": func(elems []string) string {
			quoted := make([]string, 0, len(elems))
			for _, s := range elems {
				quoted = append(quoted, fmt.Sprintf("%q", s))
			}

			return strings.Join(quoted, ", ")
		},
		// Sorted so the rendered hcl is stable across runs; go map iteration
		// order is randomized.
		"joinMapQuoted": func(args map[string]string) string {
			pairs := make([]string, 0, len(args))
			for _, k := range slices.Sorted(maps.Keys(args)) {
				pairs = append(pairs, fmt.Sprintf("%q = %q", k, args[k]))
			}

			return strings.Join(pairs, "\n")
		},
	}

	tmpl, err := template.
		New("tmpl").
		Funcs(tplFuncs).
		Funcs(sprig.FuncMap()).
		Parse(tplStr)
	if err != nil {
		return "", errs.Errorf(
			ErrTemplateParse,
			"unable to parse buildx hcl template string: %w",
			err,
		)
	}

	var out strings.Builder

	err = tmpl.Execute(&out, map[string]any{"Services": services})
	if err != nil {
		return "", errs.Errorf(
			ErrTemplateExecute,
			"failed to execute buildx hcl template: %w",
			err,
		)
	}

	return out.String(), nil
}

func (b *HclBuilder) template() string {
	return `
group "default" {
  targets = [{{ .Services | joinServiceNamesQuoted }}]
}

{{- range .Services }}
target "{{ .Name }}" {
  context = "{{ .Context }}"
  dockerfile = "{{ .Dockerfile }}"
  tags = ["{{ .Registry }}:{{ .Tag }}"]
  platforms = [{{ .Platforms | joinQuoted }}]
{{- if .Args }}
  args = {
{{- .Args | joinMapQuoted | nindent 4 }}
  }
{{- end }}
}
{{- end }}

`
}
