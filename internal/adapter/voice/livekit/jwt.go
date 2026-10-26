// SPDX-License-Identifier: AGPL-3.0-only

package livekit

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// The JWTs of LiveKit are HS256 only, signed with the API secret of the API key in iss
// (ADR-0102 §2): the access tokens of the participants, the tokens of the server's own API
// calls, and the tokens of the webhooks. They are written with crypto/hmac rather than a JWT
// library or LiveKit's SDK.

// videoGrant is the video grant of a LiveKit token. The fields without omitempty are written
// even when false: LiveKit reads a missing canPublish or canPublishData as true.
type videoGrant struct {
	RoomCreate        bool     `json:"roomCreate,omitempty"`
	RoomList          bool     `json:"roomList,omitempty"`
	RoomAdmin         bool     `json:"roomAdmin,omitempty"`
	RoomJoin          bool     `json:"roomJoin,omitempty"`
	Room              string   `json:"room,omitempty"`
	CanSubscribe      *bool    `json:"canSubscribe,omitempty"`
	CanPublish        *bool    `json:"canPublish,omitempty"`
	CanPublishData    *bool    `json:"canPublishData,omitempty"`
	CanPublishSources []string `json:"canPublishSources,omitempty"`
}

// claims are the claims of a LiveKit token that the adapter writes or reads.
type claims struct {
	Issuer    string      `json:"iss"`
	Subject   string      `json:"sub,omitempty"`
	ID        string      `json:"jti,omitempty"`
	NotBefore int64       `json:"nbf"`
	ExpiresAt int64       `json:"exp"`
	Video     *videoGrant `json:"video,omitempty"`
	// SHA256 is the base64 SHA-256 of the body of a webhook delivery.
	SHA256 string `json:"sha256,omitempty"`
}

// jwtHeader is the header of every token the adapter signs.
var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// sign returns c as a JWT signed with secret.
func sign(c claims, secret string) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	signing := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	return signing + "." + base64.RawURLEncoding.EncodeToString(mac(signing, secret)), nil
}

func mac(signing, secret string) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signing))
	return h.Sum(nil)
}

// The reasons a token is refused; the webhook endpoint logs the kind, never the token.
var (
	errMalformedToken = errors.New("livekit: malformed token")
	errTokenAlgorithm = errors.New("livekit: the token is not HS256")
	errTokenSignature = errors.New("livekit: the token's signature does not match")
	errTokenIssuer    = errors.New("livekit: the token's API key is not the configured one")
	errTokenTime      = errors.New("livekit: the token is expired or not yet valid")
)

// clockSkew is how far the clocks of LiveKit and the server may differ for the times of a token.
const clockSkew = time.Minute

// verify checks a token signed with secret by apiKey, valid at now, and returns its claims.
func verify(token, apiKey, secret string, now time.Time) (claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims{}, errMalformedToken
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims{}, errMalformedToken
	}
	var h struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(header, &h); err != nil {
		return claims{}, errMalformedToken
	}
	if h.Alg != "HS256" {
		return claims{}, errTokenAlgorithm
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims{}, errMalformedToken
	}
	if !hmac.Equal(sig, mac(parts[0]+"."+parts[1], secret)) {
		return claims{}, errTokenSignature
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims{}, errMalformedToken
	}
	var c claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return claims{}, errMalformedToken
	}
	if subtle.ConstantTimeCompare([]byte(c.Issuer), []byte(apiKey)) != 1 {
		return claims{}, errTokenIssuer
	}
	if c.ExpiresAt == 0 || now.After(time.Unix(c.ExpiresAt, 0).Add(clockSkew)) ||
		(c.NotBefore != 0 && now.Before(time.Unix(c.NotBefore, 0).Add(-clockSkew))) {
		return claims{}, errTokenTime
	}
	return c, nil
}

func ptr(b bool) *bool { return &b }
