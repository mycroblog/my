// SPDX-License-Identifier: AGPL-3.0-only

package acmetls

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/acme/autocert"
	_ "modernc.org/sqlite"

	"github.com/mycroblog/my/internal/adapter/blob/fs"
	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	kvdb "github.com/mycroblog/my/internal/adapter/kv/db"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
)

// countingKM counts UnwrapKey calls, which a KMS would bill.
type countingKM struct {
	kernel.KeyManager
	unwraps atomic.Int64
}

func (c *countingKM) UnwrapKey(ctx context.Context, id string, w, aad []byte) ([]byte, error) {
	c.unwraps.Add(1)
	return c.KeyManager.UnwrapKey(ctx, id, w, aad)
}

func TestBlobCacheKeysAndAAD(t *testing.T) {
	ctx := t.Context()
	store, err := fs.New(fs.Options{Dir: t.TempDir(), BaseURL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	c := &BlobCache{Store: store, Sealer: kernel.NewSealer(testkit.NewKeyManager("local:test")), Prefix: "tls/acme/acme-v02.api.letsencrypt.org"}
	for _, name := range []string{"acme_account+key", "example.com", "example.com+rsa", "AbC-_xyz+http-01", "example.com+token"} {
		key := c.BlobKey(name)
		if err := blob.ValidateKey(key); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if bytes.HasPrefix([]byte(key), []byte(blob.PublicPrefix)) {
			t.Errorf("%s is public", key)
		}
	}
	if _, err := c.Get(ctx, "example.com"); !errors.Is(err, autocert.ErrCacheMiss) {
		t.Fatalf("miss: %v", err)
	}
	if err := c.Put(ctx, "example.com", []byte("chain and key")); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Get(ctx, "example.com"); err != nil || string(got) != "chain and key" {
		t.Fatalf("get: %q %v", got, err)
	}
	// A value copied to another key does not open: the AAD is the blob key.
	rc, _, _ := store.Get(ctx, c.BlobKey("example.com"))
	sealed, _ := io.ReadAll(rc)
	_ = rc.Close()
	if _, err := store.Put(ctx, c.BlobKey("other.example"), bytes.NewReader(sealed), blob.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "other.example"); !errors.Is(err, kernel.ErrDecrypt) {
		t.Fatalf("swapped value opened: %v", err)
	}
}

func TestLeaseOverDBKeyValueStore(t *testing.T) {
	ctx := t.Context()
	clock := testkit.NewClock(time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC))
	d, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "kv.db"), uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Tx(ctx, func(tx uow.Tx) error {
		sqlTx, err := d.SQLTx(tx)
		if err != nil {
			return err
		}
		_, err = sqlTx.ExecContext(ctx, infrastructure.Module.SQLite)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	kv := kvdb.NewSQLite(d, clock)
	a := &Lease{KV: kv, Namespace: "acme.order", TTL: 10 * time.Minute}
	b := &Lease{KV: kv, Namespace: "acme.order", TTL: 10 * time.Minute}

	ta, ok, err := a.TryAcquire(ctx, "example.com")
	if err != nil || !ok {
		t.Fatalf("a: %v %v", ok, err)
	}
	if _, ok, _ := b.TryAcquire(ctx, "example.com"); ok {
		t.Fatal("b took a held lease")
	}
	clock.Advance(9 * time.Minute)
	if ok, _ := a.Renew(ctx, "example.com", ta); !ok {
		t.Fatal("renew failed")
	}
	clock.Advance(9 * time.Minute)
	if _, ok, _ := b.TryAcquire(ctx, "example.com"); ok {
		t.Fatal("b took a renewed lease")
	}
	clock.Advance(2 * time.Minute) // a stalled: the lease expires
	tb, ok, _ := b.TryAcquire(ctx, "example.com")
	if !ok {
		t.Fatal("b could not take an expired lease")
	}
	if ok, _ := a.Renew(ctx, "example.com", ta); ok {
		t.Fatal("a renewed a lease that b holds")
	}
	if err := a.Release(ctx, "example.com", ta); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := a.TryAcquire(ctx, "example.com"); ok {
		t.Fatal("a's stale release freed b's lease")
	}
	_ = b.Release(ctx, "example.com", tb)
	if _, ok, _ := a.TryAcquire(ctx, "example.com"); !ok {
		t.Fatal("released lease not free")
	}
}

func openSpikeDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "tls.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(SQLiteSchema); err != nil {
		t.Fatal(err)
	}
	return db
}

func selfSigned(t *testing.T, host string, notBefore time.Time, life time.Duration) ([]byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: notBefore, NotAfter: notBefore.Add(life)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), key
}

func TestDBStoreBackoffFromEvents(t *testing.T) {
	ctx := t.Context()
	clock := testkit.NewClock(time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC))
	km := &countingKM{KeyManager: testkit.NewKeyManager("local:test")}
	s := &DBStore{DB: openSpikeDB(t), Sealer: kernel.NewSealer(km), Clock: clock, NewID: testkit.NewIDGenerator(clock).NewID}
	const dir = "https://ca.example/dir"

	// Failures back off 15m, 30m, 1h, ... from the last attempt.
	for i, want := range []time.Duration{15 * time.Minute, 30 * time.Minute, time.Hour} {
		a, _ := s.RecordAttempt(ctx, "example.com", dir, "")
		if err := s.RecordFailure(ctx, a, "urn:ietf:params:acme:error:connection", "refused"); err != nil {
			t.Fatal(err)
		}
		next, n, err := s.NextAttempt(ctx, "example.com")
		if err != nil || n != i+1 || !next.Equal(clock.Now().Add(want)) {
			t.Fatalf("after %d failures: next %v (want +%v) n %d err %v", i+1, next.Sub(clock.Now()), want, n, err)
		}
		clock.Advance(want)
	}
	// An abandoned attempt (the process died, no outcome) counts too.
	_, _ = s.RecordAttempt(ctx, "example.com", dir, "")
	if _, n, _ := s.NextAttempt(ctx, "example.com"); n != 4 {
		t.Fatalf("abandoned attempt not counted: %d", n)
	}
	// A certificate resets the count.
	clock.Advance(time.Minute)
	a, _ := s.RecordAttempt(ctx, "example.com", dir, "")
	chain, key := selfSigned(t, "example.com", clock.Now(), 45*24*time.Hour)
	id, err := s.InsertCertificate(ctx, a, "example.com", dir, chain, key)
	if err != nil {
		t.Fatal(err)
	}
	if next, n, _ := s.NextAttempt(ctx, "example.com"); n != 0 || !next.IsZero() {
		t.Fatalf("after success: %v %d", next, n)
	}
	if got, _ := s.LatestID(ctx, "example.com"); got != id {
		t.Fatalf("latest %s, want %s", got, id)
	}
	// The sealed key is bound to its row: copied to another row it does not open.
	a2, _ := s.RecordAttempt(ctx, "example.com", dir, id)
	id2, _ := s.InsertCertificate(ctx, a2, "example.com", dir, chain, key)
	if _, err := s.DB.Exec(`UPDATE tls_certificates SET sealed_private_key = (SELECT sealed_private_key FROM tls_certificates WHERE id = ?) WHERE id = ?`, id, id2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(ctx, id2); !errors.Is(err, kernel.ErrDecrypt) {
		t.Fatalf("swapped key opened: %v", err)
	}
	if _, err := s.Load(ctx, id); err != nil {
		t.Fatal(err)
	}
}

func TestBackoffCap(t *testing.T) {
	if Backoff(1) != 15*time.Minute || Backoff(7) != 16*time.Hour || Backoff(8) != 24*time.Hour || Backoff(100) != 24*time.Hour {
		t.Fatalf("%v %v %v", Backoff(1), Backoff(7), Backoff(8))
	}
}
