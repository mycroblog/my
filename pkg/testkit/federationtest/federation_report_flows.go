// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
)

// fedReports stands in for the moderation module's reports, by ID.
type fedReports struct {
	mu      sync.Mutex
	reports map[string]appmoderation.Report
}

func (r *fedReports) FindReport(_ context.Context, id string) (appmoderation.Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rep, ok := r.reports[id]
	if !ok {
		return appmoderation.Report{}, fmt.Errorf("report %s: %w", id, appmoderation.ErrNotFound)
	}
	return rep, nil
}

// InstanceActor implements federation.InstanceActorReader for the stand-in accounts: the local
// Application actor, when one is registered.
func (a *fedAccounts) InstanceActor(_ context.Context) (appaccount.Actor, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, actor := range a.actors {
		if actor.IsInstanceActor() {
			return actor, nil
		}
	}
	return appaccount.Actor{}, fmt.Errorf("instance actor: %w", appaccount.ErrNotFound)
}

// instanceActor registers the instance actor and returns its ID.
func (e *fedEnv) instanceActor() string {
	id := e.ids.NewID()
	e.accounts.mu.Lock()
	defer e.accounts.mu.Unlock()
	e.accounts.actors[id] = appaccount.Actor{
		Account:   account.Account{ID: id, URI: account.ActorURI(testkit.ContractHost, id), Domain: account.LocalDomain, ActorType: account.ActorApplication},
		Lifecycle: account.LifecycleState{AccountID: id, State: account.StateActive},
	}
	return id
}

// remoteStatus registers a status of a remote author and returns it.
func (e *fedEnv) remoteStatus(author string) content.Status {
	id := e.ids.NewID()
	s := content.Status{
		ID: id, URI: e.uri(author) + "/statuses/" + id, AuthorAccountID: author,
		Visibility: content.VisibilityPublic, PublishedAt: e.clock.Now(),
	}
	e.statuses.mu.Lock()
	e.statuses.statuses[id] = s
	e.statuses.mu.Unlock()
	return s
}

// report records a report of target by reporter, emits it as the moderation module does, and
// runs its fan-out.
func (e *fedEnv) report(t *testing.T, reporter, target string, forwarded bool, statusIDs ...string) appmoderation.Report {
	t.Helper()
	e.clock.Advance(time.Second)
	r := appmoderation.Report{
		ID: e.ids.NewID(), ReporterAccountID: reporter, TargetAccountID: target, Category: moderation.CategorySpam,
		Comment: "Spam from " + target, Forwarded: forwarded, FiledAt: e.clock.Now(), StatusIDs: statusIDs,
	}
	e.reports.mu.Lock()
	e.reports.reports[r.ID] = r
	e.reports.mu.Unlock()
	e.emit(t, appmoderation.ReportFiled{Report: r})
	e.drain(t)
	return r
}

func fedForwardsReports(t *testing.T, e *fedEnv) {
	ctx := t.Context()
	instance := e.instanceActor()
	alice := e.local()
	bob := e.remote("b.example", true)
	spam := e.remoteStatus(bob)

	// A forwarded report is a Flag of the instance actor, delivered to the reported actor's own
	// inbox (not the shared one), about the actor and the statuses that still exist.
	r := e.report(t, alice, bob, true, spam.ID, e.ids.NewID())
	act := e.activity(t, federation.OutboundReport, r.ID, federation.ActivityFlag)
	if act.ActorAccountID != instance || !strings.HasPrefix(string(act.ActivityURI), "https://"+testkit.ContractHost+"/ap/activities/") {
		t.Errorf("Flag = %+v, want the instance actor's under /ap/activities/", act)
	}
	if got, want := e.inboxes(t, act.ID), []string{e.uri(bob) + "/inbox"}; !slices.Equal(got, want) {
		t.Errorf("Flag inboxes = %v, want %v", got, want)
	}
	doc := e.document(t, act.ID)
	if doc["type"] != "Flag" || doc["id"] != string(act.ActivityURI) || doc["actor"] != e.uri(instance) || doc["content"] != r.Comment {
		t.Errorf("Flag document = %v, want the instance actor's Flag with the comment", doc)
	}
	if got, want := strs(doc["object"]), []string{e.uri(bob), spam.URI}; !slices.Equal(got, want) {
		t.Errorf("Flag object = %v, want %v", got, want)
	}
	if _, ok := doc["to"]; ok {
		t.Errorf("Flag to = %v, want no audience", doc["to"])
	}
	raw, err := e.h.Federation.FindOutboundDocument(ctx, act.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), alice) {
		t.Errorf("Flag document %s names the reporter %s", raw, alice)
	}

	// Reports that are not forwarded, reports of local accounts, and reports received in a Flag
	// (never forwarded) send nothing.
	carol := e.local()
	for _, other := range []appmoderation.Report{
		e.report(t, alice, bob, false, spam.ID),
		e.report(t, alice, carol, false),
		e.report(t, bob, alice, false),
	} {
		if got, err := e.h.Federation.ListOutboundActivitiesOf(ctx, federation.OutboundReport, other.ID); err != nil || len(got) != 0 {
			t.Errorf("activities about report %s of %s = %+v, %v; want none", other.ID, other.TargetAccountID, got, err)
		}
	}

	// A moderator's forward after filing sends the same Flag, to the same inbox, with the
	// moderator's text and never the reporter's comment.
	later := e.report(t, alice, bob, false, spam.ID)
	e.clock.Advance(time.Second)
	later.Forward = &moderation.ReportForward{ID: e.ids.NewID(), ReportID: later.ID, Comment: "Forwarded by the moderators", ForwardedAt: e.clock.Now()}
	e.reports.mu.Lock()
	e.reports.reports[later.ID] = later
	e.reports.mu.Unlock()
	e.emit(t, appmoderation.ReportForwarded{Report: later})
	e.drain(t)
	act = e.activity(t, federation.OutboundReport, later.ID, federation.ActivityFlag)
	if act.ActorAccountID != instance {
		t.Errorf("Flag of a forward = %+v, want the instance actor's", act)
	}
	if got, want := e.inboxes(t, act.ID), []string{e.uri(bob) + "/inbox"}; !slices.Equal(got, want) {
		t.Errorf("Flag inboxes of a forward = %v, want %v", got, want)
	}
	doc = e.document(t, act.ID)
	if doc["content"] != "Forwarded by the moderators" {
		t.Errorf("Flag content of a forward = %v, want the moderator's text", doc["content"])
	}
	if raw, err := e.h.Federation.FindOutboundDocument(ctx, act.ID); err != nil || strings.Contains(string(raw), later.Comment) {
		t.Errorf("Flag of a forward = %s, %v; want no reporter's comment %q", raw, err, later.Comment)
	}

	// A later forward of a hook's report without the moderator's text sends an empty content,
	// not the plugin's note.
	silent := e.report(t, alice, bob, false, spam.ID)
	e.clock.Advance(time.Second)
	silent.Comment = "classifier spam-v2 score 0.98"
	silent.HookOrigin = &moderation.ReportHookOrigin{DecisionID: e.ids.NewID(), Hook: "inbound.status", ReasonCode: "spam"}
	silent.Forward = &moderation.ReportForward{ID: e.ids.NewID(), ReportID: silent.ID, ForwardedAt: e.clock.Now()}
	e.reports.mu.Lock()
	e.reports.reports[silent.ID] = silent
	e.reports.mu.Unlock()
	e.emit(t, appmoderation.ReportForwarded{Report: silent})
	e.drain(t)
	act = e.activity(t, federation.OutboundReport, silent.ID, federation.ActivityFlag)
	if doc := e.document(t, act.ID); doc["content"] != "" {
		t.Errorf("Flag content of a forward without text = %v, want empty", doc["content"])
	}
	if raw, err := e.h.Federation.FindOutboundDocument(ctx, act.ID); err != nil || strings.Contains(string(raw), "classifier") {
		t.Errorf("Flag of a hook's report = %s, %v; want no plugin note", raw, err)
	}

	// A suspended server gets no Flag.
	dave := e.remote("d.example", false)
	e.domains["d.example"] = true
	suspended := e.report(t, alice, dave, true)
	act = e.activity(t, federation.OutboundReport, suspended.ID, federation.ActivityFlag)
	if got := e.inboxes(t, act.ID); len(got) != 0 {
		t.Errorf("Flag inboxes on a suspended server = %v, want none", got)
	}
}

func (a *fedAccounts) InstanceActorTx(ctx context.Context, _ uow.Tx) (appaccount.Actor, error) {
	return a.InstanceActor(ctx)
}
