// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"strings"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/moderation"
)

func init() {
	inboxOrderCases = append(inboxOrderCases, moderationOrderCases...)
}

// The local accounts and status of the moderation handlers' cases.
const (
	inboxLocalX      = "00000000-0000-7000-8000-00000000000a"
	inboxLocalY      = "00000000-0000-7000-8000-00000000000b"
	inboxLocalStatus = "00000000-0000-7000-8000-0000000000a1"
)

func inboxLocalURI(id string) string { return account.ActorURI(contractHost, id) }

// inboxModeration is the graph and moderation modules on the harness, with the handlers of
// Block, Undo Block, and Flag registered on the env's processor.
type inboxModeration struct {
	graph      *appgraph.Service
	moderation *appmoderation.Service
	accounts   *stubActors
}

// inboxObjects finds the local accounts among the stand-in actors and the one local status.
type inboxObjects struct{ accounts *stubActors }

func (o inboxObjects) FindByURI(_ context.Context, uri string) (appaccount.Actor, error) {
	o.accounts.mu.Lock()
	defer o.accounts.mu.Unlock()
	for _, a := range o.accounts.actors {
		if a.Account.URI == uri {
			return a, nil
		}
	}
	return appaccount.Actor{}, appaccount.ErrNotFound
}

func (o inboxObjects) FindStatus(_ context.Context, id string) (appcontent.Status, error) {
	if id != inboxLocalStatus {
		return appcontent.Status{}, appcontent.ErrNotFound
	}
	return content.Status{ID: id, URI: content.StatusURI(contractHost, id), AuthorAccountID: inboxLocalY, IsLocal: true}, nil
}

// wireModeration registers the real handlers of Block, Undo Block, and Flag on e's processor.
func (e *inboxProcessEnv) wireModeration(t *testing.T) *inboxModeration {
	t.Helper()
	if e.h.Graph == nil || e.h.Moderation == nil {
		t.Fatal("the harness has no graph or moderation repository")
	}
	appgraph.RegisterProjectors(e.projectors, e.h.Graph)
	actors := &stubActors{actors: map[string]appaccount.Actor{}}
	for _, uri := range []federation.IRI{inboxAlice, inboxBob} {
		a, err := e.actors.Resolve(t.Context(), string(uri))
		if err != nil {
			t.Fatal(err)
		}
		a.Lifecycle = account.LifecycleState{AccountID: a.Account.ID, State: account.StateActive}
		actors.set(a)
	}
	for _, id := range []string{inboxLocalX, inboxLocalY} {
		actors.set(appaccount.Actor{
			Account:   account.Account{ID: id, URI: inboxLocalURI(id), Domain: account.LocalDomain, ActorType: account.ActorPerson},
			Local:     &account.LocalAccount{AccountID: id, Username: "u" + id[len(id)-2:]},
			Lifecycle: account.LifecycleState{AccountID: id, State: account.StateActive},
		})
	}
	ids := NewIDGenerator(e.clock)
	m := &inboxModeration{
		accounts: actors,
		graph: appgraph.NewService(appgraph.Deps{
			UnitOfWork: e.h.UnitOfWork, Repository: e.h.Graph, Accounts: actors, Clock: e.clock, IDs: ids, Host: contractHost,
		}),
		moderation: appmoderation.NewService(appmoderation.Deps{
			UnitOfWork: e.h.UnitOfWork, Repository: e.h.Moderation, Accounts: actors, Clock: e.clock, IDs: ids,
		}),
	}
	appfederation.RegisterModerationHandlers(e.processor, appfederation.ModerationHandlerDeps{
		Blocks: m.graph, Reports: m.moderation, Policy: appmoderation.NewFederationPolicy(e.h.Moderation, e.clock),
		Objects: inboxObjects{accounts: actors}, Host: contractHost,
	})
	return m
}

func inboxBlockDoc(t *testing.T, id, object string) []byte {
	t.Helper()
	return inboxDoc(t, "Block", id, "", object)
}

func inboxFlagDoc(t *testing.T, id, actor string, objects ...string) []byte {
	t.Helper()
	fields := map[string]any{"type": "Flag", "actor": actor, "object": objects, "content": "spam!"}
	if id != "" {
		fields["id"] = id
	}
	return inboxActivity(t, fields)
}

// inboxHandlesModeration checks the real handlers of Block, Undo Block, and Flag.
func inboxHandlesModeration(t *testing.T, e *inboxProcessEnv) {
	ctx := t.Context()
	e.processor = e.newProcessor()
	m := e.wireModeration(t)
	alice := NamedID("account:" + inboxAlice)

	// A Block of a local actor is recorded; one of a remote or unknown actor is dropped.
	block := inboxAlice + "#block-1"
	if got := e.process(t, inboxBlockDoc(t, block, inboxLocalURI(inboxLocalX))); got != appfederation.InboxHandled {
		t.Errorf("Block of a local actor = %q, want handled", got)
	}
	if ok, err := m.graph.IsBlocking(ctx, alice, inboxLocalX); err != nil || !ok {
		t.Errorf("IsBlocking(alice, x) = %v, %v; want true", ok, err)
	}
	for i, object := range []string{inboxBob, inboxLocalURI("00000000-0000-7000-8000-0000000000ff")} {
		if got := e.process(t, inboxBlockDoc(t, inboxAlice+"#block-dropped-"+string(rune('a'+i)), object)); got != appfederation.InboxDropped {
			t.Errorf("Block of %s = %q, want dropped", object, got)
		}
	}

	// The Undo of the Block, embedded or by its URI, lifts it.
	if got := e.process(t, inboxDoc(t, "Undo", inboxAlice+"#undo-1", "", block)); got != appfederation.InboxHandled {
		t.Errorf("Undo of the Block = %q, want handled", got)
	}
	if ok, _ := m.graph.IsBlocking(ctx, alice, inboxLocalX); ok {
		t.Error("alice still blocks x after the Undo")
	}
	// A Block without an id is undone by its object.
	if got := e.process(t, inboxBlockDoc(t, "", inboxLocalURI(inboxLocalY))); got != appfederation.InboxHandled {
		t.Errorf("Block without an id = %q, want handled", got)
	}
	embedded := map[string]any{"type": "Block", "actor": inboxAlice, "object": inboxLocalURI(inboxLocalY)}
	if got := e.process(t, inboxDoc(t, "Undo", inboxAlice+"#undo-2", "", embedded)); got != appfederation.InboxHandled {
		t.Errorf("Undo of a Block without an id = %q, want handled", got)
	}
	if ok, _ := m.graph.IsBlocking(ctx, alice, inboxLocalY); ok {
		t.Error("alice still blocks y after the Undo")
	}

	// A Flag of local actors and statuses is a report of the first local account.
	flag := inboxAlice + "#flag-1"
	if got := e.process(t, inboxFlagDoc(t, flag, inboxAlice, inboxBob, inboxLocalURI(inboxLocalY), content.StatusURI(contractHost, inboxLocalStatus))); got != appfederation.InboxHandled {
		t.Errorf("Flag = %q, want handled", got)
	}
	reports, err := m.moderation.ReportsOf(ctx, inboxLocalY)
	if err != nil || len(reports) != 1 {
		t.Fatalf("ReportsOf(y) = %+v, %v; want 1", reports, err)
	}
	if r := reports[0]; r.URI != flag || r.ReporterAccountID != alice || r.Comment != "spam!" || len(r.StatusIDs) != 1 || r.StatusIDs[0] != inboxLocalStatus {
		t.Errorf("report = %+v, want alice's Flag %s of y with the local status", r, flag)
	}
	// A Flag of only a local status reports its author; one of nothing local is dropped.
	if got := e.process(t, inboxFlagDoc(t, "", inboxBob, content.StatusURI(contractHost, inboxLocalStatus))); got != appfederation.InboxHandled {
		t.Errorf("Flag of a status = %q, want handled", got)
	}
	if reports, _ := m.moderation.ReportsOf(ctx, inboxLocalY); len(reports) != 2 || !strings.HasPrefix(reports[1].URI, "urn:my:digest:") {
		t.Errorf("reports of y = %+v, want a second one keyed by the digest URI", reports)
	}
	if got := e.process(t, inboxFlagDoc(t, inboxAlice+"#flag-2", inboxAlice, inboxBob)); got != appfederation.InboxDropped {
		t.Errorf("Flag of nothing local = %q, want dropped", got)
	}

	// The Flags of a domain with reject_reports are dropped (federation.md §11).
	if _, err := m.moderation.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "example", Kind: moderation.DomainRejectReports}); err != nil {
		t.Fatal(err)
	}
	if got := e.process(t, inboxFlagDoc(t, inboxAlice+"#flag-3", inboxAlice, inboxLocalURI(inboxLocalX))); got != appfederation.InboxDropped {
		t.Errorf("Flag of a domain with reject_reports = %q, want dropped", got)
	}
	if reports, err := m.moderation.ReportsOf(ctx, inboxLocalX); err != nil || len(reports) != 0 {
		t.Errorf("reports of x = %+v, %v; want none", reports, err)
	}
}

// inboxBlockRows are the block between alice and x: whether it is in force, and whether the
// Block is gone (retracted, or tombstoned before it arrived).
type inboxBlockRows struct {
	Blocking, Gone bool
}

var moderationOrderCases = []inboxOrderCase{
	{
		name: "BlockAndUndo",
		activities: func(t *testing.T) [][]byte {
			b := inboxAlice + "#block-1"
			return [][]byte{
				inboxBlockDoc(t, b, inboxLocalURI(inboxLocalX)),
				inboxDoc(t, "Undo", inboxAlice+"#undo-1", "", map[string]any{"id": b, "type": "Block", "actor": inboxAlice, "object": inboxLocalURI(inboxLocalX)}),
			}
		},
		wire: func(t *testing.T, e *inboxProcessEnv) { e.wireModeration(t) },
		rows: func(t *testing.T, e *inboxProcessEnv) any {
			ctx := t.Context()
			r := inboxBlockRows{Gone: e.tombstoned(t, inboxAlice+"#block-1")}
			err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				h, err := e.h.Graph.BlockByURI(ctx, tx, inboxAlice+"#block-1")
				switch {
				case errors.Is(err, appgraph.ErrNotFound):
					return nil
				case err != nil:
					return err
				}
				r.Gone = r.Gone || h.Retraction != nil
				_, err = e.h.Graph.BlockEdge(ctx, tx, h.Block.BlockerAccountID, h.Block.BlockedAccountID)
				r.Blocking = err == nil
				if errors.Is(err, appgraph.ErrNotFound) {
					err = nil
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			return r
		},
		want: inboxBlockRows{Blocking: false, Gone: true},
	},
	{
		name: "FlagsOfOneActivity",
		activities: func(t *testing.T) [][]byte {
			return [][]byte{
				inboxFlagDoc(t, inboxAlice+"#flag-1", inboxAlice, inboxLocalURI(inboxLocalX)),
				inboxFlagDoc(t, "", inboxAlice, inboxLocalURI(inboxLocalX), content.StatusURI(contractHost, inboxLocalStatus)),
			}
		},
		wire: func(t *testing.T, e *inboxProcessEnv) { e.wireModeration(t) },
		rows: func(t *testing.T, e *inboxProcessEnv) any {
			rs, err := e.h.Moderation.ListReportsOf(t.Context(), inboxLocalX)
			if err != nil {
				t.Fatal(err)
			}
			return len(rs)
		},
		want: 2,
	},
}
