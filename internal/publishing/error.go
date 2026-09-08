// Package publishing looks up the public urls ngrok is serving for a project.
package publishing

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	ErrNgrokNotConfigured errs.Kind = "publishing.ngrok_not_configured"
	ErrNgrokFailedRequest errs.Kind = "publishing.ngrok_failed_request"
	ErrNgrokResponseJSON  errs.Kind = "publishing.ngrok_response_json"
	ErrNgrokURLParse      errs.Kind = "publishing.ngrok_url_parse"
)
