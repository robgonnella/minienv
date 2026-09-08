package git

import "context"

type Client interface {
	ShortSha(ctx context.Context) (string, error)
}
