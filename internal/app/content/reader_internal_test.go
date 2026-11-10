// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"testing"

	"github.com/mycroblog/my/internal/domain/content"
)

// TestOwnApprovals: the viewer's reply and boost show the approval they wait for, or its refusal;
// an approved or withdrawn one shows none (the client API's approval and boost_approval).
func TestOwnApprovals(t *testing.T) {
	st := content.Status{URI: "https://my.example/ap/objects/1"}
	reply := func(s content.InteractionRequestState) OwnInteraction {
		return OwnInteraction{Kind: content.InteractionReply, InteractingURI: st.URI, State: s}
	}
	boost := func(s content.InteractionRequestState) OwnInteraction {
		return OwnInteraction{Kind: content.InteractionAnnounce, TargetURI: st.URI, State: s}
	}
	for _, tt := range []struct {
		name                 string
		own                  []OwnInteraction
		wantReply, wantBoost content.InteractionRequestState
	}{
		{"pending", []OwnInteraction{reply(content.InteractionRequestPending), boost(content.InteractionRequestPending)},
			content.InteractionRequestPending, content.InteractionRequestPending},
		{"rejected", []OwnInteraction{reply(content.InteractionRequestRejected), boost(content.InteractionRequestRejected)},
			content.InteractionRequestRejected, content.InteractionRequestRejected},
		{"accepted", []OwnInteraction{reply(content.InteractionRequestAccepted), boost(content.InteractionRequestAccepted)}, "", ""},
		{"withdrawn", []OwnInteraction{reply(content.InteractionRequestWithdrawn), boost(content.InteractionRequestWithdrawn)}, "", ""},
		{"a boost asked again after a withdrawal", []OwnInteraction{boost(content.InteractionRequestWithdrawn), boost(content.InteractionRequestPending)},
			"", content.InteractionRequestPending},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if r, b := ownApprovals(tt.own, st); r != tt.wantReply || b != tt.wantBoost {
				t.Errorf("ownApprovals = %q, %q; want %q, %q", r, b, tt.wantReply, tt.wantBoost)
			}
		})
	}
}
