// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// The voice chat notifications (ADR-0100 §8): the voice module calls VoiceNotifier inside its
// own transactions, as no module imports the voice module. voice_room_started goes to a local
// follower of the host who sees the room, from the job of the room's opening, and
// voice_room_invited to the account invited to a room, in the transaction of the invitation.
// Both are from an account, so the exclusions of the other notifications from accounts apply:
// none to the source itself or to a remote account, none when either blocks the other or the
// recipient mutes the source with hide_notifications, and none from a source with
// unsolicited_reach to a recipient who does not follow it.

// VoiceNotifier creates the voice chat notifications.
type VoiceNotifier struct {
	p *projector
}

// NewVoiceNotifier returns a VoiceNotifier on the ports of the projectors.
func NewVoiceNotifier(d ProjectorDeps) *VoiceNotifier { return &VoiceNotifier{p: &projector{d: d}} }

// RoomStartedTx notifies recipientID inside tx that hostID opened the room roomID at at, once
// per room and recipient (notification.VoiceRoomStartedKey).
func (v *VoiceNotifier) RoomStartedTx(ctx context.Context, tx uow.Tx, roomID, hostID, recipientID string, at time.Time) error {
	return v.p.notify(ctx, tx, notification.Notification{
		RecipientAccountID: recipientID, SourceAccountID: hostID, Type: notification.TypeVoiceRoomStarted, VoiceRoomID: roomID,
		DedupeKey: notification.VoiceRoomStartedKey(roomID), CreatedAt: at,
	})
}

// RoomInvitedTx notifies inviteeID inside tx that inviterID invited it to the room roomID with
// the invitation invitationID at at, once per invitation (notification.VoiceRoomInvitedKey).
func (v *VoiceNotifier) RoomInvitedTx(ctx context.Context, tx uow.Tx, invitationID, roomID, inviterID, inviteeID string, at time.Time) error {
	return v.p.notify(ctx, tx, notification.Notification{
		RecipientAccountID: inviteeID, SourceAccountID: inviterID, Type: notification.TypeVoiceRoomInvited, VoiceRoomID: roomID,
		DedupeKey: notification.VoiceRoomInvitedKey(invitationID), CreatedAt: at,
	})
}
