// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The moderation of the voice chat rooms (ADR-0100 §11): the target of a report of a room
// (reports.go), a moderator ending a room (tag admin-moderation, manage_reports), and the
// records of a reported room in the report's detail. They work while the voice chat is off too,
// except that a report then names no room.

// VoiceModeration runs the moderation of the voice chat rooms: *appvoice.Service implements it.
type VoiceModeration interface {
	ReportTarget(ctx context.Context, accountID, roomID string) (appvoice.ReportTarget, error)
	EndByModerator(ctx context.Context, in appvoice.ModeratorEndInput) error
	RoomRecords(ctx context.Context, roomID string) (appvoice.RoomRecords, error)
}

// EndVoiceRoomAsModerator serves POST /api/v1/admin/voice_rooms/{id}/end.
func (s *Server) EndVoiceRoomAsModerator(ctx context.Context, req EndVoiceRoomAsModeratorRequestObject) (EndVoiceRoomAsModeratorResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageReports); err != nil {
		return nil, err
	} else if p != nil {
		return EndVoiceRoomAsModeratordefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.VoiceModeration == nil {
		return EndVoiceRoomAsModeratordefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	in := appvoice.ModeratorEndInput{RoomID: req.Id, ByUserID: v.UserID, ByAccountID: v.AccountID}
	if req.Body != nil {
		in.Note = deref(req.Body.Reason)
	}
	err = s.o.VoiceModeration.EndByModerator(ctx, in)
	switch {
	case errors.Is(err, appvoice.ErrNotFound):
		return EndVoiceRoomAsModeratordefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appvoice.ErrInvalidEndNote):
		l := s.Localizer(ctx)
		return EndVoiceRoomAsModeratordefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsTooLong(appvoice.MaxEndNote, "reason"))), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: end voice room %s: %w", req.Id, err)
	}
	return EndVoiceRoomAsModerator204Response{}, nil
}

// reportVoiceRoom presents the voice chat room of a report with its records for the
// moderators, nil for a report of no room or of a room that the server does not have.
func (s *Server) reportVoiceRoom(ctx context.Context, r moderation.Report) (*AdminReportVoiceRoom, error) {
	if r.VoiceRoom == nil || s.o.VoiceModeration == nil {
		return nil, nil
	}
	rec, err := s.o.VoiceModeration.RoomRecords(ctx, r.VoiceRoom.RoomID)
	if errors.Is(err, appvoice.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the voice chat room of report %s: %w", r.ID, err)
	}
	out := &AdminReportVoiceRoom{
		Id: rec.ID, ReportedRevisionId: r.VoiceRoom.RevisionID, OpenedAt: rec.OpenedAt.UTC(),
		Revisions:      make([]AdminVoiceRoomRevision, 0, len(rec.Revisions)),
		Participations: make([]AdminVoiceRoomParticipation, 0, len(rec.Participations)),
		Bans:           make([]AdminVoiceRoomBan, 0, len(rec.Bans)),
	}
	if e := rec.Ending; e != nil {
		at, reason := e.EndedAt.UTC(), string(e.Reason)
		out.EndedAt, out.EndReason = &at, &reason
		if e.EndedByAccountID != "" {
			by := e.EndedByAccountID
			out.EndedByAccountId = &by
		}
		if e.Note != "" {
			note := e.Note
			out.EndNote = &note
		}
	}
	for _, rv := range rec.Revisions {
		tags := make([]string, 0, len(rv.Settings.Hashtags))
		for _, t := range rv.Settings.Hashtags {
			tags = append(tags, t.Name)
		}
		out.Revisions = append(out.Revisions, AdminVoiceRoomRevision{
			Id: rv.ID, Name: rv.Settings.Name, Description: rv.Settings.Description, Hashtags: tags,
			Visibility: VoiceRoomVisibility(rv.Settings.Visibility), Speakers: VoiceRoomSpeakers(rv.Settings.Speakers),
			RevisedByAccountId: rv.RevisedByAccountID, RevisedAt: rv.RevisedAt.UTC(),
		})
	}
	for _, p := range rec.Participations {
		item := AdminVoiceRoomParticipation{AccountId: p.AccountID, JoinedAt: p.JoinedAt.UTC()}
		if p.End != nil {
			at, reason := p.End.EndedAt.UTC(), string(p.End.Reason)
			item.LeftAt, item.LeaveReason = &at, &reason
		}
		out.Participations = append(out.Participations, item)
	}
	for _, b := range rec.Bans {
		out.Bans = append(out.Bans, AdminVoiceRoomBan{AccountId: b.AccountID, BannedByAccountId: b.BannedByAccountID, BannedAt: b.BannedAt.UTC()})
	}
	return out, nil
}
