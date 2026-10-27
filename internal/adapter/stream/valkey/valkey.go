// SPDX-License-Identifier: AGPL-3.0-only

// Package valkey implements the stream.Bus port on Valkey Pub/Sub, or on another server that
// speaks the Redis protocol (stream = "valkey"; ADR-0050 §2). Every process PUBLISHes to one
// channel, <prefix>stream, and a listening process SUBSCRIBEs to it on a connection of its own,
// so that every listening process receives every payload. Clustered servers forward a PUBLISH
// to every node, so one channel works there too.
//
// Valkey Pub/Sub is at most once: a payload published while a listener is disconnected is lost.
// The listener's connection is dedicated and never reconnects by itself, so a lost connection
// (or one the server cut because the listener fell behind its output buffer) closes Broken
// instead of hiding the gap; the next Listen subscribes on a new connection. The connection is
// the listener's only resource on the server, so the adapter needs no stream.Sweeper.
package valkey

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/mycroblog/my/internal/adapter/valkey"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
)

// Channel returns the Pub/Sub channel of the Buses whose keys start with keyPrefix.
func Channel(keyPrefix string) string { return keyPrefix + "stream" }

// Bus is the Valkey Pub/Sub bus. It is safe for concurrent use.
type Bus struct {
	client   valkey.Client
	channel  string
	reporter kernel.ErrorReporter
}

var _ stream.Bus = (*Bus)(nil)

// New returns the bus on client whose channel is Channel(keyPrefix). Buses with different
// prefixes on one server do not receive each other's payloads. reporter receives the panics of
// the Bus's goroutines (kernel.Go); nil only panics again.
func New(client valkey.Client, keyPrefix string, reporter kernel.ErrorReporter) *Bus {
	return &Bus{client: client, channel: Channel(keyPrefix), reporter: reporter}
}

// Send implements stream.Bus with a PUBLISH on a pooled connection. valkey-go retries no
// PUBLISH by itself, so a pooled connection that the server cut (a restart, a failover) and
// that the client has not noticed yet would fail the first PUBLISH on it and lose the payload
// although the server is back. The PUBLISH is marked retryable instead: valkey-go sends it
// again on a new connection until ctx's deadline. A PUBLISH that reached the server before
// its reply was lost then arrives twice, which stream.Bus allows.
func (b *Bus) Send(ctx context.Context, payload []byte) error {
	cmd := b.client.B().Publish().Channel(b.channel).Message(string(payload)).Build().ToRetryable()
	if err := b.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("stream/valkey: publish: %w", err)
	}
	return nil
}

// Listen implements stream.Bus: it takes a connection of its own from the client and returns
// once the server has confirmed the SUBSCRIBE.
func (b *Bus) Listen(ctx context.Context, receive func(payload []byte)) (stream.Listener, error) {
	type result struct {
		l   *listener
		err error
	}
	// The pool may dial a new connection without ctx; the start runs aside so that Listen
	// returns by ctx's deadline, and a start that ends late closes what it opened.
	done := make(chan result, 1)
	kernel.Go(ctx, "stream.valkey.subscribe", b.reporter, func(ctx context.Context) {
		l, err := b.listen(ctx, receive)
		done <- result{l, err}
	})
	select {
	case r := <-done:
		if r.err != nil {
			return nil, r.err
		}
		return r.l, nil
	case <-ctx.Done():
		base := context.WithoutCancel(ctx)
		kernel.Go(base, "stream.valkey.close_late", b.reporter, func(context.Context) {
			if r := <-done; r.l != nil {
				r.l.closeAside(base)
			}
		})
		return nil, fmt.Errorf("stream/valkey: subscribe %s: %w", b.channel, ctx.Err())
	}
}

func (b *Bus) listen(ctx context.Context, receive func(payload []byte)) (*listener, error) {
	conn, release := b.client.Dedicate()
	l := &listener{conn: conn, release: release, broken: make(chan struct{})}
	hooks := conn.SetPubSubHooks(valkeygo.PubSubHooks{
		OnMessage: func(m valkeygo.PubSubMessage) {
			if m.Channel == b.channel {
				receive([]byte(m.Message))
			}
		},
	})
	kernel.Go(context.WithoutCancel(ctx), "stream.valkey.watch", b.reporter, func(context.Context) { l.watch(hooks) })
	if err := conn.Do(ctx, conn.B().Subscribe().Channel(b.channel).Build()).Error(); err != nil {
		l.closeAside(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("stream/valkey: subscribe %s: %w", b.channel, err)
	}
	return l, nil
}

// listener is one SUBSCRIBE on a dedicated connection.
type listener struct {
	conn    valkeygo.DedicatedClient
	release func()
	// broken is closed once the hooks are not called again: the connection was lost or closed.
	broken chan struct{}
	once   sync.Once
}

// watch closes broken when the connection's hooks stop, which valkey-go reports by closing
// hooks, after at most one error.
func (l *listener) watch(hooks <-chan error) {
	for range hooks {
		// The error, if any, says only why the connection ended.
	}
	close(l.broken)
}

// closeTimeout bounds the Close of a listener that Listen does not return.
const closeTimeout = 10 * time.Second

// closeAside closes a listener that Listen does not return, within closeTimeout.
func (l *listener) closeAside(base context.Context) {
	ctx, cancel := context.WithTimeout(base, closeTimeout)
	defer cancel()
	_ = l.Close(ctx)
}

// Broken implements stream.Listener.
func (l *listener) Broken() <-chan struct{} { return l.broken }

// Close implements stream.Listener: it UNSUBSCRIBEs, so that the server has ended the
// subscription when Close returns (closing the connection alone ends it only once the server
// notices), closes the dedicated connection, and waits until the hooks are not called again.
func (l *listener) Close(ctx context.Context) error {
	l.once.Do(func() {
		select {
		case <-l.broken: // the connection is gone, and its subscription with it
		default:
			_ = l.conn.Do(ctx, l.conn.B().Unsubscribe().Build()).Error()
		}
		l.conn.Close()
		l.release()
	})
	select {
	case <-l.broken:
		return nil
	case <-ctx.Done():
		return errors.Join(errors.New("stream/valkey: close the listener"), ctx.Err())
	}
}
