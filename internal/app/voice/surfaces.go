// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The places that tell of the rooms (ADR-0100 §8, §9, §13): the notifications of a room's
// opening and of an invitation, the start post, the page of a room for the visitors who do not
// see it, the entry on the profiles of its host and speakers, and the history of the hosts.

// Notifier creates the notifications of the rooms inside their transactions (ADR-0100 §8).
// *appnotification.VoiceNotifier implements it; the notification module applies the
// exclusions of the other notifications from accounts (mutes, blocks).
type Notifier interface {
	// RoomStartedTx tells recipientID that hostID opened the room roomID, once per room and
	// recipient (voice_room_started).
	RoomStartedTx(ctx context.Context, tx uow.Tx, roomID, hostID, recipientID string, at time.Time) error
	// RoomInvitedTx tells inviteeID that inviterID invited it to the room roomID, once per
	// invitation (voice_room_invited).
	RoomInvitedTx(ctx context.Context, tx uow.Tx, invitationID, roomID, inviterID, inviteeID string, at time.Time) error
}

// Posts writes the start posts of the rooms with the content module's create operation
// (ADR-0100 §9): *appcontent.Service implements it.
type Posts interface {
	PrepareCreate(ctx context.Context, in appcontent.CreateInput) (appcontent.PreparedCreate, error)
	CreateTx(ctx context.Context, tx uow.Tx, p appcontent.PreparedCreate) (appcontent.Post, error)
}

// ErrNoStartPost reports a start post of a room whose visibility has none: a room for the
// mutuals or by invitation (ADR-0100 §9), or a server that writes no posts.
var ErrNoStartPost = errors.New("voice: the room has no start post")

// StartPost is the post that the host writes when opening a room (ADR-0100 §9).
type StartPost struct {
	// Text is the body that the host wrote, which may be empty; the link of the room is
	// appended to it.
	Text string
	// Language is a BCP 47 tag; "" means the host's default post language.
	Language string
}

// RoomURL is the page of a room on host (ADR-0100 §9).
func RoomURL(host, roomID string) string { return "https://" + host + "/voice/" + roomID }

// startPostText returns the body of a start post: the text of the host with the link of the
// room appended on a line of its own.
func startPostText(text, link string) string {
	text = strings.TrimSpace(normalizeLines(text))
	if text == "" {
		return link
	}
	return text + "\n\n" + link
}

// prepareStartPost checks and builds the start post of a room before the transaction of Open.
func (s *Service) prepareStartPost(ctx context.Context, accountID, roomID string, settings voice.Settings, p *StartPost) (*appcontent.PreparedCreate, error) {
	if p == nil {
		return nil, nil
	}
	if s.d.Posts == nil {
		return nil, ErrNoStartPost
	}
	in := appcontent.CreateInput{
		AuthorAccountID: accountID,
		TextInput:       appcontent.TextInput{Text: startPostText(p.Text, RoomURL(s.d.Host, roomID)), Language: p.Language},
	}
	// The post is local for a room of the local users and private for a room of the host's
	// followers; a room for the mutuals or by invitation has no start post (ADR-0100 §9).
	switch settings.Visibility {
	case voice.VisibilityLocal:
		in.Visibility = "local"
	case voice.VisibilityFollowers:
		in.Visibility = "private"
	default:
		return nil, ErrNoStartPost
	}
	prepared, err := s.d.Posts.PrepareCreate(ctx, in)
	if err != nil {
		return nil, err
	}
	return &prepared, nil
}

// JobNotifyStarted tells the local followers of the host who see a room that it opened
// (voice_room_started, ADR-0100 §8). Its payload names the room and the visibility it opened
// with; its idempotency key is the room's.
const JobNotifyStarted = "voice.notify_started"

// notifyBatch is how many followers one transaction of JobNotifyStarted notifies.
const notifyBatch = 100

type notifyStartedPayload struct {
	Room       string           `json:"room"`
	Host       string           `json:"host"`
	Visibility voice.Visibility `json:"visibility"`
	At         time.Time        `json:"at"`
}

// enqueueNotifyStarted enqueues JobNotifyStarted in the transaction of Open.
func (s *Service) enqueueNotifyStarted(ctx context.Context, tx uow.Tx, room Room, rev Revision) error {
	if s.d.Notifier == nil {
		return nil
	}
	b, err := json.Marshal(notifyStartedPayload{Room: room.ID, Host: room.OpenedByAccountID, Visibility: rev.Settings.Visibility, At: room.OpenedAt})
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobNotifyStarted, Payload: b, IdempotencyKey: JobNotifyStarted + ":" + room.ID})
}

// NotifyStarted runs JobNotifyStarted: it walks the local followers of the host and notifies
// each one who sees the room both with the visibility it opened with and now, so that widening
// the visibility after the opening notifies nobody more. A room that ended or changed its host
// meanwhile notifies nobody more. Running it again notifies nobody twice (the dedupe key of the
// room).
func (s *Service) NotifyStarted(ctx context.Context, j queue.Job) queue.Result {
	var p notifyStartedPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("decode %s: %w", JobNotifyStarted, err))
	}
	if s.d.Notifier == nil {
		return queue.Done()
	}
	if err := s.notifyStarted(ctx, p); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

func (s *Service) notifyStarted(ctx context.Context, p notifyStartedPayload) error {
	before := ""
	for {
		ids, next, err := s.d.Relations.LocalFollowers(ctx, p.Host, before, notifyBatch)
		if err != nil {
			return err
		}
		v, err := s.view(ctx, p.Room)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if v.Head.State != voice.StateOpen || v.Head.HostAccountID != p.Host {
			return nil
		}
		var to []string
		for _, id := range ids {
			ok, err := s.seesOpened(ctx, v, p.Visibility, id)
			if err != nil {
				return err
			}
			if ok {
				to = append(to, id)
			}
		}
		if len(to) > 0 {
			err := s.roomTx(ctx, []string{p.Room}, func(tx uow.Tx, rec RoomRecord) error {
				if rec.Head.State != voice.StateOpen {
					return nil
				}
				for _, id := range to {
					if err := s.d.Notifier.RoomStartedTx(ctx, tx, p.Room, p.Host, id, p.At); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		if next == "" {
			return nil
		}
		before = next
	}
}

// seesOpened reports whether accountID sees the room of v now and would have seen it with the
// visibility opened.
func (s *Service) seesOpened(ctx context.Context, v RoomView, opened voice.Visibility, accountID string) (bool, error) {
	viewer, m, err := s.viewer(ctx, accountID)
	if err != nil {
		return false, err
	}
	f, err := s.facts(ctx, v, viewer, m)
	if err != nil {
		return false, err
	}
	room := policyRoom(v)
	if !voice.CanSee(room, viewer, f) {
		return false, nil
	}
	// The facts are those of now; only the visibility is the one the room opened with, so that a
	// widening after the opening notifies nobody more (ADR-0100 §8) and a narrowing narrows it.
	room.Visibility = opened
	return voice.CanSee(room, viewer, f), nil
}

// ProfileRoom returns the open room where accountID is the host or a speaker, when viewerID
// sees it (ADR-0100 §8): the entry on the profile of accountID. A listener has none, and ok is
// false without one.
func (s *Service) ProfileRoom(ctx context.Context, viewerID, accountID string) (RoomView, bool, error) {
	if !s.Enabled() || viewerID == "" {
		return RoomView{}, false, nil
	}
	p, err := s.d.Repository.FindParticipant(ctx, nil, accountID)
	if errors.Is(err, ErrNotFound) {
		return RoomView{}, false, nil
	}
	if err != nil {
		return RoomView{}, false, err
	}
	if p.Role != voice.RoleSpeaker {
		return RoomView{}, false, nil
	}
	// A room whose participants expired and that voice.sweep has not ended yet shows no entry.
	if err := s.evaluate(ctx, p.RoomID, false); err != nil {
		return RoomView{}, false, err
	}
	v, err := s.visibleRoom(ctx, viewerID, p.RoomID)
	if errors.Is(err, ErrNotFound) {
		return RoomView{}, false, nil
	}
	if err != nil || v.Head.State != voice.StateOpen {
		return RoomView{}, false, err
	}
	return v, true, nil
}

// Visible returns a room that accountID sees, without evaluating its expiry: for the
// notifications that name it. ErrNotFound for any other.
func (s *Service) Visible(ctx context.Context, accountID, roomID string) (RoomView, error) {
	if err := s.enabled(); err != nil {
		return RoomView{}, err
	}
	return s.visibleRoom(ctx, accountID, roomID)
}

// History returns a page of the rooms that accountID hosted at some time, newest first, after
// the room beforeID unless it is "", and whether more follow (ADR-0100 §13).
func (s *Service) History(ctx context.Context, accountID, beforeID string, limit int) ([]RoomRecord, bool, error) {
	if err := s.enabled(); err != nil {
		return nil, false, err
	}
	recs, err := s.d.Repository.HostedRooms(ctx, nil, accountID, beforeID, limit+1)
	if err != nil {
		return nil, false, err
	}
	if len(recs) > limit {
		return recs[:limit], true, nil
	}
	return recs, false, nil
}

// Summary is what the page of a room shows to the visitors who do not see it: the remote
// visitors and those who are not logged in (ADR-0100 §9).
type Summary struct {
	RoomID string
	// Ended reports a room that ended.
	Ended bool
	// Shown reports that the instance shows the summaries of its rooms (voice_public_summary)
	// and the room is for the local users: only then do Name, and for an open room Description
	// and HostAccountID, have values.
	Shown         bool
	Name          string
	Description   string
	HostAccountID string
}

// Summary returns the summary of a room, after evaluating its expiry; ErrNotFound for an unknown
// room. It never names the participants.
func (s *Service) Summary(ctx context.Context, roomID string) (Summary, error) {
	if err := s.enabled(); err != nil {
		return Summary{}, err
	}
	if err := s.evaluate(ctx, roomID, false); err != nil {
		return Summary{}, err
	}
	rec, err := s.d.Repository.FindRoom(ctx, nil, roomID)
	if err != nil {
		return Summary{}, err
	}
	out := Summary{RoomID: rec.ID, Ended: rec.Head.State == voice.StateEnded}
	public, err := s.d.Members.PublicSummary(ctx)
	if err != nil {
		return Summary{}, err
	}
	if !public || rec.Revision.Settings.Visibility != voice.VisibilityLocal {
		return out, nil
	}
	out.Shown, out.Name = true, rec.Revision.Settings.Name
	if !out.Ended {
		out.Description, out.HostAccountID = rec.Revision.Settings.Description, rec.Head.HostAccountID
	}
	return out, nil
}
