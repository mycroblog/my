// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
)

// This file holds the case of FederationFlows that covers the admin API's view of the
// deliveries to a remote server (ADR-0058 §9): every delivery with its attempts, the masking of
// the sender, the activity URI, and a personal inbox for what is not public, the audited
// reveal, and the audited filter by a local account.

// SharedInboxesAmong implements appfederation.SharedInboxes with the shared inboxes of the
// remote actors.
func (a *fedAccounts) SharedInboxesAmong(_ context.Context, urls []string) (map[string]bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := map[string]bool{}
	for _, u := range urls {
		for _, in := range a.inboxes {
			if in.SharedInboxURL == u {
				out[u] = true
			}
		}
	}
	return out, nil
}

// deliveryLogEnv records outbound activities and their deliveries directly, so that each case
// names the object, the inbox, and the attempts it shows.
type deliveryLogEnv struct {
	*fedEnv
	log     *appfederation.DeliveryLog
	audited []moderation.AuditEntry
	actor   string
}

func newDeliveryLogEnv(t *testing.T, e *fedEnv) *deliveryLogEnv {
	t.Helper()
	d := &deliveryLogEnv{fedEnv: e, actor: e.local()}
	e.projectors.Register(appmoderation.EventAuditRecorded, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		d.audited = append(d.audited, ev.(appmoderation.AuditRecorded).Entry)
		return nil
	})
	d.log = appfederation.NewDeliveryLog(appfederation.DeliveryLogDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Federation, Statuses: e.statuses, Accounts: e.accounts,
		SharedInboxes: e.accounts, Clock: e.clock, IDs: e.ids,
	})
	return d
}

// status stores a status of the env's actor with visibility v, without sending it.
func (d *deliveryLogEnv) status(v content.Visibility) string {
	id := d.ids.NewID()
	d.statuses.mu.Lock()
	defer d.statuses.mu.Unlock()
	d.statuses.statuses[id] = content.Status{
		ID: id, URI: content.StatusURI(testkit.ContractHost, id), AuthorAccountID: d.actor, IsLocal: true, Visibility: v,
		PublishedAt: d.clock.Now(),
	}
	return id
}

// send records an activity of typ about the object of kind and id by the env's actor, delivered
// to inbox, and returns its delivery.
func (d *deliveryLogEnv) send(t *testing.T, typ federation.ActivityType, kind federation.OutboundObjectKind, objectID, inbox string) (federation.OutboundActivity, federation.Delivery) {
	t.Helper()
	d.clock.Advance(time.Second)
	id := d.ids.NewID()
	act := federation.OutboundActivity{
		ID: id, ActivityURI: federation.IRI("https://" + testkit.ContractHost + "/ap/activities/" + id), Type: typ,
		ActorAccountID: d.actor, ObjectKind: kind, ObjectID: objectID, CreatedAt: d.clock.Now(),
	}
	del := federation.Delivery{
		ID: d.ids.NewID(), OutboundActivityID: id, InboxURL: federation.IRI(inbox), Domain: hostOf(inbox), CreatedAt: d.clock.Now(),
	}
	err := d.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := d.h.Federation.InsertOutboundActivity(t.Context(), tx, act); err != nil {
			return err
		}
		_, err := d.h.Federation.InsertDelivery(t.Context(), tx, del)
		return err
	})
	if err != nil {
		t.Fatalf("record %s: %v", typ, err)
	}
	return act, del
}

// attempt records an attempt of del.
func (d *deliveryLogEnv) attempt(t *testing.T, del federation.Delivery, a federation.DeliveryAttempt) federation.DeliveryAttempt {
	t.Helper()
	d.clock.Advance(time.Second)
	a.ID, a.DeliveryID, a.AttemptedAt = d.ids.NewID(), del.ID, d.clock.Now()
	if err := d.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return d.h.Federation.InsertDeliveryAttempt(t.Context(), tx, a) }); err != nil {
		t.Fatalf("record attempt: %v", err)
	}
	return a
}

func hostOf(u string) string {
	rest := strings.TrimPrefix(u, "https://")
	host, _, _ := strings.Cut(rest, "/")
	return host
}

func fedShowsDeliveriesToOperators(t *testing.T, e *fedEnv) {
	d := newDeliveryLogEnv(t, e)
	ctx := t.Context()
	const domain = "remote.example"
	personal := "https://" + domain + "/users/bob/inbox"
	sharedActor := e.remote(domain, true)
	shared := e.accounts.inboxes[sharedActor].SharedInboxURL

	// One delivery to the personal inbox of each case, newest last.
	type sent struct {
		name string
		act  federation.OutboundActivity
		del  federation.Delivery
		mask appfederation.DeliveryMask
	}
	var all []sent
	for _, c := range []struct {
		v    content.Visibility
		mask appfederation.DeliveryMask
	}{
		{content.VisibilityPublic, ""},
		{content.VisibilityUnlisted, ""},
		{content.VisibilityPrivate, appfederation.DeliveryMaskPrivate},
		{content.VisibilityDirect, appfederation.DeliveryMaskDirect},
		{content.VisibilityLocal, appfederation.DeliveryMaskLocal},
	} {
		act, del := d.send(t, federation.ActivityCreate, federation.OutboundStatus, d.status(c.v), personal)
		all = append(all, sent{string(c.v), act, del, c.mask})
	}
	act, del := d.send(t, federation.ActivityFollow, federation.OutboundFollow, e.ids.NewID(), personal)
	all = append(all, sent{"follow of an open account", act, del, ""})
	act, del = d.send(t, federation.ActivityCreate, federation.OutboundStatus, e.ids.NewID(), personal)
	all = append(all, sent{"status that is gone", act, del, appfederation.DeliveryMaskUnknown})

	// The env's actor now approves its followers: its follows are masked from here on.
	e.accounts.mu.Lock()
	locked := e.accounts.actors[d.actor]
	locked.Profile = &account.ProfileRevision{AccountID: d.actor, Locked: true}
	e.accounts.actors[d.actor] = locked
	e.accounts.mu.Unlock()
	all[5].mask = appfederation.DeliveryMaskApprovesFollowers
	act, del = d.send(t, federation.ActivityAccept, federation.OutboundFollow, e.ids.NewID(), personal)
	all = append(all, sent{"accept of a locked account", act, del, appfederation.DeliveryMaskApprovesFollowers})
	directShared, sharedDel := d.send(t, federation.ActivityCreate, federation.OutboundStatus, d.status(content.VisibilityDirect), shared)
	all = append(all, sent{"direct to the shared inbox", directShared, sharedDel, appfederation.DeliveryMaskDirect})
	d.send(t, federation.ActivityCreate, federation.OutboundStatus, d.status(content.VisibilityPublic), "https://elsewhere.example/inbox")

	failed := d.attempt(t, all[3].del, federation.DeliveryAttempt{
		Outcome: federation.DeliveryRetryableFailure, Error: `Post "` + personal + `": connection refused`,
	})
	ok := d.attempt(t, all[3].del, federation.DeliveryAttempt{Outcome: federation.DeliverySucceeded, Scheme: "cavage", HTTPStatus: 202})
	publicOK := d.attempt(t, all[0].del, federation.DeliveryAttempt{Outcome: federation.DeliverySucceeded, Scheme: "rfc9421", HTTPStatus: 202})

	got, err := d.log.List(ctx, appfederation.DeliveryQuery{Domain: domain, Limit: 20}, "operator")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != len(all) {
		t.Fatalf("List = %d deliveries, want %d (the other server's left out): %+v", len(got), len(all), got)
	}
	for i, s := range all {
		g := got[len(all)-1-i] // newest first
		if g.ID != s.del.ID || g.ActivityType != s.act.Type || !g.CreatedAt.Equal(s.del.CreatedAt) {
			t.Errorf("%s: entry %+v, want delivery %s of a %s", s.name, g, s.del.ID, s.act.Type)
			continue
		}
		if g.Mask != s.mask {
			t.Errorf("%s: mask %q, want %q", s.name, g.Mask, s.mask)
		}
		isShared := s.del.InboxURL == federation.IRI(shared)
		switch {
		case s.mask == "" && (g.Sender == nil || g.Sender.AccountID != d.actor || g.ActivityURI != s.act.ActivityURI || g.InboxURL != s.del.InboxURL):
			t.Errorf("%s: entry %+v, want the sender, the activity URI, and the inbox", s.name, g)
		case s.mask != "" && (g.Sender != nil || g.ActivityURI != ""):
			t.Errorf("%s: entry %+v, want the sender and the activity URI masked", s.name, g)
		case s.mask != "" && !isShared && g.InboxURL != "":
			t.Errorf("%s: inbox %q, want the personal inbox masked", s.name, g.InboxURL)
		case isShared && (!g.SharedInbox || g.InboxURL != s.del.InboxURL):
			t.Errorf("%s: entry %+v, want the shared inbox shown", s.name, g)
		}
	}
	direct := got[len(all)-1-3]
	if len(direct.Attempts) != 2 || direct.Attempts[1] != ok || direct.Attempts[0].Error != `Post "`+appfederation.MaskedError+`": connection refused` ||
		direct.Attempts[0].ID != failed.ID || direct.Attempts[0].Outcome != failed.Outcome {
		t.Errorf("attempts of the direct delivery = %+v; want both, the inbox in the error masked", direct.Attempts)
	}
	if public := got[len(all)-1]; len(public.Attempts) != 1 || public.Attempts[0] != publicOK {
		t.Errorf("attempts of the public delivery = %+v; want the success %+v", public.Attempts, publicOK)
	}
	if len(d.audited) != 0 {
		t.Errorf("a list without a filter audited %+v; want nothing", d.audited)
	}

	page, err := d.log.List(ctx, appfederation.DeliveryQuery{Domain: domain, Before: got[1].ID, Limit: 2}, "operator")
	if err != nil || len(page) != 2 || page[0].ID != got[2].ID || page[1].ID != got[3].ID {
		t.Errorf("List(before the second, 2) = %+v, %v; want the third and the fourth", page, err)
	}
	byURI, err := d.log.List(ctx, appfederation.DeliveryQuery{Domain: domain, ActivityURI: all[0].act.ActivityURI, Limit: 20}, "operator")
	if err != nil || len(byURI) != 1 || byURI[0].ID != all[0].del.ID || len(d.audited) != 0 {
		t.Errorf("List(activity_uri) = %+v, %v, audited %+v; want the public delivery, unaudited", byURI, err, d.audited)
	}

	// The reveal shows the masked fields of one delivery and audits it.
	r, err := d.log.Reveal(ctx, domain, all[3].del.ID, "operator")
	if err != nil || r.Sender.AccountID != d.actor || r.Sender.Username == "" || r.ActivityURI != all[3].act.ActivityURI ||
		r.InboxURL != all[3].del.InboxURL || r.Mask != appfederation.DeliveryMaskDirect {
		t.Errorf("Reveal(direct) = %+v, %v; want its sender, activity URI, and inbox", r, err)
	}
	if len(d.audited) != 1 || d.audited[0].Action != moderation.AuditDeliveryRevealed || d.audited[0].ActorUserID != "operator" ||
		d.audited[0].TargetKind != moderation.AuditTargetDomain || d.audited[0].TargetID != domain || d.audited[0].SourceEventID != all[3].del.ID {
		t.Errorf("audited %+v; want one delivery_revealed of the delivery to %s", d.audited, domain)
	}
	if _, err := d.log.Reveal(ctx, "elsewhere.example", all[3].del.ID, "operator"); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("Reveal on another server = %v, want ErrNotFound", err)
	}
	if _, err := d.log.Reveal(ctx, domain, e.ids.NewID(), "operator"); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("Reveal of an unknown delivery = %v, want ErrNotFound", err)
	}
	if len(d.audited) != 1 {
		t.Errorf("a reveal that failed audited %+v", d.audited[1:])
	}

	// The filter by a local account is audited, with the account and the server.
	d.audited = nil
	mine, err := d.log.List(ctx, appfederation.DeliveryQuery{Domain: domain, ActorAccountID: d.actor, Limit: 20}, "operator")
	if err != nil || len(mine) != len(all) {
		t.Errorf("List(account_id) = %d, %v; want the %d deliveries of the actor", len(mine), err, len(all))
	}
	if len(d.audited) != 1 || d.audited[0].Action != moderation.AuditAccountDeliveriesViewed || d.audited[0].TargetKind != moderation.AuditTargetAccount ||
		d.audited[0].TargetID != d.actor || !strings.Contains(d.audited[0].TargetLabel, domain) {
		t.Errorf("audited %+v; want one account_deliveries_viewed of the actor and the server", d.audited)
	}
	if _, err := d.log.List(ctx, appfederation.DeliveryQuery{Domain: domain, ActorAccountID: sharedActor, Limit: 20}, "operator"); !errors.Is(err, appfederation.ErrNotLocalAccount) {
		t.Errorf("List(a remote account) = %v, want ErrNotLocalAccount", err)
	}
	if len(d.audited) != 1 {
		t.Errorf("a refused filter audited %+v", d.audited[1:])
	}
}
