package publishing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/robgonnella/minienv/internal/errs"
	"github.com/rs/zerolog/log"
)

// next_page_uri comes back relative, so requests resolve against the root.
const (
	ngrokApiBaseUrl    = "https://api.ngrok.com"
	ngrokEndpointsPath = "/endpoints"
)

const (
	maxEndpointPages    = 50
	ngrokRequestTimeout = 30 * time.Second
)

// Name is what the chart writes into ngrok.yml, and the only thing tying an
// endpoint back to a compose service.
type Endpoint struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type EndpointsResponse struct {
	Endpoints   []Endpoint `json:"endpoints"`
	NextPageURI *string    `json:"next_page_uri"`
}

type NgrokClient struct {
	apiKey     string
	baseUrl    string
	httpClient *http.Client
}

func NewNgrokClient(apiKey string) *NgrokClient {
	return &NgrokClient{
		apiKey:     apiKey,
		baseUrl:    ngrokApiBaseUrl,
		httpClient: &http.Client{Timeout: ngrokRequestTimeout},
	}
}

// A service ngrok knows nothing about is absent from the result, so an empty
// map means nothing is published rather than that something failed.
func (c *NgrokClient) ServiceUrls(
	svcNames []string,
) (map[string]url.URL, error) {
	// Reported rather than shrugged off as an empty result: the caller cannot
	// otherwise tell a missing key from an account serving nothing. Publishing
	// does not need the key, so the caller downgrades this to a warning.
	if c.apiKey == "" {
		return nil, errs.Errorf(
			KindNgrokNotConfigured,
			"looking up published service urls requires NGROK_API_KEY to be set",
		)
	}

	base, err := url.Parse(c.baseUrl)
	if err != nil {
		return nil, errs.Errorf(
			KindNgrokUrlParse,
			"failed to parse ngrok api base url %q: %w",
			c.baseUrl,
			err,
		)
	}

	mappedUrls := map[string]url.URL{}
	next := ngrokEndpointsPath

	for range maxEndpointPages {
		ref, err := url.Parse(next)
		if err != nil {
			return nil, errs.Errorf(
				KindNgrokUrlParse,
				"failed to parse ngrok api page uri %q: %w",
				next,
				err,
			)
		}

		data, err := c.getEndpoints(base.ResolveReference(ref).String())
		if err != nil {
			return nil, err
		}

		if err := collectEndpoints(
			mappedUrls,
			data.Endpoints,
			svcNames,
		); err != nil {
			return nil, err
		}

		// Bailing on a self-referential page avoids spinning up to the cap.
		if data.NextPageURI == nil ||
			*data.NextPageURI == "" ||
			*data.NextPageURI == next {
			return mappedUrls, nil
		}

		next = *data.NextPageURI
	}

	// A truncated table beats none, and the deploy already succeeded.
	log.
		Warn().
		Int("pages", maxEndpointPages).
		Msg("stopped paging ngrok endpoints at the page limit")

	return mappedUrls, nil
}

// The API lists every endpoint on the account, hence the filter.
func collectEndpoints(
	urls map[string]url.URL,
	endpoints []Endpoint,
	svcNames []string,
) error {
	for _, ep := range endpoints {
		if !slices.Contains(svcNames, ep.Name) {
			continue
		}

		endpointUrl, err := url.Parse(ep.URL)
		if err != nil {
			return errs.Errorf(
				KindNgrokUrlParse,
				"failed to parse url %q for endpoint %s: %w",
				ep.URL,
				ep.Name,
				err,
			)
		}

		urls[ep.Name] = *endpointUrl
	}

	return nil
}

func (c *NgrokClient) getEndpoints(
	endpointsUrl string,
) (*EndpointsResponse, error) {
	req, err := http.NewRequest(http.MethodGet, endpointsUrl, nil)
	if err != nil {
		return nil, errs.Errorf(
			KindNgrokFailedRequest,
			"failed to build ngrok api request: %w",
			err,
		)
	}

	// The API version header must be string "2"
	req.Header.Add("ngrok-version", "2")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errs.Errorf(
			KindNgrokFailedRequest,
			"failed to send ngrok api request: %w",
			err,
		)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, errs.Errorf(
			KindNgrokFailedRequest,
			"ngrok api request failed with status=%d, body=%s",
			res.StatusCode,
			string(body),
		)
	}

	var data EndpointsResponse
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, errs.Errorf(
			KindNgrokResponseJson,
			"failed to parse ngrok api response: %w",
			err,
		)
	}

	return &data, nil
}
