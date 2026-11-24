// SPDX-License-Identifier: AGPL-3.0-only

package s3_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"

	blobs3 "github.com/mycroblog/my/internal/adapter/blob/s3"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/pkg/testkit/blobtest"
	"github.com/mycroblog/my/pkg/testkit/s3test"
)

// awsConfig returns the configuration of a client with static credentials in region.
func awsConfig(region, keyID, secret string) aws.Config {
	return aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(keyID, secret, ""),
		HTTPClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// newFakeStore returns a Store on an in-memory fake of S3, with parts of partSize bytes.
func newFakeStore(t *testing.T, partSize int64) (*blobs3.Store, *fakeS3) {
	t.Helper()
	fake, srv := newFakeS3(t, "bucket")
	s, err := blobs3.New(awsConfig("", "key", "secret"), blobs3.Options{
		Bucket: "bucket", Endpoint: srv.URL, PathStyle: true, BaseURL: "https://example.com/", PartSize: partSize,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, fake
}

func TestBlobStoreContract(t *testing.T) {
	blobtest.BlobStoreContract(t, func(t *testing.T) blob.Store {
		s, _ := newFakeStore(t, 0)
		return s
	})
}

// The contract on a real S3-compatible server (s3test), with TEST_S3=container.
func TestBlobStoreContractOnS3Server(t *testing.T) {
	srv := s3test.New(t)
	blobtest.BlobStoreContract(t, func(t *testing.T) blob.Store {
		s, err := blobs3.New(awsConfig(s3test.Region, s3test.AccessKeyID, s3test.SecretAccessKey), blobs3.Options{
			Bucket: srv.NewBucket(t), Endpoint: srv.Endpoint, PathStyle: true, BaseURL: "https://example.com",
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return s
	})
}

// A blob larger than a part is written with a multipart upload, read back whole, and seeks.
func TestMultipartUploadOnS3Server(t *testing.T) {
	srv := s3test.New(t)
	s, err := blobs3.New(awsConfig(s3test.Region, s3test.AccessKeyID, s3test.SecretAccessKey), blobs3.Options{
		Bucket: srv.NewBucket(t), Endpoint: srv.Endpoint, PathStyle: true, BaseURL: "https://example.com",
		PartSize: 5 << 20,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	content := bytes.Repeat([]byte("0123456789abcdef"), (11<<20)/16+3)
	checkMultipart(t, s, content)
}

func checkMultipart(t *testing.T, s *blobs3.Store, content []byte) {
	t.Helper()
	info, err := s.Put(t.Context(), "media/big/full.mp4", io.MultiReader(bytes.NewReader(content)), blob.PutOptions{ContentType: "video/mp4"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if info.Size != int64(len(content)) || info.ContentType != "video/mp4" {
		t.Errorf("Put info = %+v", info)
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

func TestMultipartUpload(t *testing.T) {
	s, fake := newFakeStore(t, 1000)
	checkMultipart(t, s, bytes.Repeat([]byte("abcdefghij"), 345))
	if got := len(fake.requestsMatching("PUT media/big/full.mp4", "partNumber=")); got != 4 {
		t.Errorf("uploaded %d parts, want 4", got)
	}
}

// A blob of exactly one part is written with one PutObject.
func TestBlobOfOnePartIsOnePut(t *testing.T) {
	s, fake := newFakeStore(t, 1000)
	if _, err := s.Put(t.Context(), "k", bytes.NewReader(make([]byte, 1000)), blob.PutOptions{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got := fake.requestsMatching("POST"); len(got) != 0 {
		t.Errorf("started a multipart upload: %v", got)
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

// A reader that fails after some parts aborts the multipart upload and keeps the previous blob.
func TestFailedMultipartUploadIsAborted(t *testing.T) {
	s, fake := newFakeStore(t, 1000)
	if _, err := s.Put(t.Context(), "k", strings.NewReader("previous"), blob.PutOptions{ContentType: "text/plain"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := s.Put(t.Context(), "k", &failAfter{n: 3500}, blob.PutOptions{}); err == nil {
		t.Fatal("Put with a failing reader succeeded")
	}
	if aborted, open := fake.uploadCounts(); aborted != 1 || open != 0 {
		t.Errorf("aborted %d uploads, %d open; want 1 and 0", aborted, open)
	}
	r, _, err := s.Get(t.Context(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = r.Close() }()
	if b, _ := io.ReadAll(r); string(b) != "previous" {
		t.Errorf("after a failed Put: %q", b)
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

// newPrefixedStore returns a Store under prefix on the fake bucket of srv, with parts of partSize bytes.
func newPrefixedStore(t *testing.T, endpoint, prefix string, partSize int64) *blobs3.Store {
	t.Helper()
	s, err := blobs3.New(awsConfig("", "key", "secret"), blobs3.Options{
		Bucket: "bucket", Endpoint: endpoint, PathStyle: true, BaseURL: "https://example.com", Prefix: prefix, PartSize: partSize,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestBlobStoreContractUnderAPrefix(t *testing.T) {
	blobtest.BlobStoreContract(t, func(t *testing.T) blob.Store {
		_, srv := newFakeS3(t, "bucket")
		return newPrefixedStore(t, srv.URL, "tenant-a/", 0)
	})
}

func TestSharedBucketContract(t *testing.T) {
	blobtest.SharedBucketContract(t, func(t *testing.T) (blob.Store, blob.Store) {
		_, srv := newFakeS3(t, "bucket")
		return newPrefixedStore(t, srv.URL, "tenant-a/", 0), newPrefixedStore(t, srv.URL, "hosting/tenant-b/", 0)
	})
}

// The contract of a shared bucket on a real S3-compatible server (s3test), with TEST_S3=container.
func TestSharedBucketContractOnS3Server(t *testing.T) {
	srv := s3test.New(t)
	blobtest.SharedBucketContract(t, func(t *testing.T) (blob.Store, blob.Store) {
		bucket := srv.NewBucket(t)
		stores := make([]blob.Store, 2)
		for i, prefix := range []string{"tenant-a/", "hosting/tenant-b/"} {
			s, err := blobs3.New(awsConfig(s3test.Region, s3test.AccessKeyID, s3test.SecretAccessKey), blobs3.Options{
				Bucket: bucket, Endpoint: srv.Endpoint, PathStyle: true, BaseURL: "https://example.com", Prefix: prefix,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			stores[i] = s
		}
		return stores[0], stores[1]
	})
}

// Every object of a prefixed store, a multipart upload's too, is named under the prefix, and its
// signed URL names that object.
func TestPrefixNamesEveryObject(t *testing.T) {
	fake, srv := newFakeS3(t, "bucket")
	s := newPrefixedStore(t, srv.URL, "tenant-a/", 1000)
	checkMultipart(t, s, bytes.Repeat([]byte("abcdefghij"), 345))
	if _, err := s.Put(t.Context(), "uploads/x", strings.NewReader("x"), blob.PutOptions{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	for _, r := range fake.requestsMatching() {
		if _, key, _ := strings.Cut(r, " "); !strings.HasPrefix(key, "tenant-a/") {
			t.Errorf("request %q names an object outside the prefix", r)
		}
	}
	u, err := s.SignedURL(t.Context(), "uploads/x", time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	if p, err := url.Parse(u); err != nil || p.Path != "/bucket/tenant-a/uploads/x" {
		t.Errorf("SignedURL = %q, want the object under the prefix", u)
	}
	if got, want := s.PublicURL("media/x/full.jpg"), "https://example.com/media/x/full.jpg"; got != want {
		t.Errorf("PublicURL = %q, want %q without the prefix", got, want)
	}
}

func TestNewRefusesAnInvalidPrefix(t *testing.T) {
	for _, prefix := range []string{"tenant-a", "/tenant-a/", "a//", "../a/", "tenant a/"} {
		if _, err := blobs3.New(awsConfig("eu-west-1", "k", "s"), blobs3.Options{Bucket: "b", Prefix: prefix}); err == nil {
			t.Errorf("New with the prefix %q succeeded", prefix)
		}
	}
}

func TestPublicURLJoinsTheBaseURL(t *testing.T) {
	s, _ := newFakeStore(t, 0)
	if got, want := s.PublicURL("media/x/full.jpg"), "https://example.com/media/x/full.jpg"; got != want {
		t.Errorf("PublicURL = %q, want %q", got, want)
	}
}

func TestSignedURLIsAPresignedGet(t *testing.T) {
	s, _ := newFakeStore(t, 0)
	u, err := s.SignedURL(t.Context(), "uploads/x", 5*time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	p, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse %q: %v", u, err)
	}
	if p.Path != "/bucket/uploads/x" || p.Query().Get("X-Amz-Expires") != "300" || p.Query().Get("X-Amz-Signature") == "" {
		t.Errorf("SignedURL = %q", u)
	}
	if _, err := s.SignedURL(t.Context(), "../x", time.Minute); !errors.Is(err, blob.ErrInvalidKey) {
		t.Errorf("SignedURL of an invalid key: %v, want ErrInvalidKey", err)
	}
}

func TestNewRequiresABucketAndARegion(t *testing.T) {
	if _, err := blobs3.New(awsConfig("eu-west-1", "k", "s"), blobs3.Options{}); err == nil {
		t.Error("New without a bucket succeeded")
	}
	if _, err := blobs3.New(awsConfig("", "k", "s"), blobs3.Options{Bucket: "b"}); err == nil {
		t.Error("New on Amazon S3 without a region succeeded")
	}
	if _, err := blobs3.New(awsConfig("", "k", "s"), blobs3.Options{Bucket: "b", Endpoint: "http://127.0.0.1:9000"}); err != nil {
		t.Errorf("New with an endpoint and no region: %v", err)
	}
}
