package image

import "context"

type ServiceProperties struct {
	Name       string
	Registry   string
	Tag        string
	Context    string
	Dockerfile string
	Platforms  []string
	Args       map[string]string
}

func (bs *ServiceProperties) LogFields() map[string]any {
	return map[string]any{
		"name":       bs.Name,
		"registry":   bs.Registry,
		"tag":        bs.Tag,
		"context":    bs.Context,
		"dockerfile": bs.Dockerfile,
		"platforms":  bs.Platforms,
	}
}

type Client interface {
	BuildAndPush(ctx context.Context, services []ServiceProperties) error
}
