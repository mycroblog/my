// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	appvoice "github.com/mycroblog/my/internal/app/voice"
)

// The connections of the voice chat rooms (ADR-0101 §3, §4, §6): the room stream, the P2P
// signals, the microphone, the reactions, and the grants of the sessions.

// VoiceConnections are the use cases of the connections of a room: *appvoice.Service
// implements them.
type VoiceConnections interface {
	OpenStream(ctx context.Context, accountID, roomID, session string) (appvoice.RoomStream, error)
	StreamCurrent(ctx context.Context, st appvoice.RoomStream) (bool, error)
	RenewStream(ctx context.Context, st appvoice.RoomStream) (bool, error)
	StreamState(ctx context.Context, st appvoice.RoomStream, notBefore time.Time) (appvoice.StreamState, error)
	Message(ctx context.Context, st appvoice.RoomStream, id string) (appvoice.Message, bool, error)
	Blocked(ctx context.Context, a, b string) (bool, error)
	Signal(ctx context.Context, accountID, roomID string, in appvoice.SignalInput) error
	Mic(ctx context.Context, accountID, roomID, session string, on bool) error
	React(ctx context.Context, accountID, roomID, emoji string) error
	RefreshGrant(ctx context.Context, accountID, roomID, session string) (appvoice.Grant, error)
}

const (
	// DefaultVoiceStreamHeartbeat is how often a room stream sends a comment line and checks
	// its access token and its session again (ADR-0101 §3).
	DefaultVoiceStreamHeartbeat = 15 * time.Second
	// voiceStateInterval is the least time between two state events of a room stream.
	voiceStateInterval = time.Second
	// voiceRecentMessages is the number of message IDs a room stream remembers having sent.
	voiceRecentMessages = 256
	streamKindVoice     = "voice"
)

// voiceStreamPattern is the http.ServeMux pattern of the room stream.
const voiceStreamPattern = "GET " + BasePath + "/voice/rooms/{id}/stream"

// registerVoiceStream registers GET /voice/rooms/{id}/stream (operation streamVoiceRoom), which
// the strict server does not generate (ADR-0029), behind the same gate and guard as the
// generated voice chat operations.
func (s *Server) registerVoiceStream(r Router) {
	r.Handle(voiceStreamPattern, s.voiceGate(s.guard(voiceStreamPattern, http.HandlerFunc(s.serveVoiceStream))))
}

// writeProblemOf writes p.
func (s *Server) writeProblemOf(w http.ResponseWriter, r *http.Request, p problem) {
	detail := ""
	if p.Body.Detail != nil {
		detail = *p.Body.Detail
	}
	if p.Headers.RetryAfter != nil {
		w.Header().Set("Retry-After", strconv.Itoa(*p.Headers.RetryAfter))
	}
	s.writeProblem(w, r, p.StatusCode, p.Body.Type, p.Body.Title, detail)
}

// serveVoiceStream serves GET /api/v1/voice/rooms/{id}/stream: the Server-Sent Events of a
// room for the viewer's current live session (ADR-0101 §3). It subscribes to the signals of the
// room, reads the room again for each voice_room signal, at most once per voiceStateInterval,
// and sends the events that are for the session. It ends after removed, replaced, or ended,
// when the client goes, the subscription ends, a write times out, the access token is no
// longer valid, or another stream of the session opened, which every heartbeat checks again.
func (s *Server) serveVoiceStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v, ok := ViewerFrom(ctx)
	if !ok {
		s.responseError(w, r, errNoViewer)
		return
	}
	rooms := s.o.Voice
	roomID := r.PathValue("id")
	st, err := rooms.OpenStream(ctx, v.AccountID, roomID, r.URL.Query().Get("session"))
	if err != nil {
		if p, ok := s.voiceProblem(ctx, err); ok {
			s.writeProblemOf(w, r, p)
		} else {
			s.responseError(w, r, err)
		}
		return
	}
	if s.o.VoiceStreams == nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, ProblemTypeBlank, s.localizer(r).ServerErrorsStreamingUnavailable(), "")
		return
	}
	sub, err := s.o.VoiceStreams.SubscribeRoom(ctx, roomID, st.Session)
	switch {
	case errors.Is(err, stream.ErrTooManySubscriptions), errors.Is(err, stream.ErrClosed), errors.Is(err, stream.ErrUnavailable):
		if !errors.Is(err, stream.ErrClosed) {
			s.o.Logger.WarnContext(ctx, "voice room stream refused", slog.Any("error", err))
		}
		w.Header().Set("Retry-After", strconv.Itoa(streamRetryAfter))
		s.writeProblem(w, r, http.StatusServiceUnavailable, ProblemTypeBlank, s.localizer(r).ServerErrorsStreamingUnavailable(), "")
		return
	case ctx.Err() != nil:
		return
	case err != nil:
		s.responseError(w, r, err)
		return
	}
	defer sub.Close()

	vs := &voiceStream{
		streamState: streamState{s: s, w: w, rc: http.NewResponseController(w), viewer: v},
		rooms:       rooms, st: st, sentMessages: newRecentIDs(voiceRecentMessages), renewed: time.Now(),
	}
	token, _, _ := bearerToken(r.Header)
	if err := vs.rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		s.responseError(w, r, err)
		return
	}
	kind := kernel.NewLabels(labelStreamKind, streamKindVoice)
	s.streams.Add(ctx, 1, kind)
	defer s.streams.Add(context.WithoutCancel(ctx), -1, kind)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", cacheNoStore)
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if vs.write(": connected\n\n") != nil {
		return
	}
	if done, err := vs.state(ctx, time.Now()); done || err != nil {
		vs.fail(ctx, err)
		return
	}

	heartbeat := s.o.VoiceStreamHeartbeat
	if heartbeat <= 0 {
		heartbeat = DefaultVoiceStreamHeartbeat
	}
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	// pending holds back a state event that would come sooner than voiceStateInterval after
	// the last one; noticed is when the newest voice_room signal arrived.
	pending := time.NewTimer(0)
	<-pending.C
	waiting := false
	var noticed time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Done():
			return
		case <-ticker.C:
			if _, err := s.o.Authenticator.AuthenticateBearer(ctx, token); err != nil {
				return
			}
			if s.streamClosed(ctx) {
				return
			}
			current, err := rooms.StreamCurrent(ctx, st)
			if err == nil && current && time.Since(vs.renewed) >= appvoice.StreamClaimRenewal {
				current, err = rooms.RenewStream(ctx, st)
				vs.renewed = time.Now()
			}
			if err != nil || !current {
				return // another stream of the session opened
			}
			if done, err := vs.state(ctx, time.Now()); done || err != nil {
				vs.fail(ctx, err)
				return
			}
			if vs.write(": heartbeat\n\n") != nil {
				return
			}
		case <-pending.C:
			waiting = false
			if done, err := vs.state(ctx, noticed); done || err != nil {
				vs.fail(ctx, err)
				return
			}
		case e := <-sub.Events():
			if e.Kind == stream.KindVoiceRoom {
				noticed = time.Now()
				if wait := voiceStateInterval - time.Since(vs.lastState); wait > 0 {
					if !waiting {
						waiting = true
						pending.Reset(wait)
					}
					continue
				}
				if done, err := vs.state(ctx, noticed); done || err != nil {
					vs.fail(ctx, err)
					return
				}
				continue
			}
			if err := vs.handle(ctx, e); err != nil {
				vs.fail(ctx, err)
				return
			}
		}
	}
}

// voiceStream is one open room stream.
type voiceStream struct {
	streamState
	rooms VoiceRooms
	st    appvoice.RoomStream
	// lastState is when the last state event was sent, and blocked the participants in a
	// block relation with the viewer then.
	lastState time.Time
	blocked   map[string]bool
	listed    map[string]bool
	// sentMessages holds the IDs of the messages recently sent, which a cross-process broker
	// may deliver twice; renewed is when the claim of the stream on its session was renewed.
	sentMessages *recentIDs
	renewed      time.Time
}

// fail logs an error that ended the stream, other than a failed write.
func (vs *voiceStream) fail(ctx context.Context, err error) {
	if err != nil && !errors.Is(err, errStreamWrite) && ctx.Err() == nil {
		vs.s.o.Logger.ErrorContext(ctx, "voice room stream", slog.String("room", vs.st.RoomID), slog.Any("error", err))
	}
}

// state reads the room from a read started no earlier than notBefore and sends it (state), or
// the end of the stream (removed, replaced, ended), and then reports done.
func (vs *voiceStream) state(ctx context.Context, notBefore time.Time) (done bool, err error) {
	state, err := vs.rooms.StreamState(ctx, vs.st, notBefore)
	if err != nil {
		return false, err
	}
	vs.lastState = time.Now()
	if state.End != "" {
		return true, vs.event(string(state.End), VoiceStreamEnd{Reason: state.Reason})
	}
	rooms, err := vs.s.newVoiceRoomsMarked(ctx, []appvoice.RoomView{state.Room}, state.Blocked)
	if err != nil {
		return false, err
	}
	room := rooms[0]
	vs.blocked, vs.listed = state.Blocked, map[string]bool{}
	for i, p := range room.Participants {
		vs.listed[p.Account.Id] = true
		room.Participants[i].Blocked = state.Blocked[p.Account.Id]
		if session, ok := state.Sessions[p.Account.Id]; ok {
			room.Participants[i].Session = &session
		}
	}
	return false, vs.event("state", room)
}

// handle sends what a signal of the room means for the session.
func (vs *voiceStream) handle(ctx context.Context, e stream.Event) error {
	switch e.Kind {
	case stream.KindVoiceMic:
		return vs.event("mic", VoiceStreamMic{Participant: e.ParticipantID, On: e.On})
	case stream.KindVoiceReaction:
		blocked := vs.blocked[e.ParticipantID]
		if !vs.listed[e.ParticipantID] {
			// A participant who joined after the last state.
			var err error
			if blocked, err = vs.rooms.Blocked(ctx, vs.viewer.AccountID, e.ParticipantID); err != nil {
				return err
			}
		}
		if blocked {
			return nil
		}
		return vs.event("reaction", VoiceStreamReaction{Participant: e.ParticipantID, Emoji: e.Emoji})
	case stream.KindVoiceMute:
		if e.SessionID != vs.st.Session {
			return nil
		}
		return vs.event("muted", VoiceStreamMuted{By: e.By})
	case stream.KindVoiceSignal:
		if e.To != vs.st.Session || vs.sentMessages.has(e.MessageID) {
			return nil
		}
		m, ok, err := vs.rooms.Message(ctx, vs.st, e.MessageID)
		if err != nil || !ok {
			return err
		}
		vs.sentMessages.add(e.MessageID)
		switch {
		case m.Signal != nil:
			sig := VoiceStreamSignal{From: m.Signal.From, Kind: VoiceSignalKind(m.Signal.Kind)}
			if m.Signal.SDP != "" {
				sig.Sdp = &m.Signal.SDP
			}
			if m.Signal.Candidate != "" {
				sig.Candidate = &m.Signal.Candidate
			}
			return vs.event("signal", sig)
		case m.Grant != nil:
			return vs.event("grant", VoiceStreamGrant{Grant: newVoiceGrant(m.Grant.Grant), Reconnect: m.Grant.Reconnect})
		}
	}
	return nil
}

// voiceConnectionProblem maps the errors of the connections of a room; it reports false for any
// other error.
func (s *Server) voiceConnectionProblem(ctx context.Context, err error) (problem, bool) {
	var se *appvoice.SignalError
	switch {
	case errors.As(err, &se):
		return s.unprocessable(ctx, se.Field), true
	case errors.Is(err, appvoice.ErrInvalidEmoji):
		return s.unprocessable(ctx, "emoji"), true
	}
	return problem{}, false
}

// SendVoiceSignal serves POST /api/v1/voice/rooms/{id}/signals.
func (s *Server) SendVoiceSignal(ctx context.Context, req SendVoiceSignalRequestObject) (SendVoiceSignalResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[SendVoiceSignaldefaultApplicationProblemPlusJSONResponse](p, err)
	}
	b := req.Body
	in := appvoice.SignalInput{Session: b.Session, To: b.To, Kind: appvoice.SignalKind(b.Kind), SDP: deref(b.Sdp), Candidate: deref(b.Candidate)}
	if err := rooms.Signal(ctx, v.AccountID, req.Id, in); err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[SendVoiceSignaldefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return SendVoiceSignal204Response{}, nil
}

// SetVoiceMic serves POST /api/v1/voice/rooms/{id}/mic.
func (s *Server) SetVoiceMic(ctx context.Context, req SetVoiceMicRequestObject) (SetVoiceMicResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[SetVoiceMicdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	if err := rooms.Mic(ctx, v.AccountID, req.Id, req.Body.Session, req.Body.On); err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[SetVoiceMicdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return SetVoiceMic204Response{}, nil
}

// ReactInVoiceRoom serves POST /api/v1/voice/rooms/{id}/reactions.
func (s *Server) ReactInVoiceRoom(ctx context.Context, req ReactInVoiceRoomRequestObject) (ReactInVoiceRoomResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[ReactInVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	if err := rooms.React(ctx, v.AccountID, req.Id, req.Body.Emoji); err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[ReactInVoiceRoomdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return ReactInVoiceRoom204Response{}, nil
}

// RefreshVoiceGrant serves POST /api/v1/voice/rooms/{id}/grant.
func (s *Server) RefreshVoiceGrant(ctx context.Context, req RefreshVoiceGrantRequestObject) (RefreshVoiceGrantResponseObject, error) {
	rooms, v, p, err := s.voiceRooms(ctx)
	if p != nil || err != nil {
		return respond[RefreshVoiceGrantdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	g, err := rooms.RefreshGrant(ctx, v.AccountID, req.Id, req.Body.Session)
	if err != nil {
		p, err := s.voiceFail(ctx, err)
		return respond[RefreshVoiceGrantdefaultApplicationProblemPlusJSONResponse](p, err)
	}
	return RefreshVoiceGrant200JSONResponse{Body: newVoiceGrant(g), Headers: RefreshVoiceGrant200ResponseHeaders{CacheControl: voiceCache()}}, nil
}
