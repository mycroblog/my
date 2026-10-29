// SPDX-License-Identifier: AGPL-3.0-only

package gcs_test

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/option"

	blobgcs "github.com/mycroblog/my/internal/adapter/blob/gcs"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/pkg/testkit"
)

// newKey returns an RSA key of the service account.
func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate a key: %v", err)
	}
	return key
}

// newFakeStore returns a Store on an in-memory fake of Cloud Storage, with chunks of chunkSize
// bytes, whose every request reaches the fake, without credentials: signing goes through the
// metadata server and signBlob of the fake.
func newFakeStore(t *testing.T, chunkSize int, opts ...option.ClientOption) (*blobgcs.Store, *fakeGCS) {
	t.Helper()
	fake, srv := newFakeGCS(t, "bucket", newKey(t))
	u, _ := url.Parse(srv.URL)
	// The metadata server of the fake names the service account that signs without a key.
	t.Setenv("GCE_METADATA_HOST", u.Host)
	t.Setenv("STORAGE_EMULATOR_HOST", "")
	hc := &http.Client{Transport: redirect{srv: u}, Timeout: 30 * time.Second}
	s, err := blobgcs.New(t.Context(), blobgcs.Options{
		Bucket: "bucket", BaseURL: "https://example.com/", ChunkSize: chunkSize,
		ClientOptions: append([]option.ClientOption{option.WithHTTPClient(hc)}, opts...),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, fake
}

func TestBlobStoreContract(t *testing.T) {
	testkit.BlobStoreContract(t, func(t *testing.T) blob.Store {
		s, _ := newFakeStore(t, 0)
		return s
	})
}

// A blob larger than a chunk is written with a resumable upload, read back whole, and seeks.
func TestResumableUpload(t *testing.T) {
	s, fake := newFakeStore(t, 256<<10)
	content := bytes.Repeat([]byte("0123456789abcdef"), (600<<10)/16+3)
	info, err := s.Put(t.Context(), "media/big/full.mp4", bytes.NewReader(content), blob.PutOptions{ContentType: "video/mp4"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if info.Size != int64(len(content)) || info.ContentType != "video/mp4" {
		t.Errorf("Put info = %+v", info)
	}
	if got := len(fake.requestsMatching("/upload/storage/v1/b/bucket/o?", "upload_id=")); got != 3 {
		t.Errorf("uploaded %d chunks, want 3", got)
	}
	r, info, err := s.Get(t.Context(), "media/big/full.mp4")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = r.Close() }()
	if info.Size != int64(len(content)) || info.ContentType != "video/mp4" {
		t.Errorf("Get info = %+v", info)
	}
	rs, ok := r.(io.ReadSeeker)
	if !ok {
		t.Fatal("Get's reader does not seek")
	}
	head := make([]byte, 100)
	if _, err := io.ReadFull(rs, head); err != nil || !bytes.Equal(head, content[:100]) {
		t.Fatalf("read the start: %v", err)
	}
	off := int64(len(content)) - 1000
	if _, err := rs.Seek(off, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	tail, err := io.ReadAll(rs)
	if err != nil || !bytes.Equal(tail, content[off:]) {
		t.Fatalf("read after Seek: %d bytes, %v", len(tail), err)
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	all, err := io.ReadAll(rs)
	if err != nil || !bytes.Equal(all, content) {
		t.Fatalf("read after a seek to the start: %d bytes, %v", len(all), err)
	}
}

// failAfter returns n bytes and then an error.
type failAfter struct{ n int }

func (f *failAfter) Read(p []byte) (int, error) {
	if f.n == 0 {
		return 0, errors.New("connection reset")
	}
	k := min(len(p), f.n)
	f.n -= k
	return k, nil
}

// A reader that fails after some chunks leaves the resumable upload unfinished and keeps the
// previous blob.
func TestFailedResumableUploadKeepsThePreviousBlob(t *testing.T) {
	s, fake := newFakeStore(t, 256<<10)
	if _, err := s.Put(t.Context(), "k", strings.NewReader("previous"), blob.PutOptions{ContentType: "text/plain"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := s.Put(t.Context(), "k", &failAfter{n: 700 << 10}, blob.PutOptions{}); err == nil {
		t.Fatal("Put with a failing reader succeeded")
	}
	if fake.openUploads() != 1 {
		t.Errorf("%d open uploads, want the one that failed", fake.openUploads())
	}
	r, info, err := s.Get(t.Context(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = r.Close() }()
	if b, _ := io.ReadAll(r); string(b) != "previous" || info.ContentType != "text/plain" {
		t.Errorf("after a failed Put: %q %+v", b, info)
	}
}

// A reader that seeks after the blob was replaced fails rather than mixing the two blobs.
func TestSeekAfterOverwriteFails(t *testing.T) {
	s, _ := newFakeStore(t, 0)
	if _, err := s.Put(t.Context(), "k", strings.NewReader("first content"), blob.PutOptions{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	r, _, err := s.Get(t.Context(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = r.Close() }()
	if _, err := s.Put(t.Context(), "k", strings.NewReader("second content"), blob.PutOptions{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	rs := r.(io.ReadSeeker)
	if _, err := rs.Seek(6, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if b, err := io.ReadAll(rs); err == nil {
		t.Errorf("read %q of a replaced blob after a seek", b)
	}
}

func TestPublicURLJoinsTheBaseURL(t *testing.T) {
	s, _ := newFakeStore(t, 0)
	if got, want := s.PublicURL("media/x/full.jpg"), "https://example.com/media/x/full.jpg"; got != want {
		t.Errorf("PublicURL = %q, want %q", got, want)
	}
}

// checkSignedURL checks that u is a V4 signed GET of uploads/x of the bucket for 5 minutes, by
// email, with only the host signed, and that key signed it.
func checkSignedURL(t *testing.T, u, email string, key *rsa.PrivateKey) {
	t.Helper()
	p, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse %q: %v", u, err)
	}
	q := p.Query()
	if p.Scheme != "https" || p.Host != "storage.googleapis.com" || p.Path != "/bucket/uploads/x" {
		t.Errorf("SignedURL = %q, want https://storage.googleapis.com/bucket/uploads/x", u)
	}
	if got := q.Get("X-Goog-Algorithm"); got != "GOOG4-RSA-SHA256" {
		t.Errorf("X-Goog-Algorithm = %q", got)
	}
	// The library counts the whole seconds left from its own clock, so a second may have passed.
	if got := q.Get("X-Goog-Expires"); got != "300" && got != "299" {
		t.Errorf("X-Goog-Expires = %q, want 300", got)
	}
	if got := q.Get("X-Goog-SignedHeaders"); got != "host" {
		t.Errorf("X-Goog-SignedHeaders = %q, want host", got)
	}
	if got := q.Get("X-Goog-Credential"); !strings.HasPrefix(got, email+"/") || !strings.HasSuffix(got, "/auto/storage/goog4_request") {
		t.Errorf("X-Goog-Credential = %q", got)
	}
	date, err := time.Parse("20060102T150405Z", q.Get("X-Goog-Date"))
	if err != nil || time.Since(date) > time.Minute || time.Until(date) > time.Minute {
		t.Errorf("X-Goog-Date = %q", q.Get("X-Goog-Date"))
	}
	// The signature is the key's over the string to sign of the canonical GET request.
	sig, err := hex.DecodeString(q.Get("X-Goog-Signature"))
	if err != nil {
		t.Fatalf("X-Goog-Signature: %v", err)
	}
	q.Del("X-Goog-Signature")
	canonical := strings.Join([]string{
		"GET", p.EscapedPath(), strings.ReplaceAll(q.Encode(), "+", "%20"), "host:" + p.Host, "", "host", "UNSIGNED-PAYLOAD",
	}, "\n")
	reqSum := sha256.Sum256([]byte(canonical))
	scope := strings.TrimPrefix(q.Get("X-Goog-Credential"), email+"/")
	toSign := strings.Join([]string{"GOOG4-RSA-SHA256", q.Get("X-Goog-Date"), scope, hex.EncodeToString(reqSum[:])}, "\n")
	sum := sha256.Sum256([]byte(toSign))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Errorf("the signature does not verify with the service account's key: %v", err)
	}
}

// Without a private key, the signed URL is signed with signBlob as the service account of the
// metadata server.
func TestSignedURLWithoutAKeyUsesSignBlob(t *testing.T) {
	s, fake := newFakeStore(t, 0)
	u, err := s.SignedURL(t.Context(), "uploads/x", 5*time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	checkSignedURL(t, u, serviceAccount, fake.key)
	if got := fake.requestsMatching("POST iamcredentials.googleapis.com /v1/projects/-/serviceAccounts/" + serviceAccount + ":signBlob"); len(got) != 1 {
		t.Errorf("signBlob requests = %v, want 1", got)
	}
	if _, err := s.SignedURL(t.Context(), "../x", time.Minute); !errors.Is(err, blob.ErrInvalidKey) {
		t.Errorf("SignedURL of an invalid key: %v, want ErrInvalidKey", err)
	}
}

// With the private key of a service account in the credentials, the signed URL is signed
// locally, without signBlob.
func TestSignedURLWithAKeySignsLocally(t *testing.T) {
	key := newKey(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal the key: %v", err)
	}
	const email = "keyed@project.iam.gserviceaccount.com"
	creds, err := json.Marshal(map[string]string{ //nolint:gosec // a key generated for the test
		"type": "service_account", "project_id": "project", "private_key_id": "k1", "client_email": email,
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
		"token_uri":   "https://oauth2.googleapis.com/token",
	})
	if err != nil {
		t.Fatalf("marshal the credentials: %v", err)
	}
	s, fake := newFakeStore(t, 0, option.WithAuthCredentialsJSON(option.ServiceAccount, creds))
	u, err := s.SignedURL(t.Context(), "uploads/x", 5*time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	checkSignedURL(t, u, email, key)
	if got := fake.requestsMatching("signBlob"); len(got) != 0 {
		t.Errorf("signed with signBlob: %v", got)
	}
}

func TestNewRequiresABucket(t *testing.T) {
	if _, err := blobgcs.New(t.Context(), blobgcs.Options{}); err == nil {
		t.Error("New without a bucket succeeded")
	}
}
