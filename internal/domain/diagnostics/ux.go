// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics

import (
	"math"
	"regexp"
	"slices"
	"time"
)

// UXMetric is what a UX measurement of the web client measures (ADR-0167 §1, §2): the kind of
// time or call quality, and so what its name names.
type UXMetric string

// The metrics of the UX measurements. The durations are milliseconds; the packet loss is a
// percentage.
const (
	// UXLCP is the largest contentful paint of a route, named by the route's ID.
	UXLCP UXMetric = "lcp"
	// UXINP is the interaction to next paint of a route, named by the route's ID.
	UXINP UXMetric = "inp"
	// UXLongFrame is a long animation frame on a route, named by the route's ID.
	UXLongFrame UXMetric = "long_frame"
	// UXTransition is a navigation, named by its interaction kind (UXInteractionKinds).
	UXTransition UXMetric = "transition"
	// UXPress is the time from a press to its response, named by the action ID (post.reply).
	UXPress UXMetric = "press"
	// UXCallRTT, UXCallJitter, and UXCallPacketLoss are the quality of a voice call, named by
	// the implementation of the room session (UXCallTransports).
	UXCallRTT        UXMetric = "call_rtt"
	UXCallJitter     UXMetric = "call_jitter"
	UXCallPacketLoss UXMetric = "call_packet_loss"
)

// UXMetrics are the metrics in the order the admin view lists them.
var UXMetrics = []UXMetric{UXLCP, UXINP, UXLongFrame, UXTransition, UXPress, UXCallRTT, UXCallJitter, UXCallPacketLoss}

// UXInteractionKinds are the names of UXTransition: the interaction kinds of the design system.
var UXInteractionKinds = []string{"push", "pop", "modal", "replace", "none"}

// UXCallTransports are the names of the call metrics: the implementations of the room session.
var UXCallTransports = []string{"p2p", "livekit", "sora"}

// UXOtherName is the name that stands for the names past MaxUXNames of a metric.
const UXOtherName = "other"

// MaxUXNames bounds the names that a process keeps of each metric; the later ones count as
// UXOtherName, so that a client cannot grow the rows and the metric series without bound.
const MaxUXNames = 200

// UXRetention is how long the hourly counts of the UX measurements stay.
const UXRetention = 30 * 24 * time.Hour

var (
	// reUXRoute matches a route ID of the web client's router: a path pattern such as
	// "/_app/statuses/$id", or "__root__".
	reUXRoute = regexp.MustCompile(`^(?:__root__|/[A-Za-z0-9_$./()\[\]{}-]{0,127})$`)
	// reUXAction matches an action ID: dotted lower-case words such as "post.reply".
	reUXAction = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}(?:\.[a-z][a-z0-9_]{0,31}){1,3}$`)
)

// ValidUXName reports whether name is a name of m: a route ID, an interaction kind, an action ID,
// or a call transport. An unknown metric has no valid name.
func ValidUXName(m UXMetric, name string) bool {
	switch m {
	case UXLCP, UXINP, UXLongFrame:
		return reUXRoute.MatchString(name)
	case UXTransition:
		return slices.Contains(UXInteractionKinds, name)
	case UXPress:
		return reUXAction.MatchString(name)
	case UXCallRTT, UXCallJitter, UXCallPacketLoss:
		return slices.Contains(UXCallTransports, name)
	}
	return false
}

// The upper bounds of the buckets of the UX measurements, each bucket holding the values above
// the bound before it. A bucket is stored by its index, so the bounds only grow at the end.
// The durations' bounds hold the "good" and "poor" thresholds of INP (200 ms, 500 ms) and of LCP
// (2.5 s, 4 s); a value past the last bound counts in one more bucket.
var (
	uxDurationBounds = []float64{25, 50, 100, 200, 300, 500, 800, 1000, 1500, 2500, 4000, 6000, 10000}
	uxPercentBounds  = []float64{0, 0.5, 1, 2, 5, 10, 20}
)

// UXBounds returns the upper bounds of the buckets of m: UXBuckets(m) is one more than their
// count, the last bucket having no bound.
func UXBounds(m UXMetric) []float64 {
	if m == UXCallPacketLoss {
		return uxPercentBounds
	}
	return uxDurationBounds
}

// UXBuckets returns the number of buckets of m.
func UXBuckets(m UXMetric) int { return len(UXBounds(m)) + 1 }

// MaxUXValue is the largest value of a measurement, in milliseconds or percent: a value above
// it, below zero, or not a number is not a measurement.
const MaxUXValue = 3_600_000

// ValidUXValue reports whether v is a value of m.
func ValidUXValue(m UXMetric, v float64) bool {
	if math.IsNaN(v) || v < 0 {
		return false
	}
	if m == UXCallPacketLoss {
		return v <= 100
	}
	return v <= MaxUXValue
}

// UXBucket returns the index of the bucket of the value v of m.
func UXBucket(m UXMetric, v float64) int {
	bounds := UXBounds(m)
	for i, b := range bounds {
		if v <= b {
			return i
		}
	}
	return len(bounds)
}

// UXQuantileBound returns the upper bound of the bucket that holds the quantile q (0 < q <= 1)
// of the counts of the buckets of m, and false when there are no counts or the quantile is in
// the last bucket, which has no bound.
func UXQuantileBound(m UXMetric, counts []int64, q float64) (float64, bool) {
	var total int64
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return 0, false
	}
	rank := int64(math.Ceil(q * float64(total)))
	bounds := UXBounds(m)
	var seen int64
	for i, c := range counts {
		seen += c
		if seen >= rank {
			if i < len(bounds) {
				return bounds[i], true
			}
			return 0, false
		}
	}
	return 0, false
}
