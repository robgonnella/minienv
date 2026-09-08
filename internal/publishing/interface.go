package publishing

import (
	"context"
	"net/url"
)

type Client interface {
	ServiceUrls(ctx context.Context, svcNames []string) (map[string]url.URL, error)
}
