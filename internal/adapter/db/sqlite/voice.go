// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"encoding/json"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// VoiceRepository implements appvoice.Repository (data-model §4.14, ADR-0100 §12).
type VoiceRepository struct {
	db *DB
}

var _ appvoice.Repository = (*VoiceRepository)(nil)

// VoiceRepository returns the voice module's repository on this database.
func (d *DB) VoiceRepository() *VoiceRepository { return &VoiceRepository{db: d} }

// q returns the queries in tx, or on the reader when tx is nil.
func (r *VoiceRepository) q(tx uow.Tx) (*sqlitegen.Queries, error) {
	if tx == nil {
		return r.db.reader(), nil
	}
	return queries(tx)
}

// InsertRoom implements appvoice.Repository.
func (r *VoiceRepository) InsertRoom(ctx context.Context, tx uow.Tx, room appvoice.Room) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertVoiceRoom(ctx, sqlitegen.InsertVoiceRoomParams{
		ID: room.ID, OpenedByAccountID: room.OpenedByAccountID, OpenedAt: toMillis(room.OpenedAt), Transport: string(room.Transport),
	}), "insert voice room")
}

// InsertRevision implements appvoice.Repository.
func (r *VoiceRepository) InsertRevision(ctx context.Context, tx uow.Tx, rev appvoice.Revision) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	s := rev.Settings
	if err := q.InsertVoiceRoomRevision(ctx, sqlitegen.InsertVoiceRoomRevisionParams{
		ID: rev.ID, RoomID: rev.RoomID, Name: s.Name, Description: s.Description, Visibility: string(s.Visibility),
		Speakers: string(s.Speakers), RevisedByAccountID: rev.RevisedByAccountID, RevisedAt: toMillis(rev.RevisedAt),
	}); err != nil {
		return wrap(err, "insert voice room revision")
	}
	for i, t := range s.Hashtags {
		if err := q.InsertVoiceRoomRevisionHashtag(ctx, sqlitegen.InsertVoiceRoomRevisionHashtagParams{
			RevisionID: rev.ID, Position: int64(i), Name: t.Name, NameNormalized: t.NameNormalized,
		}); err != nil {
			return wrap(err, "insert voice room hashtag")
		}
	}
	return nil
}

// InsertHostChange implements appvoice.Repository.
func (r *VoiceRepository) InsertHostChange(ctx context.Context, tx uow.Tx, c appvoice.HostChange) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertVoiceRoomHostChange(ctx, sqlitegen.InsertVoiceRoomHostChangeParams{
		ID: c.ID, RoomID: c.RoomID, HostAccountID: c.HostAccountID, Reason: string(c.Reason), ChangedAt: toMillis(c.ChangedAt),
	}), "insert voice room host change")
}

// InsertCohostChange implements appvoice.Repository.
func (r *VoiceRepository) InsertCohostChange(ctx context.Context, tx uow.Tx, c appvoice.CohostChange) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertVoiceRoomCohostChange(ctx, sqlitegen.InsertVoiceRoomCohostChangeParams{
		ID: c.ID, RoomID: c.RoomID, AccountID: c.AccountID, Change: string(c.Change), ChangedAt: toMillis(c.ChangedAt),
	}), "insert voice room co-host change")
}

// InsertParticipation implements appvoice.Repository.
func (r *VoiceRepository) InsertParticipation(ctx context.Context, tx uow.Tx, p appvoice.Participation) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertVoiceRoomParticipation(ctx, sqlitegen.InsertVoiceRoomParticipationParams{
		ID: p.ID, RoomID: p.RoomID, AccountID: p.AccountID, JoinedAt: toMillis(p.JoinedAt),
	}), "insert voice room participation")
}

// InsertParticipationEnd implements appvoice.Repository.
func (r *VoiceRepository) InsertParticipationEnd(ctx context.Context, tx uow.Tx, e appvoice.ParticipationEnd) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertVoiceRoomParticipationEnd(ctx, sqlitegen.InsertVoiceRoomParticipationEndParams{
		ParticipationID: e.ParticipationID, Reason: string(e.Reason), EndedAt: toMillis(e.EndedAt),
	})
	return n > 0, wrap(err, "insert voice room participation end")
}

// InsertRoleChange implements appvoice.Repository.
func (r *VoiceRepository) InsertRoleChange(ctx context.Context, tx uow.Tx, c appvoice.RoleChange) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertVoiceRoomRoleChange(ctx, sqlitegen.InsertVoiceRoomRoleChangeParams{
		ID: c.ID, ParticipationID: c.ParticipationID, Role: string(c.Role), ChangedByAccountID: c.ChangedByAccountID,
		ChangedAt: toMillis(c.ChangedAt),
	}), "insert voice room role change")
}

// InsertSpeakGrant implements appvoice.Repository.
func (r *VoiceRepository) InsertSpeakGrant(ctx context.Context, tx uow.Tx, g appvoice.SpeakGrant) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertVoiceRoomSpeakGrant(ctx, sqlitegen.InsertVoiceRoomSpeakGrantParams{
		ID: g.ID, RoomID: g.RoomID, AccountID: g.AccountID, Change: string(g.Change), ChangedByAccountID: g.ChangedByAccountID,
		ChangedAt: toMillis(g.ChangedAt),
	}), "insert voice room speak grant")
}

// InsertHandRaise implements appvoice.Repository.
func (r *VoiceRepository) InsertHandRaise(ctx context.Context, tx uow.Tx, h appvoice.HandRaise) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertVoiceRoomHandRaise(ctx, sqlitegen.InsertVoiceRoomHandRaiseParams{
		ID: h.ID, ParticipationID: h.ParticipationID, Change: string(h.Change), ChangedAt: toMillis(h.ChangedAt),
	}), "insert voice room hand raise")
}

// InsertInvitation implements appvoice.Repository.
func (r *VoiceRepository) InsertInvitation(ctx context.Context, tx uow.Tx, i appvoice.Invitation) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertVoiceRoomInvitation(ctx, sqlitegen.InsertVoiceRoomInvitationParams{
		ID: i.ID, RoomID: i.RoomID, InviteeAccountID: i.InviteeAccountID, Role: string(i.Role),
		InvitedByAccountID: i.InvitedByAccountID, InvitedAt: toMillis(i.InvitedAt),
	})
	return n > 0, wrap(err, "insert voice room invitation")
}

// InsertInvitationRevocation implements appvoice.Repository.
func (r *VoiceRepository) InsertInvitationRevocation(ctx context.Context, tx uow.Tx, rev appvoice.InvitationRevocation) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertVoiceRoomInvitationRevocation(ctx, sqlitegen.InsertVoiceRoomInvitationRevocationParams{
		InvitationID: rev.InvitationID, RevokedByAccountID: rev.RevokedByAccountID, RevokedAt: toMillis(rev.RevokedAt),
	})
	return n > 0, wrap(err, "insert voice room invitation revocation")
}

// InsertBan implements appvoice.Repository.
func (r *VoiceRepository) InsertBan(ctx context.Context, tx uow.Tx, b appvoice.Ban) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertVoiceRoomBan(ctx, sqlitegen.InsertVoiceRoomBanParams{
		ID: b.ID, RoomID: b.RoomID, AccountID: b.AccountID, BannedByAccountID: b.BannedByAccountID, BannedAt: toMillis(b.BannedAt),
	})
	return n > 0, wrap(err, "insert voice room ban")
}

// InsertEnding implements appvoice.Repository.
func (r *VoiceRepository) InsertEnding(ctx context.Context, tx uow.Tx, e appvoice.Ending) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertVoiceRoomEnding(ctx, sqlitegen.InsertVoiceRoomEndingParams{
		RoomID: e.RoomID, Reason: string(e.Reason), EndedByAccountID: toNullString(e.EndedByAccountID), EndedAt: toMillis(e.EndedAt),
	})
	return n > 0, wrap(err, "insert voice room ending")
}

// PutHostGrace implements appvoice.Repository.
func (r *VoiceRepository) PutHostGrace(ctx context.Context, tx uow.Tx, g appvoice.HostGrace) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertVoiceRoomHostGrace(ctx, sqlitegen.UpsertVoiceRoomHostGraceParams{RoomID: g.RoomID, EndsAt: toMillis(g.EndsAt)}),
		"put voice room host grace")
}

// DeleteHostGrace implements appvoice.Repository.
func (r *VoiceRepository) DeleteHostGrace(ctx context.Context, tx uow.Tx, roomID string) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteVoiceRoomHostGrace(ctx, roomID)
	return n > 0, wrap(err, "delete voice room host grace")
}

// FindHostGrace implements appvoice.Repository.
func (r *VoiceRepository) FindHostGrace(ctx context.Context, tx uow.Tx, roomID string) (appvoice.HostGrace, error) {
	q, err := r.q(tx)
	if err != nil {
		return appvoice.HostGrace{}, err
	}
	row, err := q.GetVoiceRoomHostGrace(ctx, roomID)
	if err != nil {
		return appvoice.HostGrace{}, notFound(err, appvoice.ErrNotFound, "voice room host grace")
	}
	return appvoice.HostGrace{RoomID: row.RoomID, EndsAt: fromMillis(row.EndsAt)}, nil
}

func voiceRoomRecord(row sqlitegen.GetVoiceRoomRow) appvoice.RoomRecord {
	rec := appvoice.RoomRecord{
		Room: appvoice.Room{
			ID: row.ID, OpenedByAccountID: row.OpenedByAccountID, OpenedAt: fromMillis(row.OpenedAt), Transport: voice.Transport(row.Transport),
		},
		Head: appvoice.Head{
			RoomID: row.ID, RevisionID: row.RevisionID, HostAccountID: row.HostAccountID, State: voice.State(row.State),
			ParticipantsCount: int(row.ParticipantsCount),
		},
		Revision: appvoice.Revision{
			ID: row.RevisionID, RoomID: row.ID,
			Settings: voice.Settings{
				Name: row.Name, Description: row.Description, Visibility: voice.Visibility(row.Visibility), Speakers: voice.Speakers(row.Speakers),
			},
			RevisedByAccountID: row.RevisedByAccountID, RevisedAt: fromMillis(row.RevisedAt),
		},
	}
	if row.EndReason.Valid && row.EndedAt.Valid {
		rec.Ending = &appvoice.Ending{
			RoomID: row.ID, Reason: voice.EndReason(row.EndReason.String), EndedByAccountID: row.EndedByAccountID.String,
			EndedAt: fromMillis(row.EndedAt.Int64),
		}
	}
	return rec
}

// withVoiceHashtags fills the hashtags of the revisions of recs.
func withVoiceHashtags(ctx context.Context, q *sqlitegen.Queries, recs []appvoice.RoomRecord) error {
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
		return wrap(err, "list voice room hashtags")
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
func (r *VoiceRepository) FindRoom(ctx context.Context, tx uow.Tx, id string) (appvoice.RoomRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return appvoice.RoomRecord{}, err
	}
	row, err := q.GetVoiceRoom(ctx, id)
	if err != nil {
		return appvoice.RoomRecord{}, notFound(err, appvoice.ErrNotFound, "voice room")
	}
	recs := []appvoice.RoomRecord{voiceRoomRecord(row)}
	if err := withVoiceHashtags(ctx, q, recs); err != nil {
		return appvoice.RoomRecord{}, err
	}
	return recs[0], nil
}

// OpenRooms implements appvoice.Repository.
func (r *VoiceRepository) OpenRooms(ctx context.Context, tx uow.Tx, tagNormalized string) ([]appvoice.RoomRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListOpenVoiceRooms(ctx, tagNormalized)
	if err != nil {
		return nil, wrap(err, "list open voice rooms")
	}
	recs := make([]appvoice.RoomRecord, len(rows))
	for i, row := range rows {
		recs[i] = voiceRoomRecord(sqlitegen.GetVoiceRoomRow(row))
	}
	if err := withVoiceHashtags(ctx, q, recs); err != nil {
		return nil, err
	}
	return recs, nil
}

func voiceParticipant(row sqlitegen.VoiceRoomParticipant) appvoice.Participant {
	return appvoice.Participant{
		AccountID: row.AccountID, RoomID: row.RoomID, ParticipationID: row.ParticipationID, Role: voice.Role(row.Role),
		CohostChangeID: row.CohostChangeID.String, HandRaised: row.HandRaised != 0,
	}
}

// Participants implements appvoice.Repository.
func (r *VoiceRepository) Participants(ctx context.Context, tx uow.Tx, roomID string) ([]appvoice.Participant, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListVoiceRoomParticipants(ctx, roomID)
	if err != nil {
		return nil, wrap(err, "list voice room participants")
	}
	out := make([]appvoice.Participant, len(rows))
	for i, row := range rows {
		out[i] = voiceParticipant(row)
	}
	return out, nil
}

// FindParticipant implements appvoice.Repository.
func (r *VoiceRepository) FindParticipant(ctx context.Context, tx uow.Tx, accountID string) (appvoice.Participant, error) {
	q, err := r.q(tx)
	if err != nil {
		return appvoice.Participant{}, err
	}
	row, err := q.GetVoiceRoomParticipant(ctx, accountID)
	if err != nil {
		return appvoice.Participant{}, notFound(err, appvoice.ErrNotFound, "voice room participant")
	}
	return voiceParticipant(row), nil
}

// FindInvitation implements appvoice.Repository.
func (r *VoiceRepository) FindInvitation(ctx context.Context, tx uow.Tx, roomID, accountID string) (appvoice.InvitationRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return appvoice.InvitationRecord{}, err
	}
	row, err := q.GetVoiceRoomInvitation(ctx, sqlitegen.GetVoiceRoomInvitationParams{RoomID: roomID, InviteeAccountID: accountID})
	if err != nil {
		return appvoice.InvitationRecord{}, notFound(err, appvoice.ErrNotFound, "voice room invitation")
	}
	return appvoice.InvitationRecord{
		Invitation: appvoice.Invitation{
			ID: row.ID, RoomID: row.RoomID, InviteeAccountID: row.InviteeAccountID, Role: voice.Role(row.Role),
			InvitedByAccountID: row.InvitedByAccountID, InvitedAt: fromMillis(row.InvitedAt),
		},
		Revoked: row.Revoked != 0,
	}, nil
}

// Invitations implements appvoice.Repository.
func (r *VoiceRepository) Invitations(ctx context.Context, tx uow.Tx, roomID, afterID string, limit int) ([]appvoice.Invitation, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListVoiceRoomInvitations(ctx, sqlitegen.ListVoiceRoomInvitationsParams{RoomID: roomID, ID: afterID, Limit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list voice room invitations")
	}
	out := make([]appvoice.Invitation, len(rows))
	for i, row := range rows {
		out[i] = appvoice.Invitation{
			ID: row.ID, RoomID: row.RoomID, InviteeAccountID: row.InviteeAccountID, Role: voice.Role(row.Role),
			InvitedByAccountID: row.InvitedByAccountID, InvitedAt: fromMillis(row.InvitedAt),
		}
	}
	return out, nil
}

// Banned implements appvoice.Repository.
func (r *VoiceRepository) Banned(ctx context.Context, tx uow.Tx, roomID, accountID string) (bool, error) {
	q, err := r.q(tx)
	if err != nil {
		return false, err
	}
	banned, err := q.IsVoiceRoomBanned(ctx, sqlitegen.IsVoiceRoomBannedParams{RoomID: roomID, AccountID: accountID})
	return banned != 0, wrap(err, "find voice room ban")
}

// SpeakGranted implements appvoice.Repository.
func (r *VoiceRepository) SpeakGranted(ctx context.Context, tx uow.Tx, roomID, accountID string) (bool, error) {
	q, err := r.q(tx)
	if err != nil {
		return false, err
	}
	granted, err := q.IsVoiceRoomSpeakGranted(ctx, sqlitegen.IsVoiceRoomSpeakGrantedParams{RoomID: roomID, AccountID: accountID})
	return granted != 0, wrap(err, "find voice room speak grant")
}

// RefreshHead implements appvoice.Repository.
func (r *VoiceRepository) RefreshHead(ctx context.Context, tx uow.Tx, roomID string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteVoiceRoomHead(ctx, roomID); err != nil {
		return wrap(err, "delete voice room head")
	}
	return wrap(q.InsertVoiceRoomHead(ctx, roomID), "derive voice room head")
}

// RefreshParticipant implements appvoice.Repository.
func (r *VoiceRepository) RefreshParticipant(ctx context.Context, tx uow.Tx, roomID, accountID string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteVoiceRoomParticipant(ctx, sqlitegen.DeleteVoiceRoomParticipantParams{RoomID: roomID, AccountID: accountID}); err != nil {
		return wrap(err, "delete voice room participant")
	}
	err = q.InsertVoiceRoomParticipant(ctx, sqlitegen.InsertVoiceRoomParticipantParams{RoomID: roomID, AccountID: accountID})
	if isUniqueViolation(err, "voice_room_participants.account_id") {
		return appvoice.ErrInAnotherRoom
	}
	return wrap(err, "derive voice room participant")
}

// RefreshRoomParticipants implements appvoice.Repository.
func (r *VoiceRepository) RefreshRoomParticipants(ctx context.Context, tx uow.Tx, roomID string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteVoiceRoomParticipants(ctx, roomID); err != nil {
		return wrap(err, "delete voice room participants")
	}
	err = q.InsertVoiceRoomParticipants(ctx, roomID)
	if isUniqueViolation(err, "voice_room_participants.account_id") {
		return appvoice.ErrInAnotherRoom
	}
	return wrap(err, "derive voice room participants")
}

// RebuildProjections implements appvoice.Repository.
func (r *VoiceRepository) RebuildProjections(ctx context.Context, tx uow.Tx) error {
	return execAll(ctx, tx, "rebuild voice projections",
		(*sqlitegen.Queries).DeleteAllVoiceRoomHeads, (*sqlitegen.Queries).RebuildVoiceRoomHeads,
		(*sqlitegen.Queries).DeleteAllVoiceRoomParticipants, (*sqlitegen.Queries).RebuildVoiceRoomParticipants)
}

// LockRoom implements appvoice.Repository. SQLite runs one write transaction at a time, so
// there is nothing to lock.
func (r *VoiceRepository) LockRoom(context.Context, uow.Tx, string) error { return nil }

// LockOpening implements appvoice.Repository; see LockRoom.
func (r *VoiceRepository) LockOpening(context.Context, uow.Tx) error { return nil }
