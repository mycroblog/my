// SPDX-License-Identifier: MIT

package mykit

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIBasePath is the path of the client API under the server's origin (`servers[0].url` of api/openapi.yaml).
const APIBasePath = "/api/v1"

// defaultUserAgent is the product token of the library in User-Agent.
const defaultUserAgent = "mykit-go"

// Config configures a client of one server.
type Config struct {
	// Server is the origin of the server, such as "https://example.com". The client API is under APIBasePath.
	Server string
	// HTTPClient sends the requests. Nil means a new http.Client; the library never uses http.DefaultClient.
	HTTPClient *http.Client
	// TokenSource gives the access token of the requests. Nil sends them without one (anonymous).
	TokenSource TokenSource
	// AcceptLanguage is sent as Accept-Language, so that the server localizes the title and detail of its
	// problems. Empty leaves it to the server.
	AcceptLanguage string
	// UserAgent is the product of the application, such as "mybot/1.2"; User-Agent carries it before the
	// library's own token.
	UserAgent string
	// Retry is the policy for 429 and 503 answers. The zero value retries with the defaults.
	Retry RetryPolicy
}

// RetryPolicy decides how a request answered with 429 or 503 is sent again. Only requests that are safe to
// repeat are retried: GET, HEAD, and requests with an Idempotency-Key.
type RetryPolicy struct {
	// Disabled turns the retries off: the 429 or 503 becomes a *Problem at once.
	Disabled bool
	// MaxRetries is how many times a request is sent again. Zero means 3.
	MaxRetries int
	// MaxWait is the longest the client waits before a retry. When Retry-After asks for longer, the answer
	// becomes a *Problem instead. Zero means one minute.
	MaxWait time.Duration
}

func (p RetryPolicy) maxRetries() int {
	if p.Disabled {
		return 0
	}
	if p.MaxRetries <= 0 {
		return 3
	}
	return p.MaxRetries
}

func (p RetryPolicy) maxWait() time.Duration {
	if p.MaxWait <= 0 {
		return time.Minute
	}
	return p.MaxWait
}

// API is a client of one server's client API: the generated client (every operation as <Op>WithResponse),
// with the cross-cutting behaviour of the library. An answer with a status of 400 or more is an error that
// errors.As turns into a *Problem, and a request that got no answer is a *NetworkError.
type API struct {
	*ClientWithResponses

	origin    *url.URL
	transport *transport
}

// New returns the client that cfg configures.
func New(cfg Config) (*API, error) {
	origin, err := parseOrigin(cfg.Server)
	if err != nil {
		return nil, err
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{}
	}
	ua := defaultUserAgent
	if cfg.UserAgent != "" {
		ua = cfg.UserAgent + " " + defaultUserAgent
	}
	t := &transport{
		client:         hc,
		tokens:         cfg.TokenSource,
		acceptLanguage: cfg.AcceptLanguage,
		userAgent:      ua,
		retry:          cfg.Retry,
		basePath:       APIBasePath,
		sleep:          sleepContext,
	}
	gen, err := NewClientWithResponses(origin.String()+APIBasePath+"/", WithHTTPClient(t))
	if err != nil {
		return nil, err
	}
	return &API{ClientWithResponses: gen, origin: origin, transport: t}, nil
}

// parseOrigin reads the server's origin: an absolute http or https URL without a path, query, or fragment.
func parseOrigin(server string) (*url.URL, error) {
	if server == "" {
		return nil, errors.New("mykit: Config.Server is empty")
	}
	u, err := url.Parse(strings.TrimSuffix(server, "/"))
	if err != nil {
		return nil, fmt.Errorf("mykit: Config.Server: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("mykit: Config.Server %q is not an origin such as https://example.com", server)
	}
	return u, nil
}
