// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The reads in a transaction (the ...Tx reads of the repositories and the use cases over them,
// ADR-0060): run in the transaction of an event by a projector, they see what the transaction
// wrote, and they answer as the reads outside transactions do once it has committed.

// accountReadsInTransaction reads an account whose profile the transaction revises.
func accountReadsInTransaction(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.register(t, "fern")
	var inTx appaccount.Actor
	ran := inTransactionOf(e.projectors, appaccount.EventProfileRevised, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		id := ev.(appaccount.ProfileRevised).Revision.AccountID
		a, err := e.accounts.FindByIDTx(ctx, tx, id)
		if err != nil || a.Profile == nil || a.Profile.DisplayName != "Fern" || a.Local == nil || a.Handle == nil || len(a.Keys) == 0 {
			t.Errorf("FindByIDTx = %+v, %v; want the revised profile", a, err)
		}
		inTx = a
		if la, err := e.accounts.LocalAccountOfUserTx(ctx, tx, reg.User.ID); err != nil || la.AccountID != id {
			t.Errorf("LocalAccountOfUserTx = %+v, %v", la, err)
		}
		if urls, err := e.accounts.RemoteProfileURLsTx(ctx, tx, []string{id}); err != nil || len(urls) != 0 {
			t.Errorf("RemoteProfileURLsTx(local) = %v, %v; want none", urls, err)
		}
	})
	name := "Fern"
	e.clock.Advance(time.Second)
	if _, err := e.accounts.UpdateProfile(ctx, reg.AccountID, appaccount.ProfileUpdate{DisplayName: &name}); err != nil {
		t.Fatalf("UpdateProfile = %v", err)
	}
	wantRan(t, "UpdateProfile", ran)
	stored, err := e.accounts.FindByID(ctx, reg.AccountID)
	if err != nil || stored.Profile == nil || inTx.Profile == nil || stored.Profile.ID != inTx.Profile.ID || stored.Lifecycle != inTx.Lifecycle {
		t.Errorf("FindByID after the commit = %+v, %v; want what FindByIDTx read: %+v", stored, err, inTx)
	}
}

// registrationReadsInTransaction reads a registration that the transaction submits.
func registrationReadsInTransaction(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	r := e.registration(identity.RegistrationApproval, true)
	var inTx appidentity.RegistrationEntry
	ran := inTransactionOf(e.projectors, appidentity.EventRegistrationPending, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		var err error
		inTx, err = r.RegistrationTx(ctx, tx, ev.(appidentity.RegistrationPending).UserID)
		if err != nil || inTx.Username != "alice" || inTx.AccountID == "" || inTx.Email != "alice@example.com" ||
			inTx.State.State != identity.RegistrationPending {
			t.Errorf("RegistrationTx = %+v, %v; want alice's pending registration", inTx, err)
		}
	})
	in := signUpInput("alice")
	in.Reason = "ducks"
	alice := r.signUp(t, in)
	wantRan(t, "SignUp", ran)
	stored, err := r.Registration(ctx, alice.User.ID)
	if err != nil || stored.Application.ID != inTx.Application.ID || stored.Email != inTx.Email || stored.State != inTx.State {
		t.Errorf("Registration after the commit = %+v, %v; want %+v", stored, err, inTx)
	}
}
