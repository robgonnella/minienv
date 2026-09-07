package publishing

import "net/http"

// NewNgrokClientWithTransport swaps the api root and the transport, which the
// production constructor hardcodes. A RoundTripper rather than an httptest
// server, so the specs need no listener.
func NewNgrokClientWithTransport(
	apiKey string,
	baseUrl string,
	transport http.RoundTripper,
) *NgrokClient {
	return &NgrokClient{
		apiKey:     apiKey,
		baseUrl:    baseUrl,
		httpClient: &http.Client{Transport: transport},
	}
}
