// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// statusReadsPrivateMentions checks PrivateMentionTx, which the notification policy's
// unsolicited_private_mention reads (ADR-0158 §6): a direct status answers the recipient when its
// conversation holds a direct status of the recipient that mentions the source; a public status
// is not direct; an unknown status is neither.
func statusReadsPrivateMentions(t *testing.T, e *StatusEnv) {
	ctx, repo := t.Context(), e.H.Statuses
	recipient, source, other := e.IDs.NewID(), e.IDs.NewID(), e.IDs.NewID()
	started := e.NewStatus(recipient, content.VisibilityDirect)
	e.mustCreate(t, started, source)
	answer := e.NewStatus(source, content.VisibilityDirect)
	e.mustCreate(t, answer, recipient)
	cold := e.NewStatus(other, content.VisibilityDirect)
	e.mustCreate(t, cold, recipient)
	public := e.NewStatus(source, content.VisibilityPublic)
	e.mustCreate(t, public, recipient)
	thread := e.IDs.NewID()
	lone := e.IDs.NewID()
	err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		for _, c := range []content.Conversation{
			{ID: thread, URI: content.ConversationURI(testkit.ContractHost, thread), IsLocal: true},
			{ID: lone, URI: content.ConversationURI(testkit.ContractHost, lone), IsLocal: true},
		} {
			if _, err := repo.EnsureConversation(ctx, tx, c); err != nil {
				return err
			}
		}
		for id, c := range map[string]string{started.ID: thread, answer.ID: thread, cold.ID: lone, public.ID: lone} {
			if err := repo.InsertStatusConversation(ctx, tx, id, c); err != nil {
				return err
			}
		}
		cases := []struct {
			name             string
			status, sender   string
			direct, answered bool
		}{
			{"a reply to the recipient's direct status", answer.ID, source, true, true},
			{"a direct status out of the blue", cold.ID, other, true, false},
			{"a public status", public.ID, source, false, false},
			{"an unknown status", e.IDs.NewID(), source, false, false},
		}
		for _, c := range cases {
			direct, solicited, err := repo.PrivateMentionTx(ctx, tx, c.status, recipient, c.sender)
			if err != nil {
				return err
			}
			if direct != c.direct || solicited != c.answered {
				t.Errorf("PrivateMentionTx of %s = %v, %v; want %v, %v", c.name, direct, solicited, c.direct, c.answered)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
