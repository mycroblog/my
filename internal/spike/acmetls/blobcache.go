// SPDX-License-Identifier: AGPL-3.0-only

package acmetls

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"golang.org/x/crypto/acme/autocert"

	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/kernel"
)

// maxEntry bounds what Get reads: an autocert entry is a key and a chain, a few KiB.
const maxEntry = 1 << 20

// BlobCache implements autocert.Cache over a blob.Store, sealing every value with the Sealer.
//
// Key layout: Prefix + "/" + base64url(name). autocert's names contain "+" ("acme_account+key",
// "example.com+rsa", "<token>+http-01"), which blob.ValidateKey rejects, and base64url's alphabet
// is a subset of what it accepts. Prefix is outside blob.PublicPrefix ("media/"), so no route
// serves it. The AAD is the full blob key, so a value copied to another key fails to open.
type BlobCache struct {
	Store  blob.Store
	Sealer *kernel.Sealer
	// Prefix is for example "tls/acme/acme-v02.api.letsencrypt.org".
	Prefix string

	Gets, Puts atomic.Int64
}

var _ autocert.Cache = (*BlobCache)(nil)

// BlobKey returns the blob key of an autocert name.
func (c *BlobCache) BlobKey(name string) string {
	return c.Prefix + "/" + base64.RawURLEncoding.EncodeToString([]byte(name))
}

func blobAAD(key string) []byte { return kernel.AAD("blob", key) }

// Get implements autocert.Cache.
func (c *BlobCache) Get(ctx context.Context, name string) ([]byte, error) {
	c.Gets.Add(1)
	key := c.BlobKey(name)
	rc, _, err := c.Store.Get(ctx, key)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, autocert.ErrCacheMiss
	}
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	sealed, err := io.ReadAll(io.LimitReader(rc, maxEntry+1))
	if err != nil {
		return nil, err
	}
	if len(sealed) > maxEntry {
		return nil, fmt.Errorf("acmetls: %s is larger than %d bytes", key, maxEntry)
	}
	return c.Sealer.Open(ctx, sealed, blobAAD(key))
}

// Put implements autocert.Cache.
func (c *BlobCache) Put(ctx context.Context, name string, data []byte) error {
	c.Puts.Add(1)
	key := c.BlobKey(name)
	sealed, err := c.Sealer.Seal(ctx, data, blobAAD(key))
	if err != nil {
		return err
	}
	_, err = c.Store.Put(ctx, key, bytes.NewReader(sealed), blob.PutOptions{ContentType: "application/octet-stream"})
	return err
}

// Delete implements autocert.Cache.
func (c *BlobCache) Delete(ctx context.Context, name string) error {
	return c.Store.Delete(ctx, c.BlobKey(name))
}
