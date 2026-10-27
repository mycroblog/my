// SPDX-License-Identifier: AGPL-3.0-only

// Package postgres implements stream.Bus on PostgreSQL LISTEN / NOTIFY with pgx/v5 (ADR-0050 §2):
// the broker of the self-hosted PostgreSQL deployments with more than one process.
//
// Send runs SELECT pg_notify(channel, payload) on a connection of the pool, in autocommit.
// Listen opens a connection of its own, outside the pool, runs LISTEN on it, and receives with
// pgx's WaitForNotification; when that connection is lost, the listener is broken. The
// listening connection carries the application name ListenerApplicationName, so that it can be
// told apart in pg_stat_activity.
//
// LISTEN belongs to a session, so the listening connection must reach PostgreSQL directly or
// through a pooler in session mode: it does not work through a pooler in transaction mode
// (PgBouncer's transaction pooling, Neon's pooled connection string, Supabase's port 6543).
// Options.ListenURL then names a direct connection (the stream_postgres_url setting). NOTIFY
// reaches only the listeners of the same database.
//
// The listening connection goes away with the process, so the adapter needs no stream.Sweeper.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
)

// DefaultChannel is the notification channel when Options.Channel is empty.
const DefaultChannel = "my_stream"

// ListenerApplicationName is the application_name of the listening connections.
const ListenerApplicationName = "my stream listener"

// Options configures a Bus.
type Options struct {
	// Pool sends the payloads. Required.
	Pool *pgxpool.Pool
	// ListenURL is the connection string of the listening connections, in any form pgx accepts
	// (the stream_postgres_url setting). Empty means the connection settings of Pool. It must not
	// go through a pooler in transaction mode.
	ListenURL string
	// Channel is the notification channel; empty means DefaultChannel. Every process of an
	// instance uses the same one.
	Channel string
	// Reporter receives the panics of the Bus's goroutines (kernel.Go); nil only panics again.
	Reporter kernel.ErrorReporter
}

// Bus is a stream.Bus on PostgreSQL LISTEN / NOTIFY. It is safe for concurrent use.
type Bus struct {
	pool     *pgxpool.Pool
	listen   *pgx.ConnConfig
	channel  string
	reporter kernel.ErrorReporter
}

var _ stream.Bus = (*Bus)(nil)

// New returns a Bus on o. It connects to nothing: Send uses the pool, and Listen connects.
func New(o Options) (*Bus, error) {
	if o.Pool == nil {
		return nil, errors.New("stream/postgres: no pool")
	}
	var cfg *pgx.ConnConfig
	if o.ListenURL == "" {
		cfg = o.Pool.Config().ConnConfig.Copy()
	} else {
		var err error
		if cfg, err = pgx.ParseConfig(o.ListenURL); err != nil {
			// The error may quote the URL, which can hold a password; keep only its kind.
			return nil, errors.New("stream/postgres: stream_postgres_url is not a valid connection string")
		}
	}
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = map[string]string{}
	}
	cfg.RuntimeParams["application_name"] = ListenerApplicationName
	channel := o.Channel
	if channel == "" {
		channel = DefaultChannel
	}
	return &Bus{pool: o.Pool, listen: cfg, channel: channel, reporter: o.Reporter}, nil
}

// Send implements stream.Bus.
func (b *Bus) Send(ctx context.Context, payload []byte) error {
	if _, err := b.pool.Exec(ctx, "SELECT pg_notify($1, $2)", b.channel, string(payload)); err != nil {
		return fmt.Errorf("stream/postgres: notify: %w", err)
	}
	return nil
}

// Listen implements stream.Bus.
func (b *Bus) Listen(ctx context.Context, receive func(payload []byte)) (stream.Listener, error) {
	conn, err := pgx.ConnectConfig(ctx, b.listen.Copy())
	if err != nil {
		return nil, fmt.Errorf("stream/postgres: connect the listener: %w", err)
	}
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{b.channel}.Sanitize()); err != nil {
		_ = conn.Close(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("stream/postgres: listen: %w", err)
	}
	loopCtx, stop := context.WithCancel(context.Background())
	l := &listener{conn: conn, stop: stop, broken: make(chan struct{}), done: make(chan struct{}), reporter: b.reporter}
	kernel.Go(loopCtx, "stream.postgres.listen", b.reporter, func(ctx context.Context) { l.run(ctx, receive) })
	return l, nil
}

// listener is the open listener of one process: its own connection and the goroutine that
// waits on it.
type listener struct {
	conn   *pgx.Conn
	stop   context.CancelFunc
	broken chan struct{}
	done   chan struct{} // closed when run returns
	// reporter receives a panic of the goroutine that closes the connection late.
	reporter kernel.ErrorReporter

	closeOnce sync.Once
	closeErr  error
}

func (l *listener) run(ctx context.Context, receive func(payload []byte)) {
	defer close(l.done)
	for {
		n, err := l.conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() == nil {
				// The connection was lost: payloads sent from now on are missed.
				close(l.broken)
			}
			return
		}
		receive([]byte(n.Payload))
	}
}

// Broken implements stream.Listener.
func (l *listener) Broken() <-chan struct{} { return l.broken }

// Close implements stream.Listener: it stops the goroutine and closes the connection, which
// ends the LISTEN.
func (l *listener) Close(ctx context.Context) error {
	l.closeOnce.Do(func() {
		l.stop()
		select {
		case <-l.done:
		case <-ctx.Done():
			// A pgx.Conn is not safe for concurrent use: close it once the goroutine is gone.
			kernel.Go(context.WithoutCancel(ctx), "stream.postgres.close_late", l.reporter, func(context.Context) {
				<-l.done
				_ = l.conn.Close(context.Background())
			})
			l.closeErr = fmt.Errorf("stream/postgres: close the listener: %w", ctx.Err())
			return
		}
		if err := l.conn.Close(ctx); err != nil {
			l.closeErr = fmt.Errorf("stream/postgres: close the listener: %w", err)
		}
	})
	return l.closeErr
}
