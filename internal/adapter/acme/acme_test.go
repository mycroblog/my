// SPDX-License-Identifier: AGPL-3.0-only

package acme_test

import (
	"context"
	"crypto/tls"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/acme"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/pkg/testkit/instancetest"
	"github.com/mycroblog/my/pkg/testkit/pebbletest"
)

// The contract of the CA against Pebble, Let's Encrypt's test server (TEST_ACME=pebble,
// scripts/task.sh go:test-acme).
func TestPebbleACMEClientContract(t *testing.T) {
	t.Parallel()
	instancetest.ACMEClientContract(t, func(t *testing.T) instancetest.ACMEHarness {
		srv := pebbletest.New(t, pebbletest.Options{})
		c, err := acme.New(acme.Options{DirectoryURL: srv.DirectoryURL, HTTPClient: srv.HTTPClient(), PollInterval: 100 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		// The listeners answer from the challenges of the order in progress.
		var current atomic.Pointer[appinstance.ACMEChallenges]
		errNone := errors.New("no order in progress")
		srv.Answer(t, func(ctx context.Context, token string) (string, error) {
			if ch := current.Load(); ch != nil {
				return ch.HTTP01(ctx, token)
			}
			return "", errNone
		}, func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if ch := current.Load(); ch != nil {
				return ch.TLSALPN01Certificate(hello.Context(), hello.ServerName)
			}
			return nil, errNone
		})
		return instancetest.ACMEHarness{
			Client: c,
			Answer: func(_ *testing.T, ch *appinstance.ACMEChallenges) { current.Store(ch) },
			Roots:  srv.Roots(t), RejectedHost: pebbletest.BlockedHost, Profile: pebbletest.ShortProfile,
			ProfileValidity: pebbletest.ShortValidity,
		}
	})
}

func TestNewNeedsADirectoryAndAClient(t *testing.T) {
	t.Parallel()
	if _, err := acme.New(acme.Options{}); err == nil {
		t.Error("New without a directory = nil, want an error")
	}
	if _, err := acme.New(acme.Options{DirectoryURL: "https://ca.example/dir"}); err == nil {
		t.Error("New without an HTTP client = nil, want an error")
	}
}
