// SPDX-License-Identifier: AGPL-3.0-only

package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/mycroblog/my/internal/app/blob"
)

const defaultContentType = "application/octet-stream"

// maxContentTypeSize bounds the media type that Put accepts: the store keeps it in the
// object's Content-Type header.
const maxContentTypeSize = 4 << 10

// DefaultPartSize is the size of the parts of a multipart upload, and the largest blob that one
// PutObject writes.
const DefaultPartSize = 8 << 20

// abortTimeout bounds the abort of a failed multipart upload, which runs after the Put's
// context may have ended.
const abortTimeout = 30 * time.Second

// Options configure a Store.
type Options struct {
	// Bucket is the bucket that holds the blobs; required.
	Bucket string
	// Endpoint is the URL of the S3-compatible API ("https://<account>.r2.cloudflarestorage.com",
	// "https://storage.googleapis.com", "http://minio:9000"); empty uses Amazon S3's endpoint
	// of the region.
	Endpoint string
	// PathStyle addresses the bucket in the path (https://endpoint/bucket/key) instead of the
	// host name (https://bucket.endpoint/key), as MinIO and other servers without a wildcard
	// host name need.
	PathStyle bool
	// BaseURL is the URL under which the HTTP server serves the public blobs, without a trailing
	// slash: PublicURL(key) is BaseURL + "/" + key. The server serves keys under
	// blob.PublicPrefix at /media/ from the bucket, so BaseURL is the origin of the media
	// ("https://example.com"), and the bucket itself stays private.
	BaseURL string
	// PartSize is the size of the parts of a multipart upload; zero means DefaultPartSize.
	// S3 refuses parts under 5 MiB but the last.
	PartSize int64
}

// Store implements blob.Store on a bucket. A blob is one object, named by its key, whose
// Content-Type header holds the blob's media type. Put writes a blob with one PutObject or with
// a multipart upload completed only after every part was written, so a reader sees the
// previous object or the new one, never a part, and a failed Put leaves the previous object.
type Store struct {
	client   *s3.Client
	presign  *s3.PresignClient
	bucket   string
	baseURL  string
	partSize int64
}

var (
	_ blob.Store  = (*Store)(nil)
	_ blob.Signer = (*Store)(nil)
)

// New returns a Store on the bucket of o, with a client of cfg (its region, credentials, and
// HTTP client) and o's endpoint. A custom endpoint without a region signs for us-east-1, which
// MinIO and most S3-compatible servers expect; Amazon S3 needs a region.
func New(cfg aws.Config, o Options) (*Store, error) {
	if o.Bucket == "" {
		return nil, errors.New("blob/s3: Options.Bucket is required")
	}
	if cfg.Region == "" {
		if o.Endpoint == "" {
			return nil, errors.New("blob/s3: no region: set blob_s3_region or AWS_REGION")
		}
		cfg.Region = "us-east-1"
	}
	if o.PartSize <= 0 {
		o.PartSize = DefaultPartSize
	}
	client := s3.NewFromConfig(cfg, func(so *s3.Options) {
		if o.Endpoint != "" {
			so.BaseEndpoint = aws.String(o.Endpoint)
		}
		so.UsePathStyle = o.PathStyle
		// The checksums that the SDK adds by default are headers and trailers that not every
		// S3-compatible server takes (R2 and the XML API of Cloud Storage refuse some); send and
		// check them only where the operation requires them.
		so.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		so.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	return &Store{
		client:   client,
		presign:  s3.NewPresignClient(client),
		bucket:   o.Bucket,
		baseURL:  strings.TrimSuffix(o.BaseURL, "/"),
		partSize: o.PartSize,
	}, nil
}

// Put implements blob.Store.
func (s *Store) Put(ctx context.Context, key string, r io.Reader, opts blob.PutOptions) (blob.Info, error) {
	if err := blob.ValidateKey(key); err != nil {
		return blob.Info{}, err
	}
	contentType := opts.ContentType
	if contentType == "" {
		contentType = defaultContentType
	}
	if len(contentType) > maxContentTypeSize {
		return blob.Info{}, fmt.Errorf("blob/s3: content type of %d bytes exceeds %d", len(contentType), maxContentTypeSize)
	}
	if _, _, err := mime.ParseMediaType(contentType); err != nil || strings.ContainsAny(contentType, "\r\n") {
		return blob.Info{}, fmt.Errorf("blob/s3: invalid content type %q", contentType)
	}
	if err := ctx.Err(); err != nil {
		return blob.Info{}, err
	}
	r = ctxReader{ctx: ctx, r: r}
	first, err := readPart(r, s.partSize)
	if err != nil {
		return blob.Info{}, fmt.Errorf("blob/s3: put %s: %w", key, err)
	}
	var next []byte
	if int64(len(first)) == s.partSize {
		if next, err = readPart(r, s.partSize); err != nil {
			return blob.Info{}, fmt.Errorf("blob/s3: put %s: %w", key, err)
		}
	}
	size := int64(len(first))
	if len(next) == 0 {
		_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: &s.bucket, Key: &key, Body: bytes.NewReader(first),
			ContentLength: aws.Int64(size), ContentType: &contentType,
		})
	} else {
		size, err = s.putMultipart(ctx, key, contentType, r, first, next)
	}
	if err != nil {
		return blob.Info{}, fmt.Errorf("blob/s3: put %s: %w", key, err)
	}
	return blob.Info{Key: key, ContentType: contentType, Size: size, ModTime: time.Now().UTC()}, nil
}

// readPart reads up to size bytes of r; fewer means r ended.
func readPart(r io.Reader, size int64) ([]byte, error) {
	var b bytes.Buffer
	_, err := b.ReadFrom(io.LimitReader(r, size))
	return b.Bytes(), err
}

// putMultipart writes first, next, and the rest of r as the parts of a multipart upload, and
// completes it only when every part is written; it aborts the upload when anything fails. It
// returns the size of the blob.
func (s *Store) putMultipart(ctx context.Context, key, contentType string, r io.Reader, first, next []byte) (size int64, err error) {
	created, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: &s.bucket, Key: &key, ContentType: &contentType,
	})
	if err != nil {
		return 0, err
	}
	uploadID := created.UploadId
	defer func() {
		if err != nil {
			actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), abortTimeout)
			defer cancel()
			_, _ = s.client.AbortMultipartUpload(actx, &s3.AbortMultipartUploadInput{Bucket: &s.bucket, Key: &key, UploadId: uploadID})
		}
	}()
	var parts []types.CompletedPart
	part := first
	for len(part) > 0 {
		n := int32(len(parts) + 1) //nolint:gosec // bounded by the size of the blob over the part size
		out, err := s.client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket: &s.bucket, Key: &key, UploadId: uploadID, PartNumber: aws.Int32(n),
			Body: bytes.NewReader(part), ContentLength: aws.Int64(int64(len(part))),
		})
		if err != nil {
			return 0, err
		}
		parts = append(parts, types.CompletedPart{ETag: out.ETag, PartNumber: aws.Int32(n)})
		size += int64(len(part))
		if next != nil {
			part, next = next, nil
			continue
		}
		if part, err = readPart(r, s.partSize); err != nil {
			return 0, err
		}
	}
	_, err = s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: &s.bucket, Key: &key, UploadId: uploadID,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	})
	return size, err
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
// object with a ranged GetObject, on the condition that the object is still the one that Get
// opened (its ETag), so that a reader never mixes two blobs.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, blob.Info, error) {
	if err := blob.ValidateKey(key); err != nil {
		return nil, blob.Info{}, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return nil, blob.Info{}, s.wrap("get", key, err)
	}
	info := blob.Info{
		Key: key, ContentType: aws.ToString(out.ContentType), Size: aws.ToInt64(out.ContentLength),
		ModTime: aws.ToTime(out.LastModified).UTC(),
	}
	return &object{s: s, ctx: ctx, key: key, etag: aws.ToString(out.ETag), size: info.Size, body: out.Body}, info, nil
}

// Head implements blob.Store.
func (s *Store) Head(ctx context.Context, key string) (blob.Info, error) {
	if err := blob.ValidateKey(key); err != nil {
		return blob.Info{}, err
	}
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return blob.Info{}, s.wrap("head", key, err)
	}
	return blob.Info{
		Key: key, ContentType: aws.ToString(out.ContentType), Size: aws.ToInt64(out.ContentLength),
		ModTime: aws.ToTime(out.LastModified).UTC(),
	}, nil
}

// Delete implements blob.Store.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := blob.ValidateKey(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
	// S3 answers 204 for a missing key; some compatible servers answer 404.
	if err != nil && !notFound(err) {
		return fmt.Errorf("blob/s3: delete %s: %w", key, err)
	}
	return nil
}

// PublicURL implements blob.Store.
func (s *Store) PublicURL(key string) string { return s.baseURL + "/" + key }

// SignedURL implements blob.Signer: a presigned GetObject URL of the bucket's endpoint.
func (s *Store) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := blob.ValidateKey(key); err != nil {
		return "", err
	}
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("blob/s3: sign %s: %w", key, err)
	}
	return req.URL, nil
}

// wrap names the operation and the key in err, and maps a missing object to blob.ErrNotFound.
func (s *Store) wrap(op, key string, err error) error {
	if notFound(err) {
		return fmt.Errorf("blob/s3: %s: %w", key, blob.ErrNotFound)
	}
	return fmt.Errorf("blob/s3: %s %s: %w", op, key, err)
}

// notFound reports whether err is the answer for a missing object: a 404, or the NoSuchKey
// error of a GetObject.
func notFound(err error) bool {
	var noKey *types.NoSuchKey
	if errors.As(err, &noKey) {
		return true
	}
	var re *awshttp.ResponseError
	return errors.As(err, &re) && re.HTTPStatusCode() == http.StatusNotFound
}

// object is the content of a blob that Get opened. It reads the open body from the start, and
// after a seek, the rest of the object from the new position with a ranged GetObject.
type object struct {
	s    *Store
	ctx  context.Context
	key  string
	etag string
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
		in := &s3.GetObjectInput{
			Bucket: &o.s.bucket, Key: &o.key, Range: aws.String(fmt.Sprintf("bytes=%d-", o.pos)),
		}
		if o.etag != "" {
			in.IfMatch = &o.etag
		}
		out, err := o.s.client.GetObject(o.ctx, in)
		if err != nil {
			return 0, o.s.wrap("get", o.key, err)
		}
		o.body = out.Body
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
		return 0, errors.New("blob/s3: invalid whence")
	}
	if pos < 0 {
		return 0, errors.New("blob/s3: negative position")
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
