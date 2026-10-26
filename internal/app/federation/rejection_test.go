// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
)

// TestRejectionReason checks that a refusal is named from the fixed vocabulary, never with the
// header values that its error quotes.
func TestRejectionReason(t *testing.T) {
	const header = `keyId="https://evil.example/secret-value"`
	quoted := func(sentinel error) error { return fmt.Errorf("%w: %s", sentinel, header) }
	sigErr := func(status int, err error) *appfederation.SignatureError {
		return &appfederation.SignatureError{Status: status, Err: err}
	}
	for _, tc := range []struct {
		name string
		rej  *appfederation.InboxRejection
		want string
		// metric is the my.inbox.rejection of the refusal.
		metric string
	}{
		{"no signature", &appfederation.InboxRejection{Status: http.StatusUnauthorized, Err: sigErr(http.StatusUnauthorized, quoted(appfederation.ErrNoSignature))}, appfederation.ReasonNoSignature, appfederation.InboxRejectionSignature},
		{"malformed", &appfederation.InboxRejection{Status: http.StatusBadRequest, Err: sigErr(http.StatusBadRequest, quoted(appfederation.ErrMalformedSignature))}, appfederation.ReasonMalformedSignature, appfederation.InboxRejectionSignature},
		{"malformed without the sentinel", &appfederation.InboxRejection{Status: http.StatusBadRequest, Err: sigErr(http.StatusBadRequest, errors.New(header))}, appfederation.ReasonMalformedSignature, appfederation.InboxRejectionSignature},
		{"key not found", &appfederation.InboxRejection{Status: http.StatusUnauthorized, Err: sigErr(http.StatusUnauthorized, quoted(appfederation.ErrPublicKeyNotFound))}, appfederation.ReasonPublicKeyNotFound, appfederation.InboxRejectionSignature},
		{"key unavailable", &appfederation.InboxRejection{Status: http.StatusServiceUnavailable, Err: sigErr(http.StatusServiceUnavailable, quoted(appfederation.ErrPublicKeyUnavailable))}, appfederation.ReasonPublicKeyUnavailable, appfederation.InboxRejectionSignature},
		{"mismatch", &appfederation.InboxRejection{Status: http.StatusUnauthorized, Err: sigErr(http.StatusUnauthorized, quoted(appfederation.ErrSignatureMismatch))}, appfederation.ReasonSignatureMismatch, appfederation.InboxRejectionSignature},
		{"policy", &appfederation.InboxRejection{Status: http.StatusUnauthorized, Err: sigErr(http.StatusUnauthorized, errors.New("unparsable Date header: "+header))}, appfederation.ReasonSignaturePolicy, appfederation.InboxRejectionSignature},
		{"rate limited", &appfederation.InboxRejection{Status: http.StatusTooManyRequests, Err: &kernel.RateLimitedError{}}, appfederation.ReasonRateLimited, appfederation.InboxRejectionRateLimited},
		{"suspended", &appfederation.InboxRejection{Status: http.StatusForbidden, Err: errors.New("evil.example is suspended")}, appfederation.ReasonDomainSuspended, appfederation.InboxRejectionSuspended},
		{"federation off", &appfederation.InboxRejection{Status: http.StatusForbidden, Err: fmt.Errorf("%w: mode off", appfederation.ErrFederationRestricted)}, appfederation.ReasonFederationOff, appfederation.InboxRejectionFederationOff},
		{"invalid activity", &appfederation.InboxRejection{Status: http.StatusBadRequest, Err: errors.New(header)}, appfederation.ReasonInvalidActivity, appfederation.InboxRejectionInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rej.Reason(); got != tc.want {
				t.Fatalf("Reason() = %q, want %q", got, tc.want)
			}
			if got := tc.rej.MetricReason(); got != tc.metric {
				t.Errorf("MetricReason() = %q, want %q", got, tc.metric)
			}
		})
	}
}
