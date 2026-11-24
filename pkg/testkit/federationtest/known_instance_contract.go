// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// knownServers is the fixture of the known-server reads of FederationRepositoryContract, every
// server in domain order as the unfiltered list returns it:
//
//   - a.example: seen in a document only, without software;
//   - b.example: seen and failing, with two NodeInfo observations, the newer recorded first;
//   - c.example: unavailable, known only from its health;
//   - d.example: a health row without failures, known only from it;
//   - e.example: seen only, its software reporting no version;
//   - f.example: seen and unavailable.
type knownServers struct {
	all []federation.KnownInstance
	// health are the servers with a health row, as ListInstanceHealth returns them.
	health []federation.KnownInstance
}

func (r *fedRepo) knownServers(t *testing.T) knownServers {
	t.Helper()
	now := r.clock.Now()
	day := federation.FailureDay(now)
	seen := func(domain string, at time.Time) {
		t.Helper()
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Instances.InsertRemoteInstance(ctx, tx, domain, at)
			if err == nil && !ok {
				t.Errorf("InsertRemoteInstance(%s) = false, want true", domain)
			}
			return err
		})
	}
	health := func(h federation.InstanceHealth) {
		t.Helper()
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Federation.UpsertInstanceHealth(ctx, tx, h) })
	}
	observe := func(o federation.NodeInfoObservation) {
		t.Helper()
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Instances.InsertNodeInfoObservation(ctx, tx, o) })
	}

	seenA, seenB, seenE, seenF := now.Add(-4*time.Hour), now.Add(-3*time.Hour), now.Add(-2*time.Hour), now.Add(-time.Hour)
	seen("a.example", seenA)
	seen("b.example", seenB)
	seen("e.example", seenE)
	seen("f.example", seenF)

	failing := federation.InstanceHealth{Domain: "b.example", State: federation.InstanceAvailable, Since: now, FailureDays: 2, LastFailureDay: day}
	down := federation.InstanceHealth{Domain: "c.example", State: federation.InstanceUnavailable, Since: now, FailureDays: 7, LastFailureDay: day}
	healthy := federation.NewInstanceHealth("d.example", now)
	downToo := federation.InstanceHealth{Domain: "f.example", State: federation.InstanceUnavailable, Since: now, FailureDays: 8, LastFailureDay: day}
	for _, h := range []federation.InstanceHealth{downToo, healthy, failing, down} {
		health(h)
	}

	// The newer observation of b.example has the higher ID but is recorded first, so that the
	// order of the rows does not decide which one is the newest.
	olderID, newerID := r.ids.NewID(), r.ids.NewID()
	newer := federation.NodeInfoObservation{ID: newerID, Domain: "b.example", SoftwareName: "mastodon", SoftwareVersion: "4.3.1", ObservedAt: now.Add(-time.Minute)}
	observe(newer)
	observe(federation.NodeInfoObservation{ID: olderID, Domain: "b.example", SoftwareName: "mastodon", SoftwareVersion: "4.2.0", ObservedAt: now.Add(-2 * time.Minute)})
	versionless := federation.NodeInfoObservation{ID: r.ids.NewID(), Domain: "e.example", SoftwareName: "misskey", ObservedAt: now.Add(-time.Minute)}
	observe(versionless)

	a := federation.KnownInstance{InstanceHealth: federation.InstanceHealth{Domain: "a.example", State: federation.InstanceAvailable}, FirstSeenAt: seenA}
	b := federation.KnownInstance{InstanceHealth: failing, FirstSeenAt: seenB, Software: newer}
	c := federation.KnownInstance{InstanceHealth: down}
	d := federation.KnownInstance{InstanceHealth: healthy}
	e := federation.KnownInstance{InstanceHealth: federation.InstanceHealth{Domain: "e.example", State: federation.InstanceAvailable}, FirstSeenAt: seenE, Software: versionless}
	f := federation.KnownInstance{InstanceHealth: downToo, FirstSeenAt: seenF}
	return knownServers{all: []federation.KnownInstance{a, b, c, d, e, f}, health: []federation.KnownInstance{b, c, d, f}}
}

// of returns the servers of ks with the domains.
func of(ks []federation.KnownInstance, domains ...string) []federation.KnownInstance {
	var out []federation.KnownInstance
	for _, k := range ks {
		if slices.Contains(domains, k.Domain) {
			out = append(out, k)
		}
	}
	return out
}

// sameKnown reports whether got is want, times compared as instants. The Since of a server
// without a health row (want.Since zero) is not compared: nothing says what it is.
func sameKnown(got, want federation.KnownInstance) bool {
	g, w := got.InstanceHealth, want.InstanceHealth
	if w.Since.IsZero() {
		g.Since = time.Time{}
	}
	gs, ws := got.Software, want.Software
	return g.Domain == w.Domain && g.State == w.State && g.Since.Equal(w.Since) && g.FailureDays == w.FailureDays &&
		g.LastFailureDay == w.LastFailureDay && got.FirstSeenAt.Equal(want.FirstSeenAt) &&
		gs.ID == ws.ID && gs.Domain == ws.Domain && gs.SoftwareName == ws.SoftwareName &&
		gs.SoftwareVersion == ws.SoftwareVersion && gs.ObservedAt.Equal(ws.ObservedAt)
}

func sameKnownList(got, want []federation.KnownInstance) bool {
	return slices.EqualFunc(got, want, sameKnown)
}

func domainsOf(ks []federation.KnownInstance) string {
	var ds []string
	for _, k := range ks {
		ds = append(ds, k.Domain)
	}
	return "[" + strings.Join(ds, " ") + "]"
}

// fedRepoKnownInstances lists every known server, those with a health row as ListInstanceHealth
// lists them, and both by state, a server without a health row being available.
func fedRepoKnownInstances(t *testing.T, r *fedRepo) {
	ctx := t.Context()
	ks := r.knownServers(t)
	check := func(name string, got []federation.KnownInstance, err error, want []federation.KnownInstance) {
		t.Helper()
		if err != nil || !sameKnownList(got, want) {
			t.Errorf("%s = %s %+v, %v;\nwant %s %+v", name, domainsOf(got), got, err, domainsOf(want), want)
		}
	}

	// Unfiltered: the servers seen only (a, e) and those known only from their health (c, d) are
	// listed with those that have both, once each, in domain order.
	got, err := r.h.Federation.ListKnownInstances(ctx, "", "", 10)
	check("ListKnownInstances(every server)", got, err, ks.all)

	// The health rows, joined with the sighting and the newest software of their server.
	got, err = r.h.Federation.ListInstanceHealth(ctx, "", "", 10)
	check("ListInstanceHealth(every state)", got, err, ks.health)
	got, err = r.h.Federation.ListInstanceHealth(ctx, federation.InstanceAvailable, "", 10)
	check("ListInstanceHealth(available)", got, err, of(ks.all, "b.example", "d.example"))

	// Unavailable is a health row's state alone: both lists agree.
	unavailable := of(ks.all, "c.example", "f.example")
	got, err = r.h.Federation.ListInstanceHealth(ctx, federation.InstanceUnavailable, "", 10)
	check("ListInstanceHealth(unavailable)", got, err, unavailable)
	got, err = r.h.Federation.ListKnownInstances(ctx, federation.InstanceUnavailable, "", 10)
	check("ListKnownInstances(unavailable)", got, err, unavailable)

	// Available: the servers without a health row and the available health rows.
	got, err = r.h.Federation.ListKnownInstances(ctx, federation.InstanceAvailable, "", 10)
	check("ListKnownInstances(available)", got, err, of(ks.all, "a.example", "b.example", "d.example", "e.example"))
}

// fedRepoKnownInstancePages pages the known servers by their last domain, each page strictly
// after the cursor, neither repeating nor skipping a server, with and without a state.
func fedRepoKnownInstancePages(t *testing.T, r *fedRepo) {
	ctx := t.Context()
	ks := r.knownServers(t)
	for _, c := range []struct {
		state federation.InstanceState
		want  []federation.KnownInstance
	}{
		{"", ks.all},
		{federation.InstanceAvailable, of(ks.all, "a.example", "b.example", "d.example", "e.example")},
		{federation.InstanceUnavailable, of(ks.all, "c.example", "f.example")},
	} {
		for _, limit := range []int{1, 2, 4} {
			var pages []federation.KnownInstance
			after := ""
			for range len(c.want) + 1 {
				page, err := r.h.Federation.ListKnownInstances(ctx, c.state, after, limit)
				if err != nil {
					t.Fatalf("ListKnownInstances(%q, after %q, %d): %v", c.state, after, limit, err)
				}
				if len(page) > limit {
					t.Errorf("ListKnownInstances(%q, after %q, %d) = %s, more than the limit", c.state, after, limit, domainsOf(page))
				}
				if len(page) == 0 {
					break
				}
				pages = append(pages, page...)
				after = page[len(page)-1].Domain
			}
			if !sameKnownList(pages, c.want) {
				t.Errorf("ListKnownInstances(%q) by pages of %d = %s, want %s", c.state, limit, domainsOf(pages), domainsOf(c.want))
			}
		}
	}
	// A cursor that is not a known domain starts after it all the same.
	got, err := r.h.Federation.ListKnownInstances(ctx, "", "c.f", 10)
	if want := of(ks.all, "d.example", "e.example", "f.example"); err != nil || !sameKnownList(got, want) {
		t.Errorf("ListKnownInstances(after c.f) = %s, %v; want %s", domainsOf(got), err, domainsOf(want))
	}
}

// fedRepoFindKnownInstance finds each server as the list has it, and a server never known
// available, with no failures, sighting or software.
func fedRepoFindKnownInstance(t *testing.T, r *fedRepo) {
	ctx := t.Context()
	ks := r.knownServers(t)
	for _, want := range ks.all {
		if got, err := r.h.Federation.FindKnownInstance(ctx, want.Domain); err != nil || !sameKnown(got, want) {
			t.Errorf("FindKnownInstance(%s) = %+v, %v; want %+v", want.Domain, got, err, want)
		}
	}
	got, err := r.h.Federation.FindKnownInstance(ctx, "unknown.example")
	if err != nil || got.Domain != "unknown.example" || got.State != federation.InstanceAvailable || !got.Since.IsZero() ||
		got.FailureDays != 0 || got.LastFailureDay != "" || !got.FirstSeenAt.IsZero() || got.Software.ID != "" {
		t.Errorf("FindKnownInstance(unknown) = %+v, %v; want available, without failures, sighting or software", got, err)
	}
	if got, err := r.h.Federation.ListKnownInstances(ctx, "", "f.example", 10); err != nil || len(got) != 0 {
		t.Errorf("ListKnownInstances after the last = %s, %v; want none (finding a server records nothing)", domainsOf(got), err)
	}
}
