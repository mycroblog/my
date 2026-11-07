// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// The copy of remote custom emoji into local ones (ADR-0051 §4), part of
// CustomEmojiRepositoryContract.

// emojiNet is the remote servers of the images of remote emoji, as a kernel.HTTPClient that
// answers a URL with its body or a 404, and the domains whose media the instance rejects, as an
// appcontent.MediaFetchPolicy.
type emojiNet struct {
	mu       sync.Mutex
	bodies   map[string][]byte
	rejected map[string]bool
}

func (n *emojiNet) serve(url string, body []byte) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.bodies[url] = body
}

func (n *emojiNet) reject(domain string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.rejected[domain] = true
}

func (n *emojiNet) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	body, ok := n.bodies[req.URL]
	status := http.StatusOK
	if !ok {
		status = http.StatusNotFound
	}
	return &kernel.HTTPResponse{StatusCode: status, Header: http.Header{}, URL: req.URL, Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func (n *emojiNet) RejectsMedia(_ context.Context, domain string) (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.rejected[domain], nil
}

func (n *emojiNet) RejectsMediaTx(ctx context.Context, _ uow.Tx, domain string) (bool, error) {
	return n.RejectsMedia(ctx, domain)
}

// received records :shortcode: of domain with the image at imageURL, as a document received
// from an account of domain would, and returns its ID.
func (e *emojiEnv) received(t *testing.T, shortcode, domain, imageURL string) string {
	t.Helper()
	statuses := appcontent.NewService(appcontent.Deps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Statuses, RemoteMedia: e.media, Clock: e.clock, IDs: e.ids,
	})
	e.clock.Advance(time.Second)
	var ids []string
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		var err error
		ids, err = statuses.RecordRemoteEmojis(t.Context(), tx, emojiSenderA, domain, []appcontent.RemoteEmojiInput{
			{Shortcode: shortcode, URI: "https://" + domain + "/emojis/" + shortcode, ImageURL: imageURL},
		})
		return err
	})
	if err != nil || len(ids) != 1 {
		t.Fatalf("RecordRemoteEmojis(%s) = %v, %v", shortcode, ids, err)
	}
	return ids[0]
}

func emojiCopiesRemote(t *testing.T, e *emojiEnv) {
	ctx := t.Context()
	const imageURL = "https://remote.example/files/blobfox.png"
	e.net.serve(imageURL, testkit.FixturePNG(4, 3, true))
	remote := e.received(t, "blobfox", "remote.example", imageURL)
	e.clock.Advance(time.Second)
	got, err := e.service.Copy(ctx, appcontent.CopyCustomEmojiInput{ID: remote, OwnerAccountID: emojiAdmin, ByUserID: emojiAdminUser})
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if !got.Emoji.Local() || got.Emoji.ID == remote || got.Emoji.Shortcode != "blobfox" || !got.Emoji.CreatedAt.Equal(e.clock.Now()) {
		t.Errorf("Emoji = %+v", got.Emoji)
	}
	if got.Settings != content.DefaultCustomEmojiSettings(got.Emoji.ID) || len(got.Aliases) != 0 {
		t.Errorf("Settings = %+v, aliases %q", got.Settings, got.Aliases)
	}
	if got.View == nil || !strings.HasPrefix(got.View.URL, e.blobs.PublicURL("")) {
		t.Errorf("View = %+v, want the local emoji's own image", got.View)
	}
	if ev := e.takeEvents(); !slices.Equal(ev, []string{appcontent.CustomEmojiCopied}) {
		t.Errorf("events = %v", ev)
	}
	// The copy is a local emoji that local posts use; the remote one stays as it was.
	ids, err := e.service.LocalEmojiIDs(ctx, "a :blobfox: post")
	if err != nil || !slices.Equal(ids, []string{got.Emoji.ID}) {
		t.Errorf("LocalEmojiIDs = %v, %v; want the copy %s", ids, err, got.Emoji.ID)
	}
	if tags, err := e.media.EmojiTags(ctx, ids); err != nil || len(tags) != 1 || !tags[0].Local || tags[0].Shortcode != "blobfox" {
		t.Errorf("EmojiTags = %+v, %v", tags, err)
	}
	if r, err := e.service.Get(ctx, remote); err != nil || r.Emoji.Local() || !r.Settings.Enabled {
		t.Errorf("Get(remote) = %+v, %v", r, err)
	}
	// The remote one's shortcode, now taken, is refused; a new one names another copy.
	if _, err := e.service.Copy(ctx, appcontent.CopyCustomEmojiInput{ID: remote, OwnerAccountID: emojiAdmin}); emojiReason(err) != content.EmojiShortcodeTaken {
		t.Errorf("Copy(taken) = %v", err)
	}
	other, err := e.service.Copy(ctx, appcontent.CopyCustomEmojiInput{ID: remote, Shortcode: "blobfox_2", OwnerAccountID: emojiAdmin})
	if err != nil || other.Emoji.Shortcode != "blobfox_2" || !other.Emoji.Local() {
		t.Errorf("Copy(blobfox_2) = %+v, %v", other.Emoji, err)
	}
	if _, err := e.service.Copy(ctx, appcontent.CopyCustomEmojiInput{ID: got.Emoji.ID, Shortcode: "again", OwnerAccountID: emojiAdmin}); emojiReason(err) != content.EmojiLocal {
		t.Errorf("Copy(local) = %v", err)
	}
	if _, err := e.service.Copy(ctx, appcontent.CopyCustomEmojiInput{ID: e.ids.NewID(), OwnerAccountID: emojiAdmin}); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Copy(unknown) = %v", err)
	}
}

func emojiRefusesCopies(t *testing.T, e *emojiEnv) {
	ctx := t.Context()
	images := map[string][]byte{
		"https://remote.example/files/photo.jpg":   testkit.FixtureJPEG(4, 3, 1),
		"https://remote.example/files/large.png":   append(testkit.FixturePNG(4, 3, true), make([]byte, 64<<10)...),
		"https://remote.example/files/corrupt.png": append(testkit.FixturePNG(4, 3, true), "corrupt"...),
		"https://rejected.example/files/ok.png":    testkit.FixturePNG(4, 3, true),
	}
	for u, body := range images {
		e.net.serve(u, body)
	}
	e.net.reject("rejected.example")
	for name, tc := range map[string]struct {
		shortcode, domain, url string
		want                   content.CustomEmojiReason
	}{
		"gone":              {"gone", "remote.example", "https://remote.example/files/gone.png", content.EmojiFetchFailed},
		"rejected domain":   {"ok", "rejected.example", "https://rejected.example/files/ok.png", content.EmojiFetchFailed},
		"jpeg":              {"photo", "remote.example", "https://remote.example/files/photo.jpg", content.EmojiUnsupportedFormat},
		"corrupt":           {"corrupt", "remote.example", "https://remote.example/files/corrupt.png", content.EmojiUnsupportedFormat},
		"over the limit":    {"large", "remote.example", "https://remote.example/files/large.png", content.EmojiTooLarge},
		"invalid shortcode": {"blob-fox", "remote.example", "https://rejected.example/files/ok.png", content.EmojiInvalidShortcode},
	} {
		remote := e.received(t, tc.shortcode, tc.domain, tc.url)
		_, err := e.service.Copy(ctx, appcontent.CopyCustomEmojiInput{ID: remote, OwnerAccountID: emojiAdmin})
		if got := emojiReason(err); got != tc.want || !errors.Is(err, content.ErrInvalidCustomEmoji) {
			t.Errorf("%s: Copy = %v, want %s", name, err, tc.want)
		}
	}
	local, err := e.service.List(ctx, appcontent.CustomEmojiQuery{Locality: appcontent.LocalEmoji}, 10)
	if err != nil || len(local) != 0 {
		t.Errorf("refused copies were stored: %d, %v", len(local), err)
	}
	uploads, err := e.h.Media.ListUnattachedUploads(ctx, "", e.clock.Now().Add(time.Hour), 10)
	if err != nil || len(uploads) != 0 {
		t.Errorf("refused copies left uploads %v, %v", uploads, err)
	}
	if keys := e.blobs.Keys(); len(keys) != 0 {
		t.Errorf("refused copies left blobs %v", keys)
	}
	if ev := e.takeEvents(); len(ev) != 0 {
		t.Errorf("refused copies emitted %v", ev)
	}
}
