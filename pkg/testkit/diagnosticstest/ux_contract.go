// SPDX-License-Identifier: AGPL-3.0-only

package diagnosticstest

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/domain/diagnostics"
	"github.com/mycroblog/my/pkg/testkit"
)

// UXFactory returns the UX repository of a new, empty database. The factory registers the
// cleanup of everything it creates with t.Cleanup.
type UXFactory func(t *testing.T) appdiagnostics.UXRepository

// UXRepositoryContract verifies the UX repository of a database adapter through
// appdiagnostics.UX (ADR-0167 §3): the valid measurements are counted by hour, metric, name, and
// bucket, each once also when the flushes of several processes add to the same row; the
// summaries sum the hours of their window, by metric and then by most samples; a process keeps
// the first diagnostics.MaxUXNames names of a metric and counts the later ones as
// diagnostics.UXOtherName, in the counts and in the metrics, and takes nothing after Close; and
// the counts older than diagnostics.UXRetention are pruned.
func UXRepositoryContract(t *testing.T, newRepo UXFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, repo appdiagnostics.UXRepository)
	}{
		{"CountsAndSums", uxCountsAndSums},
		{"KeepsTheFirstNamesOfAMetric", uxNames},
		{"PrunesOldCounts", uxPrune},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newRepo(t)) })
	}
}

func newUX(t *testing.T, repo appdiagnostics.UXRepository, clock *testkit.Clock) *appdiagnostics.UX {
	t.Helper()
	u := appdiagnostics.NewUX(appdiagnostics.UXOptions{Repository: repo, Clock: clock, Meter: testkit.NewMeter(), Manual: true})
	t.Cleanup(func() { _ = u.Close(context.Background()) })
	return u
}

func uxCountsAndSums(t *testing.T, repo appdiagnostics.UXRepository) {
	ctx := t.Context()
	clock := testkit.NewClock(time.Date(2026, 11, 12, 10, 30, 0, 0, time.UTC))
	a, b := newUX(t, repo, clock), newUX(t, repo, clock)
	a.Record(ctx, []appdiagnostics.UXMeasurement{
		{Metric: diagnostics.UXINP, Name: "/_app/home", Value: 80},
		{Metric: diagnostics.UXINP, Name: "/_app/home", Value: 150},
		{Metric: diagnostics.UXPress, Name: "post.reply", Value: 40},
		{Metric: diagnostics.UXPress, Name: "https://example.com/", Value: 40}, // not a name: dropped
		{Metric: diagnostics.UXINP, Name: "/_app/home", Value: -1},             // not a value: dropped
	})
	b.Record(ctx, []appdiagnostics.UXMeasurement{{Metric: diagnostics.UXINP, Name: "/_app/home", Value: 90}})
	if err := a.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := b.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	clock.Advance(time.Hour)
	b.Record(ctx, []appdiagnostics.UXMeasurement{{Metric: diagnostics.UXINP, Name: "/_app/home", Value: 5000}})
	if err := b.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	got, err := a.Summaries(ctx, clock.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("Summaries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("summaries = %+v, want inp and press", got)
	}
	inp, press := got[0], got[1]
	wantINP := make([]int64, diagnostics.UXBuckets(diagnostics.UXINP))
	wantINP[diagnostics.UXBucket(diagnostics.UXINP, 80)] = 2 // 80 and 90
	wantINP[diagnostics.UXBucket(diagnostics.UXINP, 150)] = 1
	wantINP[diagnostics.UXBucket(diagnostics.UXINP, 5000)] = 1
	if inp.Metric != diagnostics.UXINP || inp.Name != "/_app/home" || inp.Samples != 4 || !slices.Equal(inp.Buckets, wantINP) {
		t.Errorf("inp = %+v, want 4 samples in %v", inp, wantINP)
	}
	if press.Metric != diagnostics.UXPress || press.Name != "post.reply" || press.Samples != 1 {
		t.Errorf("press = %+v", press)
	}

	// The window starts at the hour of since: the last hour only.
	got, err = a.Summaries(ctx, clock.Now())
	if err != nil {
		t.Fatalf("Summaries: %v", err)
	}
	if len(got) != 1 || got[0].Samples != 1 {
		t.Errorf("summaries of the last hour = %+v, want the one sample", got)
	}
}

func uxNames(t *testing.T, repo appdiagnostics.UXRepository) {
	ctx := t.Context()
	meter := testkit.NewMeter()
	u := appdiagnostics.NewUX(appdiagnostics.UXOptions{
		Repository: repo, Clock: testkit.NewClock(time.Date(2026, 11, 12, 10, 30, 0, 0, time.UTC)), Meter: meter, Manual: true,
	})
	ms := make([]appdiagnostics.UXMeasurement, diagnostics.MaxUXNames+5)
	for i := range ms {
		ms[i] = appdiagnostics.UXMeasurement{Metric: diagnostics.UXPress, Name: fmt.Sprintf("action.n%d", i), Value: 10}
	}
	u.Record(ctx, ms)
	u.Record(ctx, []appdiagnostics.UXMeasurement{{Metric: diagnostics.UXCallPacketLoss, Name: "sora", Value: 1.5}})
	if err := u.Close(ctx); err != nil {
		t.Fatal(err)
	}
	sums, err := u.Summaries(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != diagnostics.MaxUXNames+2 {
		t.Fatalf("%d summaries, want %d names, other, and the call", len(sums), diagnostics.MaxUXNames+2)
	}
	if sums[0].Name != diagnostics.UXOtherName || sums[0].Samples != 5 {
		t.Errorf("first summary = %+v, want the 5 samples of other", sums[0])
	}
	if got := len(meter.Measurements("my.ux.duration", map[string]string{"my.ux.name": diagnostics.UXOtherName})); got != 5 {
		t.Errorf("my.ux.duration of other = %d measurements, want 5", got)
	}
	if got := meter.Sum("my.ux.call.packet_loss", map[string]string{"my.ux.name": "sora"}); got != 1.5 {
		t.Errorf("my.ux.call.packet_loss = %v, want 1.5", got)
	}
	if err := meter.Err(); err != nil {
		t.Error(err)
	}
	u.Record(ctx, ms[:1])
	if err := u.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if after, _ := u.Summaries(ctx, time.Time{}); len(after) != len(sums) || after[1].Samples != 1 {
		t.Errorf("recorded after Close: %d summaries", len(after))
	}
}

func uxPrune(t *testing.T, repo appdiagnostics.UXRepository) {
	ctx := t.Context()
	clock := testkit.NewClock(time.Date(2026, 11, 12, 10, 30, 0, 0, time.UTC))
	u := newUX(t, repo, clock)
	m := []appdiagnostics.UXMeasurement{{Metric: diagnostics.UXLCP, Name: "/_app/home", Value: 1200}}
	u.Record(ctx, m)
	if err := u.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	clock.Advance(diagnostics.UXRetention + 2*time.Hour)
	u.Record(ctx, m)
	if err := u.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	n, err := u.PruneCounts(ctx)
	if err != nil || n != 1 {
		t.Errorf("PruneCounts = %d, %v; want 1", n, err)
	}
	got, err := u.Summaries(ctx, time.Time{})
	if err != nil || len(got) != 1 || got[0].Samples != 1 {
		t.Errorf("summaries after the prune = %+v, %v", got, err)
	}
	if n, err := u.PruneCounts(ctx); err != nil || n != 0 {
		t.Errorf("second prune: %d, %v", n, err)
	}
}
