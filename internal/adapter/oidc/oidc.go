// SPDX-License-Identifier: AGPL-3.0-only

// Package oidc implements identity.OIDCClient with the standard library alone (ADR-0106 §1,
// §5): OpenID Connect Discovery 1.0, the provider's JWKS (RFC 7517), and the verification of an
// ID token (OpenID Connect Core 1.0 §3.1.3.7) signed with RS256, ES256, or EdDSA (Ed25519).
//
// Every request goes through kernel.HTTPClient with kernel.HTTPPurposeLoginProvider, which
// never follows a redirect, bounds the response, and keeps private networks out unless the
// operator lists them (login_provider_allowed_destinations). The Discovery documents and the
// JWKS are kept in memory as long as their cache directives allow (kernel.ResponseTTL), at most
// MaxTTL, for at most MaxEntries URLs each.
package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
)

// Defaults of Options.
const (
	// DefaultTTL is how long a response without cache directives is reused.
	DefaultTTL = 15 * time.Minute
	// DefaultMaxTTL bounds how long any response is reused.
	DefaultMaxTTL = 24 * time.Hour
	// DefaultMaxEntries bounds the Discovery documents and the JWKS kept, each.
	DefaultMaxEntries = 256
	// ClockSkew is the leeway of the time claims of an ID token.
	ClockSkew = time.Minute
	// maxTokenBytes bounds an ID token.
	maxTokenBytes = 16 << 10
)

// Options configure a Client.
type Options struct {
	// HTTP sends the requests; it is required.
	HTTP kernel.HTTPClient
	// Clock tells the time of the caches and of the token's time claims; it is required.
	Clock kernel.Clock
	// DefaultTTL, MaxTTL, and MaxEntries override the defaults above when positive.
	DefaultTTL time.Duration
	MaxTTL     time.Duration
	MaxEntries int
}

// Client is the identity.OIDCClient adapter. It is safe for concurrent use.
type Client struct {
	opts      Options
	discovery *cache[appidentity.OIDCProvider]
	jwks      *cache[[]jwk]
}

var _ appidentity.OIDCClient = (*Client)(nil)

// New returns a Client.
func New(opts Options) *Client {
	if opts.DefaultTTL <= 0 {
		opts.DefaultTTL = DefaultTTL
	}
	if opts.MaxTTL <= 0 {
		opts.MaxTTL = DefaultMaxTTL
	}
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = DefaultMaxEntries
	}
	return &Client{
		opts:      opts,
		discovery: newCache[appidentity.OIDCProvider](opts.MaxEntries),
		jwks:      newCache[[]jwk](opts.MaxEntries),
	}
}

// discoveryDocument is the part of a Discovery document the client reads.
type discoveryDocument struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserinfoEndpoint      string   `json:"userinfo_endpoint"`
	JWKSURI               string   `json:"jwks_uri"`
	SigningAlgs           []string `json:"id_token_signing_alg_values_supported"`
	CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
}

// Discover implements identity.OIDCClient.
func (c *Client) Discover(ctx context.Context, issuer string) (appidentity.OIDCProvider, error) {
	if err := checkURL(issuer); err != nil {
		return appidentity.OIDCProvider{}, fmt.Errorf("%w: issuer: %w", appidentity.ErrOIDCDiscovery, err)
	}
	if u, _ := url.Parse(issuer); u.RawQuery != "" || u.Fragment != "" {
		return appidentity.OIDCProvider{}, fmt.Errorf("%w: the issuer %q has a query or a fragment", appidentity.ErrOIDCDiscovery, issuer)
	}
	now := c.opts.Clock.Now()
	if p, ok := c.discovery.get(issuer, now); ok {
		return p, nil
	}
	body, ttl, err := c.fetch(ctx, strings.TrimSuffix(issuer, "/")+"/.well-known/openid-configuration", now)
	if err != nil {
		return appidentity.OIDCProvider{}, fmt.Errorf("%w: %w", appidentity.ErrOIDCDiscovery, err)
	}
	var doc discoveryDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return appidentity.OIDCProvider{}, fmt.Errorf("%w: %w", appidentity.ErrOIDCDiscovery, err)
	}
	p, err := providerOf(issuer, doc)
	if err != nil {
		return appidentity.OIDCProvider{}, fmt.Errorf("%w: %w", appidentity.ErrOIDCDiscovery, err)
	}
	c.discovery.put(issuer, p, now, ttl)
	return p, nil
}

// providerOf checks a Discovery document of issuer (OpenID Connect Discovery 1.0 §3, §4.3).
func providerOf(issuer string, doc discoveryDocument) (appidentity.OIDCProvider, error) {
	if doc.Issuer != issuer {
		return appidentity.OIDCProvider{}, fmt.Errorf("the document names the issuer %q", doc.Issuer)
	}
	for name, v := range map[string]string{
		"authorization_endpoint": doc.AuthorizationEndpoint, "token_endpoint": doc.TokenEndpoint, "jwks_uri": doc.JWKSURI,
	} {
		if err := checkURL(v); err != nil {
			return appidentity.OIDCProvider{}, fmt.Errorf("%s: %w", name, err)
		}
	}
	if doc.UserinfoEndpoint != "" {
		if err := checkURL(doc.UserinfoEndpoint); err != nil {
			return appidentity.OIDCProvider{}, fmt.Errorf("userinfo_endpoint: %w", err)
		}
	}
	var algs []string
	for _, a := range appidentity.OIDCSigningAlgs {
		if slices.Contains(doc.SigningAlgs, a) {
			algs = append(algs, a)
		}
	}
	if len(algs) == 0 {
		return appidentity.OIDCProvider{}, fmt.Errorf("id_token_signing_alg_values_supported %q has none of %q", doc.SigningAlgs, appidentity.OIDCSigningAlgs)
	}
	return appidentity.OIDCProvider{
		Issuer: issuer, AuthorizationEndpoint: doc.AuthorizationEndpoint, TokenEndpoint: doc.TokenEndpoint,
		UserinfoEndpoint: doc.UserinfoEndpoint, JWKSURI: doc.JWKSURI, SigningAlgs: algs,
		CodeChallengeMethods: doc.CodeChallengeMethods,
	}, nil
}

// checkURL accepts an absolute http or https URL with a host and without user info; the HTTP
// client decides which of them it may reach.
func checkURL(v string) error {
	u, err := url.Parse(v)
	switch {
	case err != nil:
		return err
	case u.Scheme != "https" && u.Scheme != "http":
		return fmt.Errorf("%q is not an https URL", v)
	case u.Host == "" || u.User != nil:
		return fmt.Errorf("%q has no host or has user info", v)
	}
	return nil
}

// keys returns the JWKS at uri, from the cache unless refetch, and when it was fetched.
func (c *Client) keys(ctx context.Context, uri string, now time.Time, refetch bool) (keys []jwk, fetchedAt time.Time, err error) {
	if !refetch {
		if keys, at, ok := c.jwks.getWithTime(uri, now); ok {
			return keys, at, nil
		}
	}
	body, ttl, err := c.fetch(ctx, uri, now)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("oidc: JWKS %s: %w", uri, err)
	}
	keys, err = parseJWKS(body)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("oidc: JWKS %s: %w", uri, err)
	}
	c.jwks.put(uri, keys, now, ttl)
	return keys, now, nil
}

// fetch GETs a JSON document and returns its body and how long it may be reused.
func (c *Client) fetch(ctx context.Context, uri string, now time.Time) ([]byte, time.Duration, error) {
	resp, err := c.opts.HTTP.Do(ctx, &kernel.HTTPRequest{
		URL: uri, Purpose: kernel.HTTPPurposeLoginProvider, NoRedirects: true,
		Header: http.Header{"Accept": {"application/json"}},
	})
	if err != nil {
		return nil, 0, err
	}
	body, err := resp.ReadAll()
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("GET %s answered %d", uri, resp.StatusCode)
	}
	return body, kernel.ResponseTTL(resp.Header, now, c.opts.DefaultTTL, c.opts.MaxTTL), nil
}

// cache keeps values by URL until they expire, at most max of them; when full, it drops the
// entry that expires first.
type cache[V any] struct {
	mu      sync.Mutex
	max     int
	entries map[string]cacheEntry[V]
}

type cacheEntry[V any] struct {
	value     V
	fetchedAt time.Time
	expires   time.Time
}

func newCache[V any](maxEntries int) *cache[V] {
	return &cache[V]{max: maxEntries, entries: map[string]cacheEntry[V]{}}
}

func (c *cache[V]) get(key string, now time.Time) (V, bool) {
	v, _, ok := c.getWithTime(key, now)
	return v, ok
}

func (c *cache[V]) getWithTime(key string, now time.Time) (V, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || !now.Before(e.expires) {
		var zero V
		return zero, time.Time{}, false
	}
	return e.value, e.fetchedAt, true
}

// put keeps value for ttl; with a zero ttl, it keeps only the time of the fetch.
func (c *cache[V]) put(key string, value V, now time.Time, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[key]; !ok && len(c.entries) >= c.max {
		var oldest string
		for k, e := range c.entries {
			if oldest == "" || e.expires.Before(c.entries[oldest].expires) {
				oldest = k
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[key] = cacheEntry[V]{value: value, fetchedAt: now, expires: now.Add(ttl)}
}
