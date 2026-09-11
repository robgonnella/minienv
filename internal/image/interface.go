package image

import (
	"context"

	"github.com/robgonnella/minienv/internal/config"
)

type ServiceProperties struct {
	Name       string
	Registry   string
	Tag        string
	Context    string
	Dockerfile string
	Platforms  []string
	Args       map[string]string
}

func NewServiceProperties(
	composeSvc config.ComposeService,
	extSvcImage config.ServiceImage,
) ServiceProperties {
	return ServiceProperties{
		Name:       composeSvc.Name,
		Registry:   extSvcImage.Repository,
		Tag:        extSvcImage.Tag,
		Context:    composeSvc.Build.Context,
		Dockerfile: composeSvc.Build.Dockerfile,
		Platforms:  extSvcImage.Platforms,
		Args:       composeSvc.Build.Args.ToMapping(),
	}
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
