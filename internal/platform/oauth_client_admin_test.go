// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/platform"
)

// TestOAuthClientAdministration runs the administrators' operations on OAuth clients against a
// real database (ADR-0090 §1, FR-003): disabling a client revokes all its tokens and lifting
// the disablement does not restore them, denying a host revokes the tokens of its clients and
// refuses their authorization requests, and each operation is in the audit log.
func TestOAuthClientAdministration(t *testing.T) {
	t.Parallel()
	site := newMetadataSite(t)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), HTTPClient: site.client()})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ctx := t.Context()
	alice, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "alice", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	admin := alice.User.ID
	verifier := strings.Repeat("v", 43)
	// authorize fetches the no-store document every time, so it forgets the URL's fetch limit
	// (ADR-0090 §7) first.
	authorize := func(clientID string) (identity.Authorization, error) {
		resetFetchLimit(t, app, clientID)
		return app.Identity.ValidateAuthorization(ctx, identity.AuthorizationRequest{
			ResponseType: "code", ClientID: clientID, RedirectURI: metadataRedirect, Scope: "read",
			CodeChallenge: domain.S256Challenge(verifier), CodeChallengeMethod: domain.CodeChallengeMethodS256,
		})
	}
	// signIn authorizes the client and exchanges the code for tokens.
	signIn := func(clientID string) identity.IssuedTokens {
		t.Helper()
		a, err := authorize(clientID)
		if err != nil {
			t.Fatalf("ValidateAuthorization(%s) = %v", clientID, err)
		}
		code, err := app.Identity.IssueAuthorizationCode(ctx, a, admin)
		if err != nil {
			t.Fatal(err)
		}
		tokens, err := app.Identity.ExchangeAuthorizationCode(ctx, identity.CodeExchange{
			ClientCredentials: identity.ClientCredentials{ClientID: clientID}, Code: code, RedirectURI: metadataRedirect, CodeVerifier: verifier,
		})
		if err != nil {
			t.Fatalf("ExchangeAuthorizationCode = %v", err)
		}
		return tokens
	}
	auditCount := func(q appmoderation.AuditQuery) int {
		t.Helper()
		entries, err := app.Moderation.AuditLog(ctx, q, 50)
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}

	app1 := site.json("/one.json", `{"client_id":"{{id}}","client_name":"One","redirect_uris":["`+metadataRedirect+`"]}`, "Cache-Control", "no-store")
	tokens := signIn(app1)
	if _, err := app.Identity.AuthenticateAccessToken(ctx, tokens.AccessToken); err != nil {
		t.Fatalf("AuthenticateAccessToken = %v", err)
	}
	listing, err := app.Identity.FindOAuthClientListing(ctx, tokens.Client.ID)
	if err != nil || listing.Disabled || listing.Registration != domain.ClientRegisteredByMetadataDocument {
		t.Fatalf("FindOAuthClientListing = %+v, %v; want an enabled metadata document client", listing, err)
	}

	// Disabling revokes every token at once, refuses the client, and is audited once however
	// often it is repeated; lifting restores the client but not the tokens.
	for range 2 {
		if err := app.Identity.DisableOAuthClient(ctx, tokens.Client.ID, admin); err != nil {
			t.Fatalf("DisableOAuthClient = %v", err)
		}
	}
	if _, err := app.Identity.AuthenticateAccessToken(ctx, tokens.AccessToken); err == nil {
		t.Error("AuthenticateAccessToken after the disablement succeeded, want a revoked token")
	}
	if _, err := authorize(app1); err == nil {
		t.Error("ValidateAuthorization of a disabled client succeeded")
	}
	disabled := appmoderation.AuditQuery{Action: appmoderation.AuditOAuthClientDisabled, TargetKind: appmoderation.AuditTargetOAuthClient, TargetID: tokens.Client.ID}
	entries, err := app.Moderation.AuditLog(ctx, disabled, 10)
	if err != nil || len(entries) != 1 || entries[0].ActorUserID != admin || !strings.Contains(entries[0].TargetLabel, app1) || entries[0].SourceEventID == "" {
		t.Errorf("audit log of the disablement = %+v, %v; want one entry by the admin labeled with the client_id", entries, err)
	}
	if l, err := app.Identity.FindOAuthClientListing(ctx, tokens.Client.ID); err != nil || !l.Disabled {
		t.Errorf("FindOAuthClientListing = %+v, %v; want disabled", l, err)
	}
	for range 2 {
		if err := app.Identity.EnableOAuthClient(ctx, tokens.Client.ID, admin); err != nil {
			t.Fatalf("EnableOAuthClient = %v", err)
		}
	}
	if n := auditCount(appmoderation.AuditQuery{Action: appmoderation.AuditOAuthClientEnabled, TargetKind: appmoderation.AuditTargetOAuthClient}); n != 1 {
		t.Errorf("audit entries of the lift = %d, want 1", n)
	}
	if _, err := app.Identity.AuthenticateAccessToken(ctx, tokens.AccessToken); err == nil {
		t.Error("a token revoked by the disablement works again after the lift")
	}
	tokens = signIn(app1)

	// The list shows the client under its route and none under another.
	byRoute, err := app.Identity.ListOAuthClients(ctx, domain.ClientRegisteredByMetadataDocument, "", 10)
	if err != nil || len(byRoute) != 1 || byRoute[0].ID != tokens.Client.ID {
		t.Errorf("ListOAuthClients(metadata_document) = %+v, %v; want the client", byRoute, err)
	}
	if other, err := app.Identity.ListOAuthClients(ctx, domain.ClientRegisteredDynamically, "", 10); err != nil || len(other) != 0 {
		t.Errorf("ListOAuthClients(dynamic) = %+v, %v; want none", other, err)
	}

	// Denying the host revokes the tokens of its clients, refuses their authorization requests,
	// and is audited once however often it is repeated.
	for range 2 {
		if err := app.Identity.DenyOAuthClientHost(ctx, "127.0.0.1", admin); err != nil {
			t.Fatalf("DenyOAuthClientHost = %v", err)
		}
	}
	if _, err := app.Identity.AuthenticateAccessToken(ctx, tokens.AccessToken); err == nil {
		t.Error("AuthenticateAccessToken after the denial of the host succeeded, want a revoked token")
	}
	if _, err := authorize(app1); err == nil {
		t.Error("ValidateAuthorization of a client of a denied host succeeded")
	}
	denials, err := app.Identity.ListOAuthClientHostDenials(ctx, "", 10)
	if err != nil || len(denials) != 1 || denials[0].Host != "127.0.0.1" || denials[0].DeniedByUserID != admin {
		t.Errorf("ListOAuthClientHostDenials = %+v, %v; want the host denied by the admin", denials, err)
	}
	denied := appmoderation.AuditQuery{Action: appmoderation.AuditOAuthClientHostDenied, TargetKind: appmoderation.AuditTargetOAuthClientHost, TargetID: "127.0.0.1"}
	if entries, err := app.Moderation.AuditLog(ctx, denied, 10); err != nil || len(entries) != 1 || entries[0].ActorUserID != admin {
		t.Errorf("audit log of the denial = %+v, %v; want one entry by the admin", entries, err)
	}
	for range 2 {
		if err := app.Identity.AllowOAuthClientHost(ctx, "127.0.0.1", admin); err != nil {
			t.Fatalf("AllowOAuthClientHost = %v", err)
		}
	}
	if n := auditCount(appmoderation.AuditQuery{Action: appmoderation.AuditOAuthClientHostAllowed, TargetKind: appmoderation.AuditTargetOAuthClientHost}); n != 1 {
		t.Errorf("audit entries of the lift of the host = %d, want 1", n)
	}
	if denials, err := app.Identity.ListOAuthClientHostDenials(ctx, "", 10); err != nil || len(denials) != 0 {
		t.Errorf("ListOAuthClientHostDenials after the lift = %+v, %v; want none", denials, err)
	}
	if _, err := authorize(app1); err != nil {
		t.Errorf("ValidateAuthorization after the lift = %v", err)
	}
}
