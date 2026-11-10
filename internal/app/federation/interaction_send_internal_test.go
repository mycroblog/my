// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"encoding/json"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/federation"
)

// TestAnswerObject: the answer of a ReplyRequest or AnnounceRequest names the request embedded
// and then the interaction (ADR-0155 §6), which GoToSocial reads; that of a QuoteRequest embeds
// the request alone (ADR-0069 §2).
func TestAnswerObject(t *testing.T) {
	const (
		request     = "https://gts.example/users/carol/announce_requests/1"
		actor       = "https://gts.example/users/carol"
		target      = "https://remote.example/ap/objects/1"
		interacting = "https://gts.example/users/carol/statuses/2"
	)
	for _, tt := range []struct {
		kind appcontent.InteractionKind
		typ  federation.ActivityType
		want string
	}{
		{appcontent.InteractionReply, federation.ActivityReplyRequest,
			`[{"id":"` + request + `","type":"ReplyRequest","actor":"` + actor + `","to":[],"cc":[],"object":"` + target + `","instrument":"` + interacting + `"},"` + interacting + `"]`},
		{appcontent.InteractionAnnounce, federation.ActivityAnnounceRequest,
			`[{"id":"` + request + `","type":"AnnounceRequest","actor":"` + actor + `","to":[],"cc":[],"object":"` + target + `","instrument":"` + interacting + `"},"` + interacting + `"]`},
		{appcontent.InteractionQuote, federation.ActivityQuoteRequest,
			`{"id":"` + request + `","type":"QuoteRequest","actor":"` + actor + `","to":[],"cc":[],"object":"` + target + `","instrument":"` + interacting + `"}`},
	} {
		t.Run(string(tt.kind), func(t *testing.T) {
			got, err := json.Marshal(answerObject(tt.kind, request, tt.typ, actor, target, interacting))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("object =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}
