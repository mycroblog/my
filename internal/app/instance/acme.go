// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"time"
)

// The ACME challenge types that the server answers (ADR-0163 §3): TLS-ALPN-01 on the TLS listener
// and HTTP-01 on the plain HTTP port. DNS-01 is not answered.
const (
	ACMEChallengeTLSALPN01 = "tls-alpn-01"
	ACMEChallengeHTTP01    = "http-01"
)

// ACMEClient is the port of an ACME certificate authority (RFC 8555) at one directory URL
// (ADR-0163 §2). The adapter lives in internal/adapter/acme and passes
// instancetest.ACMEClientContract. It reaches only the CA that the operator configured, through
// an HTTP client that internal/platform gives it, which follows no redirects.
//
// Every method is safe for concurrent use. An error that the CA answered with a problem document
// wraps an *ACMEProblem.
type ACMEClient interface {
	// DirectoryURL returns the URL of the CA's directory, which names the CA in the records of
	// the accounts, the orders, and the certificates.
	DirectoryURL() string
	// RegisterAccount registers the account of key, agreeing to the CA's terms of service, with
	// contact (mailto: URLs, possibly none), and returns the account's URL. Registering a key
	// that has an account returns that account.
	RegisterAccount(ctx context.Context, key crypto.Signer, contact []string) (string, error)
	// Order places the order of req, answers one challenge of each authorization with
	// req.Solver, using only the types of req.Challenges, finalizes it with a request signed by
	// req.Key, and returns the certificate chain in PEM, the leaf first.
	Order(ctx context.Context, req ACMEOrderRequest) (string, error)
	// Revoke asks the CA to revoke leaf, signed by account, for reason.
	Revoke(ctx context.Context, account ACMEAccount, leaf *x509.Certificate, reason ACMERevocationReason) error
	// RenewalInfo returns the CA's suggested renewal window of leaf (ARI, RFC 9773); an error
	// wrapping ErrACMERenewalInfoUnsupported when the CA offers none.
	RenewalInfo(ctx context.Context, leaf *x509.Certificate) (ACMERenewalInfo, error)
}

// ErrACMERenewalInfoUnsupported reports that the CA does not offer ARI.
var ErrACMERenewalInfoUnsupported = errors.New("instance: the CA offers no renewal information")

// ACMEAccount is a registered account of the CA: its URL and its key.
type ACMEAccount struct {
	URL string
	Key crypto.Signer
}

// ACMEOrderRequest is an order of a certificate for one host.
type ACMEOrderRequest struct {
	Account ACMEAccount
	// Host is the DNS name that the certificate covers.
	Host string
	// Key is the key of the certificate, which signs the certificate request.
	Key crypto.Signer
	// Profile is the CA's profile of the certificate (draft-ietf-acme-profiles); empty is the
	// CA's default.
	Profile string
	// Replaces is the certificate that this one replaces (RFC 9773 §5), nil for a first order.
	// A CA that refuses it, because another order replaced it already, gets the order without
	// it.
	Replaces *x509.Certificate
	// Challenges are the challenge types that may be answered, by preference.
	Challenges []string
	// Solver presents the answers of the challenges and removes them.
	Solver ACMESolver
}

// ACMEChallenge is a challenge of an authorization that the server answers.
type ACMEChallenge struct {
	// Type is ACMEChallengeTLSALPN01 or ACMEChallengeHTTP01.
	Type string
	// Host is the DNS name that the challenge validates.
	Host string
	// Token names the HTTP-01 resource (RFC 8555 §8.3).
	Token string
	// KeyAuthorization is the token and the thumbprint of the account key (RFC 8555 §8.1):
	// the body of the HTTP-01 resource and, hashed, the TLS-ALPN-01 extension.
	KeyAuthorization string
}

// ACMESolver presents the answer of a challenge before the CA validates it, and removes it after.
type ACMESolver interface {
	Present(ctx context.Context, ch ACMEChallenge) error
	CleanUp(ctx context.Context, ch ACMEChallenge) error
}

// ACMERenewalInfo is the renewal window that the CA suggests for a certificate (RFC 9773 §4.2).
type ACMERenewalInfo struct {
	WindowStart time.Time
	WindowEnd   time.Time
	// RetryAfter is when to ask again, zero when the CA did not say.
	RetryAfter time.Time
	// ExplanationURL is a page that explains the window, possibly empty.
	ExplanationURL string
}

// ACMERevocationReason is the reason code of a revocation (RFC 5280 §5.3.1).
type ACMERevocationReason int

// The reason codes that the server sends.
const (
	ACMERevocationUnspecified   ACMERevocationReason = 0
	ACMERevocationKeyCompromise ACMERevocationReason = 1
	ACMERevocationSuperseded    ACMERevocationReason = 4
)

// ACMEProblem is an error that the CA answered with a problem document (RFC 8555 §6.7).
type ACMEProblem struct {
	// Type is the problem's URI, such as urn:ietf:params:acme:error:rejectedIdentifier.
	Type   string
	Detail string
}

func (p *ACMEProblem) Error() string {
	if p.Detail == "" {
		return "acme: " + p.Type
	}
	return "acme: " + p.Type + ": " + p.Detail
}
