// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The voice chat rooms (ADR-0100 §14, ADR-0101 §6; tag voice, scopes read:voice and
// write:voice): the list and the rooms, opening, changing, and ending them, joining and leaving,
// the heartbeats of the sessions, and handing a room over. Every operation answers 404 while the
// voice chat is off.

// The problem types of the voice chat rooms.
const (
	ProblemTypeVoiceNotEligible      = "/api/v1/problems/voice-not-eligible"
	ProblemTypeVoiceNotHost          = "/api/v1/problems/voice-not-host"
	ProblemTypeVoiceRoomFull         = "/api/v1/problems/voice-room-full"
	ProblemTypeVoiceTooManyRooms     = "/api/v1/problems/voice-too-many-rooms"
	ProblemTypeVoiceHostingElsewhere = "/api/v1/problems/voice-hosting-elsewhere"
	ProblemTypeVoiceHostMustChoose   = "/api/v1/problems/voice-host-must-choose"
	ProblemTypeVoiceInvalidNextHost  = "/api/v1/problems/voice-invalid-next-host"
	ProblemTypeVoiceConflict         = "/api/v1/problems/voice-conflict"
	ProblemTypeVoiceUnavailable      = "/api/v1/problems/voice-unavailable"
)

// VoiceRooms runs the voice chat rooms: *appvoice.Service implements it.
type VoiceRooms interface {
	Open(ctx context.Context, accountID string, in appvoice.SettingsInput) (appvoice.Joined, error)
	Get(ctx context.Context, accountID, roomID string) (appvoice.RoomView, error)
	List(ctx context.Context, accountID string, q appvoice.ListQuery) ([]appvoice.ListedRoom, bool, error)
	Update(ctx context.Context, accountID, roomID string, p appvoice.SettingsPatch) (appvoice.RoomView, error)
	End(ctx context.Context, accountID, roomID string) error
	Join(ctx context.Context, accountID, roomID string) (appvoice.Joined, error)
	Leave(ctx context.Context, accountID, roomID string, in appvoice.LeaveInput) error
	Heartbeat(ctx context.Context, accountID, roomID, session string) error
	HandOver(ctx context.Context, byAccountID, roomID, accountID string) (appvoice.RoomView, error)
}

// voiceRooms returns the rooms and the viewer, or the 404 of a server whose voice chat is off.
func (s *Server) voiceRooms(ctx context.Context) (VoiceRooms, Viewer, *problem, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, Viewer{}, nil, err
	}
	if s.o.Voice == nil {
		p := s.notFoundResponse(ctx)
		return nil, Viewer{}, &p, nil
	}
	return s.o.Voice, v, nil, nil
}

// voiceProblem maps the errors of the room use cases; it reports false for any other error.
func (s *Server) voiceProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	var se *voice.SettingsError
	switch {
	case errors.Is(err, appvoice.ErrNotFound), errors.Is(err, appvoice.ErrDisabled):
		return s.notFoundResponse(ctx), true
	case errors.As(err, &se):
		return s.unprocessable(ctx, string(se.Field)), true
	case errors.Is(err, appvoice.ErrNotEligible):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeVoiceNotEligible, l.ServerErrorsVoiceNotEligible(), ""), true
	case errors.Is(err, appvoice.ErrNotHost):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeVoiceNotHost, l.ServerErrorsVoiceNotHost(), ""), true
	case errors.Is(err, appvoice.ErrRoomFull):
		return s.problem(ctx, http.StatusConflict, ProblemTypeVoiceRoomFull, l.ServerErrorsVoiceRoomFull(), ""), true
	case errors.Is(err, appvoice.ErrTooManyRooms):
		return s.problem(ctx, http.StatusConflict, ProblemTypeVoiceTooManyRooms, l.ServerErrorsVoiceTooManyRooms(), ""), true
	case errors.Is(err, appvoice.ErrHostingElsewhere):
		return s.problem(ctx, http.StatusConflict, ProblemTypeVoiceHostingElsewhere, l.ServerErrorsVoiceHostingElsewhere(),
			l.ServerErrorsVoiceHostingElsewhereDetail()), true
	case errors.Is(err, appvoice.ErrHostMustChoose):
		return s.problem(ctx, http.StatusConflict, ProblemTypeVoiceHostMustChoose, l.ServerErrorsVoiceHostMustChoose(), ""), true
	case errors.Is(err, appvoice.ErrInvalidNextHost):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeVoiceInvalidNextHost, l.ServerErrorsVoiceInvalidNextHost(), ""), true
	case errors.Is(err, appvoice.ErrConflict):
		return s.problem(ctx, http.StatusConflict, ProblemTypeVoiceConflict, l.ServerErrorsVoiceConflict(), ""), true
	case errors.Is(err, appvoice.ErrTransportUnavailable):
		return s.problem(ctx, http.StatusServiceUnavailable, ProblemTypeVoiceUnavailable, l.ServerErrorsVoiceUnavailable(), ""), true
	}
	return problem{}, false
}

// voiceFail returns the problem of err, or err itself for an unexpected error.
func (s *Server) voiceFail(ctx context.Context, err error) (*problem, error) {
	if p, ok := s.voiceProblem(ctx, err); ok {
		return &p, nil
	}
	return nil, err
}

// newVoiceRooms presents rooms, reading the accounts of their hosts and participants together.
func (s *Server) newVoiceRooms(ctx context.Context, views []appvoice.RoomView) ([]VoiceRoom, error) {
	ids := map[string]bool{}
	var actors []appaccount.Actor
	read := func(id string) error {
		if ids[id] {
			return nil
		}
		ids[id] = true
		a, err := s.o.Accounts.FindByID(ctx, id)
		if errors.Is(err, appaccount.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		actors = append(actors, a)
		return nil
	}
	for _, v := range views {
		if err := read(v.Head.HostAccountID); err != nil {
			return nil, err
		}
		for _, p := range v.Participants {
			if err := read(p.AccountID); err != nil {
				return nil, err
			}
		}
	}
	accounts, err := s.newAccounts(ctx, actors)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Account, len(accounts))
	for i, a := range actors {
		byID[a.Account.ID] = accounts[i]
	}
	out := make([]VoiceRoom, 0, len(views))
	for _, v := range views {
		st := v.Revision.Settings
		r := VoiceRoom{
			Id: v.ID, Name: st.Name, Description: st.Description, Hashtags: make([]string, 0, len(st.Hashtags)),
			Visibility: VoiceRoomVisibility(st.Visibility), Speakers: VoiceRoomSpeakers(st.Speakers),
			Host: byID[v.Head.HostAccountID], OpenedAt: v.OpenedAt.UTC(), ParticipantsCount: len(v.Participants),
			Participants: make([]VoiceRoomParticipant, 0, len(v.Participants)),
		}
		for _, t := range st.Hashtags {
			r.Hashtags = append(r.Hashtags, t.Name)
		}
		for _, p := range v.Participants {
			a, ok := byID[p.AccountID]
			if !ok {
				continue
			}
			host := p.AccountID == v.Head.HostAccountID
			r.Participants = append(r.Participants, VoiceRoomParticipant{
				Account: a, Role: VoiceRole(p.Role), Host: host, Cohost: p.Cohost() && !host, HandRaised: p.HandRaised,
			})
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Server) newVoiceRoom(ctx context.Context, v appvoice.RoomView) (VoiceRoom, error) {
	rooms, err := s.newVoiceRooms(ctx, []appvoice.RoomView{v})
	if err != nil {
		return VoiceRoom{}, err
	}
	return rooms[0], nil
}

func (s *Server) newVoiceRoomJoin(ctx context.Context, j appvoice.Joined) (VoiceRoomJoin, error) {
	room, err := s.newVoiceRoom(ctx, j.Room)
	if err != nil {
		return VoiceRoomJoin{}, err
	}
	g := VoiceGrant{
		Kind: j.Grant.Kind, Options: map[string]string{}, IceServers: make([]VoiceIceServer, 0, len(j.Grant.ICE)),
		RelayOnly: j.Grant.RelayOnly, ExpiresAt: j.Grant.ExpiresAt.UTC(),
	}
	for k, v := range j.Grant.Options {
		g.Options[k] = v
	}
	if j.Grant.URL != "" {
		g.Url = &j.Grant.URL
	}
	if j.Grant.Token != "" {
		g.Token = &j.Grant.Token
	}
	for _, ice := range j.Grant.ICE {
		srv := VoiceIceServer{Urls: append([]string{}, ice.URLs...)}
		if ice.Username != "" {
			srv.Username = &ice.Username
		}
		if ice.Credential != "" {
			srv.Credential = &ice.Credential
		}
		g.IceServers = append(g.IceServers, srv)
	}
	return VoiceRoomJoin{Room: room, Session: j.Session, Grant: g}, nil
}

// encodeVoiceCursor writes the position of a room in the list as a cursor.
func encodeVoiceCursor(k appvoice.ListKey) *Cursor {
	f := "0"
	if k.Followed {
		f = "1"
	}
	return encodeCursor(f + ":" + strconv.Itoa(k.Participants) + ":" + k.RoomID)
}

// decodeVoiceCursor reads the position that encodeVoiceCursor wrote.
func decodeVoiceCursor(s string) (appvoice.ListKey, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 || (parts[0] != "0" && parts[0] != "1") || !kernel.ValidID(parts[2]) {
		return appvoice.ListKey{}, false
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 0 {
		return appvoice.ListKey{}, false
	}
	return appvoice.ListKey{Followed: parts[0] == "1", Participants: n, RoomID: parts[2]}, true
}

func voiceCache() *string {
	c := cacheNoStore
	return &c
}

// ListVoiceRooms serves GET /api/v1/voice/rooms.
func (s *Server) ListVoiceRooms(ctx context.Context, req ListVoiceRoomsRequestObject) (ListVoiceRoomsResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[ListVoiceRoomsdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	q := s.parsePageOf(ctx, req.Params.Cursor, req.Params.Limit, func(c string) bool { _, ok := decodeVoiceCursor(c); return ok })
	if q.problem != nil {
		return ListVoiceRoomsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	lq := appvoice.ListQuery{Tag: deref(req.Params.Tag), Limit: q.limit}
	if q.before != "" {
		k, _ := decodeVoiceCursor(q.before)
		lq.After = &k
	}
	listed, more, err := rooms.List(ctx, v.AccountID, lq)
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[ListVoiceRoomsdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	views := make([]appvoice.RoomView, len(listed))
	for i, r := range listed {
		views[i] = r.RoomView
	}
	items, err := s.newVoiceRooms(ctx, views)
	if err != nil {
		return nil, err
	}
	out := VoiceRoomPage{Items: items}
	if more {
		out.NextCursor = encodeVoiceCursor(listed[len(listed)-1].Key)
	}
	return ListVoiceRooms200JSONResponse{Body: out, Headers: ListVoiceRooms200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// OpenVoiceRoom serves POST /api/v1/voice/rooms.
func (s *Server) OpenVoiceRoom(ctx context.Context, req OpenVoiceRoomRequestObject) (OpenVoiceRoomResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[OpenVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	b := req.Body
	in := appvoice.SettingsInput{Name: b.Name, Description: deref(b.Description)}
	if b.Hashtags != nil {
		in.Hashtags = *b.Hashtags
	}
	if b.Visibility != nil {
		in.Visibility = voice.Visibility(*b.Visibility)
	}
	if b.Speakers != nil {
		in.Speakers = voice.Speakers(*b.Speakers)
	}
	j, err := rooms.Open(ctx, v.AccountID, in)
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[OpenVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newVoiceRoomJoin(ctx, j)
	if err != nil {
		return nil, err
	}
	return OpenVoiceRoom201JSONResponse{Body: out, Headers: OpenVoiceRoom201ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// respond returns the default response of p, or err.
func respond[T ~struct {
	Body       Problem
	Headers    ProblemResponseHeaders
	StatusCode int
}](p *problem, err error) (T, error) {
	if err != nil {
		var zero T
		return zero, err
	}
	return T(*p), nil
}

// GetVoiceRoom serves GET /api/v1/voice/rooms/{id}.
func (s *Server) GetVoiceRoom(ctx context.Context, req GetVoiceRoomRequestObject) (GetVoiceRoomResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[GetVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	room, err := rooms.Get(ctx, v.AccountID, req.Id)
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[GetVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newVoiceRoom(ctx, room)
	if err != nil {
		return nil, err
	}
	return GetVoiceRoom200JSONResponse{Body: out, Headers: GetVoiceRoom200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// UpdateVoiceRoom serves PATCH /api/v1/voice/rooms/{id}.
func (s *Server) UpdateVoiceRoom(ctx context.Context, req UpdateVoiceRoomRequestObject) (UpdateVoiceRoomResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[UpdateVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	b := req.Body
	patch := appvoice.SettingsPatch{Name: b.Name, Description: b.Description, Hashtags: b.Hashtags}
	if b.Visibility != nil {
		vis := voice.Visibility(*b.Visibility)
		patch.Visibility = &vis
	}
	if b.Speakers != nil {
		sp := voice.Speakers(*b.Speakers)
		patch.Speakers = &sp
	}
	room, err := rooms.Update(ctx, v.AccountID, req.Id, patch)
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[UpdateVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newVoiceRoom(ctx, room)
	if err != nil {
		return nil, err
	}
	return UpdateVoiceRoom200JSONResponse{Body: out, Headers: UpdateVoiceRoom200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// EndVoiceRoom serves DELETE /api/v1/voice/rooms/{id}.
func (s *Server) EndVoiceRoom(ctx context.Context, req EndVoiceRoomRequestObject) (EndVoiceRoomResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[EndVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	if err := rooms.End(ctx, v.AccountID, req.Id); err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[EndVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return EndVoiceRoom204Response{}, nil
}

// JoinVoiceRoom serves POST /api/v1/voice/rooms/{id}/participation.
func (s *Server) JoinVoiceRoom(ctx context.Context, req JoinVoiceRoomRequestObject) (JoinVoiceRoomResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[JoinVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	j, err := rooms.Join(ctx, v.AccountID, req.Id)
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[JoinVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newVoiceRoomJoin(ctx, j)
	if err != nil {
		return nil, err
	}
	return JoinVoiceRoom200JSONResponse{Body: out, Headers: JoinVoiceRoom200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// LeaveVoiceRoom serves DELETE /api/v1/voice/rooms/{id}/participation.
func (s *Server) LeaveVoiceRoom(ctx context.Context, req LeaveVoiceRoomRequestObject) (LeaveVoiceRoomResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[LeaveVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	in := appvoice.LeaveInput{NextHostID: deref(knownIDPtr(req.Params.NextHostId)), End: req.Params.End != nil && *req.Params.End}
	if err := rooms.Leave(ctx, v.AccountID, req.Id, in); err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[LeaveVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return LeaveVoiceRoom204Response{}, nil
}

// SendVoiceHeartbeat serves POST /api/v1/voice/rooms/{id}/heartbeat.
func (s *Server) SendVoiceHeartbeat(ctx context.Context, req SendVoiceHeartbeatRequestObject) (SendVoiceHeartbeatResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[SendVoiceHeartbeatdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	if err := rooms.Heartbeat(ctx, v.AccountID, req.Id, req.Body.Session); err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[SendVoiceHeartbeatdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return SendVoiceHeartbeat204Response{}, nil
}

// HandOverVoiceRoom serves POST /api/v1/voice/rooms/{id}/host.
func (s *Server) HandOverVoiceRoom(ctx context.Context, req HandOverVoiceRoomRequestObject) (HandOverVoiceRoomResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[HandOverVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	room, err := rooms.HandOver(ctx, v.AccountID, req.Id, knownID(req.Body.AccountId))
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[HandOverVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	out, err := s.newVoiceRoom(ctx, room)
	if err != nil {
		return nil, err
	}
	return HandOverVoiceRoom200JSONResponse{Body: out, Headers: HandOverVoiceRoom200ResponseHeaders{CacheControl: voiceCache()}}, nil
}

// isVoicePattern reports whether the http.ServeMux pattern of an operation is a voice chat one.
func isVoicePattern(pattern string) bool {
	_, path, _ := strings.Cut(pattern, " ")
	return strings.HasPrefix(path, BasePath+"/voice/")
}

// voiceGate answers 404 to every voice chat operation while the voice chat is off, before the
// guard looks at the access token (ADR-0100 §2): a server without voice chat has no such API.
func (s *Server) voiceGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.o.Voice == nil {
			s.writeProblem(w, r, http.StatusNotFound, ProblemTypeBlank, s.localizer(r).ServerErrorsNotFound(), "")
			return
		}
		next.ServeHTTP(w, r)
	})
}
