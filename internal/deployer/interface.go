// Package deployer is the contract every deployment target implements.
package deployer

import (
	"context"
	"net/url"

	"github.com/robgonnella/minienv/internal/config"
)

type Deployer interface {
	Active() bool
	String() string
	ConfigField() string
	Init(ctx context.Context, project *config.ComposeProject) error
	Deploy(ctx context.Context) error
	Destroy(ctx context.Context) error
	PublishedServiceUrls(ctx context.Context) (map[string]url.URL, error)
}
