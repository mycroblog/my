// SPDX-License-Identifier: AGPL-3.0-only

package p2p

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // coturn's TURN REST API fixes HMAC-SHA1 (ADR-0101 §5)
	"encoding/base64"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/voice"
)

// Coturn is the TURN source of voice_turn = coturn: the time-limited credentials of coturn's
// use-auth-secret (ADR-0101 §5), computed here without any I/O. The username is
// "<expiry in Unix seconds>:<session ID>" and the password base64(HMAC-SHA1(secret, username)).
// coturn checks the expiry when an allocation first authenticates and keeps the allocation
// after it, so the client takes new credentials into a new allocation by an ICE restart.
type Coturn struct {
	urls   []string
	secret []byte
	clock  kernel.Clock
}

var _ TURN = (*Coturn)(nil)

// NewCoturn returns the coturn source of the TURN servers urls (turn: / turns:) and their
// static-auth-secret.
func NewCoturn(urls []string, secret string, clock kernel.Clock) (*Coturn, error) {
	if len(urls) == 0 || secret == "" {
		return nil, errors.New("p2p: coturn needs its TURN URLs and its secret")
	}
	return &Coturn{urls: slices.Clone(urls), secret: []byte(secret), clock: clock}, nil
}

// Name implements TURN.
func (*Coturn) Name() string { return "coturn" }

// Credentials implements TURN: new credentials that expire ttl from now, rounded down to the
// second of the username.
func (c *Coturn) Credentials(_ context.Context, session voice.Session, ttl time.Duration) (Credentials, error) {
	expires := c.clock.Now().Add(ttl).Truncate(time.Second)
	username := strconv.FormatInt(expires.Unix(), 10) + ":" + session.ID
	mac := hmac.New(sha1.New, c.secret)
	mac.Write([]byte(username))
	return Credentials{
		Servers:   []voice.ICEServer{{URLs: slices.Clone(c.urls), Username: username, Credential: base64.StdEncoding.EncodeToString(mac.Sum(nil))}},
		ExpiresAt: expires,
	}, nil
}

// Revoke implements TURN. coturn's credentials cannot be revoked; they expire.
func (*Coturn) Revoke(context.Context, voice.Session) error { return nil }
