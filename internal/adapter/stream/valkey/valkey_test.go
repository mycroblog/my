// SPDX-License-Identifier: AGPL-3.0-only

package valkey_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	valkeygo "github.com/valkey-io/valkey-go"

	streamvalkey "github.com/mycroblog/my/internal/adapter/stream/valkey"
	"github.com/mycroblog/my/internal/adapter/valkey"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/valkeytest"
)

// TestMain stops the Valkey container of package valkeytest after the tests.
func TestMain(m *testing.M) {
	os.Exit(valkeytest.Main(m))
}

// The contract on miniredis needs neither Docker nor a server.
func TestStreamBusContractMiniredis(t *testing.T) {
	testkit.StreamBusContract(t, func(t *testing.T) testkit.StreamBus {
		m := miniredis.RunT(t)
		c := open(t, "redis://"+m.Addr())
		return testkit.StreamBus{
			NewBus:      func(*testing.T) stream.Bus { return streamvalkey.New(c, "", nil) },
			Unreachable: unreachable,
			// Closing the server cuts every connection; the publishers reconnect to the restarted one.
			Break: func(t *testing.T) {
				m.Close()
				if err := m.Restart(); err != nil {
					t.Fatal(err)
				}
			},
			Resources: func(t *testing.T) int { return numsub(t, c, streamvalkey.Channel("")) },
		}
	})
}

// The contract on a Valkey server (pkg/testkit/valkeytest), only with TEST_STREAM=container
// (scripts/task.sh go:test-stream; CI on main and nightly, ADR-0050 §5): pull requests run it on miniredis.
func TestStreamBusContractValkey(t *testing.T) {
	if os.Getenv("TEST_STREAM") != "container" {
		t.Skip("the Valkey server runs with TEST_STREAM=container (scripts/task.sh go:test-stream)")
	}
	testkit.StreamBusContract(t, func(t *testing.T) testkit.StreamBus {
		s := valkeytest.New(t)
		// The connections of the test carry a name of the test's own, by which Break finds the
		// listeners to cut on the shared server.
		name := strings.TrimSuffix(strings.ReplaceAll(s.KeyPrefix, ":", "-"), "-")
		c := openNamed(t, s.URL, name)
		return testkit.StreamBus{
			NewBus:      func(*testing.T) stream.Bus { return streamvalkey.New(c, s.KeyPrefix, nil) },
			Unreachable: unreachable,
			Break:       func(t *testing.T) { killSubscribers(t, c, name) },
			Resources:   func(t *testing.T) int { return numsub(t, c, streamvalkey.Channel(s.KeyPrefix)) },
		}
	})
}

// Relays on Buses of one server carry each other's signals.
func TestStreamBrokerContractRelay(t *testing.T) {
	testkit.StreamBrokerContract(t, func(t *testing.T, o stream.HubOptions) testkit.StreamBroker {
		m := miniredis.RunT(t)
		c := open(t, "redis://"+m.Addr())
		local := stream.NewRelay(streamvalkey.New(c, "", nil), stream.RelayOptions{Hub: o})
		remote := stream.NewRelay(streamvalkey.New(c, "", nil), stream.RelayOptions{})
		for _, r := range []*stream.Relay{local, remote} {
			t.Cleanup(func() {
				if err := r.Close(context.Background()); err != nil {
					t.Errorf("Close: %v", err)
				}
			})
		}
		return testkit.StreamBroker{
			Publisher: local, Subscriber: local, Remote: remote,
			Break: func(t *testing.T) {
				m.Close()
				if err := m.Restart(); err != nil {
					t.Fatal(err)
				}
			},
		}
	})
}

// Send delivers its payload when the pooled connection it uses was cut and the client has not
// noticed yet, as right after a server restart: the PUBLISH on it fails, and Send sends it
// again on a new connection.
func TestSendOnCutConnection(t *testing.T) {
	m := miniredis.RunT(t)
	listening := streamvalkey.New(open(t, "redis://"+m.Addr()), "", nil)
	const payload = "after the cut"
	arrived := make(chan struct{}, 1)
	l, err := listening.Listen(t.Context(), func(p []byte) {
		if string(p) == payload {
			arrived <- struct{}{}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close(context.Background()) })

	// The sender reaches the server through a proxy that cuts its open connections, the way a
	// restarted server does before the client notices. The client is a single one, which keeps
	// to the proxy's address instead of the node addresses that CLUSTER SHARDS reports.
	p := newCutProxy(t, m.Addr())
	opt, err := valkeygo.ParseURL("redis://" + p.addr())
	if err != nil {
		t.Fatal(err)
	}
	opt.DisableCache, opt.ForceSingleClient = true, true
	c, err := valkeygo.NewClient(opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	sending := streamvalkey.New(c, "", nil)
	if err := sending.Send(t.Context(), []byte("before the cut")); err != nil {
		t.Fatal(err)
	}
	p.cut()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := sending.Send(ctx, []byte(payload)); err != nil {
		t.Fatalf("Send on a cut connection: %v", err)
	}
	select {
	case <-arrived:
	case <-ctx.Done():
		t.Error("the payload sent on a cut connection did not arrive")
	}
}

// cutProxy forwards TCP connections to a server; cut makes every connection open at that
// time close, without forwarding, as soon as its client writes again.
type cutProxy struct {
	ln     net.Listener
	mu     sync.Mutex
	doomed []*atomic.Bool
}

func newCutProxy(t *testing.T, target string) *cutProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &cutProxy{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			client, err := ln.Accept()
			if err != nil {
				return
			}
			server, err := net.Dial("tcp", target)
			if err != nil {
				_ = client.Close()
				continue
			}
			doomed := new(atomic.Bool)
			p.mu.Lock()
			p.doomed = append(p.doomed, doomed)
			p.mu.Unlock()
			closeBoth := func() { _ = client.Close(); _ = server.Close() }
			go func() {
				_, _ = io.Copy(client, server)
				closeBoth()
			}()
			go func() {
				defer closeBoth()
				buf := make([]byte, 4096)
				for {
					n, err := client.Read(buf)
					if err != nil || doomed.Load() {
						return
					}
					if _, err := server.Write(buf[:n]); err != nil {
						return
					}
				}
			}()
		}
	}()
	return p
}

func (p *cutProxy) addr() string { return p.ln.Addr().String() }

func (p *cutProxy) cut() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, d := range p.doomed {
		d.Store(true)
	}
}

// Buses with different key prefixes on one server do not receive each other's payloads.
func TestBusesAreSeparate(t *testing.T) {
	m := miniredis.RunT(t)
	c := open(t, "redis://"+m.Addr())
	a, b := streamvalkey.New(c, "a:", nil), streamvalkey.New(c, "b:", nil)
	got := make(chan []byte, 1)
	l, err := b.Listen(t.Context(), func(p []byte) { got <- p })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close(context.Background()) })
	if err := a.Send(t.Context(), []byte("for a")); err != nil {
		t.Fatal(err)
	}
	if err := b.Send(t.Context(), []byte("for b")); err != nil {
		t.Fatal(err)
	}
	if p := <-got; string(p) != "for b" {
		t.Fatalf("b's listener received %q; want only b's payload", p)
	}
}

// unreachable returns a Bus whose server has stopped since the client connected.
func unreachable(t *testing.T) stream.Bus {
	m := miniredis.RunT(t)
	c := open(t, "redis://"+m.Addr())
	m.Close()
	return streamvalkey.New(c, "", nil)
}

// open connects to the server at url and closes the client when t ends.
func open(t *testing.T, url string) valkey.Client {
	t.Helper()
	c, err := valkey.Open(t.Context(), valkey.Options{URL: url})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

// openNamed is open for a client whose connections are named name (CLIENT SETNAME).
func openNamed(t *testing.T, url, name string) valkey.Client {
	t.Helper()
	opt, err := valkeygo.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	opt.DisableCache = true
	opt.ClientName = name
	c, err := valkeygo.NewClient(opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

// numsub returns the number of connections subscribed to channel.
func numsub(t *testing.T, c valkey.Client, channel string) int {
	t.Helper()
	n, err := c.Do(t.Context(), c.B().PubsubNumsub().Channel(channel).Build()).AsIntMap()
	if err != nil {
		t.Fatal(err)
	}
	return int(n[channel])
}

// killSubscribers cuts the connections named name that are subscribed to a channel.
func killSubscribers(t *testing.T, c valkey.Client, name string) {
	t.Helper()
	list, err := c.Do(t.Context(), c.B().ClientList().Build()).ToString()
	if err != nil {
		t.Fatal(err)
	}
	killed := 0
	sc := bufio.NewScanner(strings.NewReader(list))
	for sc.Scan() {
		fields := map[string]string{}
		for f := range strings.FieldsSeq(sc.Text()) {
			if k, v, ok := strings.Cut(f, "="); ok {
				fields[k] = v
			}
		}
		if fields["name"] != name || fields["sub"] == "0" || fields["sub"] == "" {
			continue
		}
		id, err := strconv.ParseInt(fields["id"], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Do(t.Context(), c.B().ClientKill().Id(id).Build()).Error(); err != nil {
			t.Fatal(err)
		}
		killed++
	}
	if killed == 0 {
		t.Fatal("no subscribed connection to cut")
	}
}
