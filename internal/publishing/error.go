package publishing

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	KindNgrokNotConfigured errs.Kind = "publishing.ngrok_not_configured"
	KindNgrokFailedRequest errs.Kind = "publishing.ngrok_failed_request"
	KindNgrokResponseJson  errs.Kind = "publishing.ngrok_response_json"
	KindNgrokUrlParse      errs.Kind = "publishing.ngrok_url_parse"
)
