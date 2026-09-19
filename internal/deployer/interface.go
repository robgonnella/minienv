// Package deployer is the contract every deployment target implements.
package deployer

import (
	"context"
	"net/url"
)

type Deployer interface {
	String() string
	Init(ctx context.Context) error
	Deploy(ctx context.Context) error
	Destroy(ctx context.Context) error
	PublishedServiceUrls(ctx context.Context) (map[string]url.URL, error)
}
