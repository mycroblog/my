// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"strings"
	"time"
	"unicode/utf8"
)

// TLSAccount is the ACME account of a CA, one per directory URL (tls_acme_accounts, ADR-0163 §4).
// SealedPrivateKey is the account key in PKCS #8, sealed with the Sealer bound to ID.
type TLSAccount struct {
	ID               string
	DirectoryURL     string
	AccountURL       string
	SealedPrivateKey []byte
	CreatedAt        time.Time
}

// TLSOrderAttempt is an order of a certificate for Host, recorded just before the order
// (tls_order_attempts). Profile is empty for the CA's default; ReplacesCertificateID is empty for
// a first order. Its result is a TLSOrderFailure or a TLSCertificate; an attempt whose process died
// has neither.
type TLSOrderAttempt struct {
	ID                    string
	Host                  string
	DirectoryURL          string
	Profile               string
	ReplacesCertificateID string
	OccurredAt            time.Time
}

// MaxTLSOrderFailureDetail is the longest detail of a failed order that is kept, in characters.
const MaxTLSOrderFailureDetail = 1000

// TLSOrderFailure is the failure of the order AttemptID (tls_order_failures). ProblemType is the
// type of the RFC 8555 problem, or the class of a failure on the client's side.
type TLSOrderFailure struct {
	AttemptID   string
	ProblemType string
	Detail      string
	OccurredAt  time.Time
}

// The classes of the failures of an order on the client's side, recorded as the ProblemType of a
// TLSOrderFailure when the CA sent no problem document.
const (
	// TLSOrderFailureClient is a failure without a problem from the CA: the CA could not be
	// reached, or its answer could not be used.
	TLSOrderFailureClient = "client"
	// TLSOrderFailureCanceled is an order stopped before its end: the process shut down, or it
	// lost the lease of the order.
	TLSOrderFailureCanceled = "canceled"
)

// TruncateTLSOrderFailureDetail returns detail cut to MaxTLSOrderFailureDetail characters, with
// invalid UTF-8 replaced.
func TruncateTLSOrderFailureDetail(detail string) string {
	detail = strings.ToValidUTF8(detail, "\uFFFD")
	if utf8.RuneCountInString(detail) <= MaxTLSOrderFailureDetail {
		return detail
	}
	return string([]rune(detail)[:MaxTLSOrderFailureDetail])
}

// TLSCertificate is a certificate issued for Host by the order AttemptID (tls_certificates),
// inserted once and never changed. ChainPEM is the leaf first, then its intermediates;
// SealedPrivateKey is its key in PKCS #8, sealed with the Sealer bound to ID.
type TLSCertificate struct {
	ID               string
	AttemptID        string
	Host             string
	DirectoryURL     string
	Serial           string
	NotBefore        time.Time
	NotAfter         time.Time
	ChainPEM         string
	SealedPrivateKey []byte
	CreatedAt        time.Time
}

// TLSRevocationReason is why a certificate was revoked.
type TLSRevocationReason string

// The reasons of a revocation.
const (
	TLSRevocationKeyCompromise TLSRevocationReason = "key_compromise"
	TLSRevocationSuperseded    TLSRevocationReason = "superseded"
	TLSRevocationOperator      TLSRevocationReason = "operator"
)

// Valid reports whether r is a known reason.
func (r TLSRevocationReason) Valid() bool {
	switch r {
	case TLSRevocationKeyCompromise, TLSRevocationSuperseded, TLSRevocationOperator:
		return true
	}
	return false
}

// TLSCertificateRevocation is the revocation of CertificateID after the CA revoked it
// (tls_certificate_revocations). ByUserID is empty for the CLI.
type TLSCertificateRevocation struct {
	ID            string
	CertificateID string
	Reason        TLSRevocationReason
	ByUserID      string
	OccurredAt    time.Time
}

// TLSOrderRetryDelay returns how long to wait after the last of n consecutive attempts that have
// no certificate before the next one (ADR-0163 §7): 1 minute after the first, 10 after the
// second, 100 after the third, and then a day. n = 0 waits for nothing.
func TLSOrderRetryDelay(n int) time.Duration {
	switch {
	case n <= 0:
		return 0
	case n == 1:
		return time.Minute
	case n == 2:
		return 10 * time.Minute
	case n == 3:
		return 100 * time.Minute
	default:
		return 24 * time.Hour
	}
}

// TLSOrderBackoff is the wait before the next order of a host, derived from the attempts after
// its current certificate that have no certificate (ADR-0163 §7).
type TLSOrderBackoff struct {
	// Failures is the number of those attempts: failed, or never finished because their process
	// died.
	Failures int
	// NotBefore is the earliest time of the next order; zero when nothing holds it back.
	NotBefore time.Time
}

// NewTLSOrderBackoff derives the backoff from the times of the attempts without a certificate
// after the current certificate, in any order.
func NewTLSOrderBackoff(unresulted []time.Time) TLSOrderBackoff {
	if len(unresulted) == 0 {
		return TLSOrderBackoff{}
	}
	last := unresulted[0]
	for _, at := range unresulted[1:] {
		if at.After(last) {
			last = at
		}
	}
	return TLSOrderBackoff{Failures: len(unresulted), NotBefore: last.Add(TLSOrderRetryDelay(len(unresulted)))}
}

// Allows reports whether an order may start at now.
func (b TLSOrderBackoff) Allows(now time.Time) bool { return !now.Before(b.NotBefore) }
