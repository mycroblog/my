// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"context"
	"sync"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
)

// RefreshReplayFlows verifies the replay of a lost refresh response (ADR-0090 §18) through the
// identity use cases, for the web client's cookie and for confidential and public apps: a
// refresh token used within appidentity.RefreshReplayWindow whose successor is unused gets the
// successor pair again, with what is left of its access token and whatever scope it asks for;
// concurrent refreshes with one token get one pair; a reuse after the successor was used, after
// the window, or after a revocation for another reason revokes the whole family; and another
// client is refused.
func RefreshReplayFlows(t *testing.T, newHarness AccountFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *AccountEnv)
	}{
		{"ReplaysALostRefreshResponse", oauthReplaysALostRefreshResponse},
		{"RevokesTheFamilyOnAReplayAfterTheSuccessorWasUsed", oauthRevokesTheFamilyOnAReplayAfterTheSuccessorWasUsed},
		{"EndsTheReplayAfterTheWindow", oauthEndsTheReplayAfterTheWindow},
		{"GivesConcurrentRefreshesOnePair", oauthGivesConcurrentRefreshesOnePair},
		{"DoesNotReplayTokensRevokedForAnotherReason", oauthDoesNotReplayTokensRevokedForAnotherReason},
		{"RefusesAReplayToAnotherClient", oauthRefusesAReplayToAnotherClient},
		{"ReplaysIgnoreTheScope", oauthReplaysIgnoreTheScope},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, NewAccountEnv(t, newHarness)) })
	}
}

// refresher is a client that refreshes its own tokens: the web client through its cookie, or
// an app through the refresh_token grant.
type refresher struct {
	name string
	// tokens starts a new token family of the client.
	tokens  func(t *testing.T) appidentity.IssuedTokens
	refresh func(ctx context.Context, refreshToken, scope string) (appidentity.IssuedTokens, error)
}

// refreshers registers alice and returns her ID with the web client, a confidential app, and a
// public app, each refreshing alice's tokens.
func (e *AccountEnv) refreshers(t *testing.T) (string, []refresher) {
	t.Helper()
	alice := e.Register(t, "alice").User.ID
	list := []refresher{{
		name: "web client",
		tokens: func(t *testing.T) appidentity.IssuedTokens {
			t.Helper()
			got, _ := e.WebTokensFor(t, alice)
			return got
		},
		refresh: func(ctx context.Context, refreshToken, _ string) (appidentity.IssuedTokens, error) {
			return e.Identity.RefreshWebClient(ctx, refreshToken)
		},
	}}
	for _, confidential := range []bool{true, false} {
		reg := e.registerOwnApp(t, alice, confidential)
		creds := appidentity.ClientCredentials{ClientID: reg.Client.ClientID, ClientSecret: reg.ClientSecret}
		name := "public app"
		if confidential {
			name = "confidential app"
		}
		list = append(list, refresher{
			name: name,
			tokens: func(t *testing.T) appidentity.IssuedTokens {
				t.Helper()
				return e.ownAppTokensFor(t, reg.OAuthApp, creds, alice)
			},
			refresh: func(ctx context.Context, refreshToken, scope string) (appidentity.IssuedTokens, error) {
				return e.Identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: creds, RefreshToken: refreshToken, Scope: scope})
			},
		})
	}
	return alice, list
}

// rotate refreshes with a refresh token that was not used yet and fails t unless it gets a new pair.
func (r refresher) rotate(t *testing.T, refreshToken string) appidentity.IssuedTokens {
	t.Helper()
	got, err := r.refresh(t.Context(), refreshToken, "")
	if err != nil {
		t.Fatalf("%s: refresh = %v", r.name, err)
	}
	if !got.ReplayedAt.IsZero() {
		t.Errorf("%s: ReplayedAt of a rotation = %v, want zero", r.name, got.ReplayedAt)
	}
	return got
}

// samePair reports whether two results hold the same access and refresh tokens.
func samePair(a, b appidentity.IssuedTokens) bool {
	return a.AccessToken != "" && a.AccessToken == b.AccessToken && a.RefreshToken == b.RefreshToken && a.Token.ID == b.Token.ID
}

// wantNoPair fails t unless a refused refresh returned no tokens.
func wantNoPair(t *testing.T, got appidentity.IssuedTokens, what string) {
	t.Helper()
	if got.AccessToken != "" || got.RefreshToken != "" {
		t.Errorf("%s returned the tokens %q, %q; want none", what, got.AccessToken, got.RefreshToken)
	}
}

func oauthReplaysALostRefreshResponse(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	_, list := e.refreshers(t)
	for _, r := range list {
		first := r.tokens(t)
		e.Clock.Advance(time.Minute)
		second := r.rotate(t, first.RefreshToken)
		if second.ExpiresIn() != appidentity.AccessTokenTTL {
			t.Errorf("%s: ExpiresIn of a rotation = %v, want %v", r.name, second.ExpiresIn(), appidentity.AccessTokenTTL)
		}

		// The response was lost: the previous refresh token gets the same pair, with what is left
		// of its access token.
		e.Clock.Advance(10 * time.Minute)
		again, err := r.refresh(ctx, first.RefreshToken, "")
		if err != nil {
			t.Errorf("%s: refreshing with the previous refresh token = %v, want the same pair", r.name, err)
			continue
		}
		if !samePair(again, second) {
			t.Errorf("%s: replay = %q, %q (token %s); want the rotation's %q, %q (token %s)", r.name,
				again.AccessToken, again.RefreshToken, again.Token.ID, second.AccessToken, second.RefreshToken, second.Token.ID)
		}
		if !again.ReplayedAt.Equal(e.Clock.Now()) {
			t.Errorf("%s: ReplayedAt = %v, want %v", r.name, again.ReplayedAt, e.Clock.Now())
		}
		if want := appidentity.AccessTokenTTL - 10*time.Minute; again.ExpiresIn() != want {
			t.Errorf("%s: ExpiresIn of a replay = %v, want %v", r.name, again.ExpiresIn(), want)
		}
		e.WantAccess(t, second.AccessToken, true, r.name+": the access token of a replayed pair")

		// Once the access token expired, the replay still returns the pair, with nothing left.
		e.Clock.Advance(2 * time.Hour)
		late, err := r.refresh(ctx, first.RefreshToken, "")
		if err != nil || !samePair(late, second) || late.ExpiresIn() != 0 {
			t.Errorf("%s: replay after the access token expired = %q, ExpiresIn %v, %v; want the same pair, 0", r.name,
				late.AccessToken, late.ExpiresIn(), err)
		}

		// The successor's refresh token rotates as usual, in the same family.
		third := r.rotate(t, second.RefreshToken)
		if third.AccessToken == second.AccessToken || third.RefreshToken == second.RefreshToken || third.Token.FamilyID != first.Token.FamilyID {
			t.Errorf("%s: rotation after a replay = %+v, want a new pair in the family %s", r.name, third.Token, first.Token.FamilyID)
		}
		e.WantAccess(t, third.AccessToken, true, r.name+": the access token after a replay")
	}
}

func oauthRevokesTheFamilyOnAReplayAfterTheSuccessorWasUsed(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	_, list := e.refreshers(t)
	for _, r := range list {
		first := r.tokens(t)
		second := r.rotate(t, first.RefreshToken)
		third := r.rotate(t, second.RefreshToken)

		got, err := r.refresh(ctx, first.RefreshToken, "")
		wantOAuthError(t, err, appidentity.OAuthInvalidGrant, r.name+": the previous refresh token after its successor was used")
		wantNoPair(t, got, r.name+": the previous refresh token after its successor was used")
		e.WantAccess(t, third.AccessToken, false, r.name+": the newest access token after the reuse")
		_, err = r.refresh(ctx, third.RefreshToken, "")
		wantOAuthError(t, err, appidentity.OAuthInvalidGrant, r.name+": the newest refresh token after the reuse")
		_, err = r.refresh(ctx, second.RefreshToken, "")
		wantOAuthError(t, err, appidentity.OAuthInvalidGrant, r.name+": the newest pair's predecessor after the reuse")
	}
}

func oauthEndsTheReplayAfterTheWindow(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	_, list := e.refreshers(t)
	for _, r := range list {
		first := r.tokens(t)
		// The window runs from the rotation, not from the issue of the previous pair.
		e.Clock.Advance(23 * time.Hour)
		second := r.rotate(t, first.RefreshToken)

		e.Clock.Advance(appidentity.RefreshReplayWindow - time.Second)
		if again, err := r.refresh(ctx, first.RefreshToken, ""); err != nil || !samePair(again, second) {
			t.Errorf("%s: replay just before the window ends = %q, %v; want the same pair", r.name, again.AccessToken, err)
		}

		e.Clock.Advance(time.Second)
		got, err := r.refresh(ctx, first.RefreshToken, "")
		wantOAuthError(t, err, appidentity.OAuthInvalidGrant, r.name+": the previous refresh token at the end of the window")
		wantNoPair(t, got, r.name+": the previous refresh token at the end of the window")
		_, err = r.refresh(ctx, second.RefreshToken, "")
		wantOAuthError(t, err, appidentity.OAuthInvalidGrant, r.name+": the successor's refresh token after a reuse past the window")
	}
}

func oauthGivesConcurrentRefreshesOnePair(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	_, list := e.refreshers(t)
	const n = 8
	for _, r := range list {
		first := r.tokens(t)
		results := make([]appidentity.IssuedTokens, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() { results[i], errs[i] = r.refresh(ctx, first.RefreshToken, "") })
		}
		wg.Wait()

		issued := 0
		for i := range n {
			if errs[i] != nil {
				t.Fatalf("%s: concurrent refresh %d = %v, want every one to work", r.name, i, errs[i])
			}
			if !samePair(results[i], results[0]) {
				t.Errorf("%s: concurrent refresh %d = %q, %q; want the pair %q, %q of the others", r.name, i,
					results[i].AccessToken, results[i].RefreshToken, results[0].AccessToken, results[0].RefreshToken)
			}
			if results[i].ReplayedAt.IsZero() {
				issued++
			}
		}
		if issued != 1 {
			t.Errorf("%s: %d of %d concurrent refreshes issued the pair, want 1 and the rest replays", r.name, issued, n)
		}
		e.WantAccess(t, results[0].AccessToken, true, r.name+": the access token of concurrent refreshes")

		next := r.rotate(t, results[0].RefreshToken)
		if next.AccessToken == results[0].AccessToken || next.RefreshToken == results[0].RefreshToken {
			t.Errorf("%s: the pair of concurrent refreshes did not rotate", r.name)
		}
		e.WantAccess(t, next.AccessToken, true, r.name+": the access token after concurrent refreshes")
	}
}

func oauthDoesNotReplayTokensRevokedForAnotherReason(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	alice, list := e.refreshers(t)
	password := "correct horse battery staple"
	revocations := []struct {
		name string
		do   func(appidentity.IssuedTokens) error
	}{
		{"the user revoked the app", func(pair appidentity.IssuedTokens) error {
			return e.Identity.RevokeAuthorizedApp(ctx, alice, pair.Client.ID)
		}},
		{"a password change", func(appidentity.IssuedTokens) error {
			next := password + " again"
			err := e.Identity.ChangePassword(ctx, appidentity.ChangePasswordInput{
				UserID: alice, Confirmation: appidentity.Confirmation{Password: password}, NewPassword: next,
			})
			password = next
			return err
		}},
	}
	for _, r := range list {
		for _, revoke := range revocations {
			what := r.name + ", " + revoke.name
			first := r.tokens(t)
			second := r.rotate(t, first.RefreshToken)
			if err := revoke.do(second); err != nil {
				t.Fatalf("%s: %v", what, err)
			}
			got, err := r.refresh(ctx, first.RefreshToken, "")
			wantOAuthError(t, err, appidentity.OAuthInvalidGrant, what+": the previous refresh token")
			wantNoPair(t, got, what+": the previous refresh token")
			e.WantAccess(t, second.AccessToken, false, what+": the successor's access token")
			got, err = r.refresh(ctx, second.RefreshToken, "")
			wantOAuthError(t, err, appidentity.OAuthInvalidGrant, what+": the successor's refresh token")
			wantNoPair(t, got, what+": the successor's refresh token")
		}
	}
}

func oauthRefusesAReplayToAnotherClient(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	_, list := e.refreshers(t)
	e.ThirdPartyApp(t)
	for _, r := range list {
		first := r.tokens(t)
		r.rotate(t, first.RefreshToken)
		got, err := e.Identity.RefreshAccessToken(ctx, appidentity.RefreshRequest{ClientCredentials: appCreds, RefreshToken: first.RefreshToken})
		wantOAuthError(t, err, appidentity.OAuthInvalidGrant, r.name+": the previous refresh token presented by another app")
		wantNoPair(t, got, r.name+": the previous refresh token presented by another app")
	}
	for _, r := range list[1:] {
		first := r.tokens(t)
		r.rotate(t, first.RefreshToken)
		got, err := e.Identity.RefreshWebClient(ctx, first.RefreshToken)
		wantOAuthError(t, err, appidentity.OAuthInvalidGrant, r.name+": the previous refresh token through the web client's cookie")
		wantNoPair(t, got, r.name+": the previous refresh token through the web client's cookie")
	}
}

func oauthReplaysIgnoreTheScope(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	_, list := e.refreshers(t)
	for _, r := range list[1:] { // the apps: the web client's cookie carries no scope
		first := r.tokens(t)
		second := r.rotate(t, first.RefreshToken)
		for _, scope := range []string{"read:statuses", "write:statuses", "read write"} {
			got, err := r.refresh(ctx, first.RefreshToken, scope)
			if err != nil || !samePair(got, second) || got.Token.Scope != second.Token.Scope {
				t.Errorf("%s: replay with the scope %q = %q (scope %q), %v; want the pair %q (scope %q)", r.name, scope,
					got.AccessToken, got.Token.Scope, err, second.AccessToken, second.Token.Scope)
			}
		}
	}
}
