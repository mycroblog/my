// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/pkg/testkit"
)

// fedListsKnownServersForOperators lists every known server through Health: unfiltered, those
// seen in a document come with those that have a health row; troubled keeps the latter; the
// state keeps a server without health as available; and a server never known is found
// available.
func fedListsKnownServersForOperators(t *testing.T, e *fedEnv) {
	d := newDeliveryEnv(t, e)
	ctx := t.Context()
	now := e.clock.Now()
	tx := func(fn func(ctx context.Context, tx uow.Tx) error) {
		t.Helper()
		if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return fn(ctx, tx) }); err != nil {
			t.Fatal(err)
		}
	}
	seen := func(domain string, at time.Time) {
		t.Helper()
		tx(func(ctx context.Context, tx uow.Tx) error {
			_, err := e.h.Instances.InsertRemoteInstance(ctx, tx, domain, at)
			return err
		})
	}
	// a.example and d.example are seen only, b.example is known only from its health, and
	// c.example is seen and failing.
	seen("a.example", now.Add(-3*time.Hour))
	seen("c.example", now.Add(-2*time.Hour))
	seen("d.example", now.Add(-time.Hour))
	e.markUnavailable(t, "b.example")
	tx(func(ctx context.Context, tx uow.Tx) error {
		return e.h.Federation.UpsertInstanceHealth(ctx, tx, federation.InstanceHealth{
			Domain: "c.example", State: federation.InstanceAvailable, Since: now, FailureDays: 2, LastFailureDay: "20261016",
		})
	})
	software := map[string]federation.NodeInfoObservation{}
	for _, domain := range []string{"a.example", "c.example"} {
		o := federation.NodeInfoObservation{ID: e.ids.NewID(), Domain: domain, SoftwareName: "mastodon", SoftwareVersion: "4.3.1", ObservedAt: now}
		tx(func(ctx context.Context, tx uow.Tx) error { return e.h.Instances.InsertNodeInfoObservation(ctx, tx, o) })
		software[domain] = o
	}

	domains := func(ks []federation.KnownInstance) []string {
		out := []string{}
		for _, k := range ks {
			out = append(out, k.Domain)
		}
		return out
	}
	for _, tc := range []struct {
		filter appfederation.InstanceFilter
		after  string
		limit  int
		want   []string
	}{
		{appfederation.InstanceFilter{}, "", 10, []string{"a.example", "b.example", "c.example", "d.example"}},
		{appfederation.InstanceFilter{}, "a.example", 2, []string{"b.example", "c.example"}},
		{appfederation.InstanceFilter{Troubled: true}, "", 10, []string{"b.example", "c.example"}},
		{appfederation.InstanceFilter{State: federation.InstanceAvailable}, "", 10, []string{"a.example", "c.example", "d.example"}},
		{appfederation.InstanceFilter{State: federation.InstanceAvailable}, "c.example", 10, []string{"d.example"}},
		{appfederation.InstanceFilter{State: federation.InstanceAvailable, Troubled: true}, "", 10, []string{"c.example"}},
		{appfederation.InstanceFilter{State: federation.InstanceUnavailable}, "", 10, []string{"b.example"}},
		{appfederation.InstanceFilter{State: federation.InstanceUnavailable, Troubled: true}, "", 10, []string{"b.example"}},
	} {
		ks, err := d.health.List(ctx, tc.filter, tc.after, tc.limit)
		if err != nil || !slices.Equal(domains(ks), tc.want) {
			t.Errorf("List(%+v, %q, %d) = %v, %v; want %v", tc.filter, tc.after, tc.limit, domains(ks), err, tc.want)
		}
	}
	for _, f := range []appfederation.InstanceFilter{{State: "gone"}, {State: "gone", Troubled: true}} {
		if _, err := d.health.List(ctx, f, "", 10); !errors.Is(err, appfederation.ErrInvalidInstanceState) {
			t.Errorf("List(%+v) = %v, want ErrInvalidInstanceState", f, err)
		}
	}

	// Each server carries its sighting and its software, listed or found.
	all, err := d.health.List(ctx, appfederation.InstanceFilter{}, "", 10)
	if err != nil || len(all) != 4 {
		t.Fatalf("List(every server) = %v, %v", domains(all), err)
	}
	if a := all[0]; a.State != federation.InstanceAvailable || !a.FirstSeenAt.Equal(now.Add(-3*time.Hour)) || a.Software.ID != software["a.example"].ID {
		t.Errorf("a.example listed = %+v, want available with its sighting and software", a)
	}
	if b := all[1]; b.State != federation.InstanceUnavailable || !b.FirstSeenAt.IsZero() || b.Software.ID != "" {
		t.Errorf("b.example listed = %+v, want unavailable without a sighting or software", b)
	}
	if c, err := d.health.Find(ctx, "c.example"); err != nil || c.FailureDays != 2 || !c.FirstSeenAt.Equal(now.Add(-2*time.Hour)) || c.Software.ID != software["c.example"].ID {
		t.Errorf("Find(c.example) = %+v, %v; want its failures, sighting and software", c, err)
	}
	u, err := d.health.Find(ctx, "unknown.example")
	if err != nil || u.Domain != "unknown.example" || u.State != federation.InstanceAvailable || !u.Since.IsZero() || u.FailureDays != 0 ||
		u.LastFailureDay != "" || !u.FirstSeenAt.IsZero() || u.Software.ID != "" {
		t.Errorf("Find(unknown.example) = %+v, %v; want available with zero fields and no error", u, err)
	}
	if c, err := d.health.MarkAvailable(ctx, "c.example", testkit.NamedID("operator")); err != nil || c.FailureDays != 0 ||
		!c.FirstSeenAt.Equal(now.Add(-2*time.Hour)) || c.Software.ID != software["c.example"].ID {
		t.Errorf("MarkAvailable(c.example) = %+v, %v; want cleared, with its sighting and software", c, err)
	}
}
