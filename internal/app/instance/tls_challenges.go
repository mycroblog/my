// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The KeyValueStore namespaces of the answers of the ACME challenges (ADR-0163 §3): the key
// authorization of an HTTP-01 challenge by its token, and of a TLS-ALPN-01 challenge by its host.
const (
	ACMEHTTP01Namespace    = "acme.http01"
	ACMETLSALPN01Namespace = "acme.tlsalpn01"
)

// ACMEChallengeTTL is how long an answer stays in the KeyValueStore: longer than any validation,
// short enough that an answer its process failed to remove does not linger.
const ACMEChallengeTTL = 10 * time.Minute

// oidACMEIdentifier is id-pe-acmeIdentifier, the extension of the TLS-ALPN-01 certificate (RFC
// 8737 §3).
var oidACMEIdentifier = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 31}

// ACMEChallenges keeps the answers of the ACME challenges in the shared KeyValueStore, so that
// every process answers the CA's validation, wherever it lands (ADR-0163 §3). It is the
// ACMESolver of the orders and what the listeners answer the CA from.
type ACMEChallenges struct {
	kv kernel.KeyValueStore
}

// NewACMEChallenges returns the answers kept in kv, which every process of the server shares (kv
// = db or valkey; kv = memory only with one process).
func NewACMEChallenges(kv kernel.KeyValueStore) *ACMEChallenges {
	return &ACMEChallenges{kv: kv}
}

// Present stores the answer of ch for ACMEChallengeTTL.
func (c *ACMEChallenges) Present(ctx context.Context, ch ACMEChallenge) error {
	ns, key, err := challengeKey(ch)
	if err != nil {
		return err
	}
	if err := c.kv.Set(ctx, ns, key, []byte(ch.KeyAuthorization), ACMEChallengeTTL); err != nil {
		return fmt.Errorf("instance: store the answer of the %s challenge of %s: %w", ch.Type, ch.Host, err)
	}
	return nil
}

// CleanUp removes the answer of ch.
func (c *ACMEChallenges) CleanUp(ctx context.Context, ch ACMEChallenge) error {
	ns, key, err := challengeKey(ch)
	if err != nil {
		return err
	}
	return c.kv.Delete(ctx, ns, key)
}

func challengeKey(ch ACMEChallenge) (namespace, key string, err error) {
	switch ch.Type {
	case ACMEChallengeHTTP01:
		if ch.Token == "" {
			return "", "", errors.New("instance: an http-01 challenge without a token")
		}
		return ACMEHTTP01Namespace, ch.Token, nil
	case ACMEChallengeTLSALPN01:
		return ACMETLSALPN01Namespace, normalizeChallengeHost(ch.Host), nil
	}
	return "", "", fmt.Errorf("instance: the %q challenge is not answered", ch.Type)
}

// normalizeChallengeHost lower-cases host and drops a trailing dot, as server names compare.
func normalizeChallengeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

// HTTP01 returns the body of the HTTP-01 resource of token, an error wrapping
// kernel.ErrKeyNotFound when no order waits for it.
func (c *ACMEChallenges) HTTP01(ctx context.Context, token string) (string, error) {
	b, err := c.kv.Get(ctx, ACMEHTTP01Namespace, token)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// TLSALPN01Certificate returns the certificate that answers the TLS-ALPN-01 challenge of host
// (RFC 8737 §3), built from the answer in the KeyValueStore; an error wrapping
// kernel.ErrKeyNotFound when no order waits for it. Only the handshakes that offer acme-tls/1
// alone call it: the other handshakes never read the KeyValueStore (ADR-0163 §3, §6).
func (c *ACMEChallenges) TLSALPN01Certificate(ctx context.Context, host string) (*tls.Certificate, error) {
	host = normalizeChallengeHost(host)
	keyAuth, err := c.kv.Get(ctx, ACMETLSALPN01Namespace, host)
	if err != nil {
		return nil, err
	}
	return TLSALPN01Certificate(host, string(keyAuth))
}

// TLSALPN01Certificate builds the self-signed certificate that answers a TLS-ALPN-01 challenge
// of host with keyAuthorization (RFC 8737 §3): host as its only name, and the SHA-256 digest of
// keyAuthorization in its critical acmeIdentifier extension.
func TLSALPN01Certificate(host, keyAuthorization string) (*tls.Certificate, error) {
	digest := sha256.Sum256([]byte(keyAuthorization))
	value, err := asn1.Marshal(digest[:])
	if err != nil {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "ACME challenge"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{host},
		ExtraExtensions:       []pkix.Extension{{Id: oidACMEIdentifier, Critical: true, Value: value}},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return nil, fmt.Errorf("instance: build the tls-alpn-01 certificate of %s: %w", host, err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, nil
}
