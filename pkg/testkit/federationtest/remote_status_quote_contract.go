// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// The cases of RemoteStatusContract on the quotes of posts (ADR-0069) and on the pins that the
// featured collections of remote accounts become (ADR-0006).

// remoteStatusRepoQuotes checks the quote repository: a status quotes one status, found by its
// status and listed in pages by status ID and per quoted URI, newest first; the state of a quote
// is derived from its form, its detachment, and its request, whose answers read back in order;
// the requests are found by ID and interaction, paged per author, and listed per interactor; the
// projection of their states is upserted, listed, and deleted for its rebuild; the policies are
// upserted and deleted; an approval is recorded once per kind, interaction, and target, found
// with its revocation, which is recorded once.
func remoteStatusRepoQuotes(t *testing.T, r *remoteStatusRepo) {
	if r.h.Quotes == nil {
		t.Skip("the harness has no quote repository")
	}
	ctx := t.Context()
	quoted := "https://remote.example/notes/quoted"
	var quotes []content.Quote
	for _, form := range []content.QuoteForm{content.QuoteFormFEP044f, content.QuoteFormLegacy, content.QuoteFormFEP044f} {
		quotes = append(quotes, content.Quote{StatusID: r.ids.NewID(), QuotedURI: quoted, Form: form})
	}
	other := content.Quote{StatusID: r.ids.NewID(), QuotedURI: "https://remote.example/notes/other", Form: content.QuoteFormLegacy}
	insert := func(q content.Quote, want bool) {
		t.Helper()
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Quotes.InsertQuote(ctx, tx, q)
			if err == nil && ok != want {
				t.Errorf("InsertQuote(%s) = %v, want %v", q.StatusID, ok, want)
			}
			return err
		})
	}
	for _, q := range append(slices.Clone(quotes), other) {
		insert(q, true)
	}
	insert(content.Quote{StatusID: quotes[0].StatusID, QuotedURI: "https://remote.example/notes/else", Form: content.QuoteFormLegacy}, false)
	if got, err := r.h.Quotes.FindQuote(ctx, quotes[0].StatusID); err != nil || got != quotes[0] {
		t.Errorf("FindQuote = %+v, %v; want the first %+v", got, err, quotes[0])
	}
	if _, err := r.h.Quotes.FindQuote(ctx, r.ids.NewID()); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindQuote(a status quoting nothing) = %v, want ErrNotFound", err)
	}
	ids := []string{r.ids.NewID()}
	for _, q := range append(slices.Clone(quotes), other) {
		ids = append(ids, q.StatusID)
	}
	if got, err := r.h.Quotes.FindQuotes(ctx, ids); err != nil || len(got) != len(quotes)+1 || got[quotes[0].StatusID] != quotes[0] || got[other.StatusID] != other {
		t.Errorf("FindQuotes = %+v, %v; want each quote, as FindQuote", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Quotes.FindQuoteTx(ctx, tx, other.StatusID); err != nil || got != other {
			t.Errorf("FindQuoteTx = %+v, %v; want %+v", got, err, other)
		}
		first, err := r.h.Quotes.ListQuotesAfter(ctx, tx, "", 2)
		if err != nil || !slices.Equal(first, quotes[:2]) {
			t.Errorf("ListQuotesAfter first page = %+v, %v; want %+v", first, err, quotes[:2])
		}
		rest, err := r.h.Quotes.ListQuotesAfter(ctx, tx, quotes[1].StatusID, 10)
		if want := []content.Quote{quotes[2], other}; err != nil || !slices.Equal(rest, want) {
			t.Errorf("ListQuotesAfter the second = %+v, %v; want %+v", rest, err, want)
		}
		return nil
	})
	if got, err := r.h.Quotes.ListQuotesOf(ctx, quoted, "", 2); err != nil || !slices.Equal(got, []content.Quote{quotes[2], quotes[1]}) {
		t.Errorf("ListQuotesOf newest page = %+v, %v; want the two newest", got, err)
	}
	if got, err := r.h.Quotes.ListQuotesOf(ctx, quoted, quotes[1].StatusID, 10); err != nil || !slices.Equal(got, quotes[:1]) {
		t.Errorf("ListQuotesOf before the second = %+v, %v; want the first", got, err)
	}

	// The state of a quote is derived from its form, its detachment, and its request.
	stored := map[string]content.Status{}
	for _, q := range []content.Quote{quotes[0], quotes[2]} {
		st := r.remoteStatus("")
		st.ID = q.StatusID
		r.insert(t, st)
		stored[q.StatusID] = st
	}
	auth1 := "https://remote.example/auth/1"
	request := func(q content.Quote) content.InteractionRequest {
		return content.InteractionRequest{
			ID: r.ids.NewID(), Kind: content.InteractionQuote, Direction: content.DirectionRemote, TargetURI: q.QuotedURI,
			InteractingURI: stored[q.StatusID].URI, InteractingAccountID: stored[q.StatusID].AuthorAccountID, CreatedAt: r.clock.Now(),
		}
	}
	r0, r2 := request(quotes[0]), request(quotes[2])
	events := []content.InteractionEvent{
		{ID: r.ids.NewID(), RequestID: r0.ID, Kind: content.InteractionEventAccepted, AuthorizationURI: auth1, OccurredAt: r.clock.Now()},
		{ID: r.ids.NewID(), RequestID: r0.ID, Kind: content.InteractionEventRevoked, AuthorizationURI: auth1, OccurredAt: r.clock.Now().Add(time.Second)},
	}
	accepted := content.InteractionEvent{ID: r.ids.NewID(), RequestID: r2.ID, Kind: content.InteractionEventAccepted, AuthorizationURI: auth1, OccurredAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, req := range []content.InteractionRequest{r0, r2} {
			if ok, err := r.h.Quotes.InsertInteractionRequest(ctx, tx, req); err != nil || !ok {
				t.Errorf("InsertInteractionRequest(%s) = %t, %v; want true", req.ID, ok, err)
			}
			if err := r.h.Quotes.UpsertInteractionState(ctx, tx, content.InitialInteractionState(req)); err != nil {
				return err
			}
		}
		for _, e := range append(slices.Clone(events), accepted) {
			if err := r.h.Quotes.InsertInteractionEvent(ctx, tx, e); err != nil {
				return err
			}
		}
		got, err := r.h.Quotes.ListInteractionEvents(ctx, tx, r0.ID)
		if err != nil || !slices.EqualFunc(got, events, equalInteractionEvent) {
			t.Errorf("ListInteractionEvents = %+v, %v; want %+v", got, err, events)
		}
		if got, err := r.h.Quotes.FindQuoteStateTx(ctx, tx, quotes[0].StatusID); err != nil || got.Approval != content.QuoteApprovalPending {
			t.Errorf("FindQuoteStateTx before the projection = %+v, %v; want pending", got, err)
		}
		for _, st := range []content.InteractionState{content.InteractionStateOf(r0, events), content.InteractionStateOf(r2, []content.InteractionEvent{accepted})} {
			if err := r.h.Quotes.UpsertInteractionState(ctx, tx, st); err != nil {
				return err
			}
		}
		return nil
	})
	wantStates := map[string]content.QuoteState{
		quotes[0].StatusID: {StatusID: quotes[0].StatusID, Approval: content.QuoteApprovalRevoked},
		quotes[1].StatusID: {StatusID: quotes[1].StatusID, Approval: content.QuoteApprovalLegacy},
		quotes[2].StatusID: {StatusID: quotes[2].StatusID, Approval: content.QuoteApprovalAccepted, AuthorizationURI: auth1},
		other.StatusID:     {StatusID: other.StatusID, Approval: content.QuoteApprovalLegacy},
	}
	checkStates := func(when string) {
		t.Helper()
		stateOf, err := r.h.Quotes.FindQuoteStates(ctx, ids)
		if err != nil {
			t.Fatalf("FindQuoteStates = %v", err)
		}
		for _, id := range ids {
			got, err := r.h.Quotes.FindQuoteState(ctx, id)
			w, ok := wantStates[id]
			if ok != (err == nil) || got != w {
				t.Errorf("%s: FindQuoteState(%s) = %+v, %v; want %+v (%t)", when, id, got, err, w, ok)
			}
			if g, found := stateOf[id]; found != ok || g != w {
				t.Errorf("%s: FindQuoteStates[%s] = %+v (%t), want %+v as FindQuoteState", when, id, g, found, w)
			}
		}
	}
	checkStates("answered")
	if got, err := r.h.Quotes.ListQuoteStatesByAuthorization(ctx, auth1); err != nil || !slices.Equal(got, []content.QuoteState{wantStates[quotes[2].StatusID]}) {
		t.Errorf("ListQuoteStatesByAuthorization = %+v, %v; want the accepted one", got, err)
	}
	// A detachment takes the quote away for good, recorded once.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for range 2 {
			if err := r.h.Quotes.InsertQuoteDetachment(ctx, tx, quotes[2].StatusID, r.clock.Now()); err != nil {
				return err
			}
		}
		return nil
	})
	wantStates[quotes[2].StatusID] = content.QuoteState{StatusID: quotes[2].StatusID, Approval: content.QuoteApprovalDetached}
	checkStates("detached")

	// The requests, read by ID, by interaction, in pages, and per author.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Quotes.FindInteractionRequest(ctx, tx, r0.ID); err != nil || !equalInteractionRequest(got, r0) {
			t.Errorf("FindInteractionRequest = %+v, %v; want %+v", got, err, r0)
		}
		if got, err := r.h.Quotes.ListInteractionRequestsAfter(ctx, tx, r0.ID, 10); err != nil || len(got) != 1 || !equalInteractionRequest(got[0], r2) {
			t.Errorf("ListInteractionRequestsAfter = %+v, %v; want %+v", got, err, r2)
		}
		return nil
	})
	if got, err := r.h.Quotes.FindInteractionRequestFor(ctx, nil, content.InteractionQuote, r2.InteractingURI, r2.TargetURI); err != nil || !equalInteractionRequest(got, r2) {
		t.Errorf("FindInteractionRequestFor = %+v, %v; want %+v", got, err, r2)
	}
	if _, err := r.h.Quotes.FindInteractionRequestFor(ctx, nil, content.InteractionReply, r2.InteractingURI, r2.TargetURI); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindInteractionRequestFor(another kind) = %v, want ErrNotFound", err)
	}
	author, booster := testkit.NamedID("approving-author"), testkit.NamedID("booster")
	pending := []content.InteractionRequest{
		{ID: r.ids.NewID(), Kind: content.InteractionReply, Direction: content.DirectionInbound, TargetURI: "https://local.example/s/1",
			TargetAccountID: author, InteractingURI: "https://remote.example/notes/reply", InteractingAccountID: booster, CreatedAt: r.clock.Now()},
		{ID: r.ids.NewID(), Kind: content.InteractionAnnounce, Direction: content.DirectionInbound, TargetURI: "https://local.example/s/1",
			TargetAccountID: author, InteractingURI: "https://remote.example/announces/1", InteractingAccountID: booster,
			ActivityURI: "https://remote.example/requests/1", Visibility: content.VisibilityUnlisted, CreatedAt: r.clock.Now()},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, req := range pending {
			if _, err := r.h.Quotes.InsertInteractionRequest(ctx, tx, req); err != nil {
				return err
			}
			if err := r.h.Quotes.UpsertInteractionState(ctx, tx, content.InitialInteractionState(req)); err != nil {
				return err
			}
		}
		return nil
	})
	if got, err := r.h.Quotes.ListPendingInteractionRequests(ctx, author, nil, "", 1); err != nil || len(got) != 1 || !equalInteractionRequest(got[0], pending[1]) {
		t.Errorf("ListPendingInteractionRequests newest = %+v, %v; want %+v", got, err, pending[1])
	}
	if got, err := r.h.Quotes.ListPendingInteractionRequests(ctx, author, nil, pending[1].ID, 10); err != nil || len(got) != 1 || !equalInteractionRequest(got[0], pending[0]) {
		t.Errorf("ListPendingInteractionRequests before the newest = %+v, %v; want %+v", got, err, pending[0])
	}
	if got, err := r.h.Quotes.ListPendingInteractionRequests(ctx, author, []content.InteractionKind{content.InteractionReply}, "", 10); err != nil || len(got) != 1 || got[0].ID != pending[0].ID {
		t.Errorf("ListPendingInteractionRequests of replies = %+v, %v; want the reply", got, err)
	}
	if got, err := r.h.Quotes.ListOwnInteractions(ctx, booster, []string{"https://remote.example/notes/reply", "https://local.example/s/1"}); err != nil || len(got) != 2 {
		t.Errorf("ListOwnInteractions = %+v, %v; want the reply and the boost", got, err)
	}
	if got, err := r.h.Quotes.ListOwnInteractions(ctx, booster, []string{"https://remote.example/announces/1"}); err != nil || len(got) != 0 {
		t.Errorf("ListOwnInteractions by the Announce = %+v, %v; want none (boosts are found by their target)", got, err)
	}
	states, err := r.h.Quotes.ListInteractionStates(ctx)
	if err != nil || len(states) != 4 {
		t.Errorf("ListInteractionStates = %+v, %v; want 4", states, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Quotes.DeleteAllInteractionStates(ctx, tx) })
	if got, err := r.h.Quotes.ListInteractionStates(ctx); err != nil || len(got) != 0 {
		t.Errorf("ListInteractionStates after DeleteAllInteractionStates = %+v, %v; want none", got, err)
	}

	// The interaction policies: replaced per kind, read for many statuses together.
	statusID := quotes[0].StatusID
	if got, err := r.h.Quotes.FindInteractionPolicy(ctx, statusID); err != nil || len(got) != 0 {
		t.Errorf("FindInteractionPolicy before = %+v, %v; want none", got, err)
	}
	rule := func(kind content.InteractionKind, approval content.InteractionApproval, audience content.InteractionAudience, actor string) content.InteractionRule {
		return content.InteractionRule{Kind: kind, Approval: approval, Audience: audience, ActorURI: actor}
	}
	declared := content.InteractionPolicy{
		rule(content.InteractionQuote, content.ApprovalAutomatic, content.AudienceAuthor, ""),
		rule(content.InteractionQuote, content.ApprovalManual, content.AudienceEveryone, ""),
		rule(content.InteractionReply, content.ApprovalAutomatic, content.AudienceFollowers, ""),
		rule(content.InteractionReply, content.ApprovalAutomatic, content.AudienceAccount, "https://remote.example/users/one"),
		rule(content.InteractionReply, content.ApprovalAutomatic, content.AudienceAccount, "https://remote.example/users/two"),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Quotes.ReplaceInteractionPolicy(ctx, tx, statusID, nil, declared)
	})
	sameRules := func(got, want content.InteractionPolicy) bool {
		return len(got) == len(want) && !slices.ContainsFunc(want, func(r content.InteractionRule) bool { return !slices.Contains(got, r) })
	}
	if got, err := r.h.Quotes.FindInteractionPolicy(ctx, statusID); err != nil || !sameRules(got, declared) {
		t.Errorf("FindInteractionPolicy after the insert = %+v, %v; want %+v", got, err, declared)
	}
	// Replacing the quote rules leaves the reply rules.
	replaced := content.InteractionPolicy{rule(content.InteractionQuote, content.ApprovalAutomatic, content.AudienceEveryone, "")}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Quotes.ReplaceInteractionPolicy(ctx, tx, statusID, []content.InteractionKind{content.InteractionQuote}, replaced)
	})
	want := append(slices.Clone(replaced), declared.Of(content.InteractionReply)...)
	if got, err := r.h.Quotes.FindInteractionPolicy(ctx, statusID); err != nil || !sameRules(got, want) {
		t.Errorf("FindInteractionPolicy after replacing the quote rules = %+v, %v; want %+v", got, err, want)
	}
	if got, err := r.h.Quotes.FindInteractionPolicies(ctx, []string{statusID, other.StatusID}); err != nil || len(got) != 1 || !sameRules(got[statusID], want) {
		t.Errorf("FindInteractionPolicies = %+v, %v; want the rules of %s alone, as FindInteractionPolicy", got, err, statusID)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Quotes.ReplaceInteractionPolicy(ctx, tx, statusID, content.InteractionKinds, nil)
	})
	if got, err := r.h.Quotes.FindInteractionPolicy(ctx, statusID); err != nil || len(got) != 0 {
		t.Errorf("FindInteractionPolicy after removing every kind = %+v, %v; want none", got, err)
	}

	// The approvals of this server, and their revocations.
	auth := content.InteractionAuthorization{
		ID: r.ids.NewID(), Kind: content.InteractionQuote, InteractingURI: "https://remote.example/notes/quoting",
		InteractingAccountID: testkit.NamedID("quoter"), TargetStatusID: r.ids.NewID(), CreatedAt: r.clock.Now(),
	}
	authorize := func(a content.InteractionAuthorization, want bool) {
		t.Helper()
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Quotes.InsertInteractionAuthorization(ctx, tx, a)
			if err == nil && ok != want {
				t.Errorf("InsertInteractionAuthorization(%s) = %v, want %v", a.ID, ok, want)
			}
			return err
		})
	}
	authorize(auth, true)
	again := auth
	again.ID = r.ids.NewID()
	authorize(again, false)
	reply := auth
	reply.ID, reply.Kind = r.ids.NewID(), content.InteractionReply
	authorize(reply, true)
	if got, err := r.h.Quotes.FindInteractionAuthorization(ctx, auth.ID); err != nil || !equalQuoteAuthorization(got, auth) {
		t.Errorf("FindQuoteAuthorization = %+v, %v; want %+v", got, err, auth)
	}
	if got, err := r.h.Quotes.FindInteractionAuthorizationFor(ctx, nil, content.InteractionQuote, auth.InteractingURI, auth.TargetStatusID); err != nil || !equalQuoteAuthorization(got, auth) {
		t.Errorf("FindQuoteAuthorizationFor = %+v, %v; want %+v", got, err, auth)
	}
	if _, err := r.h.Quotes.FindInteractionAuthorization(ctx, again.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindQuoteAuthorization(not recorded) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Quotes.FindInteractionAuthorizationFor(ctx, nil, content.InteractionQuote, auth.InteractingURI, r.ids.NewID()); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindQuoteAuthorizationFor(another status) = %v, want ErrNotFound", err)
	}
	revokedAt := r.clock.Now().Add(time.Minute)
	for i, want := range []bool{true, false} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Quotes.InsertInteractionAuthorizationRevocation(ctx, tx, auth.ID, revokedAt.Add(time.Duration(i)*time.Minute))
			if err == nil && ok != want {
				t.Errorf("InsertQuoteAuthorizationRevocation #%d = %v, want %v", i+1, ok, want)
			}
			return err
		})
	}
	auth.RevokedAt = &revokedAt
	if got, err := r.h.Quotes.FindInteractionAuthorization(ctx, auth.ID); err != nil || !equalQuoteAuthorization(got, auth) {
		t.Errorf("FindQuoteAuthorization after the revocation = %+v, %v; want revoked at the first %v", got, err, revokedAt)
	}
}

func equalInteractionEvent(a, b content.InteractionEvent) bool {
	return a.ID == b.ID && a.RequestID == b.RequestID && a.Kind == b.Kind && a.AuthorizationURI == b.AuthorizationURI && a.OccurredAt.Equal(b.OccurredAt)
}

func equalInteractionRequest(a, b content.InteractionRequest) bool {
	a.CreatedAt, b.CreatedAt = a.CreatedAt.UTC().Truncate(time.Millisecond), b.CreatedAt.UTC().Truncate(time.Millisecond)
	return a == b
}

func equalQuoteAuthorization(a, b content.InteractionAuthorization) bool {
	if (a.RevokedAt == nil) != (b.RevokedAt == nil) || (a.RevokedAt != nil && !a.RevokedAt.Equal(*b.RevokedAt)) {
		return false
	}
	return a.ID == b.ID && a.Kind == b.Kind && a.InteractingURI == b.InteractingURI && a.InteractingAccountID == b.InteractingAccountID &&
		a.TargetStatusID == b.TargetStatusID && a.CreatedAt.Equal(b.CreatedAt)
}

// remoteStatusRepoPins checks the pins that ReplaceRemotePins writes: a pin takes a free slot of
// at most content.MaxPins and is stored once per account and status, found, counted, and listed
// newest first; a pin is deleted alone or with all the account's others.
func remoteStatusRepoPins(t *testing.T, r *remoteStatusRepo) {
	if r.h.Pins == nil {
		t.Skip("the harness has no interaction repository")
	}
	ctx := t.Context()
	owner, other := testkit.NamedID("pin-owner"), testkit.NamedID("pin-other")
	pin := func(accountID, statusID string, want bool) content.Pin {
		t.Helper()
		p := content.Pin{ID: r.ids.NewID(), AccountID: accountID, StatusID: statusID, CreatedAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Pins.InsertPin(ctx, tx, p)
			if err == nil && ok != want {
				t.Errorf("InsertPin(%s, %s) = %v, want %v", accountID, statusID, ok, want)
			}
			return err
		})
		return p
	}
	var pins []content.Pin
	for range content.MaxPins {
		pins = append(pins, pin(owner, r.ids.NewID(), true))
	}
	pin(owner, pins[0].StatusID, false) // pinned already
	pin(owner, r.ids.NewID(), false)    // every slot taken
	kept := pin(other, r.ids.NewID(), true)

	got, err := r.h.Pins.ListPins(ctx, owner)
	if want := slices.Clone(pins); err != nil || !slices.EqualFunc(got, reversed(want), equalPin) {
		t.Errorf("ListPins = %+v, %v; want the %d pins newest first", got, err, content.MaxPins)
	}
	if got, err := r.h.Pins.FindPin(ctx, owner, pins[1].StatusID); err != nil || !equalPin(got, pins[1]) {
		t.Errorf("FindPin = %+v, %v; want %+v", got, err, pins[1])
	}
	if _, err := r.h.Pins.FindPin(ctx, other, pins[1].StatusID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindPin(another account) = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if ok, err := r.h.Pins.HasPin(ctx, tx, owner, pins[2].StatusID); err != nil || !ok {
			t.Errorf("HasPin = %v, %v; want true", ok, err)
		}
		if n, err := r.h.Pins.CountPins(ctx, tx, owner); err != nil || n != content.MaxPins {
			t.Errorf("CountPins = %d, %v; want %d", n, err, content.MaxPins)
		}
		for i, want := range []bool{true, false} {
			if ok, err := r.h.Pins.DeletePin(ctx, tx, owner, pins[2].StatusID); err != nil || ok != want {
				t.Errorf("DeletePin #%d = %v, %v; want %v", i+1, ok, err, want)
			}
		}
		if ok, err := r.h.Pins.HasPin(ctx, tx, owner, pins[2].StatusID); err != nil || ok {
			t.Errorf("HasPin after DeletePin = %v, %v; want false", ok, err)
		}
		return nil
	})
	// The freed slot takes a new pin.
	pin(owner, r.ids.NewID(), true)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if n, err := r.h.Pins.DeletePinsOf(ctx, tx, owner); err != nil || n != content.MaxPins {
			t.Errorf("DeletePinsOf = %d, %v; want %d", n, err, content.MaxPins)
		}
		return nil
	})
	if got, err := r.h.Pins.ListPins(ctx, owner); err != nil || len(got) != 0 {
		t.Errorf("ListPins after DeletePinsOf = %+v, %v; want none", got, err)
	}
	if got, err := r.h.Pins.ListPins(ctx, other); err != nil || len(got) != 1 || !equalPin(got[0], kept) {
		t.Errorf("ListPins(another account) = %+v, %v; want its pin kept", got, err)
	}
}

func equalPin(a, b content.Pin) bool {
	return a.ID == b.ID && a.AccountID == b.AccountID && a.StatusID == b.StatusID && a.CreatedAt.Equal(b.CreatedAt)
}

func reversed[T any](s []T) []T {
	slices.Reverse(s)
	return s
}
