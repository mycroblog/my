// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics_test

import (
	"math"
	"testing"

	"github.com/mycroblog/my/internal/domain/diagnostics"
)

func TestValidUXName(t *testing.T) {
	cases := []struct {
		metric diagnostics.UXMetric
		name   string
		ok     bool
	}{
		{diagnostics.UXLCP, "/_app/statuses/$id", true},
		{diagnostics.UXINP, "__root__", true},
		{diagnostics.UXLongFrame, "/", true},
		{diagnostics.UXLCP, "https://example.com/", false},
		{diagnostics.UXLCP, "/statuses/1 2", false},
		{diagnostics.UXTransition, "push", true},
		{diagnostics.UXTransition, "slide", false},
		{diagnostics.UXPress, "post.reply", true},
		{diagnostics.UXPress, "post", false},
		{diagnostics.UXPress, "Post.Reply", false},
		{diagnostics.UXCallRTT, "livekit", true},
		{diagnostics.UXCallPacketLoss, "webrtc", false},
		{diagnostics.UXMetric("cls"), "/", false},
	}
	for _, c := range cases {
		if got := diagnostics.ValidUXName(c.metric, c.name); got != c.ok {
			t.Errorf("ValidUXName(%s, %q) = %v, want %v", c.metric, c.name, got, c.ok)
		}
	}
}

func TestValidUXValue(t *testing.T) {
	cases := []struct {
		metric diagnostics.UXMetric
		value  float64
		ok     bool
	}{
		{diagnostics.UXINP, 0, true},
		{diagnostics.UXINP, diagnostics.MaxUXValue, true},
		{diagnostics.UXINP, diagnostics.MaxUXValue + 1, false},
		{diagnostics.UXINP, -1, false},
		{diagnostics.UXINP, math.NaN(), false},
		{diagnostics.UXINP, math.Inf(1), false},
		{diagnostics.UXCallPacketLoss, 100, true},
		{diagnostics.UXCallPacketLoss, 100.5, false},
	}
	for _, c := range cases {
		if got := diagnostics.ValidUXValue(c.metric, c.value); got != c.ok {
			t.Errorf("ValidUXValue(%s, %v) = %v, want %v", c.metric, c.value, got, c.ok)
		}
	}
}

func TestUXBucket(t *testing.T) {
	cases := []struct {
		metric diagnostics.UXMetric
		value  float64
		bucket int
	}{
		{diagnostics.UXINP, 0, 0},
		{diagnostics.UXINP, 25, 0},
		{diagnostics.UXINP, 25.1, 1},
		{diagnostics.UXINP, 200, 3},
		{diagnostics.UXLCP, 2500, 9},
		{diagnostics.UXLCP, 10001, 13},
		{diagnostics.UXCallPacketLoss, 0, 0},
		{diagnostics.UXCallPacketLoss, 0.1, 1},
		{diagnostics.UXCallPacketLoss, 100, 7},
	}
	for _, c := range cases {
		if got := diagnostics.UXBucket(c.metric, c.value); got != c.bucket {
			t.Errorf("UXBucket(%s, %v) = %d, want %d", c.metric, c.value, got, c.bucket)
		}
		if c.bucket >= diagnostics.UXBuckets(c.metric) {
			t.Errorf("bucket %d of %s is past UXBuckets", c.bucket, c.metric)
		}
	}
}

func TestUXQuantileBound(t *testing.T) {
	counts := make([]int64, diagnostics.UXBuckets(diagnostics.UXINP))
	if _, ok := diagnostics.UXQuantileBound(diagnostics.UXINP, counts, 0.75); ok {
		t.Error("a quantile of no counts")
	}
	counts[2], counts[3] = 3, 1 // three at most 100 ms, one at most 200 ms
	if b, ok := diagnostics.UXQuantileBound(diagnostics.UXINP, counts, 0.75); !ok || b != 100 {
		t.Errorf("p75 = %v, %v; want 100", b, ok)
	}
	counts[13] = 4 // four past the last bound
	if _, ok := diagnostics.UXQuantileBound(diagnostics.UXINP, counts, 0.75); ok {
		t.Error("a bound for a quantile in the last bucket")
	}
}
