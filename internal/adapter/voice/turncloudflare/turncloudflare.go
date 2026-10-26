// SPDX-License-Identifier: AGPL-3.0-only

// Package turncloudflare is the TURN source of voice_turn = cloudflare (ADR-0101 §5): it asks
// Cloudflare's TURN service for the short-lived credentials of a session, keeps them in the
// KeyValueStore to reuse while they last, and revokes them when the session leaves or is
// removed. Cloudflare is an API the operator configured, so the adapter has its own HTTP
// client with timeouts instead of the SSRF-guarded kernel.HTTPClient.
package turncloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/adapter/voice/p2p"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/voice"
)

// Name is the voice_turn value of the source.
const Name = "cloudflare"

// DefaultBaseURL is the API of Cloudflare's TURN service.
const DefaultBaseURL = "https://rtc.live.cloudflare.com/v1"

// MaxTTL is the longest validity of Cloudflare's TURN credentials.
const MaxTTL = 48 * time.Hour

// renewalMargin is how long before their expiry cached credentials stop being reused (half
// their validity when that is shorter), so that a client renewing its grant ahead of the expiry
// (10 minutes before it, ADR-0101 §5) gets new ones instead of the expiring ones. The cache
// keeps them until they expire, so that Revoke still finds them.
const renewalMargin = 15 * time.Minute

// kvNamespace holds the credentials of the sessions, keyed by session ID.
const kvNamespace = "voice.turn"

// maxResponseBytes bounds the body read from the API.
const maxResponseBytes = 64 << 10

// Options configure the Source.
type Options struct {
	// KeyID and APIToken are the TURN key of voice_turn_cloudflare_key_id and
	// voice_turn_cloudflare_api_token.
	KeyID    string
	APIToken string
	// KV caches the credentials of the sessions.
	KV    kernel.KeyValueStore
	Clock kernel.Clock
	// HTTPClient calls the API; nil takes a client with a 10-second timeout.
	HTTPClient *http.Client
	// BaseURL is the API; DefaultBaseURL when empty.
	BaseURL string
}

// Source is the Cloudflare TURN source. It is safe for concurrent use.
type Source struct{ o Options }

var _ p2p.TURN = (*Source)(nil)

// New returns the Source of o.
func New(o Options) (*Source, error) {
	if strings.TrimSpace(o.KeyID) == "" || strings.TrimSpace(o.APIToken) == "" {
		return nil, errors.New("turncloudflare: the key ID and the API token are required")
	}
	if o.KV == nil || o.Clock == nil {
		return nil, errors.New("turncloudflare: a key-value store and a clock are required")
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if o.BaseURL == "" {
		o.BaseURL = DefaultBaseURL
	}
	o.BaseURL = strings.TrimSuffix(o.BaseURL, "/")
	return &Source{o: o}, nil
}

// Name implements p2p.TURN.
func (*Source) Name() string { return Name }

// cached is the value of a session in the KeyValueStore.
type cached struct {
	Username  string            `json:"username"`
	Servers   []voice.ICEServer `json:"servers"`
	IssuedAt  time.Time         `json:"issued_at"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// reusable reports whether the credentials are far enough from their expiry to hand out again.
func (c cached) reusable(now time.Time) bool {
	margin := min(renewalMargin, c.ExpiresAt.Sub(c.IssuedAt)/2)
	return now.Before(c.ExpiresAt.Add(-margin))
}

// Credentials implements p2p.TURN: the cached credentials of the session while they last
// longer than the renewal margin, or new ones that last ttl (at most MaxTTL).
func (s *Source) Credentials(ctx context.Context, session voice.Session, ttl time.Duration) (p2p.Credentials, error) {
	if c, ok, err := s.cached(ctx, session.ID); err != nil {
		return p2p.Credentials{}, err
	} else if ok && c.reusable(s.o.Clock.Now()) {
		return p2p.Credentials{Servers: c.Servers, ExpiresAt: c.ExpiresAt, Reused: true}, nil
	}
	ttl = min(ttl, MaxTTL).Truncate(time.Second)
	if ttl <= 0 {
		return p2p.Credentials{}, fmt.Errorf("turncloudflare: the credential TTL %s is under a second", ttl)
	}
	now := s.o.Clock.Now()
	c, err := s.generate(ctx, ttl)
	if err != nil {
		return p2p.Credentials{}, err
	}
	c.IssuedAt, c.ExpiresAt = now, now.Add(ttl)
	b, err := json.Marshal(c)
	if err != nil {
		return p2p.Credentials{}, err
	}
	if err := s.o.KV.Set(ctx, kvNamespace, session.ID, b, ttl); err != nil {
		return p2p.Credentials{}, fmt.Errorf("turncloudflare: cache the credentials: %w", err)
	}
	return p2p.Credentials{Servers: c.Servers, ExpiresAt: c.ExpiresAt}, nil
}

func (s *Source) cached(ctx context.Context, sessionID string) (cached, bool, error) {
	b, err := s.o.KV.Get(ctx, kvNamespace, sessionID)
	if errors.Is(err, kernel.ErrKeyNotFound) {
		return cached{}, false, nil
	}
	if err != nil {
		return cached{}, false, fmt.Errorf("turncloudflare: read the cached credentials: %w", err)
	}
	var c cached
	if err := json.Unmarshal(b, &c); err != nil || len(c.Servers) == 0 {
		return cached{}, false, nil // an unreadable entry is replaced
	}
	return c, true, nil
}

// iceServer is an entry of iceServers in the API; urls is a string or a list of them.
type iceServer struct {
	URLs       json.RawMessage `json:"urls"`
	Username   string          `json:"username"`
	Credential string          `json:"credential"`
}

// generate calls generate-ice-servers.
func (s *Source) generate(ctx context.Context, ttl time.Duration) (cached, error) {
	body, err := json.Marshal(map[string]int64{"ttl": int64(ttl / time.Second)})
	if err != nil {
		return cached{}, err
	}
	var resp struct {
		ICEServers []iceServer `json:"iceServers"`
	}
	if err := s.call(ctx, "/turn/keys/"+url.PathEscape(s.o.KeyID)+"/credentials/generate-ice-servers", body, &resp); err != nil {
		return cached{}, err
	}
	var c cached
	for _, is := range resp.ICEServers {
		urls, err := decodeURLs(is.URLs)
		if err != nil {
			return cached{}, fmt.Errorf("turncloudflare: decode the ICE servers: %w", err)
		}
		if len(urls) == 0 {
			continue
		}
		c.Servers = append(c.Servers, voice.ICEServer{URLs: urls, Username: is.Username, Credential: is.Credential})
		if c.Username == "" && is.Username != "" {
			c.Username = is.Username
		}
	}
	if c.Username == "" {
		return cached{}, errors.New("turncloudflare: the response has no TURN credentials")
	}
	return c, nil
}

func decodeURLs(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return []string{one}, nil
	}
	var many []string
	err := json.Unmarshal(raw, &many)
	return many, err
}

// Revoke implements p2p.TURN: it revokes the cached credentials of the session and forgets
// them. Credentials already gone from the cache or from Cloudflare need nothing more.
func (s *Source) Revoke(ctx context.Context, session voice.Session) error {
	c, ok, err := s.cached(ctx, session.ID)
	if err != nil || !ok {
		return err
	}
	err = s.call(ctx, "/turn/keys/"+url.PathEscape(s.o.KeyID)+"/credentials/"+url.PathEscape(c.Username)+"/revoke", nil, nil)
	var se *statusError
	if err != nil && (!errors.As(err, &se) || se.status != http.StatusNotFound) {
		return err
	}
	return s.o.KV.Delete(ctx, kvNamespace, session.ID)
}

// statusError is a response of the API outside 2xx.
type statusError struct {
	path   string
	status int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("turncloudflare: %s answered %d", e.path, e.status)
}

// call POSTs body to path of the API and decodes the response into out when out is not nil.
func (s *Source) call(ctx context.Context, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.o.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.o.APIToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// The errors name the call by its operation, never by its URL, which holds the key ID.
	op := path[strings.LastIndex(path, "/")+1:]
	resp, err := s.o.HTTPClient.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("turncloudflare: call %s: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return &statusError{path: op, status: resp.StatusCode}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return fmt.Errorf("turncloudflare: decode the response of %s: %w", op, err)
	}
	return nil
}
