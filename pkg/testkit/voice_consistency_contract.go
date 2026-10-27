// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The cases of VoiceRoomsContract for the decisions that a change committed between the steps of
// a command must not undo: the admission of a join, and the current session of a participation.

// hookKV is the store of the voice service, which runs a hook once after a Get of a key.
type hookKV struct {
	kernel.KeyValueStore
	mu    sync.Mutex
	hooks map[[2]string]func()
}

// afterGet runs fn once, after the next Get of key in namespace returns its value.
func (k *hookKV) afterGet(namespace, key string, fn func()) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.hooks == nil {
		k.hooks = map[[2]string]func(){}
	}
	k.hooks[[2]string{namespace, key}] = fn
}

func (k *hookKV) Get(ctx context.Context, namespace, key string) ([]byte, error) {
	v, err := k.KeyValueStore.Get(ctx, namespace, key)
	k.mu.Lock()
	fn := k.hooks[[2]string{namespace, key}]
	delete(k.hooks, [2]string{namespace, key})
	k.mu.Unlock()
	if fn != nil {
		fn()
	}
	return v, err
}

// hookRepo is the repository of the voice service, which runs a hook once at the next call of
// Banned in a transaction: inside the transaction of a join, so the hook must not open one.
type hookRepo struct {
	appvoice.Repository
	mu       sync.Mutex
	onBanned func()
}

func (r *hookRepo) afterBanned(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onBanned = fn
}

func (r *hookRepo) Banned(ctx context.Context, tx uow.Tx, roomID, accountID string) (bool, error) {
	if tx == nil {
		return r.Repository.Banned(ctx, tx, roomID, accountID)
	}
	r.mu.Lock()
	fn := r.onBanned
	r.onBanned = nil
	r.mu.Unlock()
	if fn != nil {
		fn()
	}
	return r.Repository.Banned(ctx, tx, roomID, accountID)
}

// roomsAdmission changes what lets an account in while Join decides, and checks that the join
// does not stand on what it read before.
func roomsAdmission(t *testing.T, e *roomsEnv) {
	ctx := t.Context()

	// A block with the host that the join read before it committed, and whose projector saw no
	// participation yet, takes the account out once the join committed.
	host := e.account(true)
	r := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	joiner := e.account(false)
	fired := false
	e.repo.afterBanned(func() {
		fired = true
		e.members.set(func() { e.members.blocks[[2]string{host, joiner}] = true })
	})
	_, err := e.svc.Join(ctx, joiner, r)
	if !fired {
		t.Fatal("the hook did not run")
	}
	wantErr(t, "Join while the host blocked the account", err, appvoice.ErrNotFound)
	e.wantParticipant(t, r, joiner, false)

	// An invitation revoked after Join read it.
	host = e.account(true)
	r = e.open(t, host, appvoice.SettingsInput{Visibility: voice.VisibilityInvited}).Room.ID
	invitee := e.account(false)
	if _, err := e.svc.Invite(ctx, host, r, invitee, ""); err != nil {
		t.Fatal(err)
	}
	fired = false
	e.members.after("blocked", 2, func() {
		fired = true
		if err := e.svc.RevokeInvitation(ctx, host, r, invitee); err != nil {
			t.Error(err)
		}
	})
	_, err = e.svc.Join(ctx, invitee, r)
	if !fired {
		t.Fatal("the hook did not run")
	}
	wantErr(t, "Join with an invitation revoked meanwhile", err, appvoice.ErrNotFound)
	e.wantParticipant(t, r, invitee, false)

	// A room made visible to the invited accounts only after Join read it.
	host = e.account(true)
	r = e.open(t, host, appvoice.SettingsInput{}).Room.ID
	late := e.account(false)
	invited := voice.VisibilityInvited
	fired = false
	e.members.after("blocked", 2, func() {
		fired = true
		if _, err := e.svc.Update(ctx, host, r, appvoice.SettingsPatch{Visibility: &invited}); err != nil {
			t.Error(err)
		}
	})
	_, err = e.svc.Join(ctx, late, r)
	if !fired {
		t.Fatal("the hook did not run")
	}
	wantErr(t, "Join of a room hidden meanwhile", err, appvoice.ErrNotFound)
	e.wantParticipant(t, r, late, false)
}

// roomsNewerSession replaces the session of a participation while a heartbeat or an evaluation
// of the expiry holds what it read of the previous one.
func roomsNewerSession(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, p := e.account(true), e.account(false)
	hj := e.open(t, host, appvoice.SettingsInput{})
	r := hj.Room.ID
	first := e.join(t, p, r)
	pid := e.wantParticipant(t, r, p, true).ParticipationID

	// A heartbeat of the session that another device replaced after the heartbeat checked it.
	var second appvoice.Joined
	e.kv.afterGet("voice.session", pid, func() { second = e.join(t, p, r) })
	err := e.svc.Heartbeat(ctx, p, r, first.Session)
	if second.Session == "" {
		t.Fatal("the hook did not run")
	}
	wantErr(t, "a heartbeat of a replaced session", err, appvoice.ErrNotFound)
	e.beat(t, p, r, second.Session)

	// An evaluation that read the session expired, while the session came back and another
	// device replaced it, keeps the participation.
	e.clock.Advance(40 * time.Second)
	e.beat(t, host, r, hj.Session)
	e.clock.Advance(10 * time.Second)
	var third appvoice.Joined
	e.kv.afterGet("voice.heartbeat", second.Session, func() {
		// The heartbeat of the session lands late, and another device joins.
		if err := e.kv.Set(ctx, "voice.heartbeat", second.Session, []byte(pid), appvoice.HeartbeatLease); err != nil {
			t.Error(err)
		}
		third = e.join(t, p, r)
	})
	if _, err := e.svc.Get(ctx, host, r); err != nil {
		t.Fatal(err)
	}
	if third.Session == "" {
		t.Fatal("the hook did not run")
	}
	e.wantParticipant(t, r, p, true)
	e.beat(t, p, r, third.Session)
}

// roomsNewerMediaSession replaces the session of a participation while the media path reports
// that the previous one left.
func roomsNewerMediaSession(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	e.transport.sfu()
	host, p := e.account(true), e.account(false)
	hj := e.open(t, host, appvoice.SettingsInput{})
	r := hj.Room.ID
	first := e.join(t, p, r)
	e.transport.connect(r, hj.Session, first.Session)

	var second appvoice.Joined
	e.transport.mu.Lock()
	e.transport.onSessions = func() { second = e.join(t, p, r) }
	e.transport.mu.Unlock()
	ev := appvoice.Event{ID: "EV_LEFT", Kind: appvoice.MediaSessionLeft, Room: appvoice.RoomRef{ID: r}, Session: first.Session}
	body, err := json.Marshal([]appvoice.Event{ev})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.ReceiveMediaEvents(ctx, appvoice.MediaWebhook{Transport: "livekit", Body: body, ClientIP: "192.0.2.1"})
	if err != nil || res.Outcome != appvoice.MediaWebhookAccepted {
		t.Fatalf("delivery = %+v, %v; want accepted", res, err)
	}
	if second.Session == "" {
		t.Fatal("the hook did not run")
	}
	e.wantParticipant(t, r, p, true)
	e.beat(t, p, r, second.Session)
}
