// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The cases of AccountErasureFlows on the purge of expired sign-in records.

const authPurgeDay = 24 * time.Hour

// authPurgeUserToken stores a one-time token of the user that expires an hour from now.
func (e *erasureEnv) authPurgeUserToken(t *testing.T, userID string) string {
	t.Helper()
	now := e.clock.Now()
	id := e.ids.NewID()
	err := e.eh.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return e.eh.Identity.InsertUserToken(t.Context(), tx, identity.UserToken{
			ID: id, UserID: userID, Purpose: identity.PurposePasswordReset,
			TokenHash: appidentity.HashToken(id), ExpiresAt: now.Add(time.Hour), IssuedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("InsertUserToken = %v", err)
	}
	return id
}

func (e *erasureEnv) wantAuthRows(t *testing.T, table, column, value string, want int, when string) {
	t.Helper()
	if got := e.eh.Rows(t, table, column, value); got != want {
		t.Errorf("%s: rows of %s = %d, want %d", when, table, got, want)
	}
}

// authRecordsPurged walks the records of a user through 81 days: an unused code and a one-time
// token go 30 days after they expire, a session 30 days after it expires, and a token family and
// the session that its code names 30 days after the last token of the family expires.
func authRecordsPurged(t *testing.T, e *erasureEnv) {
	ctx := t.Context()
	userID := e.sessionUser(t).User.ID
	purger := appidentity.NewAuthRecordPurger(appidentity.AuthRecordPurgeDeps{
		UnitOfWork: e.eh.UnitOfWork, Repository: e.eh.Identity, Clock: e.clock,
	})
	run := func(when string) {
		t.Helper()
		if _, err := purger.Run(ctx); err != nil {
			t.Fatalf("%s: Run = %v", when, err)
		}
	}

	browser := e.signIn(t, appidentity.SignInInput{IP: "192.0.2.7"})
	code := e.codeInSession(t, webAuthorizationRequest(), browser.Session)
	web, err := e.identity.ExchangeAuthorizationCode(ctx, appidentity.CodeExchange{
		ClientCredentials: webCreds, Code: code, RedirectURI: contractWebRedirect, CodeVerifier: contractCodeVerifier,
	})
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode = %v", err)
	}
	family := web.Token.FamilyID
	err = e.eh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.eh.Identity.UpsertTokenFamilyLastUsed(ctx, tx, family, e.clock.Now())
	})
	if err != nil {
		t.Fatal(err)
	}

	// A session that is signed out, with a code nobody exchanged.
	other := e.signIn(t, appidentity.SignInInput{})
	e.codeInSession(t, webAuthorizationRequest(), other.Session)
	if err := e.identity.SignOut(ctx, other.Token); err != nil {
		t.Fatalf("SignOut = %v", err)
	}
	token := e.authPurgeUserToken(t, userID)
	err = e.eh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.eh.Identity.InsertUserTokenRedemption(ctx, tx, identity.UserTokenRedemption{TokenID: token, RedeemedAt: e.clock.Now()})
	})
	if err != nil {
		t.Fatal(err)
	}

	// Nothing has been expired for 30 days.
	e.clock.Advance(20 * authPurgeDay)
	rotated, err := e.identity.RefreshWebClient(ctx, web.RefreshToken) // the last refresh token now expires at day 50
	if err != nil {
		t.Fatalf("RefreshWebClient = %v", err)
	}
	run("after 20 days")
	e.wantAuthRows(t, "oauth_authorization_codes", "session_id", other.Session.ID, 1, "after 20 days")
	e.wantAuthRows(t, "user_tokens", "id", token, 1, "after 20 days")

	e.clock.Advance(11 * authPurgeDay) // day 31
	run("after 31 days")
	e.wantAuthRows(t, "oauth_authorization_codes", "session_id", other.Session.ID, 0, "an unused code 30 days after it expired")
	e.wantAuthRows(t, "user_tokens", "id", token, 0, "a one-time token 30 days after it expired")
	e.wantAuthRows(t, "user_token_redemptions", "token_id", token, 0, "a one-time token 30 days after it expired")
	e.wantAuthRows(t, "sessions", "id", other.Session.ID, 1, "a session one day after it expired")

	e.clock.Advance(30 * authPurgeDay) // day 61: both sessions expired 31 days ago; the family 11 days ago
	run("after 61 days")
	e.wantAuthRows(t, "sessions", "id", other.Session.ID, 0, "a session 30 days after it expired")
	e.wantAuthRows(t, "session_revocations", "session_id", other.Session.ID, 0, "a session 30 days after it expired")
	e.wantAuthRows(t, "session_activity", "session_id", other.Session.ID, 0, "a session 30 days after it expired")
	e.wantAuthRows(t, "sessions", "id", browser.Session.ID, 1, "the session a live family's code names")
	e.wantAuthRows(t, "oauth_access_tokens", "family_id", family, 2, "a family whose last token expired 11 days ago")
	e.wantAuthRows(t, "oauth_authorization_codes", "session_id", browser.Session.ID, 1, "the code of a kept family")

	e.clock.Advance(20 * authPurgeDay) // day 81: the last token of the family expired 31 days ago
	run("after 81 days")
	for _, tc := range []struct{ table, column, value string }{
		{"oauth_access_tokens", "family_id", family},
		{"oauth_token_revocations", "token_id", web.Token.ID},
		{"oauth_token_revocations", "token_id", rotated.Token.ID},
		{"oauth_token_family_last_used", "family_id", family},
		{"oauth_authorization_code_redemptions", "token_id", family},
		{"oauth_authorization_codes", "session_id", browser.Session.ID},
		{"sessions", "id", browser.Session.ID},
		{"session_activity", "session_id", browser.Session.ID},
	} {
		e.wantAuthRows(t, tc.table, tc.column, tc.value, 0, "after 81 days")
	}
	// The origin has a retention of its own.
	e.wantAuthRows(t, "session_origins", "session_id", browser.Session.ID, 1, "after 81 days")

	// The user's other family is still live: a purge never touches it, and repeating is harmless.
	live, _ := e.webTokensFor(t, userID)
	run("right after a new token")
	e.wantAuthRows(t, "oauth_access_tokens", "family_id", live.Token.FamilyID, 1, "a live family")
	if n, err := purger.Run(ctx); err != nil || n != 0 {
		t.Errorf("a repeated Run = %d, %v; want 0", n, err)
	}
}

// authRecordsPurgedInBatches checks that one call deletes at most its limit.
func authRecordsPurgedInBatches(t *testing.T, e *erasureEnv) {
	ctx := t.Context()
	userID := e.sessionUser(t).User.ID
	for range 3 {
		e.authPurgeUserToken(t, userID)
	}
	before := e.clock.Now().Add(2 * time.Hour)
	for _, want := range []int64{2, 1, 0} {
		var n int64
		err := e.eh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, err = e.eh.Identity.DeleteExpiredUserTokens(ctx, tx, before, 2)
			return err
		})
		if err != nil || n != want {
			t.Errorf("DeleteExpiredUserTokens = %d, %v; want %d", n, err, want)
		}
	}
}

// erasureErasesTokenFamilyLastUse checks that the erasure of a user leaves no last use of its
// token families behind.
func erasureErasesTokenFamilyLastUse(t *testing.T, e *erasureEnv) {
	ctx := t.Context()
	tokens, userID := e.webTokens(t)
	family := tokens.Token.FamilyID
	err := e.eh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := e.eh.Identity.UpsertTokenFamilyLastUsed(ctx, tx, family, e.clock.Now()); err != nil {
			return err
		}
		return e.eh.IdentityErasure.EraseUserData(ctx, tx, userID)
	})
	if err != nil {
		t.Fatal(err)
	}
	e.wantAuthRows(t, "oauth_token_family_last_used", "family_id", family, 0, "after the erasure")
}
