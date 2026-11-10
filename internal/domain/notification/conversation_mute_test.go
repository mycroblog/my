// SPDX-License-Identifier: AGPL-3.0-only

package notification

import "testing"

func TestThreadMuted(t *testing.T) {
	for _, typ := range []Type{TypeMention, TypeBoost, TypeFavourite, TypeReaction, TypeQuote, TypeStatus} {
		if !ThreadMuted(typ) {
			t.Errorf("ThreadMuted(%s) = false, want true", typ)
		}
	}
	// The notices of the instance and of the moderators, and the other types, are never muted
	// by a thread.
	for _, typ := range []Type{TypeFollow, TypeFollowRequest, TypePollClosed, TypeModerationNotice, TypeReportResolved, TypeAdminReport, TypeReplyRequest, TypeAnnouncement} {
		if ThreadMuted(typ) {
			t.Errorf("ThreadMuted(%s) = true, want false", typ)
		}
	}
}
