// SPDX-License-Identifier: AGPL-3.0-only

package otel_test

import (
	"math"
	"runtime/metrics"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel"
)

func TestHistogramPointsDoNotShareTheRuntimeSlices(t *testing.T) {
	h := &metrics.Float64Histogram{Buckets: []float64{0, 1, 2}, Counts: []uint64{1, 2}}
	dp := otel.HistogramPoints(h, time.Time{}, time.Time{})
	if len(dp) != 1 {
		t.Fatalf("points = %v", dp)
	}
	h.Counts[0], h.Buckets[1] = 99, math.Inf(1)
	if dp[0].BucketCounts[0] != 1 || dp[0].Bounds[0] != 1 {
		t.Errorf("the point changed with the runtime histogram: %v %v", dp[0].BucketCounts, dp[0].Bounds)
	}
}
