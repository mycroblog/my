// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient sends every outbound HTTP request of the server: federation fetches and
// deliveries, WebFinger, NodeInfo, the media proxy, link previews, Web Push, CAPTCHA
// verification, and the metadata documents of OAuth clients (ADR-0019 §1, ADR-0042 §3, docs/architecture/federation.md §1.1). The adapter
// lives in internal/adapter/httpclient and passes testkit.HTTPClientContract.
//
// The adapter refuses destinations that are not public: addresses outside the global unicast
// space or in the special-purpose ranges of ADR-0019 §1 (checked on the address it actually
// connects to, after name resolution and on every redirect hop), internal host names, schemes
// other than https, and ports other than 443 and 80. It ignores proxy environment variables and
// follows at most MaxRedirects redirects, never from https to http. A request may forbid
// redirects (HTTPRequest.NoRedirects) or check every redirect hop with a policy of its own
// (HTTPRequest.CheckRedirect).
//
// Do runs outside transactions (ADR-0003).
type HTTPClient interface {
	// Do sends req and returns the response once its headers arrive. The caller reads and closes
	// Body. Do fails with an error wrapping ErrForbiddenDestination for a refused destination
	// (also when a redirect leads to one), ErrTooManyRedirects, ErrResponseTooLarge when the
	// response's Content-Length exceeds the purpose's limit, ErrInvalidHTTPRequest for a
	// request that cannot be sent, and the error of the request's CheckRedirect. A status code
	// of 3xx (when the client does not follow it), 4xx or 5xx is a response, not an error.
	Do(ctx context.Context, req *HTTPRequest) (*HTTPResponse, error)
}

// HTTPPurpose names what a request fetches; it selects the limits on the response.
type HTTPPurpose int

// Purposes.
const (
	// HTTPPurposeJSON fetches ActivityPub, WebFinger, and NodeInfo documents.
	HTTPPurposeJSON HTTPPurpose = iota + 1
	// HTTPPurposeHTML fetches HTML pages for link previews.
	HTTPPurposeHTML
	// HTTPPurposeImage fetches remote images.
	HTTPPurposeImage
	// HTTPPurposeVideo fetches remote video and audio.
	HTTPPurposeVideo
	// HTTPPurposeHumanCheck posts a CAPTCHA token to the service's siteverify API (ADR-0042 §3).
	HTTPPurposeHumanCheck
	// HTTPPurposeWebPush posts an encrypted Web Push message to a push service (RFC 8030),
	// whose answer has no body that the server reads. The development profile may exempt some
	// destinations from the guard for it alone (push_allowed_destinations, ADR-0118 §3); the
	// client never follows a redirect from them.
	HTTPPurposeWebPush
	// HTTPPurposeWebhook posts a message of the instance's webhooks (ADR-0060). The client never
	// follows a redirect for it: the 3xx answer is returned as is. The deployment configuration
	// may exempt some destinations from the guard for it alone (webhook_allowed_destinations).
	HTTPPurposeWebhook
	// HTTPPurposeClientMetadata fetches the Client ID Metadata Document of an OAuth client
	// (ADR-0090 §1). The client never follows a redirect for it: the 3xx answer is returned as
	// is, and the caller refuses it. Its body is at most 5 KB, as the draft recommends.
	HTTPPurposeClientMetadata
	// HTTPPurposeLoginProvider talks to an external identity provider that an administrator
	// configured (ADR-0106 §5): OpenID Connect Discovery, the JWKS, the token exchange, and the
	// APIs of the presets. The client never follows a redirect for it: the 3xx answer is returned
	// as is, and the caller refuses it. The deployment configuration may exempt some destinations
	// from the guard for it alone (login_provider_allowed_destinations).
	HTTPPurposeLoginProvider
)

// MaxRedirects is the number of redirects the HTTPClient follows for one request.
const MaxRedirects = 5

// HTTPLimits bound one response.
type HTTPLimits struct {
	// MaxBodyBytes is the largest body the caller may read, counted after the transfer
	// decompresses it (so a compression bomb fails too).
	MaxBodyBytes int64
	// Timeout bounds the whole exchange: connecting, redirects, and reading the body.
	Timeout time.Duration
}

// DefaultHTTPLimits returns the limits of ADR-0019 §1 for p (ADR-0042 §3 for
// HTTPPurposeHumanCheck, ADR-0060 for HTTPPurposeWebhook, ADR-0090 §1 for
// HTTPPurposeClientMetadata, ADR-0106 §5 for HTTPPurposeLoginProvider; HTTPPurposeWebPush reads at most the small error document of a push
// service), and false for an unknown purpose.
func DefaultHTTPLimits(p HTTPPurpose) (HTTPLimits, bool) {
	switch p {
	case HTTPPurposeJSON, HTTPPurposeHTML:
		return HTTPLimits{MaxBodyBytes: 1 << 20, Timeout: 30 * time.Second}, true
	case HTTPPurposeImage:
		return HTTPLimits{MaxBodyBytes: 16 << 20, Timeout: 30 * time.Second}, true
	case HTTPPurposeVideo:
		return HTTPLimits{MaxBodyBytes: 100 << 20, Timeout: 60 * time.Second}, true
	case HTTPPurposeHumanCheck:
		return HTTPLimits{MaxBodyBytes: 64 << 10, Timeout: 10 * time.Second}, true
	case HTTPPurposeWebhook:
		return HTTPLimits{MaxBodyBytes: 64 << 10, Timeout: 15 * time.Second}, true
	case HTTPPurposeWebPush:
		return HTTPLimits{MaxBodyBytes: 64 << 10, Timeout: 30 * time.Second}, true
	case HTTPPurposeClientMetadata:
		return HTTPLimits{MaxBodyBytes: 5 << 10, Timeout: 10 * time.Second}, true
	case HTTPPurposeLoginProvider:
		return HTTPLimits{MaxBodyBytes: 256 << 10, Timeout: 15 * time.Second}, true
	}
	return HTTPLimits{}, false
}

// HTTPRequest is one outbound request.
type HTTPRequest struct {
	// Method is the HTTP method; empty means GET.
	Method string
	// URL is the absolute URL to request.
	URL string
	// Header holds the request headers. The client sets User-Agent and Accept-Encoding itself.
	Header http.Header
	// Body is the request body, nil for none.
	Body []byte
	// Purpose selects the response limits; it is required.
	Purpose HTTPPurpose
	// NoRedirects makes the client return a 3xx answer as is instead of following it. A POST
	// whose answer redirects must not count as delivered (a 301, 302, or 303 would be followed
	// with a GET), so deliveries set it.
	NoRedirects bool
	// CheckRedirect, when set, is called with the absolute URL of every redirect hop before the
	// client follows it, after the client's own checks of the destination. An error ends the
	// request, and Do returns an error wrapping it. Callers apply their own policy of
	// destinations with it, such as the federation domain blocks.
	CheckRedirect func(ctx context.Context, url string) error
}

// HTTPResponse is the response to an HTTPRequest.
type HTTPResponse struct {
	StatusCode int
	Header     http.Header
	// URL is the URL that answered, after redirects. Callers compare its origin with the
	// fetched document's id (docs/architecture/federation.md §6.1).
	URL string
	// Body reads the decompressed body. A read past the purpose's MaxBodyBytes fails with an
	// error wrapping ErrResponseTooLarge, and reads fail once the purpose's Timeout passes.
	// Closing it releases the connection.
	Body io.ReadCloser
}

// ReadAll reads the whole body and closes it.
func (r *HTTPResponse) ReadAll() ([]byte, error) {
	b, err := io.ReadAll(r.Body)
	if cerr := r.Body.Close(); err == nil && cerr != nil {
		err = fmt.Errorf("close response body: %w", cerr)
	}
	return b, err
}

var (
	// ErrForbiddenDestination reports a request to, or a redirect towards, a destination the
	// HTTPClient refuses (ADR-0019 §1).
	ErrForbiddenDestination = errors.New("forbidden destination")
	// ErrTooManyRedirects reports a request that redirected more than MaxRedirects times.
	ErrTooManyRedirects = errors.New("too many redirects")
	// ErrResponseTooLarge reports a response body larger than its purpose allows.
	ErrResponseTooLarge = errors.New("response too large")
	// ErrInvalidHTTPRequest reports a request that cannot be sent, such as one without a
	// purpose or with a malformed URL.
	ErrInvalidHTTPRequest = errors.New("invalid HTTP request")
)
