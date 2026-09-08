package publishing_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/publishing"
)

// The real NGROK_API_KEY is only read at the composition root, so no test
// binary can pick up a live credential.
const fakeAPIKey = "fake-api-key-for-tests"

// Never reached, so it only has to be a parseable base.
const fakeAPIBaseURL = "https://api.ngrok.test"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

var _ = Describe("NgrokClient", func() {
	var (
		requests []*http.Request
		respond  func(*http.Request) (*http.Response, error)
		subject  *publishing.NgrokClient
		apiKey   string
	)

	BeforeEach(func() {
		requests = nil
		apiKey = fakeAPIKey
		respond = func(*http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusOK,
				`{"endpoints": [], "next_page_uri": null}`,
			), nil
		}
	})

	JustBeforeEach(func() {
		subject = publishing.NewNgrokClientWithTransport(
			apiKey,
			fakeAPIBaseURL,
			roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests = append(requests, r)
				return respond(r)
			}),
		)
	})

	// The composition root builds a client whether or not the key is set, so an
	// unset key reaches this far. Only the url table needs it, so the caller
	// warns rather than failing — but it still has to hear about it.
	Context("with no api key", func() {
		BeforeEach(func() {
			apiKey = ""
		})

		It("reports that it is not configured without calling the api", func() {
			_, err := subject.ServiceUrls(context.Background(), []string{"hello"})

			Expect(err).To(MatchError(publishing.ErrNgrokNotConfigured))
			Expect(err.Error()).To(ContainSubstring("NGROK_API_KEY"))
			Expect(requests).To(BeEmpty())
		})
	})

	It("authenticates and pins the api version", func() {
		_, err := subject.ServiceUrls(context.Background(), []string{"hello"})
		Expect(err).ShouldNot(HaveOccurred())

		Expect(requests).To(HaveLen(1))
		Expect(requests[0].Method).To(Equal(http.MethodGet))
		Expect(requests[0].URL.String()).
			To(Equal(fakeAPIBaseURL + "/endpoints"))
		// The api rejects a number here; it has to be the string "2".
		Expect(requests[0].Header.Get("Ngrok-Version")).To(Equal("2"))
		Expect(requests[0].Header.Get("Authorization")).
			To(Equal("Bearer " + fakeAPIKey))
	})

	It("resolves an empty map when the account publishes nothing", func() {
		urls, err := subject.ServiceUrls(context.Background(), []string{"hello"})

		Expect(err).ShouldNot(HaveOccurred())
		Expect(urls).To(BeEmpty())
	})

	Describe("mapping endpoints to services", func() {
		BeforeEach(func() {
			respond = func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, `{
					"endpoints": [
						{"id": "1", "name": "hello", "url": "https://hello.ngrok.app"},
						{"id": "2", "name": "other-project", "url": "https://other.ngrok.app"}
					],
					"next_page_uri": null
				}`), nil
			}
		})

		It("returns a parsed url for a requested service", func() {
			urls, err := subject.ServiceUrls(context.Background(), []string{"hello"})

			Expect(err).ShouldNot(HaveOccurred())
			Expect(urls).To(HaveLen(1))
			Expect(urls["hello"].Scheme).To(Equal("https"))
			Expect(urls["hello"].Host).To(Equal("hello.ngrok.app"))
		})

		// The api lists every endpoint on the account, not just this project's,
		// so without the filter one namespace would report another's URL.
		It("omits endpoints no requested service names", func() {
			urls, err := subject.ServiceUrls(context.Background(), []string{"hello"})

			Expect(err).ShouldNot(HaveOccurred())
			Expect(urls).ToNot(HaveKey("other-project"))
		})

		It("resolves nothing when no requested service is published", func() {
			urls, err := subject.ServiceUrls(context.Background(), []string{"absent"})

			Expect(err).ShouldNot(HaveOccurred())
			Expect(urls).To(BeEmpty())
		})
	})

	// Without this a project spanning more than one page silently loses the rest.
	Describe("pagination", func() {
		BeforeEach(func() {
			respond = func(r *http.Request) (*http.Response, error) {
				if r.URL.Query().Get("before_id") == "" {
					// Relative, so it only resolves if treated as a
					// reference rather than concatenated onto the base.
					return jsonResponse(http.StatusOK, `{
						"endpoints": [
							{"id": "2", "name": "alpha", "url": "https://alpha.ngrok.app"}
						],
						"next_page_uri": "/endpoints?before_id=2"
					}`), nil
				}

				return jsonResponse(http.StatusOK, `{
					"endpoints": [
						{"id": "1", "name": "beta", "url": "https://beta.ngrok.app"}
					],
					"next_page_uri": null
				}`), nil
			}
		})

		It("merges every page into one result", func() {
			urls, err := subject.ServiceUrls(context.Background(), []string{"alpha", "beta"})

			Expect(err).ShouldNot(HaveOccurred())
			Expect(urls).To(HaveLen(2))
			Expect(urls["alpha"].Host).To(Equal("alpha.ngrok.app"))
			Expect(urls["beta"].Host).To(Equal("beta.ngrok.app"))
		})

		It("resolves the relative next page against the api root", func() {
			_, err := subject.ServiceUrls(context.Background(), []string{"alpha", "beta"})

			Expect(err).ShouldNot(HaveOccurred())
			Expect(requests).To(HaveLen(2))
			Expect(requests[1].URL.String()).
				To(Equal(fakeAPIBaseURL + "/endpoints?before_id=2"))
		})

		// Otherwise it spins to the page cap on a finished deploy.
		Context("when a page points at itself", func() {
			BeforeEach(func() {
				respond = func(*http.Request) (*http.Response, error) {
					return jsonResponse(http.StatusOK, `{
						"endpoints": [],
						"next_page_uri": "/endpoints"
					}`), nil
				}
			})

			It("gives up rather than looping", func() {
				_, err := subject.ServiceUrls(context.Background(), []string{"hello"})

				Expect(err).ShouldNot(HaveOccurred())
				Expect(requests).To(HaveLen(1))
			})
		})
	})

	Describe("failures", func() {
		Context("when the api rejects the request", func() {
			BeforeEach(func() {
				respond = func(*http.Request) (*http.Response, error) {
					return jsonResponse(
						http.StatusUnauthorized,
						`{"msg": "invalid credentials"}`,
					), nil
				}
			})

			It("reports the status and the body", func() {
				_, err := subject.ServiceUrls(context.Background(), []string{"hello"})

				Expect(err).To(MatchError(publishing.ErrNgrokFailedRequest))
				Expect(err.Error()).To(ContainSubstring("status=401"))
				Expect(err.Error()).To(ContainSubstring("invalid credentials"))
			})
		})

		Context("when the response is not json", func() {
			BeforeEach(func() {
				respond = func(*http.Request) (*http.Response, error) {
					return jsonResponse(
						http.StatusOK,
						`<html>gateway timeout</html>`,
					), nil
				}
			})

			It("reports a response parse failure", func() {
				_, err := subject.ServiceUrls(context.Background(), []string{"hello"})

				Expect(err).To(MatchError(publishing.ErrNgrokResponseJSON))
			})
		})

		Context("when an endpoint url cannot be parsed", func() {
			BeforeEach(func() {
				respond = func(*http.Request) (*http.Response, error) {
					// url.Parse accepts almost anything; a non-numeric port
					// is one of the few things it rejects.
					return jsonResponse(http.StatusOK, `{
						"endpoints": [
							{"id": "1", "name": "hello", "url": "https://a.app:port"}
						],
						"next_page_uri": null
					}`), nil
				}
			})

			It("reports a url parse failure naming the endpoint", func() {
				_, err := subject.ServiceUrls(context.Background(), []string{"hello"})

				Expect(err).To(MatchError(publishing.ErrNgrokURLParse))
				Expect(err.Error()).To(ContainSubstring("hello"))
			})
		})

		Context("when the api cannot be reached", func() {
			BeforeEach(func() {
				respond = func(*http.Request) (*http.Response, error) {
					return nil, errors.New("no route to host")
				}
			})

			It("reports a request failure and keeps the cause reachable", func() {
				_, err := subject.ServiceUrls(context.Background(), []string{"hello"})

				Expect(err).To(MatchError(publishing.ErrNgrokFailedRequest))

				// %w, not %s: the transport error says what went wrong.
				var urlErr *url.Error
				Expect(errors.As(err, &urlErr)).To(BeTrue())
				Expect(urlErr.Err).To(MatchError(ContainSubstring("no route")))
			})
		})
	})
})
