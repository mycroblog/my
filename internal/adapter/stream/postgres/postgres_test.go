// SPDX-License-Identifier: AGPL-3.0-only

package postgres_test

import (
	"context"
	"net"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	streampostgres "github.com/mycroblog/my/internal/adapter/stream/postgres"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
	"github.com/mycroblog/my/pkg/testkit/streamtest"
)

// TestMain stops the PostgreSQL server of package pgtest after the tests.
func TestMain(m *testing.M) {
	os.Exit(pgtest.Main(m))
}

func TestStreamBusContract(t *testing.T) {
	for name, listenURL := range map[string]bool{"PoolSettings": false, "ListenURL": true} {
		t.Run(name, func(t *testing.T) {
			streamtest.StreamBusContract(t, func(t *testing.T) streamtest.StreamBus {
				url := pgtest.NewDatabase(t)
				o := streampostgres.Options{}
				if listenURL {
					o.ListenURL = url
				}
				return streamtest.StreamBus{
					NewBus: func(t *testing.T) stream.Bus { return newBus(t, url, o) },
					Unreachable: func(t *testing.T) stream.Bus {
						u := silentServerURL(t)
						o := o
						if listenURL {
							o.ListenURL = u
						}
						return newBus(t, u, o)
					},
					Break: func(t *testing.T) { terminateListeners(t, url) },
					Resources: func(t *testing.T) int {
						var n int
						queryListeners(t, url, "SELECT count(*)", &n)
						return n
					},
				}
			})
		})
	}
}

// Relay on two Buses of one database, standing for two processes, routes the signals of both.
func TestStreamBrokerContract(t *testing.T) {
	streamtest.StreamBrokerContract(t, func(t *testing.T, o stream.HubOptions) streamtest.StreamBroker {
		url := pgtest.NewDatabase(t)
		local := stream.NewRelay(newBus(t, url, streampostgres.Options{}), stream.RelayOptions{Hub: o})
		other := stream.NewRelay(newBus(t, url, streampostgres.Options{}), stream.RelayOptions{})
		for _, r := range []*stream.Relay{local, other} {
			t.Cleanup(func() {
				if err := r.Close(context.Background()); err != nil {
					t.Errorf("Close: %v", err)
				}
			})
		}
		return streamtest.StreamBroker{
			Publisher: local, Subscriber: local, Remote: other,
			Break: func(t *testing.T) { terminateListeners(t, url) },
		}
	})
}

func TestNewRejectsBadOptions(t *testing.T) {
	if _, err := streampostgres.New(streampostgres.Options{}); err == nil {
		t.Error("New without a pool succeeded")
	}
	pool, err := pgxpool.New(t.Context(), "postgres://my@127.0.0.1:1/my")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, err = streampostgres.New(streampostgres.Options{Pool: pool, ListenURL: "postgres://my:secret@[::1"})
	if err == nil {
		t.Fatal("New with an invalid ListenURL succeeded")
	}
	if got := err.Error(); strings.Contains(got, "secret") {
		t.Errorf("the error quotes the password: %q", got)
	}
}

// The listener takes any URL the server takes: PostgreSQL refuses a startup parameter it does not
// know, which is what pool_max_conns becomes unless it is taken out first.
func TestListenURLTakesTheParametersOfThePool(t *testing.T) {
	url := pgtest.NewDatabase(t)
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	b := newBus(t, url, streampostgres.Options{ListenURL: url + sep + "pool_max_conns=4"})
	l, err := b.Listen(t.Context(), func([]byte) {})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if err := l.Close(t.Context()); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// newBus returns a Bus with its own pool on url, closed when t ends.
func newBus(t *testing.T, url string, o streampostgres.Options) stream.Bus {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	o.Pool = pool
	b, err := streampostgres.New(o)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// silentServerURL returns the URL of a server that accepts connections and never answers, so
// that only the deadline of a call can end it. The server stops when t ends.
func silentServerURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return "postgres://my@" + ln.Addr().String() + "/my?sslmode=disable"
}

// terminateListeners ends the backends of the listening connections of url's database, as a
// lost connection would.
func terminateListeners(t *testing.T, url string) {
	t.Helper()
	var n int
	queryListeners(t, url, "SELECT count(pg_terminate_backend(pid))", &n)
	if n == 0 {
		t.Fatal("no listening connection to terminate")
	}
}

// queryListeners runs selectList over the listening connections of url's database and scans
// its single row into dest.
func queryListeners(t *testing.T, url, selectList string, dest ...any) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	err = conn.QueryRow(t.Context(),
		selectList+" FROM pg_stat_activity WHERE application_name = $1 AND datname = current_database()",
		streampostgres.ListenerApplicationName).Scan(dest...)
	if err != nil {
		t.Fatal(err)
	}
}
