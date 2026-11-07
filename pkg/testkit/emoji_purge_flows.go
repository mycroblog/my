// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// emojiTag returns the Emoji tag of a remote note with shortcode and its image on remote.example.
func emojiTag(shortcode string) map[string]any {
	return map[string]any{
		"type": "Emoji", "id": "https://remote.example/emojis/" + shortcode, "name": ":" + shortcode + ":",
		"icon": map[string]any{"type": "Image", "url": "https://remote.example/files/" + shortcode + ".png"},
	}
}

// emojiOf returns the ID of the remote emoji shortcode of remote.example, failing when there is
// none.
func (e *retentionEnv) emojiOf(t *testing.T, shortcode string) (string, string) {
	t.Helper()
	got, err := e.rh.Statuses.FindCustomEmojiByShortcode(t.Context(), shortcode, "remote.example")
	if err != nil {
		t.Fatalf("emoji %s: %v", shortcode, err)
	}
	_, img, err := e.content.CustomEmoji(t.Context(), got.ID)
	if err != nil {
		t.Fatalf("emoji %s image: %v", shortcode, err)
	}
	return got.ID, img.MediaID
}

func retentionEmojis(t *testing.T, e *retentionEnv) {
	ctx := t.Context()
	const alice = "https://remote.example/users/alice"
	aliceID := e.actor(t, alice)
	local := e.user(t, "local")
	e.follow(t, local, aliceID)

	// A note that the retention purge removes, with an emoji that nothing else uses, one whose
	// image is also an attachment of a note that stays, and one that the administrators disabled.
	e.note(t, alice, alice+"/notes/old", 100*24*time.Hour, map[string]any{
		"content": "<p>:gone: :shared: :muted:</p>", "tag": []any{emojiTag("gone"), emojiTag("shared"), emojiTag("muted")},
	})
	e.note(t, alice, alice+"/notes/attached", time.Hour, map[string]any{
		"attachment": []any{map[string]any{"type": "Document", "mediaType": "image/png", "url": "https://remote.example/files/shared.png"}},
	})
	// Notes that stay, one with an emoji that the administrators disabled.
	e.note(t, alice, alice+"/notes/kept", time.Hour, map[string]any{
		"content": "<p>:kept: :disabled:</p>", "tag": []any{emojiTag("kept"), emojiTag("disabled")},
	})
	disabled, _ := e.emojiOf(t, "disabled")
	muted, _ := e.emojiOf(t, "muted")
	for _, id := range []string{disabled, muted} {
		if err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return e.rh.Emojis.SetCustomEmojiSettings(ctx, tx, content.CustomEmojiSettings{EmojiID: id, VisibleInPicker: true})
		}); err != nil {
			t.Fatal(err)
		}
	}

	// A remote profile with an emoji.
	profiles := appaccount.NewService(appaccount.Deps{
		UnitOfWork: e.rh.UnitOfWork, Repository: e.rh.Accounts, Clock: e.clock, IDs: e.ids,
		Sealer: kernel.NewSealer(NewKeyManager("emoji-purge")), Host: contractHost, Sanitizer: remoteSanitizer{},
		RecordRemoteEmojis: func(ctx context.Context, tx uow.Tx, accountID, domain string, in []appaccount.RemoteEmojiInput) ([]string, error) {
			out := make([]appcontent.RemoteEmojiInput, len(in))
			for i, em := range in {
				out[i] = appcontent.RemoteEmojiInput{URI: em.URI, Shortcode: em.Shortcode, ImageURL: em.ImageURL}
			}
			return e.content.RecordRemoteEmojis(ctx, tx, accountID, domain, out)
		},
	})
	const bob = "https://remote.example/users/bob"
	e.actors.serve(bob)
	in := e.actors.served[bob]
	in.Name = ":profile:"
	in.Emojis = []appaccount.RemoteEmojiInput{{URI: "https://remote.example/emojis/profile", Shortcode: "profile", ImageURL: "https://remote.example/files/profile.png"}}
	actor, err := profiles.SaveRemoteActor(ctx, in)
	if err != nil {
		t.Fatalf("SaveRemoteActor = %v", err)
	}

	// Reactions of bob to a local status: one stays, and one is taken back.
	interactions := appcontent.NewInteractions(appcontent.InteractionDeps{
		UnitOfWork: e.rh.UnitOfWork, Statuses: e.rh.Statuses, Interactions: e.rh.Interactions,
		Accounts: e.accounts, Relationships: e.graph, RecordRemoteEmojis: e.content.RecordRemoteEmojis,
		Clock: e.clock, IDs: e.ids, Host: contractHost,
	})
	post, err := e.content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: local, TextInput: appcontent.TextInput{Text: "react"}, Visibility: content.VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	react := func(shortcode string) appcontent.RemoteReactionEmoji {
		return appcontent.RemoteReactionEmoji{Domain: "remote.example", Custom: &appcontent.RemoteEmojiInput{
			URI: "https://remote.example/emojis/" + shortcode, Shortcode: shortcode, ImageURL: "https://remote.example/files/" + shortcode + ".png",
		}}
	}
	for _, shortcode := range []string{"reacted", "unreacted"} {
		if _, err := interactions.RecordRemoteReaction(ctx, appcontent.RemoteReactionInput{
			URI: bob + "/likes/" + shortcode, AccountID: actor.Account.ID, StatusID: post.Status.ID, Emoji: react(shortcode),
		}); err != nil {
			t.Fatalf("RecordRemoteReaction(%s) = %v", shortcode, err)
		}
	}
	if err := interactions.RemoveRemoteReaction(ctx, appcontent.RemoteReactionUndo{URI: bob + "/likes/unreacted", AccountID: actor.Account.ID}); err != nil {
		t.Fatalf("RemoveRemoteReaction = %v", err)
	}

	// A local emoji, which the purge never touches.
	localEmoji := content.CustomEmoji{ID: e.ids.NewID(), Shortcode: "local", CreatedAt: e.clock.Now()}
	if err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := e.rh.Emojis.InsertCustomEmoji(ctx, tx, localEmoji); err != nil {
			return err
		}
		return e.rh.Emojis.InsertCustomEmojiImage(ctx, tx, content.CustomEmojiImage{
			ID: e.ids.NewID(), EmojiID: localEmoji.ID, MediaID: e.ids.NewID(), RevisedAt: e.clock.Now(),
		})
	}); err != nil {
		t.Fatal(err)
	}

	emojis := map[string][2]string{}
	for _, shortcode := range []string{"gone", "shared", "kept", "disabled", "muted", "profile", "reacted", "unreacted"} {
		id, media := e.emojiOf(t, shortcode)
		emojis[shortcode] = [2]string{id, media}
	}
	if n := e.purgeAll(t, e.retention(retentionPeriod, 0, 0)); n != 1 {
		t.Fatalf("the retention purge removed %d statuses, want 1", n)
	}
	purge := appcontent.NewEmojiPurge(appcontent.EmojiPurgeDeps{
		UnitOfWork: e.rh.UnitOfWork, Repository: e.rh.Emojis, Referrers: []appcontent.EmojiReferrer{profiles},
		MediaReferrers: []appcontent.MediaReferrer{profiles}, Batch: 1,
	})
	runAll := func() int {
		t.Helper()
		total, after := 0, ""
		for range 100 {
			next, n, err := purge.Run(ctx, after)
			if err != nil {
				t.Fatalf("Run(%q) = %v", after, err)
			}
			total += n
			if next == "" {
				return total
			}
			after = next
		}
		t.Fatal("the emoji purge did not reach the end")
		return total
	}
	if n := runAll(); n != 3 {
		t.Errorf("purged %d emoji, want 3 (gone, shared, unreacted)", n)
	}
	for shortcode, ids := range emojis {
		_, _, err := e.content.CustomEmoji(ctx, ids[0])
		_, mediaErr := e.h.Media.FindRemoteMedia(ctx, ids[1])
		switch shortcode {
		case "gone", "unreacted":
			if !errors.Is(err, appcontent.ErrNotFound) || !errors.Is(mediaErr, appcontent.ErrNotFound) {
				t.Errorf("%s: emoji %v, image %v; want both gone", shortcode, err, mediaErr)
			}
		case "shared":
			if !errors.Is(err, appcontent.ErrNotFound) || mediaErr != nil {
				t.Errorf("%s: emoji %v, image %v; want the emoji gone and the attachment kept", shortcode, err, mediaErr)
			}
		default:
			if err != nil || mediaErr != nil {
				t.Errorf("%s: emoji %v, image %v; want both kept", shortcode, err, mediaErr)
			}
		}
	}
	for _, id := range []string{disabled, muted} {
		if rec, err := e.rh.Emojis.FindCustomEmojiRecord(ctx, id); err != nil || rec.Settings.Enabled {
			t.Errorf("the disabled emoji %s = %+v, %v; want it kept disabled", id, rec.Settings, err)
		}
	}
	if _, _, err := e.content.CustomEmoji(ctx, localEmoji.ID); err != nil {
		t.Errorf("the local emoji = %v; want it kept", err)
	}
	// Running again finds nothing more; the purged emoji come back when they arrive again.
	if n := runAll(); n != 0 {
		t.Errorf("the second run purged %d emoji, want 0", n)
	}
	e.note(t, alice, alice+"/notes/again", time.Hour, map[string]any{"content": "<p>:gone:</p>", "tag": []any{emojiTag("gone")}})
	if id, _ := e.emojiOf(t, "gone"); id == emojis["gone"][0] {
		t.Errorf("the emoji came back with its purged ID %s", id)
	}
	// The disabled emoji that nothing refers to any more stays disabled when it arrives again.
	e.note(t, alice, alice+"/notes/muted-again", time.Hour, map[string]any{"content": "<p>:muted:</p>", "tag": []any{emojiTag("muted")}})
	if id, _ := e.emojiOf(t, "muted"); id != muted {
		t.Errorf("the disabled emoji arrived again as %s, want %s", id, muted)
	}
	if rec, err := e.rh.Emojis.FindCustomEmojiRecord(ctx, muted); err != nil || rec.Settings.Enabled {
		t.Errorf("the disabled emoji after it arrived again = %+v, %v; want it disabled", rec.Settings, err)
	}
}
