// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"crypto/x509"
	"testing"
	"time"
)

// The CA's Retry-After is clamped to between a minute and a day, and the renewal information is
// read again within 12 hours at most (ADR-0163 §7).
func TestRenewalInfoWait(t *testing.T) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		retryAfter time.Time
		want       time.Duration
	}{
		{time.Time{}, 12 * time.Hour},
		{now.Add(-time.Hour), time.Minute},
		{now.Add(time.Second), time.Minute},
		{now.Add(6 * time.Hour), 6 * time.Hour},
		{now.Add(13 * time.Hour), 12 * time.Hour},
		{now.Add(72 * time.Hour), 12 * time.Hour},
	} {
		if got := renewalInfoWait(now, tt.retryAfter); got != tt.want {
			t.Errorf("renewalInfoWait(%v) = %v, want %v", tt.retryAfter.Sub(now), got, tt.want)
		}
	}
}

// Without ARI, a certificate is renewed after two thirds of its lifetime, or after half of it
// when it is valid under 10 days (ADR-0163 §7).
func TestLifetimeRenewalTime(t *testing.T) {
	start := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		life, want time.Duration
	}{
		{90 * 24 * time.Hour, 60 * 24 * time.Hour},
		{45 * 24 * time.Hour, 30 * 24 * time.Hour},
		{10 * 24 * time.Hour, 10 * 24 * time.Hour * 2 / 3},
		{10*24*time.Hour - time.Second, (10*24*time.Hour - time.Second) / 2},
		{6 * 24 * time.Hour, 3 * 24 * time.Hour},
	} {
		leaf := &x509.Certificate{NotBefore: start, NotAfter: start.Add(tt.life)}
		if got := lifetimeRenewalTime(leaf); !got.Equal(start.Add(tt.want)) {
			t.Errorf("the renewal time of %v is %v after the start, want %v", tt.life, got.Sub(start), tt.want)
		}
	}
}
