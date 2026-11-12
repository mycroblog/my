// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"
)

// PushSubscription is where the Web Push notifications of a local account go: the push
// service endpoint and the keys that a browser's PushManager returned, with the notification
// types the user chose (push_subscriptions and push_subscription_alerts, current values,
// FR-206).
//
// A subscription belongs to the OAuth grant that registered it (TokenFamilyID, the family of
// tokens that refreshing the grant issues), so that each signed-in app has at most one and
// signing out of it ends it. An endpoint is unique: registering it again, under any grant,
// replaces the subscription that had it.
type PushSubscription struct {
	ID            string
	AccountID     string
	TokenFamilyID string
	// Endpoint is the push service URL that messages are posted to (RFC 8030 §5).
	Endpoint string
	// KeyP256DH is the user agent's P-256 public key and KeyAuth its authentication secret
	// (RFC 8291 §3), in unpadded base64url as PushSubscription.toJSON returns them.
	KeyP256DH string
	KeyAuth   string
	// Alerts are the notification types pushed to the subscription, in the order of Types.
	Alerts    []Type
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MaxPushEndpointBytes is the longest endpoint URL a subscription accepts.
const MaxPushEndpointBytes = 2048

var (
	// ErrInvalidPushEndpoint reports an endpoint that is not an absolute https URL of at most
	// MaxPushEndpointBytes.
	ErrInvalidPushEndpoint = errors.New("notification: the push endpoint is not an https URL")
	// ErrInvalidPushKey reports a p256dh key that is not an uncompressed P-256 point, or an auth
	// secret that is not 16 bytes, in base64url.
	ErrInvalidPushKey = errors.New("notification: the push subscription keys are invalid")
)

// NormalizeAlerts returns the valid types of alerts without duplicates, in the order of Types,
// and false when one of them is not a notification type.
func NormalizeAlerts(alerts []Type) ([]Type, bool) {
	out := make([]Type, 0, len(alerts))
	for _, t := range Types {
		if slices.Contains(alerts, t) {
			out = append(out, t)
		}
	}
	for _, t := range alerts {
		if !t.Valid() {
			return nil, false
		}
	}
	return out, true
}

// Alerted reports whether the subscription takes notifications of type t.
func (s PushSubscription) Alerted(t Type) bool { return slices.Contains(s.Alerts, t) }

// Validate checks the invariants of a subscription, after NormalizePushKeys.
func (s PushSubscription) Validate() error {
	if s.ID == "" || s.AccountID == "" || s.TokenFamilyID == "" {
		return errors.New("notification: a push subscription needs an ID, an account, and a token family")
	}
	if err := ValidatePushEndpoint(s.Endpoint); err != nil {
		return err
	}
	if _, _, err := DecodePushKeys(s.KeyP256DH, s.KeyAuth); err != nil {
		return err
	}
	if norm, ok := NormalizeAlerts(s.Alerts); !ok || !slices.Equal(norm, s.Alerts) {
		return ErrInvalidType
	}
	return nil
}

// ValidatePushEndpoint checks that endpoint is an absolute https URL with a host and without
// user information or a fragment, of at most MaxPushEndpointBytes. Whether its host is public
// is checked when a message is sent (the HTTPClient's SSRF guard, ADR-0019 §1).
func ValidatePushEndpoint(endpoint string) error {
	if len(endpoint) > MaxPushEndpointBytes {
		return ErrInvalidPushEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return ErrInvalidPushEndpoint
	}
	return nil
}

// NormalizePushKeys returns the keys in unpadded base64url, accepting padded base64url too.
// The result is unchecked; Validate checks it.
func NormalizePushKeys(p256dh, auth string) (string, string) {
	return strings.TrimRight(p256dh, "="), strings.TrimRight(auth, "=")
}

// DecodePushKeys decodes the keys of a subscription: the user agent's public key, which must
// be an uncompressed point on P-256 (65 bytes), and the 16-byte authentication secret
// (RFC 8291 §3).
func DecodePushKeys(p256dh, auth string) (*ecdh.PublicKey, []byte, error) {
	pub, err := base64.RawURLEncoding.DecodeString(p256dh)
	if err != nil || len(pub) != 65 {
		return nil, nil, ErrInvalidPushKey
	}
	key, err := ecdh.P256().NewPublicKey(pub)
	if err != nil {
		return nil, nil, ErrInvalidPushKey
	}
	secret, err := base64.RawURLEncoding.DecodeString(auth)
	if err != nil || len(secret) != 16 {
		return nil, nil, ErrInvalidPushKey
	}
	return key, secret, nil
}
