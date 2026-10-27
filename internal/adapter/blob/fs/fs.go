// SPDX-License-Identifier: AGPL-3.0-only

package fs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/mycroblog/my/internal/app/blob"
)

// Directories under the store's root. A blob lives in one file, objects/<hh>/<hash>, where
// <hash> is the hexadecimal SHA-256 of its key and <hh> its first two characters; writes go to
// tmp/ first and are renamed into place.
const (
	dirObjects = "objects"
	dirTmp     = "tmp"
)

const defaultContentType = "application/octet-stream"

// A blob's file starts with a header: fileMagic, the length of the media type as a big-endian
// uint32, and the media type; the content follows.
const fileMagic = "myblob1\n"

const headerFixedSize = len(fileMagic) + 4

// maxContentTypeSize bounds the media type that Put accepts and Get and Head read.
const maxContentTypeSize = 64 << 10

// fileMode is the permission of every file the store writes: blobs include private data.
const fileMode = 0o600

// Options configure a Store.
type Options struct {
	// Dir is the root directory of the store; it is created when missing.
	Dir string
	// BaseURL is the URL under which the HTTP server serves the public blobs, without a trailing
	// slash: PublicURL(key) is BaseURL + "/" + key. The server serves keys under
	// blob.PublicPrefix at /media/, so BaseURL is the instance origin ("https://example.com").
	BaseURL string
}

// Store implements blob.Store on a directory. Every file operation goes through an os.Root
// opened on the directory, so that no key, even one that slipped past blob.ValidateKey, and no
// symbolic link reaches a file outside it.
//
// A blob's content and media type are one file named after the hash of its key, so that every
// valid key fits the file system's limits on a name, a key may be a prefix of another, and one
// rename publishes both together. Put writes the file under tmp/, syncs it, and renames it into
// place, which is atomic on the same file system: a reader sees the previous blob or the new
// one, never a part and never one blob's content with another's media type. Several processes
// (web and worker) may share the directory.
type Store struct {
	root    *os.Root
	baseURL string
}

var _ blob.Store = (*Store)(nil)

// New opens the store in o.Dir, creating the directory when missing. Close the Store to
// release the directory.
func New(o Options) (*Store, error) {
	if o.Dir == "" {
		return nil, errors.New("blob/fs: Options.Dir is required")
	}
	if err := os.MkdirAll(o.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("blob/fs: create %s: %w", o.Dir, err)
	}
	root, err := os.OpenRoot(o.Dir)
	if err != nil {
		return nil, fmt.Errorf("blob/fs: open %s: %w", o.Dir, err)
	}
	for _, d := range []string{dirObjects, dirTmp} {
		if err := root.MkdirAll(d, 0o750); err != nil {
			_ = root.Close()
			return nil, fmt.Errorf("blob/fs: create %s: %w", filepath.Join(o.Dir, d), err)
		}
	}
	return &Store{root: root, baseURL: strings.TrimSuffix(o.BaseURL, "/")}, nil
}

// Close releases the directory.
func (s *Store) Close() error { return s.root.Close() }

// objectPath returns the file of key: objects/<hh>/<hash>.
func objectPath(key string) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	return filepath.Join(dirObjects, h[:2], h)
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
		return blob.Info{}, fmt.Errorf("blob/fs: content type of %d bytes exceeds %d", len(contentType), maxContentTypeSize)
	}
	if _, _, err := mime.ParseMediaType(contentType); err != nil || strings.ContainsAny(contentType, "\r\n") {
		return blob.Info{}, fmt.Errorf("blob/fs: invalid content type %q", contentType)
	}
	if err := ctx.Err(); err != nil {
		return blob.Info{}, err
	}
	tmp, st, err := s.writeTemp(ctx, contentType, r)
	if err != nil {
		return blob.Info{}, fmt.Errorf("blob/fs: put %s: %w", key, err)
	}
	// The write is complete once the file is renamed into place; ctx no longer matters then.
	if err := s.publish(tmp, objectPath(key)); err != nil {
		_ = s.root.Remove(tmp)
		return blob.Info{}, fmt.Errorf("blob/fs: put %s: %w", key, err)
	}
	// A rename keeps the file's modification time and size.
	return blob.Info{
		Key:         key,
		ContentType: contentType,
		Size:        st.Size() - int64(headerFixedSize+len(contentType)),
		ModTime:     st.ModTime().UTC(),
	}, nil
}

// writeTemp writes the header and the content of r to a new file under tmp/, synced to disk,
// and returns its name and its FileInfo. It removes the file when it fails.
func (s *Store) writeTemp(ctx context.Context, contentType string, r io.Reader) (_ string, _ iofs.FileInfo, err error) {
	var b [16]byte
	_, _ = rand.Read(b[:])
	name := filepath.Join(dirTmp, hex.EncodeToString(b[:]))
	f, err := s.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return "", nil, err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = s.root.Remove(name)
		}
	}()
	header := make([]byte, 0, headerFixedSize+len(contentType))
	header = append(header, fileMagic...)
	header = binary.BigEndian.AppendUint32(header, uint32(len(contentType))) //nolint:gosec // bounded by maxContentTypeSize
	header = append(header, contentType...)
	if _, err = f.Write(header); err != nil {
		return "", nil, err
	}
	if _, err = io.Copy(f, ctxReader{ctx: ctx, r: r}); err != nil {
		return "", nil, err
	}
	if err = f.Sync(); err != nil {
		return "", nil, err
	}
	st, err := f.Stat()
	if err != nil {
		return "", nil, err
	}
	if err = f.Close(); err != nil {
		return "", nil, err
	}
	return name, st, nil
}

// publish moves the temporary file tmp to name, creating name's directory, which is atomic on
// the same file system. The directories under objects/ are never removed, so no Delete races
// with it.
func (s *Store) publish(tmp, name string) error {
	if err := s.root.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		return err
	}
	return s.root.Rename(tmp, name)
}

// ctxReader stops a copy when ctx ends.
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

// object is the content of a blob's open file: it reads, seeks, and closes the file past its
// header, so that it implements io.ReadSeeker.
type object struct {
	*io.SectionReader
	f *os.File
}

func (o object) Close() error { return o.f.Close() }

// Get implements blob.Store. The reader implements io.ReadSeeker.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, blob.Info, error) {
	f, info, err := s.open(ctx, key)
	if err != nil {
		return nil, blob.Info{}, err
	}
	offset := int64(headerFixedSize + len(info.ContentType))
	return object{SectionReader: io.NewSectionReader(f, offset, info.Size), f: f}, info, nil
}

// Head implements blob.Store.
func (s *Store) Head(ctx context.Context, key string) (blob.Info, error) {
	f, info, err := s.open(ctx, key)
	if err != nil {
		return blob.Info{}, err
	}
	_ = f.Close()
	return info, nil
}

// open opens the file of key and reads its header.
func (s *Store) open(ctx context.Context, key string) (*os.File, blob.Info, error) {
	if err := blob.ValidateKey(key); err != nil {
		return nil, blob.Info{}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, blob.Info{}, err
	}
	f, err := s.root.Open(objectPath(key))
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return nil, blob.Info{}, fmt.Errorf("blob/fs: %s: %w", key, blob.ErrNotFound)
		}
		return nil, blob.Info{}, fmt.Errorf("blob/fs: open %s: %w", key, err)
	}
	info, err := readInfo(f, key)
	if err != nil {
		_ = f.Close()
		return nil, blob.Info{}, err
	}
	return f, info, nil
}

// readInfo reads the header of a blob's file.
func readInfo(f *os.File, key string) (blob.Info, error) {
	st, err := f.Stat()
	if err != nil {
		return blob.Info{}, fmt.Errorf("blob/fs: stat %s: %w", key, err)
	}
	if !st.Mode().IsRegular() {
		return blob.Info{}, fmt.Errorf("blob/fs: %s: %w", key, blob.ErrNotFound)
	}
	var fixed [headerFixedSize]byte
	if _, err := f.ReadAt(fixed[:], 0); err != nil || string(fixed[:len(fileMagic)]) != fileMagic {
		return blob.Info{}, fmt.Errorf("blob/fs: %s: not a blob file", key)
	}
	n := binary.BigEndian.Uint32(fixed[len(fileMagic):])
	if n > maxContentTypeSize || int64(headerFixedSize)+int64(n) > st.Size() {
		return blob.Info{}, fmt.Errorf("blob/fs: %s: corrupt header", key)
	}
	contentType := make([]byte, n)
	if _, err := f.ReadAt(contentType, int64(headerFixedSize)); err != nil {
		return blob.Info{}, fmt.Errorf("blob/fs: read %s: %w", key, err)
	}
	return blob.Info{
		Key:         key,
		ContentType: string(contentType),
		Size:        st.Size() - int64(headerFixedSize) - int64(n),
		ModTime:     st.ModTime().UTC(),
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
	if err := s.root.Remove(objectPath(key)); err != nil && !errors.Is(err, iofs.ErrNotExist) {
		return fmt.Errorf("blob/fs: delete %s: %w", key, err)
	}
	return nil
}

// PublicURL implements blob.Store.
func (s *Store) PublicURL(key string) string { return s.baseURL + "/" + key }
