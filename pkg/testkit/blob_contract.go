// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/app/blob"
)

// BlobStoreFactory returns a new, empty store. The factory registers the cleanup of everything
// it creates with t.Cleanup.
type BlobStoreFactory func(t *testing.T) blob.Store

// BlobStoreContract verifies an adapter of the BlobStore port: round trips with the media
// type, overwrites, missing keys, repeatable deletes, every valid key (keys that are prefixes
// of others, the longest keys and segments), long media types, atomic writes (a failed Put
// keeps the previous blob; concurrent readers never see a partial blob; concurrent writes and
// deletes never pair one blob's content with another's media type), rejection of every key
// that could escape the store, cancelled contexts, and public URLs.
func BlobStoreContract(t *testing.T, newStore BlobStoreFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, s blob.Store)
	}{
		{"RoundTrips", blobRoundTrips},
		{"DefaultsTheContentType", blobDefaultsContentType},
		{"Overwrites", blobOverwrites},
		{"ReportsMissingBlobs", blobReportsMissing},
		{"DeletesRepeatably", blobDeletesRepeatably},
		{"KeepsSiblingsOnDelete", blobKeepsSiblings},
		{"StoresKeysThatArePrefixesOfOthers", blobStoresPrefixKeys},
		{"StoresTheLongestKeys", blobStoresLongestKeys},
		{"KeepsLongMediaTypes", blobKeepsLongMediaTypes},
		{"KeepsThePreviousBlobWhenAPutFails", blobKeepsPreviousOnFailedPut},
		{"NeverShowsPartialWrites", blobNeverShowsPartialWrites},
		{"KeepsContentAndTypeTogetherUnderConcurrentPuts", blobConcurrentPutsStayConsistent},
		{"KeepsContentAndTypeTogetherWhenPutRacesDelete", blobPutRacingDeleteStaysConsistent},
		{"RejectsInvalidKeys", blobRejectsInvalidKeys},
		{"StopsOnCancelledContext", blobStopsOnCancelledContext},
		{"BuildsPublicURLs", blobBuildsPublicURLs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newStore(t)) })
	}
}

func blobPut(t *testing.T, s blob.Store, key, content, contentType string) blob.Info {
	t.Helper()
	info, err := s.Put(t.Context(), key, strings.NewReader(content), blob.PutOptions{ContentType: contentType})
	if err != nil {
		t.Fatalf("Put(%q): %v", key, err)
	}
	return info
}

func blobRead(t *testing.T, s blob.Store, key string) (string, blob.Info) {
	t.Helper()
	r, info, err := s.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("Get(%q): %v", key, err)
	}
	defer func() { _ = r.Close() }()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read %q: %v", key, err)
	}
	return string(b), info
}

func blobRoundTrips(t *testing.T, s blob.Store) {
	const key, content = "media/0192f0c1-0000-7000-8000-000000000001/full.jpg", "\xFF\xD8\xFFimage bytes"
	put := blobPut(t, s, key, content, "image/jpeg")
	if put.Key != key || put.ContentType != "image/jpeg" || put.Size != int64(len(content)) {
		t.Errorf("Put info = %+v", put)
	}
	got, info := blobRead(t, s, key)
	if got != content {
		t.Errorf("Get content = %q, want %q", got, content)
	}
	if info.Key != key || info.ContentType != "image/jpeg" || info.Size != int64(len(content)) {
		t.Errorf("Get info = %+v", info)
	}
	head, err := s.Head(t.Context(), key)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head.ContentType != "image/jpeg" || head.Size != int64(len(content)) {
		t.Errorf("Head info = %+v", head)
	}
}

func blobDefaultsContentType(t *testing.T, s blob.Store) {
	blobPut(t, s, "uploads/x", "data", "")
	if _, info := blobRead(t, s, "uploads/x"); info.ContentType != "application/octet-stream" {
		t.Errorf("ContentType = %q, want application/octet-stream", info.ContentType)
	}
}

func blobOverwrites(t *testing.T, s blob.Store) {
	blobPut(t, s, "a/b.png", "first, longer content", "image/png")
	blobPut(t, s, "a/b.png", "second", "image/gif")
	got, info := blobRead(t, s, "a/b.png")
	if got != "second" || info.ContentType != "image/gif" || info.Size != 6 {
		t.Errorf("after overwrite: %q %+v", got, info)
	}
}

func blobReportsMissing(t *testing.T, s blob.Store) {
	if _, _, err := s.Get(t.Context(), "nothing/here"); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Get missing: %v, want ErrNotFound", err)
	}
	if _, err := s.Head(t.Context(), "nothing/here"); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Head missing: %v, want ErrNotFound", err)
	}
	blobPut(t, s, "dir/file", "x", "")
	if _, err := s.Head(t.Context(), "dir"); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Head of a key prefix: %v, want ErrNotFound", err)
	}
}

func blobDeletesRepeatably(t *testing.T, s blob.Store) {
	blobPut(t, s, "media/x/full.jpg", "x", "image/jpeg")
	for i := range 2 {
		if err := s.Delete(t.Context(), "media/x/full.jpg"); err != nil {
			t.Fatalf("Delete #%d: %v", i+1, err)
		}
	}
	if _, err := s.Head(t.Context(), "media/x/full.jpg"); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Head after Delete: %v, want ErrNotFound", err)
	}
	// The key can be written again after its directory went away.
	blobPut(t, s, "media/x/full.jpg", "y", "image/jpeg")
	if got, _ := blobRead(t, s, "media/x/full.jpg"); got != "y" {
		t.Errorf("after rewrite: %q", got)
	}
}

func blobKeepsSiblings(t *testing.T, s blob.Store) {
	blobPut(t, s, "media/x/full.jpg", "full", "image/jpeg")
	blobPut(t, s, "media/x/thumbnail.jpg", "thumb", "image/jpeg")
	if err := s.Delete(t.Context(), "media/x/full.jpg"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := blobRead(t, s, "media/x/thumbnail.jpg"); got != "thumb" {
		t.Errorf("sibling = %q", got)
	}
}

func blobStoresPrefixKeys(t *testing.T, s blob.Store) {
	want := map[string]string{"x": "parent", "x/y": "child", "x/y/z": "grandchild"}
	for _, key := range []string{"x", "x/y", "x/y/z"} {
		blobPut(t, s, key, want[key], "text/plain")
	}
	for key, content := range want {
		if got, _ := blobRead(t, s, key); got != content {
			t.Errorf("Get(%q) = %q, want %q", key, got, content)
		}
	}
	if err := s.Delete(t.Context(), "x/y"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := blobRead(t, s, "x"); got != "parent" {
		t.Errorf("x after deleting x/y = %q", got)
	}
	if got, _ := blobRead(t, s, "x/y/z"); got != "grandchild" {
		t.Errorf("x/y/z after deleting x/y = %q", got)
	}
}

func blobStoresLongestKeys(t *testing.T, s blob.Store) {
	keys := []string{
		strings.Repeat("a", blob.MaxKeyLength),
		strings.Repeat("b", 300) + "/" + strings.Repeat("c", blob.MaxKeyLength-301),
		strings.Repeat("d/", blob.MaxKeyLength/2-1) + "ee",
	}
	for i, key := range keys {
		if err := blob.ValidateKey(key); err != nil {
			t.Fatalf("ValidateKey(key #%d): %v", i, err)
		}
		content := fmt.Sprintf("content %d", i)
		blobPut(t, s, key, content, "text/plain")
		if got, info := blobRead(t, s, key); got != content || info.Key != key {
			t.Errorf("Get(key #%d) = %q, key matches: %v", i, got, info.Key == key)
		}
		if err := s.Delete(t.Context(), key); err != nil {
			t.Errorf("Delete(key #%d): %v", i, err)
		}
	}
}

func blobKeepsLongMediaTypes(t *testing.T, s blob.Store) {
	contentType := "application/x-long; p=" + strings.Repeat("v", 2048)
	put := blobPut(t, s, "long-type", "x", contentType)
	if put.ContentType != contentType {
		t.Errorf("Put ContentType has %d bytes, want %d", len(put.ContentType), len(contentType))
	}
	if _, info := blobRead(t, s, "long-type"); info.ContentType != contentType {
		t.Errorf("Get ContentType has %d bytes, want %d", len(info.ContentType), len(contentType))
	}
	head, err := s.Head(t.Context(), "long-type")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head.ContentType != contentType {
		t.Errorf("Head ContentType has %d bytes, want %d", len(head.ContentType), len(contentType))
	}
}

// blobVersions are the blobs that the concurrency cases write, by content: each content (of a
// distinct length) goes with its own media type, so that a read pairing one blob's content or
// size with the other's media type is caught.
var blobVersions = map[string]string{
	"first content":                    "image/png",
	"second, a little longer, content": "image/gif",
}

// blobCheckConsistent reads key and fails unless it holds one of blobVersions whole, or, when
// allowMissing, nothing.
func blobCheckConsistent(t *testing.T, s blob.Store, key string, allowMissing bool) {
	t.Helper()
	missing := func(err error) bool { return allowMissing && errors.Is(err, blob.ErrNotFound) }
	r, info, err := s.Get(t.Context(), key)
	switch {
	case missing(err):
	case err != nil:
		t.Errorf("Get: %v", err)
	default:
		b, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Errorf("read: %v", err)
		} else if want, ok := blobVersions[string(b)]; !ok || info.ContentType != want || info.Size != int64(len(b)) {
			t.Errorf("Get = %q with %+v, want a written content with its own media type and size", b, info)
		}
	}
	head, err := s.Head(t.Context(), key)
	switch {
	case missing(err):
	case err != nil:
		t.Errorf("Head: %v", err)
	default:
		consistent := false
		for content, contentType := range blobVersions {
			consistent = consistent || head.Size == int64(len(content)) && head.ContentType == contentType
		}
		if !consistent {
			t.Errorf("Head = %+v, want the size and media type of one written blob", head)
		}
	}
}

// blobRace runs the writers and a reader of key concurrently, then checks what key holds.
func blobRace(t *testing.T, s blob.Store, key string, withDelete bool) {
	var wg sync.WaitGroup
	for content, contentType := range blobVersions {
		wg.Go(func() {
			for range 50 {
				if _, err := s.Put(t.Context(), key, strings.NewReader(content), blob.PutOptions{ContentType: contentType}); err != nil {
					t.Errorf("Put: %v", err)
					return
				}
			}
		})
	}
	if withDelete {
		wg.Go(func() {
			for range 50 {
				if err := s.Delete(t.Context(), key); err != nil {
					t.Errorf("Delete: %v", err)
					return
				}
			}
		})
	}
	wg.Go(func() {
		for range 50 {
			blobCheckConsistent(t, s, key, true)
		}
	})
	wg.Wait()
	blobCheckConsistent(t, s, key, withDelete)
}

func blobConcurrentPutsStayConsistent(t *testing.T, s blob.Store) {
	blobRace(t, s, "race/puts", false)
}

func blobPutRacingDeleteStaysConsistent(t *testing.T, s blob.Store) {
	blobRace(t, s, "race/put-delete", true)
}

// failingReader returns its content and then an error instead of EOF.
type failingReader struct{ r io.Reader }

func (f *failingReader) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if errors.Is(err, io.EOF) {
		return n, errors.New("connection reset")
	}
	return n, err
}

func blobKeepsPreviousOnFailedPut(t *testing.T, s blob.Store) {
	blobPut(t, s, "k", "previous", "text/plain")
	_, err := s.Put(t.Context(), "k", &failingReader{r: strings.NewReader("partial new content")}, blob.PutOptions{ContentType: "image/png"})
	if err == nil {
		t.Fatal("Put with a failing reader succeeded")
	}
	got, info := blobRead(t, s, "k")
	if got != "previous" || info.ContentType != "text/plain" {
		t.Errorf("after failed Put: %q %+v, want the previous blob", got, info)
	}
	_, err = s.Put(t.Context(), "fresh", &failingReader{r: strings.NewReader("partial")}, blob.PutOptions{})
	if err == nil {
		t.Fatal("Put with a failing reader succeeded")
	}
	if _, err := s.Head(t.Context(), "fresh"); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Head after failed first Put: %v, want ErrNotFound", err)
	}
}

func blobNeverShowsPartialWrites(t *testing.T, s blob.Store) {
	a := strings.Repeat("a", 256<<10)
	b := strings.Repeat("b", 64<<10)
	blobPut(t, s, "race", a, "")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; ctx.Err() == nil && i < 50; i++ {
			content := a
			if i%2 == 0 {
				content = b
			}
			if _, err := s.Put(ctx, "race", strings.NewReader(content), blob.PutOptions{}); err != nil && ctx.Err() == nil {
				t.Errorf("Put: %v", err)
				return
			}
		}
	})
	for range 50 {
		r, _, err := s.Get(t.Context(), "race")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		got, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !bytes.Equal(got, []byte(a)) && !bytes.Equal(got, []byte(b)) {
			t.Fatalf("read a partial blob of %d bytes", len(got))
		}
	}
	cancel()
	wg.Wait()
}

func blobRejectsInvalidKeys(t *testing.T, s blob.Store) {
	blobPut(t, s, "secret", "inside", "")
	invalid := []string{
		"", "/abs", "../secret", "a/../../secret", "a/./b", "a//b", "a/", "a\\b", "..\\secret",
		".hidden", "a/.b", "a b", "a%2F..", "a\x00b", "ä", "a?b", "a#b", "a:b", "C:/x",
		strings.Repeat("a", blob.MaxKeyLength+1),
	}
	for _, key := range invalid {
		if err := blob.ValidateKey(key); !errors.Is(err, blob.ErrInvalidKey) {
			t.Errorf("ValidateKey(%q) = %v, want ErrInvalidKey", key, err)
		}
		if _, err := s.Put(t.Context(), key, strings.NewReader("x"), blob.PutOptions{}); !errors.Is(err, blob.ErrInvalidKey) {
			t.Errorf("Put(%q) = %v, want ErrInvalidKey", key, err)
		}
		if r, _, err := s.Get(t.Context(), key); !errors.Is(err, blob.ErrInvalidKey) {
			if r != nil {
				_ = r.Close()
			}
			t.Errorf("Get(%q) = %v, want ErrInvalidKey", key, err)
		}
		if _, err := s.Head(t.Context(), key); !errors.Is(err, blob.ErrInvalidKey) {
			t.Errorf("Head(%q) = %v, want ErrInvalidKey", key, err)
		}
		if err := s.Delete(t.Context(), key); !errors.Is(err, blob.ErrInvalidKey) {
			t.Errorf("Delete(%q) = %v, want ErrInvalidKey", key, err)
		}
	}
	for _, key := range []string{"a", "media/0192f0c1-0000-7000-8000-000000000001/thumbnail.png", "A-Z_0.9", strings.Repeat("a", blob.MaxKeyLength)} {
		if err := blob.ValidateKey(key); err != nil {
			t.Errorf("ValidateKey(%q) = %v, want nil", key, err)
		}
	}
	if got, _ := blobRead(t, s, "secret"); got != "inside" {
		t.Errorf("secret = %q", got)
	}
}

func blobStopsOnCancelledContext(t *testing.T, s blob.Store) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Put(ctx, "k", strings.NewReader("x"), blob.PutOptions{}); err == nil {
		t.Error("Put with a cancelled context succeeded")
	}
	if _, err := s.Head(t.Context(), "k"); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Head after cancelled Put: %v, want ErrNotFound", err)
	}
}

func blobBuildsPublicURLs(t *testing.T, s blob.Store) {
	const key = "media/0192f0c1-0000-7000-8000-000000000001/full.jpg"
	u := s.PublicURL(key)
	if !strings.HasSuffix(u, "/"+key) || !strings.HasPrefix(u, "http") {
		t.Errorf("PublicURL(%q) = %q, want an http(s) URL ending in /%s", key, u, key)
	}
}
