// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// mediaNet is a kernel.HTTPClient that answers every request with body and counts the requests
// by URL.
type mediaNet struct {
	mu   sync.Mutex
	body []byte
	hits map[string]int
}

func (n *mediaNet) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.hits[req.URL]++
	return &kernel.HTTPResponse{StatusCode: http.StatusOK, Header: http.Header{}, URL: req.URL, Body: io.NopCloser(bytes.NewReader(n.body))}, nil
}

func (n *mediaNet) count(u string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.hits[u]
}

type remoteDomains struct{}

func (remoteDomains) AccountDomain(context.Context, string) (string, error) {
	return "remote.example", nil
}

type noMediaRejects struct{}

func (noMediaRejects) RejectsMedia(context.Context, string) (bool, error) { return false, nil }

func (noMediaRejects) RejectsMediaTx(context.Context, uow.Tx, string) (bool, error) {
	return false, nil
}

// cacheEnv is a media proxy with the remote media cache on the harness's repository.
type cacheEnv struct {
	*mediaEnv
	net      *mediaNet
	proxy    *appcontent.MediaProxy
	mu       sync.Mutex
	settings appcontent.ProxySettings
	data     []byte
}

func newCacheEnv(t *testing.T, e *mediaEnv, maxSize int64) *cacheEnv {
	t.Helper()
	data := FixturePNG(4, 3, false)
	c := &cacheEnv{
		mediaEnv: e, net: &mediaNet{body: data, hits: map[string]int{}}, data: data,
		settings: appcontent.ProxySettings{Policy: appcontent.RemoteMediaCacheAll, CacheTTL: time.Hour},
	}
	c.proxy = c.proxyOn(t, e.h.Media, maxSize)
	return c
}

// proxyOn returns a media proxy of the environment on another repository, which wraps the
// environment's.
func (c *cacheEnv) proxyOn(t *testing.T, repo appcontent.MediaRepository, maxSize int64) *appcontent.MediaProxy {
	t.Helper()
	p, err := appcontent.NewMediaProxy(appcontent.ProxyDeps{
		Repository: repo, HTTP: c.net, Accounts: remoteDomains{}, Policy: noMediaRejects{},
		Settings: func(context.Context) (appcontent.ProxySettings, error) {
			c.mu.Lock()
			defer c.mu.Unlock()
			return c.settings, nil
		},
		UnitOfWork: c.h.UnitOfWork, Blobs: c.blobs, Clock: c.clock, ProfileImages: c.referrers,
		MaxCacheSize: maxSize, Keys: [][]byte{bytes.Repeat([]byte("k"), 32)}, BaseURL: "https://social.example",
		Processor: FakeMediaProcessor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (c *cacheEnv) set(policy appcontent.RemoteMediaPolicy, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settings = appcontent.ProxySettings{Policy: policy, CacheTTL: ttl}
}

// record records a remote media file at remoteURL.
func (c *cacheEnv) record(t *testing.T, remoteURL string) content.RemoteMedia {
	t.Helper()
	var m content.RemoteMedia
	err := c.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		var err error
		m, err = c.service.RecordRemoteMedia(t.Context(), tx, appcontent.RemoteMediaInput{
			OwnerAccountID: mediaOwner, RemoteURL: remoteURL, DeclaredMediaType: content.MediaTypeImage,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// open opens a remote media file through the proxy and checks that it serves the file.
func (c *cacheEnv) open(t *testing.T, m content.RemoteMedia) {
	t.Helper()
	u, err := url.Parse(c.proxy.URL(m.MediaID, appcontent.ProxyFull))
	if err != nil {
		t.Fatal(err)
	}
	pm, err := c.proxy.Open(t.Context(), m.MediaID, string(appcontent.ProxyFull), u.Query().Get("sig"))
	if err != nil {
		t.Fatalf("Open(%s): %v", m.RemoteURL, err)
	}
	defer func() { _ = pm.Body.Close() }()
	got, err := io.ReadAll(pm.Body)
	if err != nil || !bytes.Equal(got, c.data) || pm.ContentType != "image/png" || pm.Length != int64(len(c.data)) {
		t.Fatalf("Open(%s) = %s, %d, %d bytes, %v", m.RemoteURL, pm.ContentType, pm.Length, len(got), err)
	}
}

func (c *cacheEnv) entry(t *testing.T, remoteURL string) (content.RemoteMediaCacheEntry, bool) {
	t.Helper()
	e, err := c.h.Media.FindRemoteMediaCache(t.Context(), remoteURL)
	if errors.Is(err, appcontent.ErrNotFound) {
		return e, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return e, true
}

func (c *cacheEnv) remoteBlobs() []string {
	var keys []string
	for _, k := range c.blobs.Keys() {
		if strings.HasPrefix(k, "remote/") {
			keys = append(keys, k)
		}
	}
	return keys
}

// mediaCachesRemoteMedia checks the remote media cache of the media proxy: none keeps nothing,
// all serves the second request from the copy, avatars keeps only the avatars, headers, and
// custom emoji images, and an expired copy or a lost blob is fetched again.
func mediaCachesRemoteMedia(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	c := newCacheEnv(t, e, 0)

	c.set(appcontent.RemoteMediaCacheNone, time.Hour)
	plain := c.record(t, "https://remote.example/plain.png")
	c.open(t, plain)
	c.open(t, plain)
	if n := c.net.count(plain.RemoteURL); n != 2 {
		t.Errorf("none fetched %d times for 2 requests, want 2", n)
	}
	if _, ok := c.entry(t, plain.RemoteURL); ok || len(c.remoteBlobs()) != 0 {
		t.Errorf("none kept a copy: blobs %v", c.remoteBlobs())
	}

	c.set(appcontent.RemoteMediaCacheAll, time.Hour)
	fetchedAt := e.clock.Now()
	c.open(t, plain)
	e.clock.Advance(time.Minute)
	c.open(t, plain)
	if n := c.net.count(plain.RemoteURL); n != 3 {
		t.Errorf("all fetched %d times in total, want 3 (the second request under all is served from the copy)", n)
	}
	got, ok := c.entry(t, plain.RemoteURL)
	want := content.RemoteMediaCacheEntry{
		URL: plain.RemoteURL, BlobKey: got.BlobKey, ContentType: "image/png", FileSize: int64(len(c.data)),
		FetchedAt: fetchedAt, ExpiresAt: fetchedAt.Add(time.Hour), LastAccessedAt: fetchedAt,
	}
	sameEntry := got.URL == want.URL && got.ContentType == want.ContentType && got.FileSize == want.FileSize &&
		got.FetchedAt.Equal(want.FetchedAt) && got.ExpiresAt.Equal(want.ExpiresAt) && got.LastAccessedAt.Equal(want.LastAccessedAt)
	if !ok || !sameEntry || !strings.HasPrefix(got.BlobKey, "remote/") {
		t.Errorf("cache entry = %+v, %v; want %+v", got, ok, want)
	}
	if !bytes.Equal(e.blobs.Bytes(got.BlobKey), c.data) {
		t.Error("the copy differs from the remote file")
	}

	// Past its expiry, the file is fetched again into a copy under a key of its own, and the
	// entry renewed; the replaced copy's blob goes with the next cleanup.
	e.clock.Advance(time.Hour)
	c.open(t, plain)
	if n := c.net.count(plain.RemoteURL); n != 4 {
		t.Errorf("an expired copy was served: %d fetches, want 4", n)
	}
	renewed, _ := c.entry(t, plain.RemoteURL)
	if !renewed.ExpiresAt.Equal(e.clock.Now().Add(time.Hour)) || renewed.BlobKey == got.BlobKey || !bytes.Equal(e.blobs.Bytes(renewed.BlobKey), c.data) {
		t.Errorf("renewed entry = %+v, the replaced one had blob %s", renewed, got.BlobKey)
	}
	if _, err := c.proxy.CleanupCache(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.blobs.Head(ctx, got.BlobKey); err == nil {
		t.Errorf("the blob %s of the replaced copy is left", got.BlobKey)
	}
	// A copy whose blob is gone is fetched again.
	if err := e.blobs.Delete(ctx, renewed.BlobKey); err != nil {
		t.Fatal(err)
	}
	c.open(t, plain)
	refetched, _ := c.entry(t, plain.RemoteURL)
	if n := c.net.count(plain.RemoteURL); n != 5 || !bytes.Equal(e.blobs.Bytes(refetched.BlobKey), c.data) {
		t.Errorf("a lost copy was not fetched again: %d fetches", n)
	}

	c.set(appcontent.RemoteMediaCacheAvatars, time.Hour)
	attachment := c.record(t, "https://remote.example/attachment.png")
	avatar := c.record(t, "https://remote.example/avatar.png")
	emoji := c.record(t, "https://remote.example/emoji.png")
	e.referrers[avatar.MediaID] = true
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.h.Statuses.InsertCustomEmojiImage(ctx, tx, content.CustomEmojiImage{
			ID: e.ids.NewID(), EmojiID: e.ids.NewID(), MediaID: emoji.MediaID, RevisedAt: e.clock.Now(),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		m    content.RemoteMedia
		kept bool
	}{{attachment, false}, {avatar, true}, {emoji, true}} {
		c.open(t, tc.m)
		c.open(t, tc.m)
		if _, ok := c.entry(t, tc.m.RemoteURL); ok != tc.kept {
			t.Errorf("avatars kept %s: %v, want %v", tc.m.RemoteURL, ok, tc.kept)
		}
		if n, want := c.net.count(tc.m.RemoteURL), map[bool]int{false: 2, true: 1}[tc.kept]; n != want {
			t.Errorf("avatars fetched %s %d times, want %d", tc.m.RemoteURL, n, want)
		}
	}
}

// mediaCachesSmallRemoteVariants checks the media proxy's small variant (ADR-0054 §7): made
// from the remote file on every request under none, and kept in the cache next to the file,
// keyed by the URL and the purpose, under all.
func mediaCachesSmallRemoteVariants(t *testing.T, e *mediaEnv) {
	c := newCacheEnv(t, e, 0)
	m := c.record(t, "https://remote.example/small.png")
	want, err := FakeMediaProcessor{}.Preview(t.Context(), c.data, media.Limits{}, media.Sizes{}, media.Request{Purpose: media.PurposeAttachment})
	if err != nil {
		t.Fatal(err)
	}
	open := func() {
		t.Helper()
		u, err := url.Parse(c.proxy.URL(m.MediaID, appcontent.ProxySmall))
		if err != nil {
			t.Fatal(err)
		}
		pm, err := c.proxy.Open(t.Context(), m.MediaID, string(appcontent.ProxySmall), u.Query().Get("sig"))
		if err != nil {
			t.Fatalf("Open(small): %v", err)
		}
		defer func() { _ = pm.Body.Close() }()
		got, err := io.ReadAll(pm.Body)
		if err != nil || !bytes.Equal(got, want.Data) || pm.ContentType != want.ContentType || pm.Length != int64(len(want.Data)) {
			t.Fatalf("Open(small) = %s, %d, %d bytes, %v; want the preview", pm.ContentType, pm.Length, len(got), err)
		}
	}
	key := m.RemoteURL + "#small-attachment-230400px"

	c.set(appcontent.RemoteMediaCacheNone, time.Hour)
	open()
	open()
	if n := c.net.count(m.RemoteURL); n != 2 {
		t.Errorf("none fetched %d times for 2 requests, want 2", n)
	}
	if _, ok := c.entry(t, key); ok {
		t.Error("none kept the small variant")
	}

	c.set(appcontent.RemoteMediaCacheAll, time.Hour)
	open()
	open()
	if n := c.net.count(m.RemoteURL); n != 3 {
		t.Errorf("all fetched %d times in total, want 3 (the second small request served from the cache)", n)
	}
	small, ok := c.entry(t, key)
	if _, full := c.entry(t, m.RemoteURL); !ok || !full || small.ContentType != want.ContentType || !bytes.Equal(e.blobs.Bytes(small.BlobKey), want.Data) {
		t.Errorf("cache entry of the small variant = %+v, %v (file kept: %v)", small, ok, full)
	}

	// The small variant of an avatar is cropped around its focal point, so two files of the same
	// URL with other focal points (as two owners may declare) are cached apart.
	const avatarURL = "https://remote.example/avatar-focus.png"
	focused := func(owner string, f *content.MediaFocus) content.RemoteMedia {
		t.Helper()
		var m content.RemoteMedia
		err := c.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			var err error
			m, err = c.service.RecordRemoteMedia(t.Context(), tx, appcontent.RemoteMediaInput{
				OwnerAccountID: owner, RemoteURL: avatarURL, DeclaredMediaType: content.MediaTypeImage,
				Purpose: content.PurposeAvatar, Focus: f,
			})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	openSmallOf := func(m content.RemoteMedia) []byte {
		t.Helper()
		u, err := url.Parse(c.proxy.URL(m.MediaID, appcontent.ProxySmall))
		if err != nil {
			t.Fatal(err)
		}
		pm, err := c.proxy.Open(t.Context(), m.MediaID, string(appcontent.ProxySmall), u.Query().Get("sig"))
		if err != nil {
			t.Fatalf("Open(small): %v", err)
		}
		defer func() { _ = pm.Body.Close() }()
		got, err := io.ReadAll(pm.Body)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	left := focused(mediaOwner, &content.MediaFocus{X: -0.5, Y: 0.25})
	right := focused("0192f0c1-0000-7000-8000-00000000b0b0", &content.MediaFocus{X: 0.5, Y: -0.25})
	centre := focused("0192f0c1-0000-7000-8000-00000000c0c0", nil)
	for _, m := range []content.RemoteMedia{left, right, centre, left, right, centre} {
		req := media.Request{Purpose: media.PurposeAvatar}
		if m.Focus != nil {
			req.FocusX, req.FocusY = m.Focus.X, m.Focus.Y
		}
		want, err := FakeMediaProcessor{}.Preview(t.Context(), c.data, media.Limits{}, media.Sizes{}, req)
		if err != nil {
			t.Fatal(err)
		}
		if got := openSmallOf(m); !bytes.Equal(got, want.Data) {
			t.Errorf("small variant with focus %+v = %q, want %q", m.Focus, got, want.Data)
		}
	}
	for _, k := range []string{avatarURL + "#small-avatar-96x96c-f-0.5,0.25", avatarURL + "#small-avatar-96x96c-f0.5,-0.25", avatarURL + "#small-avatar-96x96c-f0,0"} {
		if _, ok := c.entry(t, k); !ok {
			t.Errorf("no cache entry %s", k)
		}
	}
}

// mediaCleansUpRemoteMediaCache checks the cleanup of the remote media cache: it deletes the
// expired copies, then the least recently accessed ones over the size limit, each with its blob,
// and keeps an entry fetched again after it was read.
func mediaCleansUpRemoteMediaCache(t *testing.T, e *mediaEnv) {
	ctx := t.Context()
	size := int64(len(FixturePNG(4, 3, false)))
	c := newCacheEnv(t, e, 2*size)

	c.set(appcontent.RemoteMediaCacheAll, time.Minute)
	expiring := c.record(t, "https://remote.example/expiring.png")
	c.open(t, expiring)
	c.set(appcontent.RemoteMediaCacheAll, 10*time.Hour)
	var kept []content.RemoteMedia
	for _, name := range []string{"old", "touched", "new"} {
		e.clock.Advance(time.Millisecond)
		m := c.record(t, "https://remote.example/"+name+".png")
		c.open(t, m)
		kept = append(kept, m)
	}
	old, touched, newest := kept[0], kept[1], kept[2]
	e.clock.Advance(2 * time.Hour)
	c.open(t, touched)
	if got, _ := c.entry(t, touched.RemoteURL); !got.LastAccessedAt.Equal(e.clock.Now()) {
		t.Errorf("touched entry last accessed at %v, want %v", got.LastAccessedAt, e.clock.Now())
	}
	if n := c.net.count(touched.RemoteURL); n != 1 {
		t.Errorf("touched fetched %d times, want 1", n)
	}
	oldEntry, _ := c.entry(t, old.RemoteURL)
	expiringEntry, _ := c.entry(t, expiring.RemoteURL)

	deleted, err := c.proxy.CleanupCache(ctx)
	if err != nil || deleted != 2 {
		t.Fatalf("CleanupCache = %d, %v; want 2 (the expired copy and the least recently accessed one)", deleted, err)
	}
	for _, tc := range []struct {
		m    content.RemoteMedia
		kept bool
	}{{expiring, false}, {old, false}, {touched, true}, {newest, true}} {
		if _, ok := c.entry(t, tc.m.RemoteURL); ok != tc.kept {
			t.Errorf("after the cleanup, entry of %s exists: %v, want %v", tc.m.RemoteURL, ok, tc.kept)
		}
	}
	for _, k := range []string{oldEntry.BlobKey, expiringEntry.BlobKey} {
		if _, err := e.blobs.Head(ctx, k); err == nil {
			t.Errorf("blob %s of a deleted copy is left", k)
		}
	}
	if len(c.remoteBlobs()) != 2 {
		t.Errorf("remote blobs = %v, want the 2 kept copies", c.remoteBlobs())
	}
	if total, err := e.h.Media.RemoteMediaCacheSize(ctx); err != nil || total != 2*size {
		t.Errorf("RemoteMediaCacheSize = %d, %v; want %d", total, err, 2*size)
	}
	if deleted, err := c.proxy.CleanupCache(ctx); err != nil || deleted != 0 {
		t.Errorf("second CleanupCache = %d, %v; want 0", deleted, err)
	}

	// An entry fetched again after the cleanup read it is kept.
	newEntry, _ := c.entry(t, newest.RemoteURL)
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := e.h.Media.DeleteRemoteMediaCache(ctx, tx, newEntry.URL, newEntry.FetchedAt.Add(-time.Second))
		if ok {
			t.Error("DeleteRemoteMediaCache deleted an entry fetched at another time")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.entry(t, newest.RemoteURL); !ok {
		t.Error("the entry fetched again is gone")
	}
}
