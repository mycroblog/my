// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/voicegen"
	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The records of the rooms that the moderators read, the purge erases, and the erasure of an
// account empties (ADR-0100 §11, §13).

// Revisions implements appvoice.RecordsRepository.
func (r *Repository) Revisions(ctx context.Context, tx uow.Tx, roomID string) ([]appvoice.Revision, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListVoiceRoomRevisions(ctx, roomID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list voice room revisions")
	}
	recs := make([]appvoice.RoomRecord, len(rows))
	for i, row := range rows {
		recs[i].Revision = appvoice.Revision{
			ID: row.ID, RoomID: row.RoomID,
			Settings: voice.Settings{
				Name: row.Name, Description: row.Description, Visibility: voice.Visibility(row.Visibility), Speakers: voice.Speakers(row.Speakers),
				Icon: voice.IconOf(row.IconEmojiKey, row.IconCustomEmojiID.String),
			},
			RevisedByAccountID: row.RevisedByAccountID, RevisedAt: sqlite.FromMillis(row.RevisedAt),
		}
	}
	if err := withVoiceHashtags(ctx, q, recs); err != nil {
		return nil, err
	}
	out := make([]appvoice.Revision, len(recs))
	for i, rec := range recs {
		out[i] = rec.Revision
	}
	return out, nil
}

// Participations implements appvoice.RecordsRepository.
func (r *Repository) Participations(ctx context.Context, tx uow.Tx, roomID string) ([]appvoice.ParticipationRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListVoiceRoomParticipations(ctx, roomID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list voice room participations")
	}
	out := make([]appvoice.ParticipationRecord, len(rows))
	for i, row := range rows {
		out[i] = appvoice.ParticipationRecord{Participation: appvoice.Participation{
			ID: row.ID, RoomID: row.RoomID, AccountID: row.AccountID, JoinedAt: sqlite.FromMillis(row.JoinedAt),
		}}
		if row.EndReason.Valid && row.EndedAt.Valid {
			out[i].End = &appvoice.ParticipationEnd{ParticipationID: row.ID, Reason: voice.ParticipationEndReason(row.EndReason.String), EndedAt: sqlite.FromMillis(row.EndedAt.Int64)}
		}
	}
	return out, nil
}

// Bans implements appvoice.RecordsRepository.
func (r *Repository) Bans(ctx context.Context, tx uow.Tx, roomID string) ([]appvoice.Ban, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListVoiceRoomBans(ctx, roomID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list voice room bans")
	}
	out := make([]appvoice.Ban, len(rows))
	for i, row := range rows {
		out[i] = appvoice.Ban{ID: row.ID, RoomID: row.RoomID, AccountID: row.AccountID, BannedByAccountID: row.BannedByAccountID, BannedAt: sqlite.FromMillis(row.BannedAt)}
	}
	return out, nil
}

// RoomsToPurge implements appvoice.RecordsRepository.
func (r *Repository) RoomsToPurge(ctx context.Context, tx uow.Tx, endedBefore time.Time, after string, limit int) ([]string, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListVoiceRoomsToPurge(ctx, voicegen.ListVoiceRoomsToPurgeParams{EndedBefore: sqlite.ToMillis(endedBefore), After: after, RowLimit: int64(limit)})
	return ids, sqlite.Wrap(err, "list voice rooms to purge")
}

// RecordAccounts implements appvoice.RecordsRepository.
func (r *Repository) RecordAccounts(ctx context.Context, tx uow.Tx, roomID string) ([]string, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListVoiceRoomRecordAccounts(ctx, roomID)
	return ids, sqlite.Wrap(err, "list voice room record accounts")
}

// PurgeParticipantRecords implements appvoice.RecordsRepository.
func (r *Repository) PurgeParticipantRecords(ctx context.Context, tx uow.Tx, roomIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := sqlite.JSONList(roomIDs)
	if err != nil {
		return err
	}
	steps := []func(context.Context, string) error{
		q.PurgeVoiceRoomParticipationEnds, q.PurgeVoiceRoomRoleChanges, q.PurgeVoiceRoomHandRaises, q.PurgeVoiceRoomParticipations,
		q.PurgeVoiceRoomSpeakGrants, q.PurgeVoiceRoomInvitationRevocations, q.PurgeVoiceRoomInvitations, q.PurgeVoiceRoomBans,
	}
	for _, step := range steps {
		if err := step(ctx, ids); err != nil {
			return sqlite.Wrap(err, "purge voice room records")
		}
	}
	return nil
}

// RoomsRevisedBy implements appvoice.RecordsRepository.
func (r *Repository) RoomsRevisedBy(ctx context.Context, tx uow.Tx, accountID string, limit int) ([]string, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListVoiceRoomsRevisedBy(ctx, voicegen.ListVoiceRoomsRevisedByParams{AccountID: accountID, RowLimit: int64(limit)})
	return ids, sqlite.Wrap(err, "list voice rooms revised by an account")
}

// EraseRevisionTexts implements appvoice.RecordsRepository.
func (r *Repository) EraseRevisionTexts(ctx context.Context, tx uow.Tx, accountID string, roomIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := sqlite.JSONList(roomIDs)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.EraseVoiceRoomRevisionTexts(ctx, voicegen.EraseVoiceRoomRevisionTextsParams{AccountID: accountID, Ids: ids}), "erase voice room revision texts")
}
