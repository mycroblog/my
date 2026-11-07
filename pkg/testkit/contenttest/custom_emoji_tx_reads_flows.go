// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"context"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

// emojiReadsInTransaction reads a custom emoji, its view, and its image in the transaction that
// registers it.
func emojiReadsInTransaction(t *testing.T, e *emojiEnv) {
	ctx := t.Context()
	var inTx appcontent.AdminCustomEmoji
	ran := testkit.InTransactionOf(e.projectors, appcontent.EventCustomEmojiAdministered, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		em := ev.(appcontent.CustomEmojiAdministered).Emoji
		var err error
		inTx, err = e.service.GetTx(ctx, tx, em.ID)
		if err != nil || inTx.Emoji.Shortcode != "blobcat" || inTx.View == nil || !inTx.Settings.Enabled {
			t.Errorf("GetTx = %+v, %v; want the new emoji with its image", inTx, err)
		}
		if views, err := e.media.CustomEmojisTx(ctx, tx, []string{em.ID}); err != nil || views[em.ID].Shortcode != "blobcat" {
			t.Errorf("CustomEmojisTx = %+v, %v", views, err)
		}
		mediaID := inTx.Image.MediaID
		if imgs, err := e.media.ImagesTx(ctx, tx, []appcontent.ImageRef{{AccountID: emojiAdmin, MediaID: mediaID}}); err != nil || len(imgs) != 1 {
			t.Errorf("ImagesTx = %+v, %v; want the image", imgs, err)
		}
		if remote, err := e.h.Media.ListRemoteMediaTx(ctx, tx, []string{mediaID}); err != nil || len(remote) != 0 {
			t.Errorf("ListRemoteMediaTx(an upload) = %+v, %v; want none", remote, err)
		}
	})
	e.create(t, "blobcat")
	testkit.WantRan(t, "Create", ran)
	stored, err := e.service.Get(ctx, inTx.Emoji.ID)
	if err != nil || stored.Image.ID != inTx.Image.ID || stored.View == nil || inTx.View == nil || *stored.View != *inTx.View {
		t.Errorf("Get after the commit = %+v, %v; want %+v", stored, err, inTx)
	}
}
