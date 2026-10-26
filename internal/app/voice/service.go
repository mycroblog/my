// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The errors of the room use cases.
var (
	// ErrDisabled reports that the voice chat is off (ADR-0100 §2): the API answers 404.
	ErrDisabled = errors.New("voice: the voice chat is off")
	// ErrNotEligible reports an account that may not host a room (ADR-0100 §2).
	ErrNotEligible = errors.New("voice: the account may not host a room")
	// ErrNotHost reports an operation of the host by another participant.
	ErrNotHost = errors.New("voice: only the host may do this")
	// ErrRoomFull reports a room that holds as many people as the operator allows (ADR-0100 §5).
	ErrRoomFull = errors.New("voice: the room is full")
	// ErrTooManyRooms reports that as many rooms are open as the operator allows.
	ErrTooManyRooms = errors.New("voice: too many rooms are open")
	// ErrHostingElsewhere reports a host of an open room who joins or opens another room: the
	// host leaves it first, choosing the next host or ending it (ADR-0100 §5, §6).
	ErrHostingElsewhere = errors.New("voice: the account hosts another room")
	// ErrHostMustChoose reports a host who leaves a room with participants without choosing
	// the next host or ending it (ADR-0100 §6).
	ErrHostMustChoose = errors.New("voice: the host chooses the next host or ends the room")
	// ErrInvalidNextHost reports a next host who is not an eligible participant of the room.
	ErrInvalidNextHost = errors.New("voice: the next host is not an eligible participant")
	// ErrConflict reports a change that raced another one of the same account; trying again
	// succeeds.
	ErrConflict = errors.New("voice: the room changed meanwhile")
	// ErrTransportUnavailable reports a transport that did not give the grant of a session.
	ErrTransportUnavailable = errors.New("voice: the transport is unavailable")
)

// Member is what the rooms read of an account (ADR-0100 §2, §4).
type Member struct {
	// Local reports a local account of a user, not the instance actor.
	Local bool
	// Active reports an account that is neither suspended nor deleted.
	Active bool
	// Limited reports that a moderator limited the account.
	Limited bool
	// MayStart reports that the roles of the account's user grant start_voice_rooms.
	MayStart bool
}

// Members reads the accounts for the rooms from the account, moderation, identity, and
// instance modules; the composition root implements it.
type Members interface {
	// Member returns the facts of an account; an unknown account is the zero Member.
	Member(ctx context.Context, accountID string) (Member, error)
	// EveryoneStarts reports the instance setting voice_room_starters = everyone.
	EveryoneStarts(ctx context.Context) (bool, error)
}

// Relations reads the follows and blocks between accounts from the graph module; the
// composition root implements it. Follows count only when accepted.
type Relations interface {
	// Follows reports whether followerID follows followeeID.
	Follows(ctx context.Context, followerID, followeeID string) (bool, error)
	// Blocked reports a block, either way, between a and b.
	Blocked(ctx context.Context, a, b string) (bool, error)
	// Following returns which of ids accountID follows.
	Following(ctx context.Context, accountID string, ids []string) (map[string]bool, error)
	// BlockedAmong returns which of ids are in a block relation, either way, with accountID.
	BlockedAmong(ctx context.Context, accountID string, ids []string) (map[string]bool, error)
}

// Emojis reads the emoji of the reactions from the content module (ADR-0100 §10); the
// composition root implements it.
type Emojis interface {
	// ReactionEmoji returns the emoji that a local account reacts with: a Unicode emoji,
	// normalized, or the :shortcode: of an enabled custom emoji of this instance. Any other
	// fails with an error that wraps content.ErrInvalidReaction.
	ReactionEmoji(ctx context.Context, name string) (string, error)
}

// Config is how the operator runs the voice chat (ADR-0101 §1).
type Config struct {
	// Transport is the method of the voice setting, "" when it is off.
	Transport voice.Transport
	// Capacity bounds the people of a room, 0 for no limit.
	Capacity int
	// MaxOpenRooms bounds the rooms open at once, 0 for no limit.
	MaxOpenRooms int
	// MaxRoomDuration bounds how long a room stays open, 0 for no limit.
	MaxRoomDuration time.Duration
}

// Deps are the dependencies of the Service.
type Deps struct {
	Repository Repository
	UnitOfWork uow.UnitOfWork
	// Outbox enqueues JobApply in the transaction of a change.
	Outbox     queue.Outbox
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
	Heartbeats *Heartbeats
	// RateLimiter bounds the invitations (InvitationLimit); without one, the Service keeps the
	// counters in the KeyValueStore of Heartbeats.
	RateLimiter *kernel.RateLimiter
	// Transport carries the audio of the rooms of Config.Transport; nil when the voice chat
	// is off.
	Transport Transport
	Members   Members
	Relations Relations
	// Streams carries the signals of the room streams (ADR-0101 §3) to every process; nil
	// sends none.
	Streams stream.Publisher
	// Emojis checks the emoji of the reactions; nil allows the Unicode emoji only.
	Emojis Emojis
	Config Config
	Logger *slog.Logger
}

// Service runs the rooms (ADR-0100): opening them, their settings, joining and leaving them,
// the heartbeats of the participants, the hosts, the operations of the participants, the
// host, and the co-hosts, the invitations, and the ends of the rooms.
type Service struct {
	d Deps
	// snapshots shares the reads of a room among the room streams of this process, and
	// messages the messages for their sessions.
	snapshots *snapshots
	messages  *messages
}

// NewService returns the Service on d.
func NewService(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	if d.RateLimiter == nil && d.Heartbeats != nil {
		d.RateLimiter = kernel.NewRateLimiter(d.Heartbeats.kv, d.Clock)
	}
	return &Service{d: d, snapshots: newSnapshots(), messages: newMessages()}
}

// Enabled reports whether the voice chat runs.
func (s *Service) Enabled() bool { return s.d.Config.Transport != "" && s.d.Transport != nil }

func (s *Service) enabled() error {
	if !s.Enabled() {
		return ErrDisabled
	}
	return nil
}

func (s *Service) now() time.Time { return kernel.Normalize(s.d.Clock.Now()) }

// RoomView is a room as its viewers see it: the room, its head and current settings, and its
// participants in the order they joined.
type RoomView struct {
	RoomRecord
	Participants []Participant
}

// Joined is a session of a participant who joined or opened a room: the room, the new
// session, and the grant of the transport (ADR-0101 §6).
type Joined struct {
	Room    RoomView
	Session string
	Grant   Grant
}

// SettingsInput are the settings of a new room as the host gives them. Hashtags are the names,
// with or without the #.
type SettingsInput struct {
	Name        string
	Description string
	Hashtags    []string
	Visibility  voice.Visibility
	Speakers    voice.Speakers
}

// SettingsPatch changes the settings of a room; a nil field keeps its value.
type SettingsPatch struct {
	Name        *string
	Description *string
	Hashtags    *[]string
	Visibility  *voice.Visibility
	Speakers    *voice.Speakers
}

// settings returns the settings of in, the defaults for the empty visibility and speakers,
// validated.
func (in SettingsInput) settings() (voice.Settings, error) {
	s := voice.Settings{
		Name: in.Name, Description: normalizeLines(in.Description), Visibility: in.Visibility, Speakers: in.Speakers,
	}
	if s.Visibility == "" {
		s.Visibility = voice.DefaultVisibility
	}
	if s.Speakers == "" {
		s.Speakers = voice.DefaultSpeakers
	}
	tags, err := Hashtags(in.Hashtags)
	if err != nil {
		return voice.Settings{}, err
	}
	s.Hashtags = tags
	if err := s.Validate(Graphemes); err != nil {
		return voice.Settings{}, err
	}
	return s, nil
}

// apply returns cur changed by p, validated.
func (p SettingsPatch) apply(cur voice.Settings) (voice.Settings, error) {
	in := SettingsInput{Name: cur.Name, Description: cur.Description, Visibility: cur.Visibility, Speakers: cur.Speakers}
	for _, t := range cur.Hashtags {
		in.Hashtags = append(in.Hashtags, t.Name)
	}
	if p.Name != nil {
		in.Name = *p.Name
	}
	if p.Description != nil {
		in.Description = *p.Description
	}
	if p.Hashtags != nil {
		in.Hashtags = *p.Hashtags
	}
	if p.Visibility != nil {
		in.Visibility = *p.Visibility
	}
	if p.Speakers != nil {
		in.Speakers = *p.Speakers
	}
	if in.Visibility == "" || in.Speakers == "" {
		f := voice.FieldVisibility
		if in.Speakers == "" {
			f = voice.FieldSpeakers
		}
		return voice.Settings{}, &voice.SettingsError{Field: f}
	}
	return in.settings()
}

// normalizeLines turns the line breaks of s into "\n", as the text of a status (content.ParseText).
func normalizeLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// NormalizeTag returns the hashtag name under which the rooms are filtered.
func NormalizeTag(name string) string { return content.NormalizeTag(strings.TrimPrefix(name, "#")) }

// view reads a room and its participants; ErrNotFound for an unknown room.
func (s *Service) view(ctx context.Context, roomID string) (RoomView, error) {
	rec, err := s.d.Repository.FindRoom(ctx, nil, roomID)
	if err != nil {
		return RoomView{}, err
	}
	ps, err := s.d.Repository.Participants(ctx, nil, roomID)
	if err != nil {
		return RoomView{}, err
	}
	return RoomView{RoomRecord: rec, Participants: ps}, nil
}

// policyRoom is what the policy functions read of a room.
func policyRoom(v RoomView) voice.Room {
	s := v.Revision.Settings
	return voice.Room{Ended: v.Head.State == voice.StateEnded, HostAccountID: v.Head.HostAccountID, Visibility: s.Visibility, Speakers: s.Speakers}
}

// viewer returns the policy viewer of an account.
func (s *Service) viewer(ctx context.Context, accountID string) (voice.Viewer, Member, error) {
	m, err := s.d.Members.Member(ctx, accountID)
	if err != nil {
		return voice.Viewer{}, Member{}, err
	}
	return voice.Viewer{AccountID: accountID, Local: m.Local, Active: m.Active}, m, nil
}

// facts gathers the relations of viewer with a room for the policy functions (ADR-0100 §4).
func (s *Service) facts(ctx context.Context, v RoomView, viewer voice.Viewer, m Member) (voice.Facts, error) {
	f := voice.Facts{Limited: m.Limited}
	host := v.Head.HostAccountID
	if viewer.AccountID == "" || viewer.AccountID == host {
		return f, nil
	}
	others := []string{host}
	for _, p := range v.Participants {
		if p.AccountID == viewer.AccountID {
			f.Participant = true
			f.Cohost = p.Cohost()
		}
		if p.Cohost() && p.AccountID != host {
			others = append(others, p.AccountID)
		}
	}
	for _, id := range others {
		if id == viewer.AccountID {
			continue
		}
		blocked, err := s.d.Relations.Blocked(ctx, viewer.AccountID, id)
		if err != nil {
			return voice.Facts{}, err
		}
		if blocked {
			f.Blocked = true
			return f, nil
		}
	}
	banned, err := s.d.Repository.Banned(ctx, nil, v.ID, viewer.AccountID)
	if err != nil {
		return voice.Facts{}, err
	}
	f.Banned = banned
	inv, err := s.d.Repository.FindInvitation(ctx, nil, v.ID, viewer.AccountID)
	switch {
	case err == nil:
		f.Invited = !inv.Revoked
	case !errors.Is(err, ErrNotFound):
		return voice.Facts{}, err
	}
	if f.FollowsHost, err = s.d.Relations.Follows(ctx, viewer.AccountID, host); err != nil {
		return voice.Facts{}, err
	}
	if f.FollowedByHost, err = s.d.Relations.Follows(ctx, host, viewer.AccountID); err != nil {
		return voice.Facts{}, err
	}
	if f.SpeakGranted, err = s.d.Repository.SpeakGranted(ctx, nil, v.ID, viewer.AccountID); err != nil {
		return voice.Facts{}, err
	}
	return f, nil
}

// visible reports whether an account sees a room.
func (s *Service) visible(ctx context.Context, v RoomView, accountID string) (bool, error) {
	viewer, m, err := s.viewer(ctx, accountID)
	if err != nil {
		return false, err
	}
	f, err := s.facts(ctx, v, viewer, m)
	if err != nil {
		return false, err
	}
	return voice.CanSee(policyRoom(v), viewer, f), nil
}

// canHost reports whether an account may host a room now (ADR-0100 §2).
func (s *Service) canHost(ctx context.Context, accountID string) (bool, error) {
	m, err := s.d.Members.Member(ctx, accountID)
	if err != nil {
		return false, err
	}
	everyone, err := s.d.Members.EveryoneStarts(ctx)
	if err != nil {
		return false, err
	}
	return voice.CanHost(voice.HostFacts{Local: m.Local, Active: m.Active, Limited: m.Limited, EveryoneStarts: everyone, MayStart: m.MayStart}), nil
}

// grant asks the transport for the grant of a session, outside any transaction.
func (s *Service) grant(ctx context.Context, roomID string, session Session, canSpeak bool) (Grant, error) {
	g, err := s.d.Transport.Grant(ctx, RoomRef{ID: roomID}, session, canSpeak)
	if err != nil {
		s.d.Logger.WarnContext(ctx, "the voice transport gave no grant", slog.String("room", roomID), slog.Any("error", err))
		return Grant{}, errors.Join(ErrTransportUnavailable, err)
	}
	return g, nil
}
