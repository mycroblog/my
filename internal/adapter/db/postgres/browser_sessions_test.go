// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestBrowserSessionQueries runs the queries of the signed-in browsers (ADR-0136) on PostgreSQL,
// where the IDs that they join are of different column types: a session counts while it works or
// a first-party family started from it works, and only first-party tokens are revoked with it.
func TestBrowserSessionQueries(t *testing.T) {
	db := newTestDB(t, uow.Options{})
	repo := db.IdentityRepository()
	ctx := t.Context()
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	userID := testkit.NamedID("user")
	webClient, appClient := testkit.NamedID("web"), testkit.NamedID("app")
	endedID, activeID, goneID := testkit.NamedID("ended"), testkit.NamedID("active"), testkit.NamedID("gone")
	webFamily, appFamily := testkit.NamedID("web-family"), testkit.NamedID("app-family")

	session := func(id string) identity.Session {
		return identity.Session{ID: id, UserID: userID, TokenHash: appidentity.HashToken(id), CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	}
	grant := func(tx uow.Tx, client, family, sessionID string) error {
		code := appidentity.HashToken("code-" + family)
		c := identity.AuthorizationCode{
			CodeHash: code, ClientID: client, UserID: userID, RedirectURI: "https://example.com/cb", Scope: "read",
			CodeChallenge: "c", SessionID: sessionID, IssuedAt: now, ExpiresAt: now.Add(time.Minute),
		}
		if err := repo.InsertAuthorizationCode(ctx, tx, c); err != nil {
			return err
		}
		if _, err := repo.InsertAuthorizationCodeRedemption(ctx, tx, identity.AuthorizationCodeRedemption{CodeHash: code, TokenID: family, RedeemedAt: now}); err != nil {
			return err
		}
		return repo.InsertAccessToken(ctx, tx, identity.AccessToken{
			ID: family, FamilyID: family, ClientID: client, UserID: userID, Scope: "read",
			AccessTokenHash: appidentity.HashToken("at-" + family), RefreshTokenHash: appidentity.HashToken("rt-" + family),
			IssuedAt: now, ExpiresAt: now.Add(time.Hour), RefreshExpiresAt: now.Add(24 * time.Hour),
		})
	}
	err := db.Tx(ctx, func(tx uow.Tx) error {
		for id, registration := range map[string]identity.OAuthClientRegistration{webClient: identity.ClientRegisteredFirstParty, appClient: identity.ClientRegisteredByUser} {
			if err := repo.InsertOAuthClient(ctx, tx, identity.OAuthClient{ID: id, ClientID: id, Name: "client", Scope: "read", Registration: registration, CreatedAt: now}); err != nil {
				return err
			}
		}
		for _, id := range []string{endedID, activeID, goneID} {
			if err := repo.InsertSession(ctx, tx, session(id)); err != nil {
				return err
			}
		}
		origin := identity.SessionOrigin{SessionID: activeID, UserID: userID, IP: "203.0.113.1", UserAgent: "agent", CreatedAt: now}
		if err := repo.InsertSessionOrigin(ctx, tx, origin); err != nil {
			return err
		}
		if err := repo.UpsertSessionActivity(ctx, tx, identity.SessionActivity{SessionID: activeID, LastSeenAt: now}); err != nil {
			return err
		}
		// The session ended, but a first-party family that started from it works; the other ended
		// with only a third-party family, which does not make it a browser.
		for _, id := range []string{endedID, goneID} {
			if err := repo.InsertSessionRevocation(ctx, tx, identity.SessionRevocation{SessionID: id, Reason: identity.RevokedLogout, RevokedAt: now}); err != nil {
				return err
			}
		}
		if err := grant(tx, webClient, webFamily, endedID); err != nil {
			return err
		}
		return grant(tx, appClient, appFamily, goneID)
	})
	if err != nil {
		t.Fatal(err)
	}

	sessions, err := repo.ListBrowserSessions(ctx, userID, now)
	if err != nil {
		t.Fatalf("ListBrowserSessions: %v", err)
	}
	listed := map[string]identity.BrowserSession{}
	for _, s := range sessions {
		listed[s.ID] = s
	}
	if len(listed) != 2 || listed[endedID].ID == "" || listed[activeID].ID == "" {
		t.Fatalf("sessions = %+v, want the active one and the one with a first-party family", sessions)
	}
	if a := listed[activeID]; a.IP != "203.0.113.1" || a.UserAgent != "agent" || a.LastSeenAt == nil {
		t.Errorf("active session = %+v, want its origin and activity", a)
	}

	families, err := repo.ListBrowserSessionFamilies(ctx, userID, now)
	if err != nil {
		t.Fatalf("ListBrowserSessionFamilies: %v", err)
	}
	if len(families) != 1 || families[0].SessionID != endedID || families[0].FamilyID != webFamily {
		t.Errorf("families = %+v, want the first-party family with its session", families)
	}

	err = db.Tx(ctx, func(tx uow.Tx) error {
		for _, id := range []string{endedID, goneID} {
			if err := repo.RevokeFirstPartyTokensOfSession(ctx, tx, id, identity.TokenRevokedUser, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RevokeFirstPartyTokensOfSession: %v", err)
	}
	if active, err := repo.TokenFamilyActive(ctx, webFamily, now); err != nil || active {
		t.Errorf("first-party family active = %v, %v; want revoked", active, err)
	}
	if active, err := repo.TokenFamilyActive(ctx, appFamily, now); err != nil || !active {
		t.Errorf("third-party family active = %v, %v; want it kept", active, err)
	}
	if families, err := repo.ListBrowserSessionFamilies(ctx, userID, now); err != nil || len(families) != 0 {
		t.Errorf("families after the revocation = %+v, %v; want none", families, err)
	}
}
