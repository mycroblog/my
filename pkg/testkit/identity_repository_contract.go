// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The cases of AccountRepositoryContract on the sign-in methods of the identity repository:
// users, passwords, sessions, TOTP, and passkeys.

// password sets a new password credential of userID and returns it.
func (r *identityRepo) password(t *testing.T, userID, hash string) identity.PasswordCredential {
	t.Helper()
	r.clock.Advance(time.Second)
	c := identity.PasswordCredential{ID: r.ids.NewID(), UserID: userID, Hash: hash, SetAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertPasswordCredential(ctx, tx, c) })
	return c
}

// passkey registers a passkey of userID and returns it.
func (r *identityRepo) passkey(t *testing.T, userID, name string) identity.PasskeyCredential {
	t.Helper()
	r.clock.Advance(time.Second)
	id := r.ids.NewID()
	p := identity.PasskeyCredential{ID: id, UserID: userID, CredentialID: []byte("credential " + id), PublicKey: []byte("cose " + id), BackupEligible: true, Name: name, CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertPasskeyCredential(ctx, tx, p) })
	return p
}

func identityRepoPasswords(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice := r.user(t)
	if u, err := r.h.Identity.FindUser(ctx, alice); err != nil || u.ID != alice {
		t.Errorf("FindUser = %+v, %v", u, err)
	}
	if _, err := r.h.Identity.FindUser(ctx, r.ids.NewID()); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindUser(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Identity.FindCurrentPasswordCredential(ctx, alice); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindCurrentPasswordCredential without a password = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Identity.LastPasswordChange(ctx, alice); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("LastPasswordChange without a password = %v, want ErrNotFound", err)
	}
	r.password(t, alice, "$argon2id$first")
	second := r.password(t, alice, "$argon2id$second")
	if got, err := r.h.Identity.FindCurrentPasswordCredential(ctx, alice); err != nil || got.ID != second.ID || got.Hash != second.Hash {
		t.Errorf("FindCurrentPasswordCredential = %+v, %v; want the newest", got, err)
	}
	if at, err := r.h.Identity.LastPasswordChange(ctx, alice); err != nil || !at.Equal(second.SetAt) {
		t.Errorf("LastPasswordChange = %v, %v; want when the newest was set", at, err)
	}
	r.clock.Advance(time.Minute)
	removedAt := r.clock.Now()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.LockSignInMethods(ctx, tx, alice); err != nil {
			return err
		}
		for range 2 { // a second removal does nothing
			if err := r.h.Identity.InsertPasswordCredentialRemoval(ctx, tx, identity.PasswordCredentialRemoval{PasswordCredentialID: second.ID, RemovedAt: removedAt}); err != nil {
				return err
			}
		}
		if _, err := r.h.Identity.FindCurrentPasswordCredentialTx(ctx, tx, alice); !errors.Is(err, appidentity.ErrNotFound) {
			t.Errorf("FindCurrentPasswordCredentialTx after the removal = %v, want ErrNotFound", err)
		}
		return nil
	})
	if _, err := r.h.Identity.FindCurrentPasswordCredential(ctx, alice); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindCurrentPasswordCredential after the removal = %v, want ErrNotFound (the older one does not come back)", err)
	}
	if at, err := r.h.Identity.LastPasswordChange(ctx, alice); err != nil || !at.Equal(removedAt) {
		t.Errorf("LastPasswordChange after the removal = %v, %v; want the removal", at, err)
	}
}

func identityRepoSessions(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice, bob := r.user(t), r.user(t)
	s1 := r.session(t, alice, "192.0.2.1")
	s2 := r.session(t, alice, "192.0.2.2")
	expired := identity.Session{ID: r.ids.NewID(), UserID: alice, TokenHash: []byte("expired"), CreatedAt: r.clock.Now().Add(-48 * time.Hour), ExpiresAt: r.clock.Now().Add(-time.Hour)}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertSession(ctx, tx, expired) })
	sb := r.session(t, bob, "192.0.2.3")

	got, activity, err := r.h.Identity.FindUnrevokedSession(ctx, s1.TokenHash)
	if err != nil || got.ID != s1.ID || got.UserID != alice || !got.ExpiresAt.Equal(s1.ExpiresAt) || !activity.LastSeenAt.IsZero() {
		t.Errorf("FindUnrevokedSession = %+v, %+v, %v; want s1 without activity", got, activity, err)
	}
	if _, _, err := r.h.Identity.FindUnrevokedSession(ctx, []byte("unknown")); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindUnrevokedSession(unknown) = %v, want ErrNotFound", err)
	}
	// Expiry is the caller's to check.
	if got, _, err := r.h.Identity.FindUnrevokedSession(ctx, expired.TokenHash); err != nil || got.ID != expired.ID {
		t.Errorf("FindUnrevokedSession(expired) = %+v, %v; want the session", got, err)
	}
	for _, seen := range []time.Time{r.clock.Now().Add(time.Minute), r.clock.Now().Add(2 * time.Minute)} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.h.Identity.UpsertSessionActivity(ctx, tx, identity.SessionActivity{SessionID: s1.ID, LastSeenAt: seen})
		})
		if _, a, err := r.h.Identity.FindUnrevokedSession(ctx, s1.TokenHash); err != nil || !a.LastSeenAt.Equal(seen) {
			t.Errorf("activity = %+v, %v; want %v", a, err, seen)
		}
	}
	// A revocation keeps its first reason.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Identity.InsertSessionRevocation(ctx, tx, identity.SessionRevocation{SessionID: s1.ID, Reason: identity.RevokedLogout, RevokedAt: r.clock.Now()}); err != nil {
			return err
		}
		return r.h.Identity.InsertSessionRevocation(ctx, tx, identity.SessionRevocation{SessionID: s1.ID, Reason: identity.RevokedModeration, RevokedAt: r.clock.Now()})
	})
	if _, _, err := r.h.Identity.FindUnrevokedSession(ctx, s1.TokenHash); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindUnrevokedSession after the revocation = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Identity.RevokeSessionsOfUser(ctx, tx, alice, identity.RevokedPasswordChanged, r.clock.Now())
	})
	if _, _, err := r.h.Identity.FindUnrevokedSession(ctx, s2.TokenHash); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindUnrevokedSession(s2) after RevokeSessionsOfUser = %v, want ErrNotFound", err)
	}
	if _, _, err := r.h.Identity.FindUnrevokedSession(ctx, expired.TokenHash); err != nil {
		t.Errorf("FindUnrevokedSession(expired) after RevokeSessionsOfUser = %v, want it left alone", err)
	}
	if _, _, err := r.h.Identity.FindUnrevokedSession(ctx, sb.TokenHash); err != nil {
		t.Errorf("FindUnrevokedSession(bob) after RevokeSessionsOfUser(alice) = %v, want it left alone", err)
	}
}

func identityRepoTOTP(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice := r.user(t)
	if _, err := r.h.Identity.FindActiveTOTPEnrollment(ctx, alice); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindActiveTOTPEnrollment before any = %v, want ErrNotFound", err)
	}
	enroll := func() identity.TOTPEnrollment {
		r.clock.Advance(time.Second)
		e := identity.TOTPEnrollment{ID: r.ids.NewID(), UserID: alice, SecretEnc: []byte("sealed secret"), EnrolledAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.h.Identity.InsertTOTPEnrollment(ctx, tx, e, [][]byte{[]byte("code-1"), []byte("code-2"), []byte("code-3")})
		})
		return e
	}
	first := enroll()
	got, err := r.h.Identity.FindActiveTOTPEnrollment(ctx, alice)
	if err != nil || got.ID != first.ID || string(got.SecretEnc) != "sealed secret" || !got.EnrolledAt.Equal(first.EnrolledAt) {
		t.Errorf("FindActiveTOTPEnrollment = %+v, %v; want %+v", got, err, first)
	}
	use := func(enrollment string, code string) bool {
		var ok bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			ok, err = r.h.Identity.UseTOTPRecoveryCode(ctx, tx, identity.TOTPRecoveryCodeUse{EnrollmentID: enrollment, CodeHash: []byte(code), UsedAt: r.clock.Now()})
			return err
		})
		return ok
	}
	if !use(first.ID, "code-2") || use(first.ID, "code-2") || use(first.ID, "unknown") {
		t.Error("UseTOTPRecoveryCode: want a code to work once, and an unknown code never")
	}
	if n, err := r.h.Identity.CountUnusedTOTPRecoveryCodes(ctx, first.ID); err != nil || n != 2 {
		t.Errorf("CountUnusedTOTPRecoveryCodes = %d, %v; want 2", n, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for range 2 { // a second revocation does nothing
			if err := r.h.Identity.InsertTOTPEnrollmentRevocation(ctx, tx, identity.TOTPEnrollmentRevocation{EnrollmentID: first.ID, RevokedAt: r.clock.Now()}); err != nil {
				return err
			}
		}
		return nil
	})
	if _, err := r.h.Identity.FindActiveTOTPEnrollment(ctx, alice); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindActiveTOTPEnrollment after the revocation = %v, want ErrNotFound", err)
	}
	// A new enrollment has its own codes.
	second := enroll()
	if got, err := r.h.Identity.FindActiveTOTPEnrollment(ctx, alice); err != nil || got.ID != second.ID {
		t.Errorf("FindActiveTOTPEnrollment after enrolling again = %+v, %v", got, err)
	}
	if !use(second.ID, "code-2") {
		t.Error("UseTOTPRecoveryCode of the new enrollment's code = false")
	}
}

func identityRepoPasskeys(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice, bob := r.user(t), r.user(t)
	a1 := r.passkey(t, alice, "phone")
	a2 := r.passkey(t, alice, "laptop")
	r.passkey(t, bob, "phone")
	list, err := r.h.Identity.ListActivePasskeyCredentials(ctx, alice)
	if err != nil || len(list) != 2 || list[0].ID != a1.ID || list[1].ID != a2.ID || list[0].Name != "phone" || !list[0].BackupEligible ||
		!slices.Equal(list[0].PublicKey, a1.PublicKey) {
		t.Errorf("ListActivePasskeyCredentials = %+v, %v; want alice's two, oldest first", list, err)
	}
	if got, err := r.h.Identity.FindActivePasskeyCredential(ctx, a2.CredentialID); err != nil || got.ID != a2.ID || got.UserID != alice {
		t.Errorf("FindActivePasskeyCredential = %+v, %v", got, err)
	}
	if _, err := r.h.Identity.FindPasskeyCredential(ctx, []byte("unknown")); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindPasskeyCredential(unknown) = %v, want ErrNotFound", err)
	}
	dup := a1
	dup.ID = r.ids.NewID()
	if err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.h.Identity.InsertPasskeyCredential(ctx, tx, dup) }); err == nil {
		t.Error("InsertPasskeyCredential with a taken credential ID succeeded")
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for range 2 { // a second revocation does nothing
			if err := r.h.Identity.InsertPasskeyCredentialRevocation(ctx, tx, identity.PasskeyCredentialRevocation{PasskeyID: a1.ID, RevokedAt: r.clock.Now()}); err != nil {
				return err
			}
		}
		return nil
	})
	if _, err := r.h.Identity.FindActivePasskeyCredential(ctx, a1.CredentialID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindActivePasskeyCredential(revoked) = %v, want ErrNotFound", err)
	}
	if got, err := r.h.Identity.FindPasskeyCredential(ctx, a1.CredentialID); err != nil || got.ID != a1.ID {
		t.Errorf("FindPasskeyCredential(revoked) = %+v, %v; want the passkey", got, err)
	}
	if list, err := r.h.Identity.ListActivePasskeyCredentials(ctx, alice); err != nil || len(list) != 1 || list[0].ID != a2.ID {
		t.Errorf("ListActivePasskeyCredentials after the revocation = %+v, %v; want the laptop", list, err)
	}
}

// identityRepoTxReads reads the sign-in methods of a user inside the transaction that changes
// them, under LockSignInMethods, as the use cases that remove one do.
func identityRepoTxReads(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice := r.user(t)
	err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := r.h.Identity.LockSignInMethods(ctx, tx, alice); err != nil {
			return err
		}
		c := identity.PasswordCredential{ID: r.ids.NewID(), UserID: alice, Hash: "$argon2id$x", SetAt: r.clock.Now()}
		if err := r.h.Identity.InsertPasswordCredential(ctx, tx, c); err != nil {
			return err
		}
		p := identity.PasskeyCredential{ID: r.ids.NewID(), UserID: alice, CredentialID: []byte("in tx"), PublicKey: []byte("cose"), Name: "key", CreatedAt: r.clock.Now()}
		if err := r.h.Identity.InsertPasskeyCredential(ctx, tx, p); err != nil {
			return err
		}
		if got, err := r.h.Identity.FindCurrentPasswordCredentialTx(ctx, tx, alice); err != nil || got.ID != c.ID {
			t.Errorf("FindCurrentPasswordCredentialTx = %+v, %v; want the password written", got, err)
		}
		if got, err := r.h.Identity.ListActivePasskeyCredentialsTx(ctx, tx, alice); err != nil || len(got) != 1 || got[0].ID != p.ID {
			t.Errorf("ListActivePasskeyCredentialsTx = %+v, %v; want the passkey written", got, err)
		}
		if got, err := r.h.Identity.ListLinkedExternalIdentitiesTx(ctx, tx, alice); err != nil || len(got) != 0 {
			t.Errorf("ListLinkedExternalIdentitiesTx = %+v, %v; want none", got, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
