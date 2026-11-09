// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"encoding/json"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/voicegen"
	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// Repository implements appvoice.Repository (data-model §4.14, ADR-0100 §12).
type Repository struct {
	db *moduleDB
}

var _ appvoice.Repository = (*Repository)(nil)

// NewRepository returns the voice module's repository on this database.
func NewRepository(d *sqlite.DB) *Repository { return &Repository{db: &moduleDB{d}} }

// q returns the queries in tx, or on the reader when tx is nil.
func (r *Repository) q(tx uow.Tx) (*voicegen.Queries, error) {
	if tx == nil {
		return r.db.reader(), nil
	}
	return r.db.queries(tx)
}

// InsertRoom implements appvoice.Repository.
func (r *Repository) InsertRoom(ctx context.Context, tx uow.Tx, room appvoice.Room) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertVoiceRoom(ctx, voicegen.InsertVoiceRoomParams{
		ID: room.ID, OpenedByAccountID: room.OpenedByAccountID, OpenedAt: sqlite.ToMillis(room.OpenedAt), Transport: string(room.Transport),
	}), "insert voice room")
}

// InsertRevision implements appvoice.Repository.
func (r *Repository) InsertRevision(ctx context.Context, tx uow.Tx, rev appvoice.Revision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	s := rev.Settings
	if err := q.InsertVoiceRoomRevision(ctx, voicegen.InsertVoiceRoomRevisionParams{
		ID: rev.ID, RoomID: rev.RoomID, Name: s.Name, Description: s.Description, Visibility: string(s.Visibility),
		Speakers: string(s.Speakers), IconEmojiKey: s.Icon.Key(), IconCustomEmojiID: sqlite.ToNullString(s.Icon.CustomEmojiID),
		RevisedByAccountID: rev.RevisedByAccountID, RevisedAt: sqlite.ToMillis(rev.RevisedAt),
	}); err != nil {
		return sqlite.Wrap(err, "insert voice room revision")
	}
	for i, t := range s.Hashtags {
		if err := q.InsertVoiceRoomRevisionHashtag(ctx, voicegen.InsertVoiceRoomRevisionHashtagParams{
			RevisionID: rev.ID, Position: int64(i), Name: t.Name, NameNormalized: t.NameNormalized,
		}); err != nil {
			return sqlite.Wrap(err, "insert voice room hashtag")
		}
	}
	return nil
}

// InsertHostChange implements appvoice.Repository.
func (r *Repository) InsertHostChange(ctx context.Context, tx uow.Tx, c appvoice.HostChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertVoiceRoomHostChange(ctx, voicegen.InsertVoiceRoomHostChangeParams{
		ID: c.ID, RoomID: c.RoomID, HostAccountID: c.HostAccountID, Reason: string(c.Reason), ChangedAt: sqlite.ToMillis(c.ChangedAt),
	}), "insert voice room host change")
}

// InsertCohostChange implements appvoice.Repository.
func (r *Repository) InsertCohostChange(ctx context.Context, tx uow.Tx, c appvoice.CohostChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertVoiceRoomCohostChange(ctx, voicegen.InsertVoiceRoomCohostChangeParams{
		ID: c.ID, RoomID: c.RoomID, AccountID: c.AccountID, Change: string(c.Change), ChangedAt: sqlite.ToMillis(c.ChangedAt),
	}), "insert voice room co-host change")
}

// InsertParticipation implements appvoice.Repository.
func (r *Repository) InsertParticipation(ctx context.Context, tx uow.Tx, p appvoice.Participation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertVoiceRoomParticipation(ctx, voicegen.InsertVoiceRoomParticipationParams{
		ID: p.ID, RoomID: p.RoomID, AccountID: p.AccountID, JoinedAt: sqlite.ToMillis(p.JoinedAt),
	}), "insert voice room participation")
}

// InsertParticipationEnd implements appvoice.Repository.
func (r *Repository) InsertParticipationEnd(ctx context.Context, tx uow.Tx, e appvoice.ParticipationEnd) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertVoiceRoomParticipationEnd(ctx, voicegen.InsertVoiceRoomParticipationEndParams{
		ParticipationID: e.ParticipationID, Reason: string(e.Reason), EndedAt: sqlite.ToMillis(e.EndedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert voice room participation end")
}

// InsertRoleChange implements appvoice.Repository.
func (r *Repository) InsertRoleChange(ctx context.Context, tx uow.Tx, c appvoice.RoleChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertVoiceRoomRoleChange(ctx, voicegen.InsertVoiceRoomRoleChangeParams{
		ID: c.ID, ParticipationID: c.ParticipationID, Role: string(c.Role), ChangedByAccountID: c.ChangedByAccountID,
		ChangedAt: sqlite.ToMillis(c.ChangedAt),
	}), "insert voice room role change")
}

// InsertSpeakGrant implements appvoice.Repository.
func (r *Repository) InsertSpeakGrant(ctx context.Context, tx uow.Tx, g appvoice.SpeakGrant) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertVoiceRoomSpeakGrant(ctx, voicegen.InsertVoiceRoomSpeakGrantParams{
		ID: g.ID, RoomID: g.RoomID, AccountID: g.AccountID, Change: string(g.Change), ChangedByAccountID: g.ChangedByAccountID,
		ChangedAt: sqlite.ToMillis(g.ChangedAt),
	}), "insert voice room speak grant")
}

// InsertHandRaise implements appvoice.Repository.
func (r *Repository) InsertHandRaise(ctx context.Context, tx uow.Tx, h appvoice.HandRaise) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertVoiceRoomHandRaise(ctx, voicegen.InsertVoiceRoomHandRaiseParams{
		ID: h.ID, ParticipationID: h.ParticipationID, Change: string(h.Change), ChangedAt: sqlite.ToMillis(h.ChangedAt),
	}), "insert voice room hand raise")
}

// InsertInvitation implements appvoice.Repository.
func (r *Repository) InsertInvitation(ctx context.Context, tx uow.Tx, i appvoice.Invitation) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertVoiceRoomInvitation(ctx, voicegen.InsertVoiceRoomInvitationParams{
		ID: i.ID, RoomID: i.RoomID, InviteeAccountID: i.InviteeAccountID, Role: string(i.Role),
		InvitedByAccountID: i.InvitedByAccountID, InvitedAt: sqlite.ToMillis(i.InvitedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert voice room invitation")
}

// InsertInvitationRevocation implements appvoice.Repository.
func (r *Repository) InsertInvitationRevocation(ctx context.Context, tx uow.Tx, rev appvoice.InvitationRevocation) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertVoiceRoomInvitationRevocation(ctx, voicegen.InsertVoiceRoomInvitationRevocationParams{
		InvitationID: rev.InvitationID, RevokedByAccountID: rev.RevokedByAccountID, RevokedAt: sqlite.ToMillis(rev.RevokedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert voice room invitation revocation")
}

// InsertBan implements appvoice.Repository.
func (r *Repository) InsertBan(ctx context.Context, tx uow.Tx, b appvoice.Ban) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertVoiceRoomBan(ctx, voicegen.InsertVoiceRoomBanParams{
		ID: b.ID, RoomID: b.RoomID, AccountID: b.AccountID, BannedByAccountID: b.BannedByAccountID, BannedAt: sqlite.ToMillis(b.BannedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert voice room ban")
}

// InsertEnding implements appvoice.Repository.
func (r *Repository) InsertEnding(ctx context.Context, tx uow.Tx, e appvoice.Ending) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertVoiceRoomEnding(ctx, voicegen.InsertVoiceRoomEndingParams{
		RoomID: e.RoomID, Reason: string(e.Reason), EndedByAccountID: sqlite.ToNullString(e.EndedByAccountID), EndedAt: sqlite.ToMillis(e.EndedAt),
		Note: e.Note,
	})
	return n > 0, sqlite.Wrap(err, "insert voice room ending")
}

// PutHostGrace implements appvoice.Repository.
func (r *Repository) PutHostGrace(ctx context.Context, tx uow.Tx, g appvoice.HostGrace) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertVoiceRoomHostGrace(ctx, voicegen.UpsertVoiceRoomHostGraceParams{RoomID: g.RoomID, EndsAt: sqlite.ToMillis(g.EndsAt)}),
		"put voice room host grace")
}

// DeleteHostGrace implements appvoice.Repository.
func (r *Repository) DeleteHostGrace(ctx context.Context, tx uow.Tx, roomID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteVoiceRoomHostGrace(ctx, roomID)
	return n > 0, sqlite.Wrap(err, "delete voice room host grace")
}

// FindHostGrace implements appvoice.Repository.
func (r *Repository) FindHostGrace(ctx context.Context, tx uow.Tx, roomID string) (appvoice.HostGrace, error) {
	q, err := r.q(tx)
	if err != nil {
		return appvoice.HostGrace{}, err
	}
	row, err := q.GetVoiceRoomHostGrace(ctx, roomID)
	if err != nil {
		return appvoice.HostGrace{}, sqlite.NotFound(err, appvoice.ErrNotFound, "voice room host grace")
	}
	return appvoice.HostGrace{RoomID: row.RoomID, EndsAt: sqlite.FromMillis(row.EndsAt)}, nil
}

// FindParticipationEnd implements appvoice.Repository.
func (r *Repository) FindParticipationEnd(ctx context.Context, tx uow.Tx, participationID string) (appvoice.ParticipationEnd, error) {
	q, err := r.q(tx)
	if err != nil {
		return appvoice.ParticipationEnd{}, err
	}
	row, err := q.GetVoiceRoomParticipationEnd(ctx, participationID)
	if err != nil {
		return appvoice.ParticipationEnd{}, sqlite.NotFound(err, appvoice.ErrNotFound, "voice room participation end")
	}
	return appvoice.ParticipationEnd{ParticipationID: row.ParticipationID, Reason: voice.ParticipationEndReason(row.Reason), EndedAt: sqlite.FromMillis(row.EndedAt)}, nil
}

func voiceRoomRecord(row voicegen.GetVoiceRoomRow) appvoice.RoomRecord {
	rec := appvoice.RoomRecord{
		Room: appvoice.Room{
			ID: row.ID, OpenedByAccountID: row.OpenedByAccountID, OpenedAt: sqlite.FromMillis(row.OpenedAt), Transport: voice.Transport(row.Transport),
		},
		Head: appvoice.Head{
			RoomID: row.ID, RevisionID: row.RevisionID, HostAccountID: row.HostAccountID, State: voice.State(row.State),
			ParticipantsCount: int(row.ParticipantsCount),
		},
		Revision: appvoice.Revision{
			ID: row.RevisionID, RoomID: row.ID,
			Settings: voice.Settings{
				Name: row.Name, Description: row.Description, Visibility: voice.Visibility(row.Visibility), Speakers: voice.Speakers(row.Speakers),
				Icon: voice.IconOf(row.IconEmojiKey, row.IconCustomEmojiID.String),
			},
			RevisedByAccountID: row.RevisedByAccountID, RevisedAt: sqlite.FromMillis(row.RevisedAt),
		},
	}
	if row.EndReason.Valid && row.EndedAt.Valid {
		rec.Ending = &appvoice.Ending{
			RoomID: row.ID, Reason: voice.EndReason(row.EndReason.String), EndedByAccountID: row.EndedByAccountID.String,
			EndedAt: sqlite.FromMillis(row.EndedAt.Int64), Note: row.EndNote.String,
		}
	}
	return rec
}

// withVoiceHashtags fills the hashtags of the revisions of recs.
func withVoiceHashtags(ctx context.Context, q *voicegen.Queries, recs []appvoice.RoomRecord) error {
	if len(recs) == 0 {
		return nil
	}
	ids := make([]string, len(recs))
	byRevision := make(map[string][]int, len(recs))
	for i, rec := range recs {
		ids[i] = rec.Revision.ID
		byRevision[rec.Revision.ID] = append(byRevision[rec.Revision.ID], i)
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	rows, err := q.ListVoiceRoomRevisionHashtags(ctx, string(idsJSON))
	if err != nil {
		return sqlite.Wrap(err, "list voice room hashtags")
	}
	for _, t := range rows {
		for _, i := range byRevision[t.RevisionID] {
			recs[i].Revision.Settings.Hashtags = append(recs[i].Revision.Settings.Hashtags,
				voice.Hashtag{Name: t.Name, NameNormalized: t.NameNormalized})
		}
	}
	return nil
}

// FindRoom implements appvoice.Repository.
func (r *Repository) FindRoom(ctx context.Context, tx uow.Tx, id string) (appvoice.RoomRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return appvoice.RoomRecord{}, err
	}
	row, err := q.GetVoiceRoom(ctx, id)
	if err != nil {
		return appvoice.RoomRecord{}, sqlite.NotFound(err, appvoice.ErrNotFound, "voice room")
	}
	recs := []appvoice.RoomRecord{voiceRoomRecord(row)}
	if err := withVoiceHashtags(ctx, q, recs); err != nil {
		return appvoice.RoomRecord{}, err
	}
	return recs[0], nil
}

// OpenRooms implements appvoice.Repository.
func (r *Repository) OpenRooms(ctx context.Context, tx uow.Tx, tagNormalized string) ([]appvoice.RoomRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListOpenVoiceRooms(ctx, tagNormalized)
	if err != nil {
		return nil, sqlite.Wrap(err, "list open voice rooms")
	}
	recs := make([]appvoice.RoomRecord, len(rows))
	for i, row := range rows {
		recs[i] = voiceRoomRecord(voicegen.GetVoiceRoomRow(row))
	}
	if err := withVoiceHashtags(ctx, q, recs); err != nil {
		return nil, err
	}
	return recs, nil
}

// HostedRooms implements appvoice.Repository.
func (r *Repository) HostedRooms(ctx context.Context, tx uow.Tx, accountID, beforeID string, limit int) ([]appvoice.RoomRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListHostedVoiceRooms(ctx, voicegen.ListHostedVoiceRoomsParams{AccountID: accountID, Before: beforeID, RowLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list hosted voice rooms")
	}
	recs := make([]appvoice.RoomRecord, len(rows))
	for i, row := range rows {
		recs[i] = voiceRoomRecord(voicegen.GetVoiceRoomRow(row))
	}
	if err := withVoiceHashtags(ctx, q, recs); err != nil {
		return nil, err
	}
	return recs, nil
}

func voiceParticipant(row voicegen.VoiceRoomParticipant) appvoice.Participant {
	return appvoice.Participant{
		AccountID: row.AccountID, RoomID: row.RoomID, ParticipationID: row.ParticipationID, Role: voice.Role(row.Role),
		CohostChangeID: row.CohostChangeID.String, HandRaised: row.HandRaised != 0,
	}
}

// Participants implements appvoice.Repository.
func (r *Repository) Participants(ctx context.Context, tx uow.Tx, roomID string) ([]appvoice.Participant, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListVoiceRoomParticipants(ctx, roomID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list voice room participants")
	}
	out := make([]appvoice.Participant, len(rows))
	for i, row := range rows {
		out[i] = voiceParticipant(row)
	}
	return out, nil
}

// FindParticipant implements appvoice.Repository.
func (r *Repository) FindParticipant(ctx context.Context, tx uow.Tx, accountID string) (appvoice.Participant, error) {
	q, err := r.q(tx)
	if err != nil {
		return appvoice.Participant{}, err
	}
	row, err := q.GetVoiceRoomParticipant(ctx, accountID)
	if err != nil {
		return appvoice.Participant{}, sqlite.NotFound(err, appvoice.ErrNotFound, "voice room participant")
	}
	return voiceParticipant(row), nil
}

// FindInvitation implements appvoice.Repository.
func (r *Repository) FindInvitation(ctx context.Context, tx uow.Tx, roomID, accountID string) (appvoice.InvitationRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return appvoice.InvitationRecord{}, err
	}
	row, err := q.GetVoiceRoomInvitation(ctx, voicegen.GetVoiceRoomInvitationParams{RoomID: roomID, InviteeAccountID: accountID})
	if err != nil {
		return appvoice.InvitationRecord{}, sqlite.NotFound(err, appvoice.ErrNotFound, "voice room invitation")
	}
	return appvoice.InvitationRecord{
		Invitation: appvoice.Invitation{
			ID: row.ID, RoomID: row.RoomID, InviteeAccountID: row.InviteeAccountID, Role: voice.Role(row.Role),
			InvitedByAccountID: row.InvitedByAccountID, InvitedAt: sqlite.FromMillis(row.InvitedAt),
		},
		Revoked: row.Revoked != 0,
	}, nil
}

// Invitations implements appvoice.Repository.
func (r *Repository) Invitations(ctx context.Context, tx uow.Tx, roomID, afterID string, limit int) ([]appvoice.Invitation, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListVoiceRoomInvitations(ctx, voicegen.ListVoiceRoomInvitationsParams{RoomID: roomID, ID: afterID, Limit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list voice room invitations")
	}
	out := make([]appvoice.Invitation, len(rows))
	for i, row := range rows {
		out[i] = appvoice.Invitation{
			ID: row.ID, RoomID: row.RoomID, InviteeAccountID: row.InviteeAccountID, Role: voice.Role(row.Role),
			InvitedByAccountID: row.InvitedByAccountID, InvitedAt: sqlite.FromMillis(row.InvitedAt),
		}
	}
	return out, nil
}

// Banned implements appvoice.Repository.
func (r *Repository) Banned(ctx context.Context, tx uow.Tx, roomID, accountID string) (bool, error) {
	q, err := r.q(tx)
	if err != nil {
		return false, err
	}
	banned, err := q.IsVoiceRoomBanned(ctx, voicegen.IsVoiceRoomBannedParams{RoomID: roomID, AccountID: accountID})
	return banned != 0, sqlite.Wrap(err, "find voice room ban")
}

// SpeakGranted implements appvoice.Repository.
func (r *Repository) SpeakGranted(ctx context.Context, tx uow.Tx, roomID, accountID string) (bool, error) {
	q, err := r.q(tx)
	if err != nil {
		return false, err
	}
	granted, err := q.IsVoiceRoomSpeakGranted(ctx, voicegen.IsVoiceRoomSpeakGrantedParams{RoomID: roomID, AccountID: accountID})
	return granted != 0, sqlite.Wrap(err, "find voice room speak grant")
}

// ViewerFacts implements appvoice.Repository.
func (r *Repository) ViewerFacts(ctx context.Context, tx uow.Tx, roomID string, accountIDs []string) (map[string]appvoice.ViewerFacts, error) {
	out := make(map[string]appvoice.ViewerFacts, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	list, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListVoiceRoomViewerFacts(ctx, voicegen.ListVoiceRoomViewerFactsParams{RoomID: roomID, AccountIds: list})
	if err != nil {
		return nil, sqlite.Wrap(err, "list voice room viewer facts")
	}
	for _, row := range rows {
		out[row.AccountID] = appvoice.ViewerFacts{Banned: row.Banned, Invited: row.Invited, SpeakGranted: row.SpeakGranted != 0}
	}
	return out, nil
}

// RefreshHead implements appvoice.Repository.
func (r *Repository) RefreshHead(ctx context.Context, tx uow.Tx, roomID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteVoiceRoomHead(ctx, roomID); err != nil {
		return sqlite.Wrap(err, "delete voice room head")
	}
	return sqlite.Wrap(q.InsertVoiceRoomHead(ctx, roomID), "derive voice room head")
}

// RefreshParticipant implements appvoice.Repository.
func (r *Repository) RefreshParticipant(ctx context.Context, tx uow.Tx, roomID, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteVoiceRoomParticipant(ctx, voicegen.DeleteVoiceRoomParticipantParams{RoomID: roomID, AccountID: accountID}); err != nil {
		return sqlite.Wrap(err, "delete voice room participant")
	}
	err = q.InsertVoiceRoomParticipant(ctx, voicegen.InsertVoiceRoomParticipantParams{RoomID: roomID, AccountID: accountID})
	if sqlite.IsUniqueViolation(err, "voice_room_participants.account_id") {
		return appvoice.ErrInAnotherRoom
	}
	return sqlite.Wrap(err, "derive voice room participant")
}

// RefreshRoomParticipants implements appvoice.Repository.
func (r *Repository) RefreshRoomParticipants(ctx context.Context, tx uow.Tx, roomID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteVoiceRoomParticipants(ctx, roomID); err != nil {
		return sqlite.Wrap(err, "delete voice room participants")
	}
	err = q.InsertVoiceRoomParticipants(ctx, roomID)
	if sqlite.IsUniqueViolation(err, "voice_room_participants.account_id") {
		return appvoice.ErrInAnotherRoom
	}
	return sqlite.Wrap(err, "derive voice room participants")
}

// RebuildProjections implements appvoice.Repository.
func (r *Repository) RebuildProjections(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild voice projections",
		(*voicegen.Queries).DeleteAllVoiceRoomHeads, (*voicegen.Queries).RebuildVoiceRoomHeads,
		(*voicegen.Queries).DeleteAllVoiceRoomParticipants, (*voicegen.Queries).RebuildVoiceRoomParticipants)
}

// LockRoom implements appvoice.Repository. SQLite runs one write transaction at a time, so
// there is nothing to lock.
func (r *Repository) LockRoom(context.Context, uow.Tx, string) error { return nil }

// LockOpening implements appvoice.Repository; see LockRoom.
func (r *Repository) LockOpening(context.Context, uow.Tx) error { return nil }
