// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The operations of the participants of the voice chat rooms (ADR-0100 §5, §7, §14): speaking
// and listening, raising a hand, the operations of the host and the co-hosts on the
// participants, the co-hosts, and the invitations.

// The problem types of the participant operations.
const (
	ProblemTypeVoiceNotHostOrCohost    = "/api/v1/problems/voice-not-host-or-cohost"
	ProblemTypeVoiceCannotSpeak        = "/api/v1/problems/voice-cannot-speak"
	ProblemTypeVoiceInvalidParticipant = "/api/v1/problems/voice-invalid-participant"
	ProblemTypeVoiceTooManyCohosts     = "/api/v1/problems/voice-too-many-cohosts"
	ProblemTypeVoiceInvalidInvitee     = "/api/v1/problems/voice-invalid-invitee"
	ProblemTypeVoiceAlreadyInvited     = "/api/v1/problems/voice-already-invited"
)

// VoiceParticipants runs the operations of the participants of the rooms: *appvoice.Service
// implements it.
type VoiceParticipants interface {
	Speak(ctx context.Context, accountID, roomID string) (appvoice.RoomView, error)
	Listen(ctx context.Context, accountID, roomID string) (appvoice.RoomView, error)
	RaiseHand(ctx context.Context, accountID, roomID string) (appvoice.RoomView, error)
	LowerHand(ctx context.Context, accountID, roomID string) (appvoice.RoomView, error)
	Grant(ctx context.Context, byAccountID, roomID, accountID string) (appvoice.RoomView, error)
	Revoke(ctx context.Context, byAccountID, roomID, accountID string) (appvoice.RoomView, error)
	Dismiss(ctx context.Context, byAccountID, roomID, accountID string) (appvoice.RoomView, error)
	Mute(ctx context.Context, byAccountID, roomID, accountID string) error
	Remove(ctx context.Context, byAccountID, roomID, accountID string) error
	AssignCohost(ctx context.Context, byAccountID, roomID, accountID string) (appvoice.RoomView, error)
	RemoveCohost(ctx context.Context, byAccountID, roomID, accountID string) (appvoice.RoomView, error)
	Invite(ctx context.Context, byAccountID, roomID, accountID string, role voice.Role) (appvoice.Invitation, error)
	RevokeInvitation(ctx context.Context, byAccountID, roomID, accountID string) error
	Invitations(ctx context.Context, byAccountID, roomID, afterID string, limit int) ([]appvoice.Invitation, bool, error)
}

// voiceParticipantProblem maps the errors of the participant operations; it reports false for
// any other error.
func (s *Server) voiceParticipantProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	var limited *kernel.RateLimitedError
	switch {
	case errors.As(err, &limited):
		retry := seconds(limited.RetryAfter)
		p := s.problem(ctx, http.StatusTooManyRequests, ProblemTypeBlank, l.ServerErrorsRateLimited(), l.ServerErrorsRateLimitedDetail(retry))
		p.Headers.RetryAfter = &retry
		return p, true
	case errors.Is(err, appvoice.ErrNotHostOrCohost):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeVoiceNotHostOrCohost, l.ServerErrorsVoiceNotHostOrCohost(), ""), true
	case errors.Is(err, appvoice.ErrCannotSpeak):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeVoiceCannotSpeak, l.ServerErrorsVoiceCannotSpeak(), ""), true
	case errors.Is(err, appvoice.ErrInvalidParticipant):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeVoiceInvalidParticipant, l.ServerErrorsVoiceInvalidParticipant(), ""), true
	case errors.Is(err, appvoice.ErrTooManyCohosts):
		return s.problem(ctx, http.StatusConflict, ProblemTypeVoiceTooManyCohosts, l.ServerErrorsVoiceTooManyCohosts(), ""), true
	case errors.Is(err, appvoice.ErrInvalidInvitee):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeVoiceInvalidInvitee, l.ServerErrorsVoiceInvalidInvitee(), ""), true
	case errors.Is(err, appvoice.ErrAlreadyInvited):
		return s.problem(ctx, http.StatusConflict, ProblemTypeVoiceAlreadyInvited, l.ServerErrorsVoiceAlreadyInvited(), ""), true
	case errors.Is(err, appvoice.ErrInvalidRole):
		return s.unprocessable(ctx, "role"), true
	}
	return problem{}, false
}

// voiceRoomChange runs change on the rooms and presents the room it returns.
func (s *Server) voiceRoomChange(ctx context.Context, change func(VoiceRooms, string) (appvoice.RoomView, error)) (*VoiceRoom, *problem, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return nil, p, err
	}
	room, err := change(rooms, v.AccountID)
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return nil, p, err
	}
	out, err := s.newVoiceRoom(ctx, room)
	if err != nil {
		return nil, nil, err
	}
	return &out, nil, nil
}

// voiceOperation runs op on the rooms.
func (s *Server) voiceOperation(ctx context.Context, op func(VoiceRooms, string) error) (*problem, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return p, err
	}
	if err := op(rooms, v.AccountID); err != nil {
		return s.voiceFail(ctx, err)
	}
	return nil, nil
}

// BecomeVoiceSpeaker serves POST /api/v1/voice/rooms/{id}/speaker.
func (s *Server) BecomeVoiceSpeaker(ctx context.Context, req BecomeVoiceSpeakerRequestObject) (BecomeVoiceSpeakerResponseObject, error) {
	out, p, err := s.voiceRoomChange(ctx, func(r VoiceRooms, me string) (appvoice.RoomView, error) { return r.Speak(ctx, me, req.Id) })
	if out == nil {
		return respond[BecomeVoiceSpeakerdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return BecomeVoiceSpeaker200JSONResponse{Body: *out, Headers: BecomeVoiceSpeaker200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// BecomeVoiceListener serves DELETE /api/v1/voice/rooms/{id}/speaker.
func (s *Server) BecomeVoiceListener(ctx context.Context, req BecomeVoiceListenerRequestObject) (BecomeVoiceListenerResponseObject, error) {
	out, p, err := s.voiceRoomChange(ctx, func(r VoiceRooms, me string) (appvoice.RoomView, error) { return r.Listen(ctx, me, req.Id) })
	if out == nil {
		return respond[BecomeVoiceListenerdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return BecomeVoiceListener200JSONResponse{Body: *out, Headers: BecomeVoiceListener200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// RaiseVoiceHand serves POST /api/v1/voice/rooms/{id}/hand.
func (s *Server) RaiseVoiceHand(ctx context.Context, req RaiseVoiceHandRequestObject) (RaiseVoiceHandResponseObject, error) {
	out, p, err := s.voiceRoomChange(ctx, func(r VoiceRooms, me string) (appvoice.RoomView, error) { return r.RaiseHand(ctx, me, req.Id) })
	if out == nil {
		return respond[RaiseVoiceHanddefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return RaiseVoiceHand200JSONResponse{Body: *out, Headers: RaiseVoiceHand200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// LowerVoiceHand serves DELETE /api/v1/voice/rooms/{id}/hand.
func (s *Server) LowerVoiceHand(ctx context.Context, req LowerVoiceHandRequestObject) (LowerVoiceHandResponseObject, error) {
	out, p, err := s.voiceRoomChange(ctx, func(r VoiceRooms, me string) (appvoice.RoomView, error) { return r.LowerHand(ctx, me, req.Id) })
	if out == nil {
		return respond[LowerVoiceHanddefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return LowerVoiceHand200JSONResponse{Body: *out, Headers: LowerVoiceHand200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// RemoveVoiceParticipant serves DELETE /api/v1/voice/rooms/{id}/participants/{account_id}.
func (s *Server) RemoveVoiceParticipant(ctx context.Context, req RemoveVoiceParticipantRequestObject) (RemoveVoiceParticipantResponseObject, error) {
	p, err := s.voiceOperation(ctx, func(r VoiceRooms, me string) error { return r.Remove(ctx, me, req.Id, knownID(req.AccountId)) })
	if p != nil || err != nil {
		return respond[RemoveVoiceParticipantdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return RemoveVoiceParticipant204Response{}, nil
}

// GrantVoiceSpeaking serves POST /api/v1/voice/rooms/{id}/participants/{account_id}/grant.
func (s *Server) GrantVoiceSpeaking(ctx context.Context, req GrantVoiceSpeakingRequestObject) (GrantVoiceSpeakingResponseObject, error) {
	out, p, err := s.voiceRoomChange(ctx, func(r VoiceRooms, me string) (appvoice.RoomView, error) {
		return r.Grant(ctx, me, req.Id, knownID(req.AccountId))
	})
	if out == nil {
		return respond[GrantVoiceSpeakingdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return GrantVoiceSpeaking200JSONResponse{Body: *out, Headers: GrantVoiceSpeaking200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// RevokeVoiceSpeaking serves DELETE /api/v1/voice/rooms/{id}/participants/{account_id}/grant.
func (s *Server) RevokeVoiceSpeaking(ctx context.Context, req RevokeVoiceSpeakingRequestObject) (RevokeVoiceSpeakingResponseObject, error) {
	out, p, err := s.voiceRoomChange(ctx, func(r VoiceRooms, me string) (appvoice.RoomView, error) {
		return r.Revoke(ctx, me, req.Id, knownID(req.AccountId))
	})
	if out == nil {
		return respond[RevokeVoiceSpeakingdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return RevokeVoiceSpeaking200JSONResponse{Body: *out, Headers: RevokeVoiceSpeaking200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// DismissVoiceHand serves DELETE /api/v1/voice/rooms/{id}/participants/{account_id}/hand.
func (s *Server) DismissVoiceHand(ctx context.Context, req DismissVoiceHandRequestObject) (DismissVoiceHandResponseObject, error) {
	out, p, err := s.voiceRoomChange(ctx, func(r VoiceRooms, me string) (appvoice.RoomView, error) {
		return r.Dismiss(ctx, me, req.Id, knownID(req.AccountId))
	})
	if out == nil {
		return respond[DismissVoiceHanddefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return DismissVoiceHand200JSONResponse{Body: *out, Headers: DismissVoiceHand200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// MuteVoiceParticipant serves POST /api/v1/voice/rooms/{id}/participants/{account_id}/mute.
func (s *Server) MuteVoiceParticipant(ctx context.Context, req MuteVoiceParticipantRequestObject) (MuteVoiceParticipantResponseObject, error) {
	p, err := s.voiceOperation(ctx, func(r VoiceRooms, me string) error { return r.Mute(ctx, me, req.Id, knownID(req.AccountId)) })
	if p != nil || err != nil {
		return respond[MuteVoiceParticipantdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return MuteVoiceParticipant204Response{}, nil
}

// AssignVoiceCohost serves POST /api/v1/voice/rooms/{id}/participants/{account_id}/cohost.
func (s *Server) AssignVoiceCohost(ctx context.Context, req AssignVoiceCohostRequestObject) (AssignVoiceCohostResponseObject, error) {
	out, p, err := s.voiceRoomChange(ctx, func(r VoiceRooms, me string) (appvoice.RoomView, error) {
		return r.AssignCohost(ctx, me, req.Id, knownID(req.AccountId))
	})
	if out == nil {
		return respond[AssignVoiceCohostdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return AssignVoiceCohost200JSONResponse{Body: *out, Headers: AssignVoiceCohost200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// RemoveVoiceCohost serves DELETE /api/v1/voice/rooms/{id}/participants/{account_id}/cohost.
func (s *Server) RemoveVoiceCohost(ctx context.Context, req RemoveVoiceCohostRequestObject) (RemoveVoiceCohostResponseObject, error) {
	out, p, err := s.voiceRoomChange(ctx, func(r VoiceRooms, me string) (appvoice.RoomView, error) {
		return r.RemoveCohost(ctx, me, req.Id, knownID(req.AccountId))
	})
	if out == nil {
		return respond[RemoveVoiceCohostdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return RemoveVoiceCohost200JSONResponse{Body: *out, Headers: RemoveVoiceCohost200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// newVoiceInvitations presents invitations, reading the invited accounts together.
func (s *Server) newVoiceInvitations(ctx context.Context, invs []appvoice.Invitation) ([]VoiceRoomInvitation, error) {
	actors := make([]appaccount.Actor, 0, len(invs))
	kept := make([]appvoice.Invitation, 0, len(invs))
	for _, inv := range invs {
		a, err := s.o.Accounts.FindByID(ctx, inv.InviteeAccountID)
		if errors.Is(err, appaccount.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		actors = append(actors, a)
		kept = append(kept, inv)
	}
	accounts, err := s.newAccounts(ctx, actors)
	if err != nil {
		return nil, err
	}
	out := make([]VoiceRoomInvitation, len(kept))
	for i, inv := range kept {
		out[i] = VoiceRoomInvitation{
			Id: inv.ID, Account: accounts[i], Role: VoiceRole(inv.Role), InvitedById: inv.InvitedByAccountID, InvitedAt: inv.InvitedAt.UTC(),
		}
	}
	return out, nil
}

// ListVoiceInvitations serves GET /api/v1/voice/rooms/{id}/invitations.
func (s *Server) ListVoiceInvitations(ctx context.Context, req ListVoiceInvitationsRequestObject) (ListVoiceInvitationsResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[ListVoiceInvitationsdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	q := s.parsePageOf(ctx, req.Params.Cursor, req.Params.Limit, kernel.ValidID)
	if q.problem != nil {
		return ListVoiceInvitationsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	invs, more, err := rooms.Invitations(ctx, v.AccountID, req.Id, q.before, q.limit)
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[ListVoiceInvitationsdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	items, err := s.newVoiceInvitations(ctx, invs)
	if err != nil {
		return nil, err
	}
	out := VoiceRoomInvitationPage{Items: items}
	if more {
		out.NextCursor = encodeCursor(invs[len(invs)-1].ID)
	}
	return ListVoiceInvitations200JSONResponse{Body: out, Headers: ListVoiceInvitations200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// InviteToVoiceRoom serves POST /api/v1/voice/rooms/{id}/invitations.
func (s *Server) InviteToVoiceRoom(ctx context.Context, req InviteToVoiceRoomRequestObject) (InviteToVoiceRoomResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[InviteToVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	var role voice.Role
	if req.Body.Role != nil {
		role = voice.Role(*req.Body.Role)
	}
	inv, err := rooms.Invite(ctx, v.AccountID, req.Id, knownID(req.Body.AccountId), role)
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[InviteToVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	items, err := s.newVoiceInvitations(ctx, []appvoice.Invitation{inv})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		p := s.notFoundResponse(ctx)
		return InviteToVoiceRoomdefaultApplicationProblemPlusJSONResponse(p), nil
	}
	return InviteToVoiceRoom201JSONResponse{Body: items[0], Headers: InviteToVoiceRoom201ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// RevokeVoiceInvitation serves DELETE /api/v1/voice/rooms/{id}/invitations/{account_id}.
func (s *Server) RevokeVoiceInvitation(ctx context.Context, req RevokeVoiceInvitationRequestObject) (RevokeVoiceInvitationResponseObject, error) {
	p, err := s.voiceOperation(ctx, func(r VoiceRooms, me string) error {
		return r.RevokeInvitation(ctx, me, req.Id, knownID(req.AccountId))
	})
	if p != nil || err != nil {
		return respond[RevokeVoiceInvitationdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return RevokeVoiceInvitation204Response{}, nil
}
