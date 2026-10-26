// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"testing"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// notificationVoiceRooms checks the voice chat notifications (ADR-0100 §8): voice_room_started
// names the room and comes once per room and recipient, voice_room_invited once per invitation,
// and the mutes and blocks keep them from the recipient as they keep the other notifications.
func notificationVoiceRooms(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol, dave := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol"), e.user(t, "dave")
	v := appnotification.NewVoiceNotifier(appnotification.ProjectorDeps{
		Repository: e.nh.Notifications, Accounts: e.accounts, Relationships: e.graph, Reach: e.reach, IDs: e.ids,
	})
	if _, err := e.graph.Mute(ctx, carol, alice, appgraph.MuteInput{HideNotifications: true}); err != nil {
		t.Fatalf("Mute = %v", err)
	}
	if _, err := e.graph.Block(ctx, alice, dave); err != nil {
		t.Fatalf("Block = %v", err)
	}
	room, invitation := e.ids.NewID(), e.ids.NewID()
	notify := func(f func(tx uow.Tx) error) {
		t.Helper()
		if err := e.nh.UnitOfWork.Tx(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		notify(func(tx uow.Tx) error {
			for _, to := range []string{bob, carol, dave, alice} {
				if err := v.RoomStartedTx(ctx, tx, room, alice, to, e.clock.Now()); err != nil {
					return err
				}
			}
			return v.RoomInvitedTx(ctx, tx, invitation, room, alice, bob, e.clock.Now())
		})
	}
	ns := e.list(t, bob)
	if len(ns) != 2 || ns[0].Type != notification.TypeVoiceRoomInvited || ns[1].Type != notification.TypeVoiceRoomStarted ||
		ns[0].VoiceRoomID != room || ns[1].VoiceRoomID != room || ns[0].SourceAccountID != alice || ns[1].SourceAccountID != alice {
		t.Errorf("notifications of bob = %+v, want voice_room_invited and voice_room_started of the room from alice", ns)
	}
	checkSummaries(t, e, carol)
	checkSummaries(t, e, dave)
	checkSummaries(t, e, alice)
}
