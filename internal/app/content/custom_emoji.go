// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// CustomEmojiView is a custom emoji as clients show it (ADR-0051 §4): its shortcode without
// colons, the URL of its image, and the URL of a still image of it.
type CustomEmojiView struct {
	ID        string
	Shortcode string
	URL       string
	StaticURL string
}

// CustomEmojis returns the custom emoji with an ID in ids that can be shown, keyed by ID, reading
// them together so that a page of statuses and accounts presents its emoji without a lookup per
// emoji. A disabled emoji is left out. A local emoji is shown through the medium variants of its
// image (moving for an animation). A remote emoji is shown through the media proxy; one whose
// domain or image host is under a reject_media (the media_fetch restriction, FR-702) is left
// out, as is every remote emoji when no proxy is configured. An emoji that is left out stays its
// :shortcode: text in the clients.
func (s *MediaService) CustomEmojis(ctx context.Context, ids []string) (map[string]CustomEmojiView, error) {
	return s.customEmojis(ctx, s.reads(), ids)
}

// CustomEmojisTx returns the custom emoji with an ID in ids as CustomEmojis does, read in tx with
// what tx has written (the data of the webhook messages, ADR-0060).
func (s *MediaService) CustomEmojisTx(ctx context.Context, tx uow.Tx, ids []string) (map[string]CustomEmojiView, error) {
	return s.customEmojis(ctx, s.readsIn(tx), ids)
}

func (s *MediaService) customEmojis(ctx context.Context, reads mediaReads, ids []string) (map[string]CustomEmojiView, error) {
	ids = sortedSet(slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return id == "" }))
	if len(ids) == 0 {
		return map[string]CustomEmojiView{}, nil
	}
	emojis, err := reads.ListCurrentCustomEmojis(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("content: the custom emoji %v: %w", ids, err)
	}
	emojis = slices.DeleteFunc(emojis, func(e content.CurrentCustomEmoji) bool { return !e.Settings.Enabled })
	return s.customEmojiViews(ctx, reads, emojis)
}

// customEmojiViews presents emojis, enabled or not, as CustomEmojis does, keyed by ID.
func (s *MediaService) customEmojiViews(ctx context.Context, reads mediaReads, emojis []content.CurrentCustomEmoji) (map[string]CustomEmojiView, error) {
	out := map[string]CustomEmojiView{}
	local, err := s.localEmojiImages(ctx, reads, emojis)
	if err != nil {
		return nil, err
	}
	for _, e := range emojis {
		img, ok := local[e.Image.MediaID]
		if !e.Emoji.Local() || !ok {
			continue
		}
		moving := img.Still.Medium
		if img.Animated != nil {
			moving = img.Animated.Medium
		}
		out[e.Emoji.ID] = CustomEmojiView{ID: e.Emoji.ID, Shortcode: e.Emoji.Shortcode, URL: moving.URL, StaticURL: img.Still.Medium.URL}
	}
	if s.d.Proxy == nil {
		return out, nil
	}
	remote, err := s.remoteEmojiMedia(ctx, reads, emojis)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, e := range emojis {
		m, ok := remote[e.Image.MediaID]
		if e.Emoji.Local() || !ok {
			continue
		}
		u, err := url.Parse(m.RemoteURL)
		if err != nil || u.Hostname() == "" {
			continue
		}
		rejected, err := rejectsAny(ctx, reads.RejectsMedia, seen, e.Emoji.Domain, u.Hostname())
		if err != nil {
			return nil, err
		}
		if rejected != "" {
			continue
		}
		proxied := s.d.Proxy.URL(m.MediaID, ProxyFull)
		// The still version is the proxy's small variant (ADR-0005 §10), when the proxy can make it.
		still := proxied
		if s.d.Proxy.d.Processor != nil {
			still = s.d.Proxy.URL(m.MediaID, ProxySmall)
		}
		out[e.Emoji.ID] = CustomEmojiView{ID: e.Emoji.ID, Shortcode: e.Emoji.Shortcode, URL: proxied, StaticURL: still}
	}
	return out, nil
}

// localEmojiImages returns the images of the local emoji among emojis whose processing
// succeeded, keyed by media file ID.
func (s *MediaService) localEmojiImages(ctx context.Context, reads mediaReads, emojis []content.CurrentCustomEmoji) (map[string]Image, error) {
	var ids []string
	for _, e := range emojis {
		if e.Emoji.Local() {
			ids = append(ids, e.Image.MediaID)
		}
	}
	out := map[string]Image{}
	if len(ids) == 0 {
		return out, nil
	}
	ids = sortedSet(ids)
	variants, err := reads.ListReadyVariants(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("content: the images of the custom emoji: %w", err)
	}
	byID := map[string][]content.MediaVariant{}
	for _, v := range variants {
		byID[v.File.ID] = append(byID[v.File.ID], v.Variant)
	}
	for id, vs := range byID {
		if img, ok := s.image(vs); ok {
			out[id] = img
		}
	}
	return out, nil
}

// remoteEmojiMedia returns the remote media files of the remote emoji among emojis, keyed by ID.
func (s *MediaService) remoteEmojiMedia(ctx context.Context, reads mediaReads, emojis []content.CurrentCustomEmoji) (map[string]content.RemoteMedia, error) {
	var ids []string
	for _, e := range emojis {
		if !e.Emoji.Local() {
			ids = append(ids, e.Image.MediaID)
		}
	}
	out := map[string]content.RemoteMedia{}
	if len(ids) == 0 {
		return out, nil
	}
	remote, err := reads.ListRemoteMedia(ctx, sortedSet(ids))
	if err != nil {
		return nil, fmt.Errorf("content: the images of the custom emoji: %w", err)
	}
	for _, m := range remote {
		out[m.MediaID] = m
	}
	return out, nil
}

// CustomEmojiURI returns the ActivityPub id of a local custom emoji on host (ADR-0051 §3).
func CustomEmojiURI(host, emojiID string) string { return content.CustomEmojiURI(host, emojiID) }

// EmojiTag is a custom emoji as the Emoji tag of an ActivityPub document presents it (ADR-0051
// §3): a local emoji, whose id the caller derives from its ID (CustomEmojiURI), with the
// large variant of its image (moving for an animation), or a remote one with its recorded id
// (URI, "" for none) and the original URL of its image.
type EmojiTag struct {
	ID        string
	Shortcode string
	Local     bool
	URI       string
	// Updated is when the current image was set.
	Updated time.Time
	// MediaType is the type of URL, "" when unknown (a remote image).
	MediaType string
	URL       string
}

// EmojiTags returns the Emoji tags of the enabled custom emoji with an ID in ids, ordered by
// shortcode; emoji that cannot be presented (no processed image, a remote one without its
// recorded image) are left out.
func (s *MediaService) EmojiTags(ctx context.Context, ids []string) ([]EmojiTag, error) {
	ids = sortedSet(slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return id == "" }))
	if len(ids) == 0 {
		return nil, nil
	}
	emojis, err := s.d.Repository.ListCurrentCustomEmojis(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("content: the custom emoji %v: %w", ids, err)
	}
	emojis = slices.DeleteFunc(emojis, func(e content.CurrentCustomEmoji) bool { return !e.Settings.Enabled })
	reads := s.reads()
	local, err := s.localEmojiImages(ctx, reads, emojis)
	if err != nil {
		return nil, err
	}
	remote, err := s.remoteEmojiMedia(ctx, reads, emojis)
	if err != nil {
		return nil, err
	}
	var out []EmojiTag
	for _, e := range emojis {
		t := EmojiTag{ID: e.Emoji.ID, Shortcode: e.Emoji.Shortcode, Local: e.Emoji.Local(), URI: e.Emoji.URI, Updated: e.Image.RevisedAt}
		if t.Local {
			img, ok := local[e.Image.MediaID]
			if !ok {
				continue
			}
			v := img.Primary()
			t.MediaType, t.URL = v.ContentType, v.URL
		} else {
			m, ok := remote[e.Image.MediaID]
			if !ok {
				continue
			}
			t.URL = m.RemoteURL
		}
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b EmojiTag) int {
		return cmp.Or(strings.Compare(a.Shortcode, b.Shortcode), strings.Compare(a.ID, b.ID))
	})
	return out, nil
}
