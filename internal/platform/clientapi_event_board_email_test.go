// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/gathering"
	domainnotification "github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestEventBoardPostEmailHoldsNeitherThePostNorThePlace checks the email of an event_board_posted
// (ADR-0161 §11): a reader who turned on the email of the Events row gets one once the
// notification stayed unread, which links to the notification and holds neither the post nor the
// event's place; a reader who left it off gets none.
func TestEventBoardPostEmailHoldsNeitherThePostNorThePlace(t *testing.T) {
	t.Parallel()
	e := newEventBoardNotifyEnv(t, "alice", "gina", "mona", "max")
	ev := e.createEvent("readers", map[string]any{
		"places": []map[string]any{{"kind": "physical", "name": "Secretplace Cafe", "address": "1-2-3 Hiddenstreet"}},
	})
	e.answer("gina", ev.Id, gathering.AnswerGoing)
	e.answer("mona", ev.Id, gathering.AnswerMaybe)
	e.answer("max", ev.Id, gathering.AnswerGoing)
	e.drain()
	e.setGroup("mona", clientapi.NotificationGroupEvent, "email", true)
	before := len(e.mail.Sent())

	post := e.mustPostOnBoard("gina", ev.Id, map[string]any{"text": "boardbodyneedle bring snacks"})
	e.drain()
	notices := e.boardNotices("mona")
	if len(notices) != 1 || notices[0].StatusID != post.Id {
		t.Fatalf("mona's event_board_posted = %+v, want the post %s", notices, post.Id)
	}
	e.clock.Set(eventBoardStart.Add(domainnotification.EmailDelay + 5*time.Minute))
	e.drain()

	var monas []kernel.MailMessage
	for _, m := range e.mail.Sent()[before:] {
		if m.To == "mona@example.com" {
			monas = append(monas, m)
		} else {
			t.Errorf("an email to %s: %q", m.To, m.Subject)
		}
	}
	if len(monas) != 1 {
		t.Fatalf("%d emails to mona %+v, want 1", len(monas), monas)
	}
	m := monas[0]
	if m.Subject == "" || !strings.Contains(m.Text, "/notifications/"+notices[0].ID) {
		t.Errorf("the email to mona: subject %q, text %q, want a subject and the link to the notification %s", m.Subject, m.Text, notices[0].ID)
	}
	for _, secret := range []string{"boardbodyneedle", "bring snacks", "Secretplace", "Hiddenstreet"} {
		if strings.Contains(m.Subject+m.Text+m.HTML, secret) {
			t.Errorf("the email to mona holds %q of the post or the place:\n%s\n%s", secret, m.Subject, m.Text)
		}
	}
}
