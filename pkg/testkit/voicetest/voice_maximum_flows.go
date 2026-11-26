// SPDX-License-Identifier: AGPL-3.0-only

package voicetest

import (
	"errors"
	"strconv"
	"testing"

	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// maxOf is a room's maximum as the host gives it.
func maxOf(n int) *int { return &n }

// wantMaximumInvalid fails unless err is the room's maximum invalid.
func wantMaximumInvalid(t *testing.T, what string, err error) {
	t.Helper()
	var se *voice.SettingsError
	if !errors.As(err, &se) || se.Field != voice.FieldMaxParticipants {
		t.Errorf("%s: err = %v, want the field %s invalid", what, err, voice.FieldMaxParticipants)
	}
}

// roomsMaximum checks the room's maximum (ADR-0100 §3, §5, §16, ADR-0101 §1): the host sets it
// from voice.MinMaxParticipants to the operator's when opening, changing the settings, or by a
// schedule; it bounds the joins at the smaller of the host's and the operator's; and lowering it
// below the people in the room removes nobody.
func roomsMaximum(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	const operator = 4
	svc := e.newSvc(appvoice.Config{Transport: voice.TransportP2P, Capacity: operator})
	if got := svc.MaxParticipants(); got != operator {
		t.Errorf("MaxParticipants() = %d, want %d", got, operator)
	}

	// Outside 2 to the operator's maximum, including an explicit 0.
	for _, n := range []int{0, 1, operator + 1} {
		_, err := svc.Open(ctx, e.account(true), appvoice.SettingsInput{Name: "Room", MaxParticipants: maxOf(n)})
		wantMaximumInvalid(t, "Open with a maximum of "+strconv.Itoa(n), err)
	}

	// No maximum of the host leaves the operator's.
	free, err := svc.Open(ctx, e.account(true), appvoice.SettingsInput{Name: "Free"})
	if err != nil {
		t.Fatalf("Open without a maximum: %v", err)
	}
	if got := free.Room.Revision.Settings.MaxParticipants; got != 0 {
		t.Errorf("the host's maximum = %d, want none", got)
	}
	if got := svc.Capacity(free.Room); got != operator {
		t.Errorf("Capacity of a room without a maximum = %d, want the operator's %d", got, operator)
	}
	at, err := svc.Open(ctx, e.account(true), appvoice.SettingsInput{Name: "At", MaxParticipants: maxOf(operator)})
	if err != nil {
		t.Fatalf("Open at the operator's maximum: %v", err)
	}
	if got := svc.Capacity(at.Room); got != operator {
		t.Errorf("Capacity at the operator's maximum = %d, want %d", got, operator)
	}

	// The host's maximum bounds the joins, and the room keeps it.
	host := e.account(true)
	opened, err := svc.Open(ctx, host, appvoice.SettingsInput{Name: "Three", MaxParticipants: maxOf(3)})
	if err != nil {
		t.Fatalf("Open with a maximum of 3: %v", err)
	}
	room := opened.Room.ID
	if got := svc.Capacity(opened.Room); got != 3 {
		t.Errorf("Capacity = %d, want 3", got)
	}
	if got := e.room(t, room).Revision.Settings.MaxParticipants; got != 3 {
		t.Errorf("the stored maximum = %d, want 3", got)
	}
	a, b := e.account(false), e.account(false)
	for _, p := range []string{a, b} {
		if _, err := svc.Join(ctx, p, room); err != nil {
			t.Fatalf("Join below the maximum: %v", err)
		}
	}
	_, err = svc.Join(ctx, e.account(false), room)
	wantErr(t, "a join at the maximum", err, appvoice.ErrRoomFull)

	// Lowering the maximum below the people in the room removes nobody; later joins are refused.
	v, err := svc.Update(ctx, host, room, appvoice.SettingsPatch{MaxParticipants: maxOf(2)})
	if err != nil {
		t.Fatalf("Update to a maximum of 2: %v", err)
	}
	if got := svc.Capacity(v); got != 2 || v.Revision.Settings.MaxParticipants != 2 {
		t.Errorf("Capacity after lowering = %d, settings %d; want 2", got, v.Revision.Settings.MaxParticipants)
	}
	if ps := e.participants(t, room); len(ps) != 3 {
		t.Errorf("participants after lowering = %d, want the 3 who were in", len(ps))
	}
	_, err = svc.Join(ctx, e.account(false), room)
	wantErr(t, "a join after lowering the maximum", err, appvoice.ErrRoomFull)

	// A patch without the maximum keeps it; one outside the range is refused.
	renamed := "Renamed"
	v, err = svc.Update(ctx, host, room, appvoice.SettingsPatch{Name: &renamed})
	if err != nil {
		t.Fatalf("Update of the name: %v", err)
	}
	if got := v.Revision.Settings.MaxParticipants; got != 2 {
		t.Errorf("the maximum after a patch without it = %d, want 2", got)
	}
	for _, n := range []int{0, 1, operator + 1} {
		_, err := svc.Update(ctx, host, room, appvoice.SettingsPatch{MaxParticipants: maxOf(n)})
		wantMaximumInvalid(t, "Update to a maximum of "+strconv.Itoa(n), err)
	}
	if got := e.room(t, room).Revision.Settings.MaxParticipants; got != 2 {
		t.Errorf("the stored maximum after refused updates = %d, want 2", got)
	}

	// Raising it again admits.
	if _, err := svc.Update(ctx, host, room, appvoice.SettingsPatch{MaxParticipants: maxOf(operator)}); err != nil {
		t.Fatalf("Update to the operator's maximum: %v", err)
	}
	if _, err := svc.Join(ctx, e.account(false), room); err != nil {
		t.Errorf("Join after raising the maximum: %v", err)
	}
	_, err = svc.Join(ctx, e.account(false), room)
	wantErr(t, "a join at the operator's maximum", err, appvoice.ErrRoomFull)
}

// roomsScheduledMaximum checks the maximum of a room opened by a schedule (ADR-0100 §3, §16): it
// takes the schedule's, the operator's when the schedule gives none, and refuses one outside the
// range.
func roomsScheduledMaximum(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	const operator = 4
	svc := e.newSvc(appvoice.Config{Transport: voice.TransportP2P, Capacity: operator})

	for _, n := range []int{0, 1, operator + 1} {
		in := scheduledInput(e.account(true), "event:bad:"+strconv.Itoa(n))
		in.Settings.MaxParticipants = maxOf(n)
		_, err := svc.OpenScheduled(ctx, in)
		wantMaximumInvalid(t, "OpenScheduled with a maximum of "+strconv.Itoa(n), err)
	}

	v, err := svc.OpenScheduled(ctx, scheduledInput(e.account(true), "event:default"))
	if err != nil {
		t.Fatalf("OpenScheduled without a maximum: %v", err)
	}
	if got := svc.Capacity(v); got != operator || v.Revision.Settings.MaxParticipants != 0 {
		t.Errorf("Capacity of a scheduled room without a maximum = %d, settings %d; want %d and none", got, v.Revision.Settings.MaxParticipants, operator)
	}

	host := e.account(true)
	in := scheduledInput(host, "event:two")
	in.Settings.MaxParticipants = maxOf(2)
	v, err = svc.OpenScheduled(ctx, in)
	if err != nil {
		t.Fatalf("OpenScheduled with a maximum of 2: %v", err)
	}
	if got := svc.Capacity(v); got != 2 {
		t.Errorf("Capacity of the scheduled room = %d, want 2", got)
	}
	if got := e.room(t, v.ID).Revision.Settings.MaxParticipants; got != 2 {
		t.Errorf("the stored maximum of the scheduled room = %d, want 2", got)
	}
	if _, err := svc.Join(ctx, host, v.ID); err != nil {
		t.Fatalf("the host joins: %v", err)
	}
	if _, err := svc.Join(ctx, e.account(false), v.ID); err != nil {
		t.Fatalf("a listener joins: %v", err)
	}
	_, err = svc.Join(ctx, e.account(false), v.ID)
	wantErr(t, "a join at the scheduled room's maximum", err, appvoice.ErrRoomFull)
}
