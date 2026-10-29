// SPDX-License-Identifier: AGPL-3.0-only

package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/mycroblog/my/internal/app/blob"
)

const defaultContentType = "application/octet-stream"

// maxContentTypeSize bounds the media type that Put accepts: the store keeps it in the
// object's contentType, within the 8 KiB of metadata that Cloud Storage allows.
const maxContentTypeSize = 4 << 10

// DefaultChunkSize is the size of the chunks of a resumable upload, and the largest blob that one
// request writes.
const DefaultChunkSize = 8 << 20

// Options configure a Store.
type Options struct {
	// Bucket is the bucket that holds the blobs; required.
	Bucket string
	// BaseURL is the URL under which the HTTP server serves the public blobs, without a trailing
	// slash: PublicURL(key) is BaseURL + "/" + key. The server serves keys under
	// blob.PublicPrefix at /media/ from the bucket, so BaseURL is the origin of the media
	// ("https://example.com"), and the bucket itself stays private.
	BaseURL string
	// ChunkSize is the size of the chunks of a resumable upload, rounded up to a multiple of
	// 256 KiB; zero means DefaultChunkSize. Each Put holds one chunk in memory.
	ChunkSize int
	// ClientOptions are added to those of the client, after the ones that New sets; empty uses
	// Application Default Credentials and the endpoints of Google Cloud. Tests point the client
	// at a fake server with them.
	ClientOptions []option.ClientOption
}

// Store implements blob.Store on a bucket. A blob is one object, named by its key, whose
// contentType holds the blob's media type. Cloud Storage creates an object only when its upload
// completes, so a reader sees the previous object or the new one, never a part, and a failed Put
// leaves the previous object.
type Store struct {
	client    *storage.Client
	bucket    *storage.BucketHandle
	baseURL   string
	chunkSize int
}

var (
	_ blob.Store  = (*Store)(nil)
	_ blob.Signer = (*Store)(nil)
	_ io.Closer   = (*Store)(nil)
)

// New returns a Store on the bucket of o, with a client of the JSON API. Close releases it.
func New(ctx context.Context, o Options) (*Store, error) {
	if o.Bucket == "" {
		return nil, errors.New("blob/gcs: Options.Bucket is required")
	}
	if o.ChunkSize <= 0 {
		o.ChunkSize = DefaultChunkSize
	}
	// The client's own metrics would be exported to Cloud Monitoring; the server reports the
	// blob store's metrics itself.
	opts := append([]option.ClientOption{storage.WithJSONReads(), storage.WithDisabledClientMetrics()}, o.ClientOptions...)
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("blob/gcs: create the client: %w", err)
	}
	return &Store{
		client:    client,
		bucket:    client.Bucket(o.Bucket),
		baseURL:   strings.TrimSuffix(o.BaseURL, "/"),
		chunkSize: o.ChunkSize,
	}, nil
}

// Close releases the client.
func (s *Store) Close() error { return s.client.Close() }

// Put implements blob.Store. A blob of at most one chunk is written with one request, a larger
// one with a resumable upload, which Cloud Storage completes only after its last chunk.
func (s *Store) Put(ctx context.Context, key string, r io.Reader, opts blob.PutOptions) (blob.Info, error) {
	if err := blob.ValidateKey(key); err != nil {
		return blob.Info{}, err
	}
	contentType := opts.ContentType
	if contentType == "" {
		contentType = defaultContentType
	}
	if len(contentType) > maxContentTypeSize {
		return blob.Info{}, fmt.Errorf("blob/gcs: content type of %d bytes exceeds %d", len(contentType), maxContentTypeSize)
	}
	if _, _, err := mime.ParseMediaType(contentType); err != nil || strings.ContainsAny(contentType, "\r\n") {
		return blob.Info{}, fmt.Errorf("blob/gcs: invalid content type %q", contentType)
	}
	if err := ctx.Err(); err != nil {
		return blob.Info{}, err
	}
	// Cancelling the writer's context aborts the upload, so that a failed read never completes
	// an object with part of the blob.
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w := s.bucket.Object(key).NewWriter(wctx)
	w.ContentType = contentType
	w.ChunkSize = s.chunkSize
	if _, err := io.Copy(w, ctxReader{ctx: ctx, r: r}); err != nil {
		cancel()
		_ = w.Close()
		return blob.Info{}, fmt.Errorf("blob/gcs: put %s: %w", key, err)
	}
	if err := w.Close(); err != nil {
		return blob.Info{}, fmt.Errorf("blob/gcs: put %s: %w", key, err)
	}
	attrs := w.Attrs()
	return blob.Info{Key: key, ContentType: contentType, Size: attrs.Size, ModTime: attrs.Updated.UTC()}, nil
}

// ctxReader stops a read when ctx ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// Get implements blob.Store. The reader implements io.ReadSeeker: a seek reads the rest of the
// object from the generation that Get opened, so that a reader never mixes two blobs, and fails
// when the object was replaced or deleted since.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, blob.Info, error) {
	if err := blob.ValidateKey(key); err != nil {
		return nil, blob.Info{}, err
	}
	obj := s.bucket.Object(key)
	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, blob.Info{}, wrap("get", key, err)
	}
	info := blob.Info{
		Key: key, ContentType: r.Attrs.ContentType, Size: r.Attrs.Size, ModTime: r.Attrs.LastModified.UTC(),
	}
	o := &object{ctx: ctx, key: key, size: info.Size, body: r}
	o.obj = obj.Generation(r.Attrs.Generation)
	return o, info, nil
}

// Head implements blob.Store.
func (s *Store) Head(ctx context.Context, key string) (blob.Info, error) {
	if err := blob.ValidateKey(key); err != nil {
		return blob.Info{}, err
	}
	attrs, err := s.bucket.Object(key).Attrs(ctx)
	if err != nil {
		return blob.Info{}, wrap("head", key, err)
	}
	return blob.Info{Key: key, ContentType: attrs.ContentType, Size: attrs.Size, ModTime: attrs.Updated.UTC()}, nil
}

// Delete implements blob.Store.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := blob.ValidateKey(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.bucket.Object(key).Delete(ctx); err != nil && !notFound(err) {
		return fmt.Errorf("blob/gcs: delete %s: %w", key, err)
	}
	return nil
}

// PublicURL implements blob.Store.
func (s *Store) PublicURL(key string) string { return s.baseURL + "/" + key }

// SignedURL implements blob.Signer: a V4 signed URL of a GET of the object, signed with the
// private key of the credentials, or without one with the IAM signBlob method as the client's
// service account (roles/iam.serviceAccountTokenCreator on itself).
func (s *Store) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := blob.ValidateKey(key); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	u, err := s.bucket.SignedURL(key, &storage.SignedURLOptions{
		Scheme: storage.SigningSchemeV4, Method: http.MethodGet, Expires: time.Now().Add(ttl),
	})
	if err != nil {
		return "", fmt.Errorf("blob/gcs: sign %s: %w", key, err)
	}
	return u, nil
}

// wrap names the operation and the key in err, and maps a missing object to blob.ErrNotFound.
func wrap(op, key string, err error) error {
	if notFound(err) {
		return fmt.Errorf("blob/gcs: %s: %w", key, blob.ErrNotFound)
	}
	return fmt.Errorf("blob/gcs: %s %s: %w", op, key, err)
}

// notFound reports whether err is the answer for a missing object.
func notFound(err error) bool {
	if errors.Is(err, storage.ErrObjectNotExist) {
		return true
	}
	var ge *googleapi.Error
	return errors.As(err, &ge) && ge.Code == http.StatusNotFound
}

// object is the content of a blob that Get opened. It reads the open body from the start, and
// after a seek, the rest of the generation that Get opened from the new position.
type object struct {
	obj  *storage.ObjectHandle
	ctx  context.Context
	key  string
	size int64
	pos  int64
	body io.ReadCloser
}

// Read implements io.Reader.
func (o *object) Read(p []byte) (int, error) {
	if o.pos >= o.size {
		return 0, io.EOF
	}
	if o.body == nil {
		r, err := o.obj.NewRangeReader(o.ctx, o.pos, -1)
		if err != nil {
			return 0, wrap("get", o.key, err)
		}
		o.body = r
	}
	n, err := o.body.Read(p)
	o.pos += int64(n)
	if errors.Is(err, io.EOF) && o.pos < o.size {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

// Seek implements io.Seeker. It does no I/O: the next Read opens the object at the position.
func (o *object) Seek(offset int64, whence int) (int64, error) {
	var pos int64
	switch whence {
	case io.SeekStart:
		pos = offset
	case io.SeekCurrent:
		pos = o.pos + offset
	case io.SeekEnd:
		pos = o.size + offset
	default:
		return 0, errors.New("blob/gcs: invalid whence")
	}
	if pos < 0 {
		return 0, errors.New("blob/gcs: negative position")
	}
	if pos != o.pos && o.body != nil {
		_ = o.body.Close()
		o.body = nil
	}
	o.pos = pos
	return pos, nil
}

// Close implements io.Closer.
func (o *object) Close() error {
	if o.body == nil {
		return nil
	}
	err := o.body.Close()
	o.body = nil
	return err
}
