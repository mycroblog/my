// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The jobs of the voice module.
const (
	// JobApply applies a committed change of a room to its transport (ADR-0101 §2): it opens
	// the media path of a room, allows or stops the audio of a session, mutes a session,
	// removes a session from it, or closes it. Its idempotency key is the room and the change.
	JobApply = "voice.apply"
	// JobSweep evaluates every open room (ADR-0100 §6): the expiry of the participants and the
	// grace of the hosts, the rooms of a voice chat turned off (disabled), and the rooms open
	// longer than the operator allows (time_limit). On a transport whose media server holds
	// the sessions, it first reconciles the room with its media path (ADR-0101 §2). No payload.
	JobSweep = "voice.sweep"
	// JobCheckAccount applies a change of an account to its participation (ADR-0100 §2, §11):
	// a suspended or deleted account leaves (suspended), and a host who lost the eligibility
	// hands the room over (host_ineligible). Its payload names the account.
	JobCheckAccount = "voice.check_account"
	// JobCheckHosts checks the eligibility of the host of every open room after a change of
	// the roles or of voice_room_starters (ADR-0100 §2). No payload.
	JobCheckHosts = "voice.check_hosts"
	// SweepInterval is the period of the JobSweep schedule.
	SweepInterval = time.Minute
)

// settingVoiceRoomStarters is the instance setting of who may start rooms (ADR-0100 §2).
const settingVoiceRoomStarters = "voice_room_starters"

// The operations of JobApply.
const (
	applyOpen   = "open"
	applySpeak  = "speak"
	applyMute   = "mute"
	applyRemove = "remove"
	applyClose  = "close"
)

type applyPayload struct {
	Op      string `json:"op"`
	Room    string `json:"room"`
	Session string `json:"session,omitempty"`
	Account string `json:"account,omitempty"`
	// Participation names the participation whose current session applyMute mutes.
	Participation string `json:"participation,omitempty"`
	// Speak is whether the session of applySpeak may publish audio.
	Speak bool `json:"speak,omitempty"`
	// By is the account that muted the session of applyMute.
	By string `json:"by,omitempty"`
}

type accountPayload struct {
	Account string `json:"account"`
}

// enqueueApply enqueues JobApply in tx; key names the change within the room.
func (s *Service) enqueueApply(ctx context.Context, tx uow.Tx, p applyPayload, key string) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
		Type: JobApply, Payload: b, IdempotencyKey: "voice.apply:" + p.Room + ":" + key, OrderingKey: "voice_room:" + p.Room,
	})
}

// Apply runs JobApply. A transport that fails is retried; once the retries run out the job is
// dead and the room stays as the domain recorded it, for voice.sweep to reconcile (ADR-0101 §2).
func (s *Service) Apply(ctx context.Context, j queue.Job) queue.Result {
	var p applyPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("decode %s: %w", JobApply, err))
	}
	t := s.d.Transport
	if t == nil {
		return queue.Done()
	}
	room := RoomRef{ID: p.Room}
	var err error
	switch p.Op {
	case applyOpen:
		err = t.Open(ctx, room)
	case applySpeak:
		var reconnect bool
		reconnect, err = t.SetSpeaking(ctx, room, Session{ID: p.Session, AccountID: p.Account}, p.Speak)
		if err == nil {
			// The client of the session connects again with a new grant, which its stream
			// sends (grant, ADR-0101 §2). A transport may report it only on the first call, so
			// a retry after a failed send sends it still.
			err = s.reconnect(ctx, j.ID, room, Session{ID: p.Session, AccountID: p.Account}, p.Speak, reconnect)
		}
	case applyMute:
		session := p.Session
		if session == "" {
			if session, err = s.d.Heartbeats.current(ctx, p.Participation); err != nil {
				return queue.Retry(0, err)
			}
		}
		if session != "" {
			err = t.Mute(ctx, room, Session{ID: session, AccountID: p.Account})
		}
		if err == nil && session != "" {
			// The stream of the session tells its client to turn the microphone off
			// (muted, ADR-0101 §4); a repeated job tells it again.
			s.muted(ctx, p.Room, session, p.By)
		}
	case applyRemove:
		err = t.Remove(ctx, room, Session{ID: p.Session, AccountID: p.Account})
	case applyClose:
		err = t.Close(ctx, room)
	default:
		return queue.Fail(fmt.Errorf("%s: unknown operation %q", JobApply, p.Op))
	}
	if err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// kvReconnects keys, by job ID, the JobApply runs whose transport asked the client to connect
// again, until the grant is sent.
const kvReconnects = "voice.reconnect"

// reconnect sends the grant of a session that has to connect again, when the transport asked
// for it in this run of the job or an earlier one.
func (s *Service) reconnect(ctx context.Context, jobID string, room RoomRef, session Session, canSpeak, asked bool) error {
	if asked {
		if err := s.d.Heartbeats.kv.Set(ctx, kvReconnects, jobID, []byte{1}, sessionLease); err != nil {
			return err
		}
	} else {
		_, err := s.d.Heartbeats.kv.Get(ctx, kvReconnects, jobID)
		if errors.Is(err, kernel.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	if err := s.sendGrant(ctx, room, session, canSpeak); err != nil {
		return err
	}
	return s.d.Heartbeats.kv.Delete(ctx, kvReconnects, jobID)
}

// sendGrant sends a new grant to the stream of a session that has to connect again.
func (s *Service) sendGrant(ctx context.Context, room RoomRef, session Session, canSpeak bool) error {
	g, err := s.d.Transport.Grant(ctx, room, session, canSpeak)
	if err != nil {
		return err
	}
	s.d.Logger.DebugContext(ctx, "the voice session connects again", slog.String("room", room.ID))
	return s.send(ctx, Message{Room: room.ID, To: session.ID, Grant: &GrantMessage{Grant: g, Reconnect: true}})
}

// Sweep evaluates every open room as JobSweep does.
func (s *Service) Sweep(ctx context.Context) error {
	open, err := s.d.Repository.OpenRooms(ctx, nil, "")
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range open {
		// A media server that does not answer leaves the room to the heartbeats.
		if err := s.reconcile(ctx, r); err != nil {
			errs = append(errs, fmt.Errorf("room %s: reconcile: %w", r.ID, err))
		}
		if err := s.evaluate(ctx, r.ID, true); err != nil {
			errs = append(errs, fmt.Errorf("room %s: %w", r.ID, err))
		}
	}
	return errors.Join(errs...)
}

// CheckAccount applies the state of an account to its participation, as JobCheckAccount does.
func (s *Service) CheckAccount(ctx context.Context, accountID string) error {
	p, err := s.d.Repository.FindParticipant(ctx, nil, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	}
	rec, err := s.d.Repository.FindRoom(ctx, nil, p.RoomID)
	if err != nil {
		return err
	}
	if rec.Head.HostAccountID == accountID {
		return s.checkHost(ctx, rec)
	}
	m, err := s.d.Members.Member(ctx, accountID)
	if err != nil || (m.Local && m.Active) {
		return err
	}
	session, err := s.d.Heartbeats.current(ctx, p.ParticipationID)
	if err != nil {
		return err
	}
	now := s.now()
	err = s.roomTx(ctx, []string{p.RoomID}, func(tx uow.Tx, rec RoomRecord) error {
		if rec.Head.HostAccountID == accountID {
			// The account became the host meanwhile: it hands the room over instead.
			return ErrConflict
		}
		if _, err := s.endParticipationTx(ctx, tx, p, voice.SuspendedAccount, session, now); err != nil {
			return err
		}
		return s.endIfEmptyTx(ctx, tx, p.RoomID, now)
	})
	if err != nil {
		return err
	}
	s.forget(ctx, &previous{Participant: p, session: session})
	return nil
}

// CheckHosts checks the host of every open room, as JobCheckHosts does.
func (s *Service) CheckHosts(ctx context.Context) error {
	open, err := s.d.Repository.OpenRooms(ctx, nil, "")
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range open {
		if err := s.checkHost(ctx, r); err != nil {
			errs = append(errs, fmt.Errorf("room %s: %w", r.ID, err))
		}
	}
	return errors.Join(errs...)
}

// checkHost moves an open room whose host is no longer eligible to the next host or ends it; a
// suspended or deleted host also leaves (ADR-0100 §2, §11).
func (s *Service) checkHost(ctx context.Context, rec RoomRecord) error {
	if rec.Head.State != voice.StateOpen {
		return nil
	}
	host := rec.Head.HostAccountID
	m, err := s.d.Members.Member(ctx, host)
	if err != nil {
		return err
	}
	if !m.Local || !m.Active {
		return s.hostIneligible(ctx, rec.ID, host, true)
	}
	ok, err := s.canHost(ctx, host)
	if err != nil || ok {
		return err
	}
	return s.hostIneligible(ctx, rec.ID, host, false)
}

// RegisterJobs registers the handlers of the module's jobs and the schedule of JobSweep.
func (s *Service) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobApply, s.Apply)
	reg.Register(JobMediaEvent, s.MediaEvent)
	reg.Register(JobSweep, func(ctx context.Context, _ queue.Job) queue.Result {
		if err := s.Sweep(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	reg.Register(JobCheckAccount, func(ctx context.Context, j queue.Job) queue.Result {
		var p accountPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return queue.Fail(fmt.Errorf("decode %s: %w", JobCheckAccount, err))
		}
		if err := s.CheckAccount(ctx, p.Account); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	reg.Register(JobCheckBlock, func(ctx context.Context, j queue.Job) queue.Result {
		var p blockPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return queue.Fail(fmt.Errorf("decode %s: %w", JobCheckBlock, err))
		}
		if err := s.CheckBlock(ctx, p.Blocker, p.Blocked); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	reg.Register(JobCheckHosts, func(ctx context.Context, _ queue.Job) queue.Result {
		if err := s.CheckHosts(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	reg.Register(JobNotifyStarted, s.NotifyStarted)
	if sched != nil {
		sched.Add(queue.Schedule{Name: JobSweep, Every: SweepInterval, Job: queue.NewJob{Type: JobSweep}})
	}
}

// RegisterEligibilityProjectors subscribes the rooms to the events of the other modules that
// change who may be in a room or host it (ADR-0100 §1, §2, §11): the suspensions of
// moderation.account_suspension_changed, the deletions of account.lifecycle_event_recorded,
// and the restrictions of moderation.account_restrictions_changed enqueue JobCheckAccount for an
// account in a room; the role changes of moderation.audit_recorded and the revisions of
// voice_room_starters enqueue JobCheckHosts while rooms are open; and the blocks of
// graph.blocked between two participants of the same room enqueue JobCheckBlock. Each enqueues
// in the transaction of its event, and the job applies the change after the commit.
func (s *Service) RegisterEligibilityProjectors(r *uow.Projectors) {
	s.registerBlockProjector(r)
	r.Register(moderation.EventAccountSuspensionChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(moderation.AccountSuspensionChanged)
		if !ev.Suspended {
			return nil
		}
		return s.enqueueCheckAccount(ctx, tx, ev.AccountID, "suspension:"+strconv.FormatInt(ev.ChangedAt.UnixNano(), 10))
	})
	r.Register(account.EventLifecycleEventRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(account.LifecycleEventRecorded)
		if !ev.ClosesLocalAccount() {
			return nil
		}
		return s.enqueueCheckAccount(ctx, tx, ev.Event.AccountID, "lifecycle:"+ev.Event.ID)
	})
	r.Register(moderation.EventAccountRestrictionsChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(moderation.AccountRestrictionsChanged)
		return s.enqueueCheckAccount(ctx, tx, ev.AccountID, "restrictions:"+s.d.IDs.NewID())
	})
	r.Register(moderation.EventAuditRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(moderation.AuditRecorded)
		switch ev.Entry.Action {
		case moderation.AuditRoleGranted, moderation.AuditRoleRevoked, moderation.AuditRoleUpdated, moderation.AuditRoleDeleted:
			return s.enqueueCheckHosts(ctx, tx, "audit:"+ev.Entry.ID)
		}
		return nil
	})
	r.Register(instance.EventSettingRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(instance.SettingRevised)
		if string(ev.Revision.Key) != settingVoiceRoomStarters {
			return nil
		}
		return s.enqueueCheckHosts(ctx, tx, "setting:"+ev.Revision.ID)
	})
}

// enqueueCheckAccount enqueues JobCheckAccount in tx for an account that is in a room.
func (s *Service) enqueueCheckAccount(ctx context.Context, tx uow.Tx, accountID, cause string) error {
	_, err := s.d.Repository.FindParticipant(ctx, tx, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	}
	b, err := json.Marshal(accountPayload{Account: accountID})
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
		Type: JobCheckAccount, Payload: b, IdempotencyKey: JobCheckAccount + ":" + accountID + ":" + cause,
	})
}

// enqueueCheckHosts enqueues JobCheckHosts in tx while a room is open.
func (s *Service) enqueueCheckHosts(ctx context.Context, tx uow.Tx, cause string) error {
	open, err := s.d.Repository.OpenRooms(ctx, tx, "")
	if err != nil || len(open) == 0 {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobCheckHosts, IdempotencyKey: JobCheckHosts + ":" + cause})
}
