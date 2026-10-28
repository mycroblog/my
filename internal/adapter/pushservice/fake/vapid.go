// SPDX-License-Identifier: AGPL-3.0-only

package fake

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// maxTokenLifetime is how far in the future the exp claim of a VAPID token may be (RFC 8292 §2).
const maxTokenLifetime = 24 * time.Hour

// errNoVAPID reports a request without vapid authentication: a push service answers it 401
// (RFC 8292 §4.2). Every other error of VerifyVAPID is invalid authentication, answered 403.
var errNoVAPID = errors.New("the request has no vapid authorization")

// VAPID is a verified vapid authorization (RFC 8292 §3).
type VAPID struct {
	// Key is the application server's public key, the k parameter, as an uncompressed point.
	Key []byte
	// Subject is the sub claim: the contact of the application server.
	Subject string
	// Expires is the exp claim.
	Expires time.Time
}

// VerifyVAPID verifies the value of an Authorization header of the vapid scheme: the ES256
// signature of the JWT t with the key k, its aud claim against origin, its exp claim against now
// (in the future, at most 24 hours ahead), and the presence of its sub claim (RFC 8292 §2-§3).
// It does not compare k with the subscription's applicationServerKey; the caller does.
func VerifyVAPID(header, origin string, now time.Time) (VAPID, error) {
	scheme, params, _ := strings.Cut(strings.TrimSpace(header), " ")
	if !strings.EqualFold(scheme, "vapid") {
		return VAPID{}, errNoVAPID
	}
	var token, key string
	for p := range strings.SplitSeq(params, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(p), "=")
		if !ok {
			return VAPID{}, fmt.Errorf("the vapid parameter %q has no value", p)
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "t":
			token = strings.TrimSpace(value)
		case "k":
			key = strings.TrimSpace(value)
		}
	}
	if token == "" || key == "" {
		return VAPID{}, errors.New("the vapid authorization needs both t and k")
	}
	k, err := decodeB64(key)
	if err != nil {
		return VAPID{}, fmt.Errorf("k is not base64url: %w", err)
	}
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), k)
	if err != nil {
		return VAPID{}, fmt.Errorf("k is not an uncompressed P-256 public key: %w", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return VAPID{}, errors.New("t is not a JWS in the compact serialization")
	}
	var head struct {
		Alg string `json:"alg"`
	}
	if err := decodeJSONPart(parts[0], &head); err != nil {
		return VAPID{}, fmt.Errorf("the JWT header: %w", err)
	}
	if head.Alg != "ES256" {
		return VAPID{}, fmt.Errorf("the JWT is signed with %q; vapid needs ES256", head.Alg)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return VAPID{}, errors.New("the JWT signature is not 64 bytes of base64url (RFC 7518 §3.4)")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return VAPID{}, errors.New("the JWT signature does not verify with k")
	}

	var claims struct {
		Aud any         `json:"aud"`
		Exp json.Number `json:"exp"`
		Sub string      `json:"sub"`
	}
	if err := decodeJSONPart(parts[1], &claims); err != nil {
		return VAPID{}, fmt.Errorf("the JWT claims: %w", err)
	}
	if aud, ok := claims.Aud.(string); !ok || aud != origin {
		return VAPID{}, fmt.Errorf("aud is %v; want the push service's origin %s", claims.Aud, origin)
	}
	exp, err := claims.Exp.Int64()
	if err != nil {
		return VAPID{}, errors.New("exp is missing or not an integer")
	}
	expires := time.Unix(exp, 0)
	if !expires.After(now) {
		return VAPID{}, fmt.Errorf("the token expired at %s", expires.UTC().Format(time.RFC3339))
	}
	if expires.Sub(now) > maxTokenLifetime {
		return VAPID{}, fmt.Errorf("exp %s is more than 24 hours ahead", expires.UTC().Format(time.RFC3339))
	}
	if claims.Sub == "" {
		return VAPID{}, errors.New("sub is missing")
	}
	return VAPID{Key: k, Subject: claims.Sub, Expires: expires}, nil
}

// decodeJSONPart decodes one base64url part of a JWS as JSON, keeping numbers as json.Number.
func decodeJSONPart(part string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return fmt.Errorf("not base64url: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(v)
}

// decodeB64 decodes base64url with or without padding, as applicationServerKey and k come.
func decodeB64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}
