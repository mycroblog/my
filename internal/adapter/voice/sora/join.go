// SPDX-License-Identifier: AGPL-3.0-only

package sora

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// The join value of a grant (ADR-0102 §3) is "<payload>.<mac>": the payload is the base64url
// JSON of the room, the session, the role, and the expiry, and the mac its HMAC-SHA256 with the
// join secret, base64url. The client sends it back in the metadata of its connect message, and
// the auth webhook verifies it. The access token of Sora Cloud is a JWT, HS256 with the
// project's API key, which Sora Cloud verifies. Both are written with crypto/hmac.

// joinValue is the payload of a join value.
type joinValue struct {
	Room      string `json:"room"`
	Session   string `json:"session"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"exp"`
}

// joinContext separates the MAC of a join value from any other use of the secret.
const joinContext = "my.voice.sora.join."

// ErrJoinValue reports a join value that does not verify or has expired.
var ErrJoinValue = errors.New("sora: the join value does not verify")

func joinMAC(payload, secret string) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(joinContext + payload))
	return h.Sum(nil)
}

// signJoin returns v signed with secret.
func signJoin(v joinValue, secret string) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(b)
	return payload + "." + base64.RawURLEncoding.EncodeToString(joinMAC(payload, secret)), nil
}

// verifyJoin returns the payload of a join value signed with secret that has not expired at now.
func verifyJoin(value, secret string, now time.Time) (joinValue, error) {
	payload, sig, ok := strings.Cut(value, ".")
	if !ok || payload == "" {
		return joinValue{}, ErrJoinValue
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, joinMAC(payload, secret)) {
		return joinValue{}, ErrJoinValue
	}
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return joinValue{}, ErrJoinValue
	}
	var v joinValue
	if err := json.Unmarshal(b, &v); err != nil || v.Room == "" || v.Session == "" || (v.Role != roleSendRecv && v.Role != roleRecvOnly) {
		return joinValue{}, ErrJoinValue
	}
	if now.Unix() >= v.ExpiresAt {
		return joinValue{}, ErrJoinValue
	}
	return v, nil
}

// accessClaims are the claims of an access token of Sora Cloud.
type accessClaims struct {
	ChannelID string `json:"channel_id"`
	Role      string `json:"role"`
	NotBefore int64  `json:"nbf"`
	ExpiresAt int64  `json:"exp"`
}

// jwtHeader is the header of every access token.
var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// signAccessToken returns c as a JWT signed with the API key of the project.
func signAccessToken(c accessClaims, key string) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	signing := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(b)
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(signing))
	return signing + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil)), nil
}
