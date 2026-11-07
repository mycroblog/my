// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// RegistrationRepositoryContract verifies the registration persistence of a database adapter's
// identity repository (data-model §4.1) by its methods alone, the port contract of ADR-0153 §1:
// applications found by user and by ID, with the note of their decision; the registration state
// set by an upsert and moved from pending once; the registrations of a state listed longest in it
// first with their email, in pages, and counted; the primary email address, one per normalized
// form, and its verification recorded once; invites unique by code, found by code and by ID,
// listed newest first with their usage, the members' invites apart and their usable ones counted;
// the redemptions of an invite, which never exceed its uses when transactions redeem it
// concurrently (ADR-0153 §5); user tokens found by hash and redeemed once; and the reads of the
// same rows inside a transaction. The sign-up use cases on top of it are RegistrationFlows, which
// internal/flowtest/identity runs once on SQLite.
func RegistrationRepositoryContract(t *testing.T, newHarness AccountFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *identityRepo)
	}{
		{"RecordsApplicationsAndDecisions", registrationRepoApplications},
		{"ListsAndCountsRegistrations", registrationRepoLists},
		{"KeepsOnePrimaryEmailPerAddress", registrationRepoEmails},
		{"KeepsInvites", registrationRepoInvites},
		{"CountsUsableMemberInvites", registrationRepoMemberInvites},
		{"HoldsInviteUsesUnderConcurrency", registrationRepoConcurrentRedemptions},
		{"RedeemsUserTokensOnce", registrationRepoTokens},
		{"ReadsInATransaction", registrationRepoTxReads},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newIdentityRepo(t, newHarness)) })
	}
}

// apply records a registration application of userID with its state, as a sign-up does.
func (r *identityRepo) apply(t *testing.T, userID string, state identity.RegistrationState) identity.RegistrationApplication {
	t.Helper()
	a := identity.RegistrationApplication{ID: r.ids.NewID(), UserID: userID, Reason: "reason of " + userID, SubmittedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.InsertRegistrationApplication(ctx, tx, a); err != nil {
			return err
		}
		return r.h.Identity.UpsertRegistrationState(ctx, tx, identity.UserRegistrationState{UserID: userID, State: state, Since: r.clock.Now()})
	})
	return a
}

// email makes address the primary email address of userID and returns it.
func (r *identityRepo) email(t *testing.T, userID, address string) identity.EmailAddress {
	t.Helper()
	e := identity.EmailAddress{ID: r.ids.NewID(), UserID: userID, Address: address, AddressNormalized: strings.ToLower(address), AddedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.InsertEmailAddress(ctx, tx, e); err != nil {
			return err
		}
		return r.h.Identity.InsertPrimaryEmail(ctx, tx, e)
	})
	return e
}

// invite inserts an invite of creator (staff when creator is "").
func (r *identityRepo) invite(t *testing.T, creator string, maxUses int, expires *time.Time) identity.Invite {
	t.Helper()
	r.clock.Advance(time.Second)
	kind := identity.InviteByStaff
	if creator != "" {
		kind = identity.InviteByMember
	}
	id := r.ids.NewID()
	i := identity.Invite{ID: id, Code: strings.ToUpper(strings.ReplaceAll(id, "-", ""))[:identity.InviteCodeLength], CreatedByUserID: creator, CreatorKind: kind, MaxUses: maxUses, ExpiresAt: expires, CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertInvite(ctx, tx, i) })
	return i
}

func registrationRepoApplications(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice := r.user(t)
	a := r.apply(t, alice, identity.RegistrationPending)
	if got, err := r.h.Identity.FindRegistrationApplication(ctx, alice); err != nil || got.ID != a.ID || got.Reason != a.Reason || !got.SubmittedAt.Equal(a.SubmittedAt) {
		t.Errorf("FindRegistrationApplication = %+v, %v; want %+v", got, err, a)
	}
	if got, err := r.h.Identity.FindRegistrationApplicationByID(ctx, a.ID); err != nil || got.UserID != alice {
		t.Errorf("FindRegistrationApplicationByID = %+v, %v", got, err)
	}
	if _, err := r.h.Identity.FindRegistrationApplication(ctx, r.ids.NewID()); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindRegistrationApplication(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Identity.FindRegistrationApplicationByID(ctx, r.ids.NewID()); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindRegistrationApplicationByID(unknown) = %v, want ErrNotFound", err)
	}
	if note, err := r.h.Identity.RegistrationDecisionNote(ctx, a.ID); err != nil || note != "" {
		t.Errorf("RegistrationDecisionNote before a decision = %q, %v; want empty", note, err)
	}
	r.clock.Advance(time.Minute)
	moderator := r.user(t)
	approved := identity.UserRegistrationState{UserID: alice, State: identity.RegistrationApproved, Since: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.InsertRegistrationDecision(ctx, tx, identity.RegistrationDecision{
			ID: r.ids.NewID(), ApplicationID: a.ID, Decision: identity.DecisionApproved, DecidedBy: identity.DecidedByModerator,
			DecidedByUserID: moderator, Note: "welcome", DecidedAt: r.clock.Now(),
		}); err != nil {
			return err
		}
		for i, want := range []bool{true, false} { // only from pending
			ok, err := r.h.Identity.TransitionPendingRegistrationState(ctx, tx, approved)
			if err != nil {
				return err
			}
			if ok != want {
				t.Errorf("TransitionPendingRegistrationState #%d = %t, want %t", i+1, ok, want)
			}
		}
		return nil
	})
	if note, err := r.h.Identity.RegistrationDecisionNote(ctx, a.ID); err != nil || note != "welcome" {
		t.Errorf("RegistrationDecisionNote = %q, %v; want welcome", note, err)
	}
	if got, err := r.h.Identity.FindRegistrationState(ctx, alice); err != nil || got.State != identity.RegistrationApproved || !got.Since.Equal(approved.Since) {
		t.Errorf("FindRegistrationState = %+v, %v; want %+v", got, err, approved)
	}
	// The upsert sets any state.
	rejected := identity.UserRegistrationState{UserID: alice, State: identity.RegistrationRejected, Since: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Identity.UpsertRegistrationState(ctx, tx, rejected)
	})
	if got, err := r.h.Identity.FindRegistrationState(ctx, alice); err != nil || got.State != identity.RegistrationRejected {
		t.Errorf("FindRegistrationState after the upsert = %+v, %v; want rejected", got, err)
	}
	if _, err := r.h.Identity.FindRegistrationState(ctx, r.ids.NewID()); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindRegistrationState(unknown) = %v, want ErrNotFound", err)
	}
}

func registrationRepoLists(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	var pending []string
	for i := range 3 {
		u := r.user(t)
		r.apply(t, u, identity.RegistrationPending)
		r.email(t, u, fmt.Sprintf("user%d@example.com", i))
		pending = append(pending, u)
	}
	r.apply(t, r.user(t), identity.RegistrationApproved)
	if n, err := r.h.Identity.CountRegistrations(ctx, identity.RegistrationPending); err != nil || n != 3 {
		t.Errorf("CountRegistrations(pending) = %d, %v; want 3", n, err)
	}
	var got []string
	var after appidentity.RegistrationPosition
	for range 3 {
		page, err := r.h.Identity.ListRegistrations(ctx, identity.RegistrationPending, after, 2)
		if err != nil || len(page) > 2 {
			t.Fatalf("ListRegistrations = %d entries, %v; want at most 2", len(page), err)
		}
		for _, e := range page {
			if e.State.State != identity.RegistrationPending || e.Email == "" || e.EmailVerified || e.Application.UserID != e.State.UserID {
				t.Errorf("entry = %+v; want a pending registration with its unverified email", e)
			}
			got = append(got, e.Application.UserID)
			after = e.Position()
		}
	}
	if !slices.Equal(got, pending) {
		t.Errorf("ListRegistrations pages = %v, want %v (longest pending first)", got, pending)
	}
}

func registrationRepoEmails(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice, bob := r.user(t), r.user(t)
	e := r.email(t, alice, "Alice@Example.com")
	if got, verified, err := r.h.Identity.FindPrimaryEmail(ctx, alice); err != nil || got.ID != e.ID || got.Address != e.Address || verified {
		t.Errorf("FindPrimaryEmail = %+v, %t, %v; want the unverified address", got, verified, err)
	}
	if id, err := r.h.Identity.FindUserIDByEmail(ctx, "alice@example.com"); err != nil || id != alice {
		t.Errorf("FindUserIDByEmail = %q, %v; want alice", id, err)
	}
	if _, err := r.h.Identity.FindUserIDByEmail(ctx, "nobody@example.com"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindUserIDByEmail(unknown) = %v, want ErrNotFound", err)
	}
	taken := identity.EmailAddress{ID: r.ids.NewID(), UserID: bob, Address: "ALICE@example.com", AddressNormalized: "alice@example.com", AddedAt: r.clock.Now()}
	err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := r.h.Identity.InsertEmailAddress(ctx, tx, taken); err != nil {
			return err
		}
		return r.h.Identity.InsertPrimaryEmail(ctx, tx, taken)
	})
	if !errors.Is(err, appidentity.ErrEmailTaken) {
		t.Errorf("InsertPrimaryEmail of a taken address = %v, want ErrEmailTaken", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for range 2 { // a second verification does nothing
			if err := r.h.Identity.InsertEmailVerification(ctx, tx, identity.EmailVerification{EmailAddressID: e.ID, VerifiedAt: r.clock.Now()}); err != nil {
				return err
			}
		}
		return nil
	})
	if _, verified, err := r.h.Identity.FindPrimaryEmail(ctx, alice); err != nil || !verified {
		t.Errorf("FindPrimaryEmail after the verification = %t, %v; want verified", verified, err)
	}
	if _, _, err := r.h.Identity.FindPrimaryEmail(ctx, bob); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindPrimaryEmail(without one) = %v, want ErrNotFound", err)
	}
}

func registrationRepoInvites(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	expires := r.clock.Now().Add(time.Hour)
	a := r.invite(t, "", 2, &expires)
	b := r.invite(t, "", 0, nil)
	if got, err := r.h.Identity.FindInviteByCode(ctx, a.Code); err != nil || got.ID != a.ID || got.MaxUses != 2 || got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
		t.Errorf("FindInviteByCode = %+v, %v; want %+v", got, err, a)
	}
	if got, err := r.h.Identity.FindInviteByID(ctx, b.ID); err != nil || got.Code != b.Code || got.ExpiresAt != nil || got.CreatorKind != identity.InviteByStaff {
		t.Errorf("FindInviteByID = %+v, %v; want %+v", got, err, b)
	}
	if _, err := r.h.Identity.FindInviteByCode(ctx, "NOSUCHCODE000000"); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindInviteByCode(unknown) = %v, want ErrNotFound", err)
	}
	dup := a
	dup.ID = r.ids.NewID()
	if err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.h.Identity.InsertInvite(ctx, tx, dup) }); !errors.Is(err, appidentity.ErrInviteCodeTaken) {
		t.Errorf("InsertInvite with a taken code = %v, want ErrInviteCodeTaken", err)
	}
	u := r.user(t)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.InsertInviteRedemption(ctx, tx, identity.InviteRedemption{InviteID: a.ID, Seq: 1, UserID: u, RedeemedAt: r.clock.Now()}); err != nil {
			return err
		}
		for range 2 { // a second revocation does nothing
			if err := r.h.Identity.InsertInviteRevocation(ctx, tx, identity.InviteRevocation{InviteID: b.ID, RevokedAt: r.clock.Now()}); err != nil {
				return err
			}
		}
		if uses, revoked, err := r.h.Identity.InviteUsage(ctx, tx, a.ID); err != nil || uses != 1 || revoked {
			t.Errorf("InviteUsage(a) = %d, %t, %v; want 1 use", uses, revoked, err)
		}
		if uses, revoked, err := r.h.Identity.InviteUsage(ctx, tx, b.ID); err != nil || uses != 0 || !revoked {
			t.Errorf("InviteUsage(b) = %d, %t, %v; want revoked", uses, revoked, err)
		}
		return nil
	})
	list, err := r.h.Identity.ListInvites(ctx, "", 10)
	if err != nil || len(list) != 2 || list[0].ID != b.ID || !list[0].Revoked || list[1].ID != a.ID || list[1].Uses != 1 {
		t.Errorf("ListInvites = %+v, %v; want b (revoked) then a (one use)", list, err)
	}
	if page, err := r.h.Identity.ListInvites(ctx, b.ID, 10); err != nil || len(page) != 1 || page[0].ID != a.ID {
		t.Errorf("ListInvites(before b) = %+v, %v; want a", page, err)
	}
	if page, err := r.h.Identity.ListInvites(ctx, "", 1); err != nil || len(page) != 1 || page[0].ID != b.ID {
		t.Errorf("ListInvites(limit 1) = %+v, %v; want b", page, err)
	}
}

func registrationRepoMemberInvites(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice, bob := r.user(t), r.user(t)
	past := r.clock.Now().Add(-time.Minute)
	usable := r.invite(t, alice, 0, nil)
	expired := r.invite(t, alice, 0, &past)
	revoked := r.invite(t, alice, 0, nil)
	usedUp := r.invite(t, alice, 1, nil)
	r.invite(t, bob, 0, nil)
	r.invite(t, "", 0, nil)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.InsertInviteRevocation(ctx, tx, identity.InviteRevocation{InviteID: revoked.ID, RevokedByUserID: alice, RevokedAt: r.clock.Now()}); err != nil {
			return err
		}
		if err := r.h.Identity.InsertInviteRedemption(ctx, tx, identity.InviteRedemption{InviteID: usedUp.ID, Seq: 1, UserID: bob, RedeemedAt: r.clock.Now()}); err != nil {
			return err
		}
		if n, err := r.h.Identity.CountUsableMemberInvites(ctx, tx, alice, r.clock.Now()); err != nil || n != 1 {
			t.Errorf("CountUsableMemberInvites = %d, %v; want 1 (%s)", n, err, usable.ID)
		}
		return nil
	})
	list, err := r.h.Identity.ListMemberInvites(ctx, alice, "", 10)
	var ids []string
	for _, i := range list {
		ids = append(ids, i.ID)
	}
	if want := []string{usedUp.ID, revoked.ID, expired.ID, usable.ID}; err != nil || !slices.Equal(ids, want) {
		t.Errorf("ListMemberInvites = %v, %v; want alice's four, newest first", ids, err)
	}
	if page, err := r.h.Identity.ListMemberInvites(ctx, alice, revoked.ID, 1); err != nil || len(page) != 1 || page[0].ID != expired.ID {
		t.Errorf("ListMemberInvites(before, 1) = %+v, %v; want the expired one", page, err)
	}
}

// registrationRepoConcurrentRedemptions redeems an invite of two uses from four transactions at
// once, each counting the uses and recording the next one while uses remain: the invite ends
// with two redemptions.
func registrationRepoConcurrentRedemptions(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	i := r.invite(t, "", 2, nil)
	users := []string{r.user(t), r.user(t), r.user(t), r.user(t)}
	var wg sync.WaitGroup
	for _, u := range users {
		wg.Go(func() {
			err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				uses, _, err := r.h.Identity.InviteUsage(ctx, tx, i.ID)
				if err != nil || uses >= i.MaxUses {
					return err
				}
				return r.h.Identity.InsertInviteRedemption(ctx, tx, identity.InviteRedemption{InviteID: i.ID, Seq: uses + 1, UserID: u, RedeemedAt: r.clock.Now()})
			})
			if err != nil {
				t.Errorf("redeem = %v", err)
			}
		})
	}
	wg.Wait()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if uses, _, err := r.h.Identity.InviteUsage(ctx, tx, i.ID); err != nil || uses != 2 {
			t.Errorf("InviteUsage = %d, %v; want 2", uses, err)
		}
		return nil
	})
}

func registrationRepoTokens(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice := r.user(t)
	e := r.email(t, alice, "alice@example.com")
	tok := identity.UserToken{
		ID: r.ids.NewID(), UserID: alice, Purpose: identity.PurposeEmailVerification, EmailAddressID: e.ID,
		TokenHash: []byte("hash of the token"), ExpiresAt: r.clock.Now().Add(24 * time.Hour), IssuedAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertUserToken(ctx, tx, tok) })
	got, err := r.h.Identity.FindUserTokenByHash(ctx, tok.TokenHash)
	if err != nil || got.ID != tok.ID || got.Purpose != tok.Purpose || got.EmailAddressID != e.ID || !got.ExpiresAt.Equal(tok.ExpiresAt) {
		t.Errorf("FindUserTokenByHash = %+v, %v; want %+v", got, err, tok)
	}
	if _, err := r.h.Identity.FindUserTokenByHash(ctx, []byte("unknown")); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindUserTokenByHash(unknown) = %v, want ErrNotFound", err)
	}
	if used, err := r.h.Identity.UserTokenRedeemed(ctx, tok.ID); err != nil || used {
		t.Errorf("UserTokenRedeemed before = %t, %v; want false", used, err)
	}
	for i := range 2 {
		err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return r.h.Identity.InsertUserTokenRedemption(ctx, tx, identity.UserTokenRedemption{TokenID: tok.ID, RedeemedAt: r.clock.Now()})
		})
		if (i == 1) != errors.Is(err, appidentity.ErrInvalidToken) || i == 0 && err != nil {
			t.Errorf("InsertUserTokenRedemption #%d = %v; want ErrInvalidToken only the second time", i+1, err)
		}
	}
	if used, err := r.h.Identity.UserTokenRedeemed(ctx, tok.ID); err != nil || !used {
		t.Errorf("UserTokenRedeemed after = %t, %v; want true", used, err)
	}
}

// registrationRepoTxReads reads, inside the transaction that writes them, the rows of a
// registration that the use cases present or check under a lock.
func registrationRepoTxReads(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	var rejected []string
	for range 3 {
		u := r.user(t)
		r.apply(t, u, identity.RegistrationRejected)
		rejected = append([]string{u}, rejected...) // by ID, highest first
	}
	alice := r.user(t)
	err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		a := identity.RegistrationApplication{ID: r.ids.NewID(), UserID: alice, SubmittedAt: r.clock.Now()}
		if err := r.h.Identity.InsertRegistrationApplication(ctx, tx, a); err != nil {
			return err
		}
		if err := r.h.Identity.UpsertRegistrationState(ctx, tx, identity.UserRegistrationState{UserID: alice, State: identity.RegistrationPending, Since: r.clock.Now()}); err != nil {
			return err
		}
		e := identity.EmailAddress{ID: r.ids.NewID(), UserID: alice, Address: "alice@example.com", AddressNormalized: "alice@example.com", AddedAt: r.clock.Now()}
		if err := r.h.Identity.InsertEmailAddress(ctx, tx, e); err != nil {
			return err
		}
		if err := r.h.Identity.InsertPrimaryEmail(ctx, tx, e); err != nil {
			return err
		}
		if got, err := r.h.Identity.FindRegistrationApplicationTx(ctx, tx, alice); err != nil || got.ID != a.ID {
			t.Errorf("FindRegistrationApplicationTx = %+v, %v; want the application written", got, err)
		}
		if got, err := r.h.Identity.FindRegistrationStateTx(ctx, tx, alice); err != nil || got.State != identity.RegistrationPending {
			t.Errorf("FindRegistrationStateTx = %+v, %v; want pending", got, err)
		}
		if got, verified, err := r.h.Identity.FindPrimaryEmailTx(ctx, tx, alice); err != nil || got.ID != e.ID || verified {
			t.Errorf("FindPrimaryEmailTx = %+v, %t, %v; want the unverified address", got, verified, err)
		}
		if _, err := r.h.Identity.FindRegistrationApplicationTx(ctx, tx, r.ids.NewID()); !errors.Is(err, appidentity.ErrNotFound) {
			t.Errorf("FindRegistrationApplicationTx(unknown) = %v, want ErrNotFound", err)
		}
		var got []string
		before := ""
		for range 3 {
			ids, err := r.h.Identity.ListUserIDsInRegistrationStateTx(ctx, tx, identity.RegistrationRejected, before, 2)
			if err != nil {
				return err
			}
			for _, id := range ids {
				got = append(got, id)
				before = id
			}
		}
		if !slices.Equal(got, rejected) {
			t.Errorf("ListUserIDsInRegistrationStateTx pages = %v, want %v", got, rejected)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
