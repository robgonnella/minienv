package deployer

import (
	"net/url"

	"github.com/robgonnella/minienv/internal/config"
)

type Deployer interface {
	Active() bool
	String() string
	ConfigField() string
	Init(project *config.ComposeProject) error
	Deploy() error
	Destroy() error
	PublishedServiceUrls() (map[string]url.URL, error)
}
