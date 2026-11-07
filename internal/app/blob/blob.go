// SPDX-License-Identifier: AGPL-3.0-only

// Package blob defines the BlobStore port (ADR-0005): the storage of media and other files
// that do not belong in the database. Every module that keeps files uses it; the adapters live
// in internal/adapter/blob/* and pass blobtest.BlobStoreContract.
//
// Keys are chosen by the server, never by users (ADR-0019 §8). ValidateKey states the key
// syntax that every adapter accepts, so that a key never names a path outside the store.
package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

var (
	// ErrNotFound reports a key that holds no blob.
	ErrNotFound = errors.New("blob: not found")
	// ErrInvalidKey reports a key that ValidateKey rejects.
	ErrInvalidKey = errors.New("blob: invalid key")
)

// MaxKeyLength is the longest key in bytes.
const MaxKeyLength = 512

// Info describes a stored blob.
type Info struct {
	Key string
	// ContentType is the media type given to Put.
	ContentType string
	// Size is the length in bytes.
	Size int64
	// ModTime is when the blob was last written, as far as the adapter knows it.
	ModTime time.Time
}

// PutOptions describe a blob being written.
type PutOptions struct {
	// ContentType is the media type to serve the blob with; "" stores
	// application/octet-stream.
	ContentType string
}

// Store is the BlobStore port.
//
// Writes are atomic: a reader sees either the previous blob of a key or the complete new one,
// never a partial write. Methods fail with an error wrapping ErrInvalidKey for a key that
// ValidateKey rejects, before any I/O.
type Store interface {
	// Put stores the content of r under key, replacing any blob there, and returns its Info.
	// When r fails, the previous blob of key stays.
	Put(ctx context.Context, key string, r io.Reader, opts PutOptions) (Info, error)
	// Get opens the blob of key; close the reader when done. It fails with ErrNotFound when
	// there is none. Adapters whose content can seek (a local file) return a reader that also
	// implements io.ReadSeeker, so that servers can answer range requests.
	Get(ctx context.Context, key string) (io.ReadCloser, Info, error)
	// Head returns the Info of the blob of key. It fails with ErrNotFound when there is none.
	Head(ctx context.Context, key string) (Info, error)
	// Delete removes the blob of key. Deleting a missing blob succeeds, so that deletion is
	// safe to repeat.
	Delete(ctx context.Context, key string) error
	// PublicURL returns the URL that serves the blob of key to anyone, for blobs under the
	// public prefix (PublicPrefix). It does no I/O and does not check that the blob exists.
	PublicURL(key string) string
}

// Signer is implemented by stores that can issue time-limited URLs for private blobs.
type Signer interface {
	// SignedURL returns a URL that serves the blob of key until ttl has passed.
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// PublicPrefix starts the keys of the blobs that anyone may read: the server serves them
// under /media/ and object storage may make them public. Every other key is private.
const PublicPrefix = "media/"

// ValidateKey reports whether key is a valid blob key: 1 to MaxKeyLength bytes of segments
// separated by "/", where each segment is one or more of [A-Za-z0-9._-], does not start with
// ".", and so is never "." or "..". Keys therefore have no leading, trailing, or doubled "/",
// no backslash, and no character that a file system or URL treats specially.
func ValidateKey(key string) error {
	if key == "" || len(key) > MaxKeyLength {
		return fmt.Errorf("%w: length must be 1 to %d bytes", ErrInvalidKey, MaxKeyLength)
	}
	for seg := range strings.SplitSeq(key, "/") {
		if seg == "" || seg[0] == '.' {
			return fmt.Errorf("%w: %q has an empty segment or one starting with a dot", ErrInvalidKey, key)
		}
		for i := range len(seg) {
			c := seg[i]
			ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-'
			if !ok {
				return fmt.Errorf("%w: %q contains %q", ErrInvalidKey, key, c)
			}
		}
	}
	return nil
}
