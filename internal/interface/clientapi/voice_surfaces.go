// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The places that tell of the voice chat rooms (ADR-0100 §8, §9, §13): the summary of a room for
// the visitors who do not see it, the history of the rooms the user hosted, the entry on the
// profiles, and the rooms of the notifications.

// VoiceSurfaces reads the rooms for those places: *appvoice.Service implements it.
type VoiceSurfaces interface {
	Summary(ctx context.Context, roomID string) (appvoice.Summary, error)
	History(ctx context.Context, accountID, beforeID string, limit int) ([]appvoice.RoomRecord, bool, error)
	ProfileRoom(ctx context.Context, viewerID, accountID string) (appvoice.RoomView, bool, error)
	Visible(ctx context.Context, accountID, roomID string) (appvoice.RoomView, error)
	Icons(ctx context.Context, icons []voice.Icon) (map[voice.Icon]appvoice.IconView, error)
}

// GetVoiceRoomSummary serves GET /api/v1/voice/rooms/{id}/summary, which needs no sign-in.
func (s *Server) GetVoiceRoomSummary(ctx context.Context, req GetVoiceRoomSummaryRequestObject) (GetVoiceRoomSummaryResponseObject, error) {
	if s.o.Voice == nil {
		return GetVoiceRoomSummarydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	sum, err := s.o.Voice.Summary(ctx, req.Id)
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[GetVoiceRoomSummarydefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out := VoiceRoomSummary{Id: sum.RoomID, State: VoiceRoomSummaryStateOpen}
	if sum.Ended {
		out.State = VoiceRoomSummaryStateEnded
	}
	if sum.Shown {
		out.Name = &sum.Name
		if !sum.Ended {
			out.Description = &sum.Description
		}
		if !sum.Icon.IsZero() {
			icons, err := s.o.Voice.Icons(ctx, []voice.Icon{sum.Icon})
			if err != nil {
				return nil, err
			}
			if v, ok := icons[sum.Icon]; ok {
				out.Icon = newVoiceRoomIcon(v)
			}
		}
	}
	if sum.HostAccountID != "" {
		a, err := s.o.Accounts.FindByID(ctx, sum.HostAccountID)
		switch {
		case errors.Is(err, appaccount.ErrNotFound):
		case err != nil:
			return nil, err
		default:
			accounts, err := s.newAccounts(ctx, []appaccount.Actor{a})
			if err != nil {
				return nil, err
			}
			out.Host = &accounts[0]
		}
	}
	return GetVoiceRoomSummary200JSONResponse{Body: out, Headers: GetVoiceRoomSummary200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// ListVoiceHistory serves GET /api/v1/voice/history.
func (s *Server) ListVoiceHistory(ctx context.Context, req ListVoiceHistoryRequestObject) (ListVoiceHistoryResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[ListVoiceHistorydefaultApplicationProblemPlusJSONResponse](p, err)
	}
	q := s.parsePageOf(ctx, req.Params.Cursor, req.Params.Limit, kernel.ValidID)
	if q.problem != nil {
		return ListVoiceHistorydefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	recs, more, err := rooms.History(ctx, v.AccountID, q.before, q.limit)
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[ListVoiceHistorydefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out := VoiceRoomHistoryPage{Items: make([]VoiceRoomHistoryEntry, 0, len(recs))}
	for _, r := range recs {
		e := VoiceRoomHistoryEntry{Id: r.ID, Name: r.Revision.Settings.Name, OpenedAt: r.OpenedAt.UTC()}
		if r.Ending != nil {
			at, reason := r.Ending.EndedAt.UTC(), string(r.Ending.Reason)
			e.EndedAt, e.EndReason = &at, &reason
		}
		out.Items = append(out.Items, e)
	}
	if more {
		out.NextCursor = encodeCursor(recs[len(recs)-1].ID)
	}
	return ListVoiceHistory200JSONResponse{Body: out, Headers: ListVoiceHistory200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// voiceRoomEntry returns the open room on the profile of accountID that the viewer sees, nil for
// none or while the voice chat is off.
func (s *Server) voiceRoomEntry(ctx context.Context, v *Viewer, accountID string) (*VoiceRoomEntry, error) {
	if s.o.Voice == nil || v == nil {
		return nil, nil
	}
	room, ok, err := s.o.Voice.ProfileRoom(ctx, v.AccountID, accountID)
	if err != nil || !ok {
		return nil, err
	}
	role := VoiceRoomEntryRoleSpeaker
	if room.Head.HostAccountID == accountID {
		role = VoiceRoomEntryRoleHost
	}
	icons, err := s.voiceRoomIcons(ctx, []appvoice.RoomView{room})
	if err != nil {
		return nil, err
	}
	return &VoiceRoomEntry{
		Id: room.ID, Name: room.Revision.Settings.Name, Icon: icons[room.Revision.Settings.Icon], Role: role,
		ParticipantsCount: len(room.Participants),
	}, nil
}

// notifiedVoiceRoom returns the room of a voice notification while it is open and the viewer
// sees it, nil otherwise, reading each room once per page through seen.
func (s *Server) notifiedVoiceRoom(ctx context.Context, accountID, roomID string, seen map[string]*VoiceRoom) (*VoiceRoom, error) {
	if room, ok := seen[roomID]; ok {
		return room, nil
	}
	seen[roomID] = nil
	if s.o.Voice == nil {
		return nil, nil
	}
	view, err := s.o.Voice.Visible(ctx, accountID, roomID)
	if errors.Is(err, appvoice.ErrNotFound) || errors.Is(err, appvoice.ErrDisabled) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if view.Head.State != voice.StateOpen {
		return nil, nil
	}
	room, err := s.newVoiceRoom(ctx, view)
	if err != nil {
		return nil, err
	}
	seen[roomID] = &room
	return &room, nil
}
