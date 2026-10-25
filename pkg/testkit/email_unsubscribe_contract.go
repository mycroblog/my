// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The unsubscribe case of AccountRepositoryContract (ADR-0086 §4).

// emailUnsubscribeTurnsOffNotifications issues an unsubscribe link and uses it: checking it
// changes nothing, using it turns the preference off, using it again answers the same, and an
// unknown, foreign, or expired token changes nothing. The transactional emails carry no
// unsubscribe headers.
func emailUnsubscribeTurnsOffNotifications(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	m := &Mailer{}
	e.withMailer(m)
	reg := e.sessionUser(t)
	on := true
	turnOn := func() {
		t.Helper()
		if _, err := e.identity.UpdatePreferences(ctx, reg.User.ID, appidentity.PreferencesUpdate{EmailModeratorNotifications: &on}); err != nil {
			t.Fatal(err)
		}
	}
	wantOn := func(want bool, when string) {
		t.Helper()
		p, err := e.identity.Preferences(ctx, reg.User.ID)
		if err != nil || p.EmailModeratorNotifications != want {
			t.Errorf("email_moderator_notifications %s = %v, %v; want %v", when, p.EmailModeratorNotifications, err, want)
		}
	}
	turnOn()

	link, err := e.identity.IssueUnsubscribeLink(ctx, reg.User.ID, identity.UnsubscribeModeratorNotifications)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != contractHost || u.Path != appidentity.PathMailUnsubscribe {
		t.Fatalf("link = %q, want https://%s%s?token=...", link, contractHost, appidentity.PathMailUnsubscribe)
	}
	token := u.Query().Get("token")
	if !strings.HasPrefix(token, appidentity.EmailUnsubscribeTokenPrefix) {
		t.Errorf("token = %q, want the prefix %s", token, appidentity.EmailUnsubscribeTokenPrefix)
	}
	stored, err := e.h.Identity.FindUserTokenByHash(ctx, appidentity.HashToken(token))
	if err != nil || stored.Purpose != identity.PurposeEmailUnsubscribe || stored.UserID != reg.User.ID ||
		stored.UnsubscribeCategory != identity.UnsubscribeModeratorNotifications ||
		!stored.ExpiresAt.Equal(e.clock.Now().Add(appidentity.EmailUnsubscribeTTL)) {
		t.Errorf("stored token = %+v, %v; want an unsubscribe token of the user for %v", stored, err, appidentity.EmailUnsubscribeTTL)
	}

	// Checking the link changes nothing.
	if c, err := e.identity.CheckUnsubscribeToken(ctx, token); err != nil || c != identity.UnsubscribeModeratorNotifications {
		t.Errorf("CheckUnsubscribeToken = %q, %v", c, err)
	}
	wantOn(true, "after the check")

	// Using it turns the preference off; using it again changes nothing and answers the same.
	for range 2 {
		if c, err := e.identity.Unsubscribe(ctx, token); err != nil || c != identity.UnsubscribeModeratorNotifications {
			t.Errorf("Unsubscribe = %q, %v", c, err)
		}
		wantOn(false, "after the unsubscribe")
	}

	// Unknown tokens and tokens of other purposes do not work.
	turnOn()
	e.requestReset(t, sessionEmail)
	reset := resetToken(t, m, "Alice@Example.com")
	for _, bad := range []string{"", appidentity.EmailUnsubscribeTokenPrefix + "unknown", strings.TrimPrefix(token, appidentity.EmailUnsubscribeTokenPrefix), reset} {
		if _, err := e.identity.Unsubscribe(ctx, bad); !errors.Is(err, appidentity.ErrInvalidToken) {
			t.Errorf("Unsubscribe(%q) = %v, want ErrInvalidToken", bad, err)
		}
		if _, err := e.identity.CheckUnsubscribeToken(ctx, bad); !errors.Is(err, appidentity.ErrInvalidToken) {
			t.Errorf("CheckUnsubscribeToken(%q) = %v, want ErrInvalidToken", bad, err)
		}
	}
	// The reset email is transactional and carries no unsubscribe headers.
	for _, sent := range m.Sent() {
		if sent.Category != kernel.MailTransactional || len(sent.Headers) != 0 {
			t.Errorf("sent %q = category %q with headers %v, want a transactional email without them", sent.Subject, sent.Category, sent.Headers)
		}
	}

	// An expired link changes nothing.
	e.clock.Advance(appidentity.EmailUnsubscribeTTL + time.Second)
	if _, err := e.identity.Unsubscribe(ctx, token); !errors.Is(err, appidentity.ErrInvalidToken) {
		t.Errorf("Unsubscribe with an expired token = %v, want ErrInvalidToken", err)
	}
	wantOn(true, "after an expired unsubscribe")
}
