// SPDX-License-Identifier: AGPL-3.0-only

package fs_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/blob/fs"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/pkg/testkit"
)

func newStore(t *testing.T, dir string) *fs.Store {
	t.Helper()
	s, err := fs.New(fs.Options{Dir: dir, BaseURL: "https://example.com/"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// objectFile returns the file of key under dir: objects/<hh>/<sha256 of key>.
func objectFile(dir, key string) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	return filepath.Join(dir, "objects", h[:2], h)
}

func TestBlobStoreContract(t *testing.T) {
	testkit.BlobStoreContract(t, func(t *testing.T) blob.Store { return newStore(t, t.TempDir()) })
}

func TestPublicURLJoinsTheBaseURL(t *testing.T) {
	s := newStore(t, t.TempDir())
	if got, want := s.PublicURL("media/x/full.jpg"), "https://example.com/media/x/full.jpg"; got != want {
		t.Errorf("PublicURL = %q, want %q", got, want)
	}
}

// A symbolic link inside the store that points outside it is never followed.
func TestDoesNotFollowLinksOutOfTheStore(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	s := newStore(t, dir)
	const key = "escape/secret"
	file := objectFile(dir, key)
	if err := os.WriteFile(filepath.Join(outside, filepath.Base(file)), []byte("myblob1\n\x00\x00\x00\x0atext/plainoutside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Dir(file)); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if r, _, err := s.Get(t.Context(), key); err == nil {
		b, _ := io.ReadAll(r)
		_ = r.Close()
		t.Fatalf("Get through a link out of the store read %q", b)
	}
	const newKey = "escape/new"
	if filepath.Dir(objectFile(dir, newKey)) == filepath.Dir(file) {
		t.Fatal("the test needs keys in different directories")
	}
	if err := os.Symlink(outside, filepath.Dir(objectFile(dir, newKey))); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(t.Context(), newKey, strings.NewReader("x"), blob.PutOptions{}); err == nil {
		t.Fatal("Put through a link out of the store succeeded")
	}
	if _, err := os.Stat(filepath.Join(outside, filepath.Base(objectFile(dir, newKey)))); !os.IsNotExist(err) {
		t.Fatalf("a file was written outside the store: %v", err)
	}
}

// A blob is one file named after the hash of its key, readable only by the server's user, and
// nothing is left in tmp/.
func TestStoresOneFilePerBlob(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir)
	const key = "media/x/full.jpg"
	if _, err := s.Put(t.Context(), key, strings.NewReader("jpeg"), blob.PutOptions{ContentType: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(objectFile(dir, key))
	if err != nil || !strings.HasSuffix(string(b), "image/jpegjpeg") {
		t.Fatalf("object file = %q, %v", b, err)
	}
	st, err := os.Stat(objectFile(dir, key))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("object file mode = %v, %v; want 0600", st.Mode(), err)
	}
	assertTmpEmpty(t, dir)
	if err := s.Delete(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(objectFile(dir, key)); !os.IsNotExist(err) {
		t.Errorf("object file remains after Delete: %v", err)
	}
}

func assertTmpEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "tmp"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("tmp/ has %d leftovers, %v", len(entries), err)
	}
}

// failingReader returns its content and then an error.
type failingReader struct{ r io.Reader }

func (f *failingReader) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if errors.Is(err, io.EOF) {
		return n, errors.New("connection reset")
	}
	return n, err
}

// A failed Put removes its temporary file.
func TestFailedPutLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir)
	if _, err := s.Put(t.Context(), "k", &failingReader{r: strings.NewReader("private data")}, blob.PutOptions{}); err == nil {
		t.Fatal("Put with a failing reader succeeded")
	}
	assertTmpEmpty(t, dir)
	ctx, cancel := context.WithCancel(t.Context())
	r := io.MultiReader(strings.NewReader("private data"), readerFunc(func([]byte) (int, error) {
		cancel()
		return 0, errors.New("cancelled")
	}))
	if _, err := s.Put(ctx, "k", r, blob.PutOptions{}); err == nil {
		t.Fatal("Put with a cancelled context succeeded")
	}
	assertTmpEmpty(t, dir)
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// A context that ends after the content was read in full does not fail the Put.
func TestCancellationAfterTheContentDoesNotFailThePut(t *testing.T) {
	s := newStore(t, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	r := io.MultiReader(strings.NewReader("content"), readerFunc(func([]byte) (int, error) {
		cancel()
		return 0, io.EOF
	}))
	info, err := s.Put(ctx, "k", r, blob.PutOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if info.Size != int64(len("content")) || info.ContentType != "text/plain" {
		t.Errorf("Put info = %+v", info)
	}
	head, err := s.Head(t.Context(), "k")
	if err != nil || head != info {
		t.Errorf("Head = %+v, %v; want %+v", head, err, info)
	}
}

// Get returns a reader that seeks within the content, past the header.
func TestGetSeeksWithinTheContent(t *testing.T) {
	s := newStore(t, t.TempDir())
	if _, err := s.Put(t.Context(), "k", strings.NewReader("0123456789"), blob.PutOptions{ContentType: "text/plain"}); err != nil {
		t.Fatal(err)
	}
	rc, _, err := s.Get(t.Context(), "k")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	rs, ok := rc.(io.ReadSeeker)
	if !ok {
		t.Fatal("Get's reader is not an io.ReadSeeker")
	}
	if n, err := rs.Seek(-3, io.SeekEnd); err != nil || n != 7 {
		t.Fatalf("Seek = %d, %v", n, err)
	}
	if b, err := io.ReadAll(rs); err != nil || string(b) != "789" {
		t.Errorf("read after Seek = %q, %v", b, err)
	}
}
