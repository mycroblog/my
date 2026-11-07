// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The cases of RemoteStatusContract on the quotes of posts (ADR-0069) and on the pins that the
// featured collections of remote accounts become (ADR-0006).

// remoteStatusRepoQuotes checks the quote repository: a status quotes one status, found by its
// status and listed in pages by status ID and per quoted URI, newest first; its events read back
// in order; the policies are upserted and deleted; an approval is recorded once per quoting URI and
// quoted status, found with its revocation, which is recorded once; and the projection of the
// states is upserted, listed, and deleted for its rebuild.
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

	// The events of a quote, in the order they occurred.
	events := []content.QuoteEvent{
		{ID: r.ids.NewID(), StatusID: quotes[0].StatusID, Kind: content.QuoteEventAccepted, AuthorizationURI: "https://remote.example/auth/1", OccurredAt: r.clock.Now()},
		{ID: r.ids.NewID(), StatusID: quotes[0].StatusID, Kind: content.QuoteEventRevoked, AuthorizationURI: "https://remote.example/auth/1", OccurredAt: r.clock.Now().Add(time.Second)},
		{ID: r.ids.NewID(), StatusID: quotes[0].StatusID, Kind: content.QuoteEventDetached, OccurredAt: r.clock.Now().Add(2 * time.Second)},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, e := range events {
			if err := r.h.Quotes.InsertQuoteEvent(ctx, tx, e); err != nil {
				return err
			}
		}
		got, err := r.h.Quotes.ListQuoteEvents(ctx, tx, quotes[0].StatusID)
		if err != nil || !slices.EqualFunc(got, events, equalQuoteEvent) {
			t.Errorf("ListQuoteEvents = %+v, %v; want %+v", got, err, events)
		}
		return nil
	})

	// The policies.
	policy := content.StatusQuotePolicy{StatusID: quotes[0].StatusID, Policy: content.QuotePolicyFollowers, UpdatedAt: r.clock.Now()}
	if _, err := r.h.Quotes.FindQuotePolicy(ctx, policy.StatusID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindQuotePolicy before = %v, want ErrNotFound", err)
	}
	for _, p := range []content.QuotePolicy{content.QuotePolicyFollowers, content.QuotePolicyNobody} {
		policy.Policy, policy.UpdatedAt = p, policy.UpdatedAt.Add(time.Second)
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Quotes.UpsertQuotePolicy(ctx, tx, policy) })
		if got, err := r.h.Quotes.FindQuotePolicy(ctx, policy.StatusID); err != nil || got.Policy != p || !got.UpdatedAt.Equal(policy.UpdatedAt) {
			t.Errorf("FindQuotePolicy after the upsert of %s = %+v, %v", p, got, err)
		}
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Quotes.DeleteQuotePolicy(ctx, tx, policy.StatusID)
	})
	if _, err := r.h.Quotes.FindQuotePolicy(ctx, policy.StatusID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindQuotePolicy after the delete = %v, want ErrNotFound", err)
	}

	// The approvals of this server, and their revocations.
	auth := content.QuoteAuthorization{
		ID: r.ids.NewID(), QuotingURI: "https://remote.example/notes/quoting", QuotingAccountID: NamedID("quoter"),
		QuotedStatusID: r.ids.NewID(), CreatedAt: r.clock.Now(),
	}
	authorize := func(a content.QuoteAuthorization, want bool) {
		t.Helper()
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Quotes.InsertQuoteAuthorization(ctx, tx, a)
			if err == nil && ok != want {
				t.Errorf("InsertQuoteAuthorization(%s) = %v, want %v", a.ID, ok, want)
			}
			return err
		})
	}
	authorize(auth, true)
	again := auth
	again.ID = r.ids.NewID()
	authorize(again, false)
	if got, err := r.h.Quotes.FindQuoteAuthorization(ctx, auth.ID); err != nil || !equalQuoteAuthorization(got, auth) {
		t.Errorf("FindQuoteAuthorization = %+v, %v; want %+v", got, err, auth)
	}
	if got, err := r.h.Quotes.FindQuoteAuthorizationFor(ctx, auth.QuotingURI, auth.QuotedStatusID); err != nil || !equalQuoteAuthorization(got, auth) {
		t.Errorf("FindQuoteAuthorizationFor = %+v, %v; want %+v", got, err, auth)
	}
	if _, err := r.h.Quotes.FindQuoteAuthorization(ctx, again.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindQuoteAuthorization(not recorded) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Quotes.FindQuoteAuthorizationFor(ctx, auth.QuotingURI, r.ids.NewID()); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindQuoteAuthorizationFor(another status) = %v, want ErrNotFound", err)
	}
	revokedAt := r.clock.Now().Add(time.Minute)
	for i, want := range []bool{true, false} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Quotes.InsertQuoteAuthorizationRevocation(ctx, tx, auth.ID, revokedAt.Add(time.Duration(i)*time.Minute))
			if err == nil && ok != want {
				t.Errorf("InsertQuoteAuthorizationRevocation #%d = %v, want %v", i+1, ok, want)
			}
			return err
		})
	}
	auth.RevokedAt = &revokedAt
	if got, err := r.h.Quotes.FindQuoteAuthorization(ctx, auth.ID); err != nil || !equalQuoteAuthorization(got, auth) {
		t.Errorf("FindQuoteAuthorization after the revocation = %+v, %v; want revoked at the first %v", got, err, revokedAt)
	}

	// The projection of the states.
	states := []content.QuoteState{
		{StatusID: quotes[0].StatusID, Approval: content.QuoteApprovalAccepted, AuthorizationURI: "https://remote.example/auth/1"},
		{StatusID: quotes[1].StatusID, Approval: content.QuoteApprovalLegacy},
		{StatusID: quotes[2].StatusID, Approval: content.QuoteApprovalAccepted, AuthorizationURI: "https://remote.example/auth/1"},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, s := range append([]content.QuoteState{{StatusID: quotes[0].StatusID, Approval: content.QuoteApprovalPending}}, states...) {
			if err := r.h.Quotes.UpsertQuoteState(ctx, tx, s); err != nil {
				return err
			}
		}
		if got, err := r.h.Quotes.FindQuoteStateTx(ctx, tx, quotes[0].StatusID); err != nil || got != states[0] {
			t.Errorf("FindQuoteStateTx = %+v, %v; want the upserted %+v", got, err, states[0])
		}
		return nil
	})
	if got, err := r.h.Quotes.FindQuoteState(ctx, quotes[1].StatusID); err != nil || got != states[1] {
		t.Errorf("FindQuoteState = %+v, %v; want %+v", got, err, states[1])
	}
	if _, err := r.h.Quotes.FindQuoteState(ctx, other.StatusID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindQuoteState(none) = %v, want ErrNotFound", err)
	}
	if got, err := r.h.Quotes.ListQuoteStatesByAuthorization(ctx, "https://remote.example/auth/1"); err != nil || !slices.Equal(got, []content.QuoteState{states[0], states[2]}) {
		t.Errorf("ListQuoteStatesByAuthorization = %+v, %v; want the accepted ones in status ID order", got, err)
	}
	if got, err := r.h.Quotes.ListQuoteStates(ctx); err != nil || !slices.Equal(got, states) {
		t.Errorf("ListQuoteStates = %+v, %v; want %+v", got, err, states)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Quotes.DeleteAllQuoteStates(ctx, tx) })
	if got, err := r.h.Quotes.ListQuoteStates(ctx); err != nil || len(got) != 0 {
		t.Errorf("ListQuoteStates after DeleteAllQuoteStates = %+v, %v; want none", got, err)
	}
}

func equalQuoteEvent(a, b content.QuoteEvent) bool {
	return a.ID == b.ID && a.StatusID == b.StatusID && a.Kind == b.Kind && a.AuthorizationURI == b.AuthorizationURI && a.OccurredAt.Equal(b.OccurredAt)
}

func equalQuoteAuthorization(a, b content.QuoteAuthorization) bool {
	if (a.RevokedAt == nil) != (b.RevokedAt == nil) || (a.RevokedAt != nil && !a.RevokedAt.Equal(*b.RevokedAt)) {
		return false
	}
	return a.ID == b.ID && a.QuotingURI == b.QuotingURI && a.QuotingAccountID == b.QuotingAccountID &&
		a.QuotedStatusID == b.QuotedStatusID && a.CreatedAt.Equal(b.CreatedAt)
}

// remoteStatusRepoPins checks the pins that ReplaceRemotePins writes: a pin takes a free slot of
// at most content.MaxPins and is stored once per account and status, found, counted, and listed
// newest first; a pin is deleted alone or with all the account's others.
func remoteStatusRepoPins(t *testing.T, r *remoteStatusRepo) {
	if r.h.Pins == nil {
		t.Skip("the harness has no interaction repository")
	}
	ctx := t.Context()
	owner, other := NamedID("pin-owner"), NamedID("pin-other")
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
