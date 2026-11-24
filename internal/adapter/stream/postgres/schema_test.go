// SPDX-License-Identifier: AGPL-3.0-only

package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	streampostgres "github.com/mycroblog/my/internal/adapter/stream/postgres"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

func TestChannelFor(t *testing.T) {
	atEdge := strings.Repeat("s", 63-len(streampostgres.DefaultChannel+"_"))
	pastEdge := atEdge + "s"
	sum := sha256.Sum256([]byte(pastEdge))
	for _, c := range []struct{ name, schema, want string }{
		{"public", "public", streampostgres.DefaultChannel},
		{"short", "tenant_a", "my_stream_tenant_a"},
		{"mixed case and punctuation", "Tenant-A", "my_stream_Tenant-A"},
		{"63 bytes", atEdge, "my_stream_" + atEdge},
		{"past 63 bytes", pastEdge, "my_stream_" + hex.EncodeToString(sum[:16])},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := streampostgres.ChannelFor(c.schema)
			if got != c.want {
				t.Errorf("ChannelFor(%q) = %q, want %q", c.schema, got, c.want)
			}
			if len(got) > 63 {
				t.Errorf("ChannelFor(%q) is %d bytes, longer than a PostgreSQL identifier", c.schema, len(got))
			}
		})
	}
	if streampostgres.DefaultChannel != "my_stream" {
		t.Errorf("DefaultChannel = %q, want my_stream", streampostgres.DefaultChannel)
	}
}

// Instances that share one database, each in its own schema of database_url's search_path,
// receive only their own payloads, and an instance in public keeps a working stream. Tenant-B
// needs quoting as an identifier, in LISTEN as in the search_path.
func TestInstancesInSchemasOfOneDatabaseKeepTheirPayloadsApart(t *testing.T) {
	base := pgtest.NewDatabase(t)
	schemas := []string{"tenant_a", "Tenant-B"}
	for _, s := range schemas {
		execSQL(t, base, "CREATE SCHEMA "+pgx.Identifier{s}.Sanitize())
	}
	urls := map[string]string{"public": base}
	for _, s := range schemas {
		urls[s] = withSearchPath(t, base, pgx.Identifier{s}.Sanitize())
	}
	names := []string{"tenant_a", "Tenant-B", "public"}

	type instance struct {
		send func(payload string)
		got  *received
	}
	instances := map[string]instance{}
	for _, name := range names {
		b := newBus(t, urls[name], streampostgres.Options{})
		got := &received{}
		l, err := b.Listen(t.Context(), got.add)
		if err != nil {
			t.Fatalf("Listen of %s: %v", name, err)
		}
		t.Cleanup(func() {
			if err := l.Close(context.Background()); err != nil {
				t.Errorf("Close of %s: %v", name, err)
			}
		})
		instances[name] = instance{got: got, send: func(payload string) {
			t.Helper()
			if err := b.Send(t.Context(), []byte(payload)); err != nil {
				t.Fatalf("Send of %s: %v", name, err)
			}
		}}
	}

	// NOTIFY delivers in commit order, and each Send has committed when it returns, so a
	// listener that has its fence has every payload sent before the fences.
	for _, name := range names {
		instances[name].send("event " + name)
	}
	for _, name := range names {
		instances[name].send("fence " + name)
	}
	for _, name := range names {
		got := instances[name].got.until(t, "fence "+name)
		want := []string{"event " + name, "fence " + name}
		if !slices.Equal(got, want) {
			t.Errorf("the instance in %s received %q, want %q", name, got, want)
		}
	}
}

// A search_path none of whose schemas exists names no channel: Send and Listen say so.
func TestNoSchemaOfTheSearchPathExists(t *testing.T) {
	b := newBus(t, withSearchPath(t, pgtest.NewDatabase(t), "missing_schema"), streampostgres.Options{})
	const want = "no schema of the search_path of database_url exists"
	if err := b.Send(t.Context(), []byte("x")); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Send = %v, want an error saying %q", err, want)
	}
	l, err := b.Listen(t.Context(), func([]byte) {})
	if err == nil {
		_ = l.Close(t.Context())
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Listen = %v, want an error saying %q", err, want)
	}
}

// received collects the payloads of a listener.
type received struct {
	mu       sync.Mutex
	payloads []string
	changed  chan struct{}
}

func (r *received) add(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payloads = append(r.payloads, string(p))
	if r.changed != nil {
		close(r.changed)
		r.changed = nil
	}
}

// until waits for last and returns the payloads received up to it.
func (r *received) until(t *testing.T, last string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		r.mu.Lock()
		if i := slices.Index(r.payloads, last); i >= 0 {
			got := slices.Clone(r.payloads[:i+1])
			r.mu.Unlock()
			return got
		}
		if r.changed == nil {
			r.changed = make(chan struct{})
		}
		changed := r.changed
		r.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			r.mu.Lock()
			defer r.mu.Unlock()
			t.Fatalf("%q did not arrive; received %q", last, r.payloads)
		}
	}
}

// withSearchPath returns u with the search_path startup parameter set to searchPath.
func withSearchPath(t *testing.T, u, searchPath string) string {
	t.Helper()
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	q.Set("search_path", searchPath)
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// execSQL runs sql on a connection of its own to u.
func execSQL(t *testing.T, u, sql string) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), u)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	if _, err := conn.Exec(t.Context(), sql); err != nil {
		t.Fatal(err)
	}
}
