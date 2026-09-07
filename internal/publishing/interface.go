package publishing

import "net/url"

type Client interface {
	ServiceUrls(svcNames []string) (map[string]url.URL, error)
}
