// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The effects on statuses of the moderation actions on statuses, which the
// moderation module records inside the transaction of the action.

// DeleteByModeration records inside tx that the moderation action actionID deleted st: the
// deletion (origin moderation, naming the action), which stays as the tombstone of its URI, and
// the removal from every projection, the same as the author's deletion; a local status's Delete
// goes out like the author's (ADR-0058 §6). A post of the board of an event is deleted alike but
// announced as BoardStatusesDeleted, so that its deletion never leaves the event either
// (board.go). A status that is already deleted fails with ErrAlreadyDeleted, which abandons tx.
func (s *Service) DeleteByModeration(ctx context.Context, tx uow.Tx, st Status, actionID string, at time.Time) error {
	d := content.Deletion{
		StatusID: st.ID, URI: st.URI, AuthorAccountID: st.AuthorAccountID, Origin: content.DeletedByModeration,
		ModerationActionID: actionID, DeletedAt: at,
	}
	gid, err := boardOf(ctx, s.d.Repository, tx, st)
	if err != nil {
		return err
	}
	if err := s.d.Repository.InsertDeletion(ctx, tx, d); err != nil {
		return err
	}
	if gid != "" {
		return tx.Emit(ctx, BoardStatusesDeleted{Deletions: []StatusDeleted{{Deletion: d, Status: st}}})
	}
	return tx.Emit(ctx, StatusDeleted{Deletion: d, Status: st})
}

// DeleteManyByModeration is DeleteByModeration of sts, with one insert of their deletions, one
// StatusesDeleted for the ordinary statuses, and one BoardStatusesDeleted for the posts of the
// boards of events among them.
func (s *Service) DeleteManyByModeration(ctx context.Context, tx uow.Tx, sts []Status, actionID string, at time.Time) error {
	ds := make([]content.Deletion, len(sts))
	var local []string
	for i, st := range sts {
		ds[i] = content.Deletion{
			StatusID: st.ID, URI: st.URI, AuthorAccountID: st.AuthorAccountID, Origin: content.DeletedByModeration,
			ModerationActionID: actionID, DeletedAt: at,
		}
		if st.IsLocal {
			local = append(local, st.ID)
		}
	}
	boards := map[string]string{}
	if len(local) > 0 {
		var err error
		if boards, err = s.d.Repository.StatusBoardsAmong(ctx, tx, local); err != nil {
			return err
		}
	}
	if err := s.d.Repository.InsertDeletions(ctx, tx, ds); err != nil {
		return err
	}
	ev := StatusesDeleted{Deletions: make([]StatusDeleted, 0, len(sts))}
	var onBoards BoardStatusesDeleted
	for i, st := range sts {
		if _, ok := boards[st.ID]; ok {
			onBoards.Deletions = append(onBoards.Deletions, StatusDeleted{Deletion: ds[i], Status: st})
		} else {
			ev.Deletions = append(ev.Deletions, StatusDeleted{Deletion: ds[i], Status: st})
		}
	}
	if len(onBoards.Deletions) > 0 {
		if err := tx.Emit(ctx, onBoards); err != nil {
			return err
		}
		if len(ev.Deletions) == 0 {
			return nil
		}
	}
	return tx.Emit(ctx, ev)
}

// MarkManySensitiveByModeration is MarkSensitiveByModeration of each of statusIDs, with one
// statement.
func (s *Service) MarkManySensitiveByModeration(ctx context.Context, tx uow.Tx, statusIDs []string, actionID string) error {
	return s.d.Repository.InsertSensitivityOverrides(ctx, tx, statusIDs, actionID)
}

// MarkSensitiveByModeration records inside tx that the mark_statuses_sensitive action actionID
// shows a status as sensitive until it is lifted (status_sensitivity_overrides).
func (s *Service) MarkSensitiveByModeration(ctx context.Context, tx uow.Tx, statusID, actionID string) error {
	return s.d.Repository.InsertSensitivityOverride(ctx, tx, statusID, actionID)
}

// ClearSensitivityOf ends inside tx what a lifted mark_statuses_sensitive action showed as
// sensitive; the statuses that other actions mark stay marked.
func (s *Service) ClearSensitivityOf(ctx context.Context, tx uow.Tx, actionID string) error {
	return s.d.Repository.DeleteSensitivityOverridesOf(ctx, tx, actionID)
}

// MarkSensitiveByHook records inside tx that the mark_sensitive decision decisionID of a hook shows
// a status as sensitive until a moderator cancels it (ADR-0147 §8). The mark belongs to the status,
// so it stays on later revisions, whatever is decided about them.
func (s *Service) MarkSensitiveByHook(ctx context.Context, tx uow.Tx, statusID, decisionID string) error {
	return s.d.Repository.InsertSensitivityOverrideOfHookDecision(ctx, tx, statusID, decisionID)
}

// ClearSensitivityOfHookDecision ends inside tx what a cancelled decision of a hook showed as
// sensitive. What moderation actions and other decisions mark stays marked, and the author's own
// Sensitive and the force_sensitive restriction are not touched.
func (s *Service) ClearSensitivityOfHookDecision(ctx context.Context, tx uow.Tx, decisionID string) error {
	return s.d.Repository.DeleteSensitivityOverridesOfHookDecision(ctx, tx, decisionID)
}
