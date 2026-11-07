// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/pkg/testkit"
)

// The unsubscribe case of AccountRepositoryContract (ADR-0086 §4, ADR-0086 §11).

// stubUnsubscriptions is an in-memory identity.EmailUnsubscriptions: the categories that
// unsubscribe links turn off, and those each user turned off.
type stubUnsubscriptions struct {
	mu  sync.Mutex
	off map[string][]identity.UnsubscribeCategory
}

func newStubUnsubscriptions() *stubUnsubscriptions {
	return &stubUnsubscriptions{off: map[string][]identity.UnsubscribeCategory{}}
}

// Unsubscribable takes the rows of the notification settings whose email the user chooses, of
// which the contract uses these.
func (s *stubUnsubscriptions) Unsubscribable(c identity.UnsubscribeCategory) bool {
	return c == "mention" || c == "admin_report"
}

func (s *stubUnsubscriptions) Unsubscribe(_ context.Context, userID string, c identity.UnsubscribeCategory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, done := range s.off[userID] {
		if done == c {
			return nil
		}
	}
	s.off[userID] = append(s.off[userID], c)
	return nil
}

// turnedOff returns the categories that userID turned off, in order.
func (s *stubUnsubscriptions) turnedOff(userID string) []identity.UnsubscribeCategory {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]identity.UnsubscribeCategory(nil), s.off[userID]...)
}

// reset forgets what userID turned off.
func (s *stubUnsubscriptions) reset(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.off, userID)
}

// emailUnsubscribeTurnsOffNotifications issues an unsubscribe link and uses it: checking it
// changes nothing, using it turns off the email of its category and nothing else, using it again
// answers the same, and an unknown, foreign, or expired token or a category that no link turns
// off changes nothing. The transactional emails carry no unsubscribe headers.
func emailUnsubscribeTurnsOffNotifications(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	m := &testkit.Mailer{}
	e.withMailer(m)
	reg := e.SessionUser(t)
	const category identity.UnsubscribeCategory = "admin_report"
	wantOff := func(when string, want ...identity.UnsubscribeCategory) {
		t.Helper()
		got := e.unsubscriptions.turnedOff(reg.User.ID)
		if len(got) != len(want) || len(want) == 1 && got[0] != want[0] {
			t.Errorf("turned off %s = %v, want %v", when, got, want)
		}
	}

	if _, err := e.Identity.IssueUnsubscribeLink(ctx, reg.User.ID, "service_closure"); err == nil {
		t.Error("IssueUnsubscribeLink(service_closure) = nil, want an error: its emails cannot be turned off")
	}
	link, err := e.Identity.IssueUnsubscribeLink(ctx, reg.User.ID, category)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != testkit.ContractHost || u.Path != appidentity.PathMailUnsubscribe {
		t.Fatalf("link = %q, want https://%s%s?token=...", link, testkit.ContractHost, appidentity.PathMailUnsubscribe)
	}
	token := u.Query().Get("token")
	if !strings.HasPrefix(token, appidentity.EmailUnsubscribeTokenPrefix) {
		t.Errorf("token = %q, want the prefix %s", token, appidentity.EmailUnsubscribeTokenPrefix)
	}
	stored, err := e.H.Identity.FindUserTokenByHash(ctx, appidentity.HashToken(token))
	if err != nil || stored.Purpose != identity.PurposeEmailUnsubscribe || stored.UserID != reg.User.ID ||
		stored.UnsubscribeCategory != category ||
		!stored.ExpiresAt.Equal(e.Clock.Now().Add(appidentity.EmailUnsubscribeTTL)) {
		t.Errorf("stored token = %+v, %v; want an unsubscribe token of the user for %v", stored, err, appidentity.EmailUnsubscribeTTL)
	}

	// Checking the link changes nothing.
	if c, err := e.Identity.CheckUnsubscribeToken(ctx, token); err != nil || c != category {
		t.Errorf("CheckUnsubscribeToken = %q, %v", c, err)
	}
	wantOff("after the check")

	// Using it turns off its category; using it again changes nothing and answers the same.
	for range 2 {
		if c, err := e.Identity.Unsubscribe(ctx, token); err != nil || c != category {
			t.Errorf("Unsubscribe = %q, %v", c, err)
		}
		wantOff("after the unsubscribe", category)
	}

	// Unknown tokens and tokens of other purposes do not work.
	e.unsubscriptions.reset(reg.User.ID)
	e.requestReset(t, sessionEmail)
	reset := resetToken(t, m, "Alice@Example.com")
	for _, bad := range []string{"", appidentity.EmailUnsubscribeTokenPrefix + "unknown", strings.TrimPrefix(token, appidentity.EmailUnsubscribeTokenPrefix), reset} {
		if _, err := e.Identity.Unsubscribe(ctx, bad); !errors.Is(err, appidentity.ErrInvalidToken) {
			t.Errorf("Unsubscribe(%q) = %v, want ErrInvalidToken", bad, err)
		}
		if _, err := e.Identity.CheckUnsubscribeToken(ctx, bad); !errors.Is(err, appidentity.ErrInvalidToken) {
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
	e.Clock.Advance(appidentity.EmailUnsubscribeTTL + time.Second)
	if _, err := e.Identity.Unsubscribe(ctx, token); !errors.Is(err, appidentity.ErrInvalidToken) {
		t.Errorf("Unsubscribe with an expired token = %v, want ErrInvalidToken", err)
	}
	wantOff("after an expired unsubscribe")
}
