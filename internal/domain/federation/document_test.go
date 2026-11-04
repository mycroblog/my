// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/federation"
)

// The Like of a reaction carries the emoji twice, as Misskey sends it, with the Emoji tag of a
// custom emoji and the terms of _misskey_reaction in its context (ADR-0051 §8).
func TestNewReactionLike(t *testing.T) {
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tag := federation.EmojiTag("https://my.example/emojis/1", "blobcat", at, "image/png", "https://my.example/media/1.png")
	like := federation.NewReactionLike(federation.NewActivityDocument("https://my.example/ap/activities/1", federation.ActivityLike,
		"https://my.example/users/1", at, federation.Audience{}, federation.IRI("https://remote.example/notes/1")), ":blobcat:", &tag)
	b, err := like.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["type"] != "Like" || got["content"] != ":blobcat:" || got["_misskey_reaction"] != ":blobcat:" || got["object"] != "https://remote.example/notes/1" {
		t.Errorf("Like = %s", b)
	}
	tags, _ := got["tag"].([]any)
	if len(tags) != 1 || tags[0].(map[string]any)["name"] != ":blobcat:" {
		t.Errorf("tag = %v, want the Emoji", got["tag"])
	}
	ctx, _ := got["@context"].([]any)
	terms, _ := ctx[len(ctx)-1].(map[string]any)
	if terms["_misskey_reaction"] != "misskey:_misskey_reaction" || terms["misskey"] != "https://misskey-hub.net/ns#" || terms["Emoji"] != "toot:Emoji" {
		t.Errorf("@context = %v", got["@context"])
	}
	if e := like.Embedded(); e.Context != nil || e.Content != ":blobcat:" {
		t.Errorf("Embedded = %+v, want the Like without its context", e)
	}
	// A Unicode emoji has no tag.
	b, err = federation.NewReactionLike(like.ActivityDocument, "👍", nil).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got = nil
	if err := json.Unmarshal(b, &got); err != nil || got["_misskey_reaction"] != "👍" || got["tag"] != nil {
		t.Errorf("Like of a Unicode emoji = %s, %v", b, err)
	}
}
