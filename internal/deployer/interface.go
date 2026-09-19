// Package deployer is the contract every deployment target implements.
package deployer

import (
	"context"
	"net/url"

	"github.com/robgonnella/minienv/internal/config"
)

type ServiceDirs map[string]string

func (s ServiceDirs) Dir(name string) string {
	return s[name]
}

type HostEnv map[string][]string

func (h HostEnv) Keys(name string) []string {
	return h[name]
}

type Deployer interface {
	String() string
	Init(ctx context.Context, project config.ComposeProject) error
	Deploy(ctx context.Context) error
	Destroy(ctx context.Context) error
	PublishedServiceUrls(ctx context.Context) (map[string]url.URL, error)
}
