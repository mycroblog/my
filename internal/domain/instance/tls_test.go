// SPDX-License-Identifier: AGPL-3.0-only

package instance_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/domain/instance"
)

func TestTLSOrderBackoff(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		attempts int
		wait     time.Duration
	}{
		{0, 0},
		{1, time.Minute},
		{2, 10 * time.Minute},
		{3, 100 * time.Minute},
		{4, 24 * time.Hour},
		{5, 24 * time.Hour},
	}
	for _, tc := range cases {
		var times []time.Time
		for i := range tc.attempts {
			times = append(times, start.Add(time.Duration(i)*time.Hour))
		}
		// The order of the attempts does not matter; the last one counts.
		for i, j := 0, len(times)-1; i < j; i, j = i+1, j-1 {
			times[i], times[j] = times[j], times[i]
		}
		b := instance.NewTLSOrderBackoff(times)
		if got := instance.TLSOrderRetryDelay(tc.attempts); got != tc.wait {
			t.Errorf("TLSOrderRetryDelay(%d) = %s, want %s", tc.attempts, got, tc.wait)
		}
		if b.Failures != tc.attempts {
			t.Errorf("NewTLSOrderBackoff(%d attempts).Failures = %d", tc.attempts, b.Failures)
		}
		if tc.attempts == 0 {
			if !b.NotBefore.IsZero() || !b.Allows(start) {
				t.Errorf("NewTLSOrderBackoff(none) = %+v, want no wait", b)
			}
			continue
		}
		last := start.Add(time.Duration(tc.attempts-1) * time.Hour)
		if want := last.Add(tc.wait); !b.NotBefore.Equal(want) || b.Allows(want.Add(-time.Nanosecond)) || !b.Allows(want) {
			t.Errorf("NewTLSOrderBackoff(%d attempts) = %+v, want no order before %s", tc.attempts, b, want)
		}
	}
}

func TestTruncateTLSOrderFailureDetail(t *testing.T) {
	t.Parallel()
	if got := instance.TruncateTLSOrderFailureDetail("short"); got != "short" {
		t.Errorf("TruncateTLSOrderFailureDetail(short) = %q", got)
	}
	long := strings.Repeat("あ", instance.MaxTLSOrderFailureDetail+1)
	if got := instance.TruncateTLSOrderFailureDetail(long); utf8.RuneCountInString(got) != instance.MaxTLSOrderFailureDetail {
		t.Errorf("TruncateTLSOrderFailureDetail of %d characters kept %d", instance.MaxTLSOrderFailureDetail+1, utf8.RuneCountInString(got))
	}
	if got := instance.TruncateTLSOrderFailureDetail("a\xffb"); !utf8.ValidString(got) {
		t.Errorf("TruncateTLSOrderFailureDetail(invalid UTF-8) = %q, want valid UTF-8", got)
	}
}

func TestTLSRevocationReasonValid(t *testing.T) {
	t.Parallel()
	for _, r := range []instance.TLSRevocationReason{instance.TLSRevocationKeyCompromise, instance.TLSRevocationSuperseded, instance.TLSRevocationOperator} {
		if !r.Valid() {
			t.Errorf("%q.Valid() = false", r)
		}
	}
	if instance.TLSRevocationReason("lost").Valid() {
		t.Error(`"lost".Valid() = true`)
	}
}
