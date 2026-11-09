// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"sync"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
)

// bearerBatch is an Authenticator that checks many tokens at once, as *appidentity.Service does.
type bearerBatch interface {
	// ValidBearers reports, by token, whether AuthenticateBearer would accept each of tokens.
	ValidBearers(ctx context.Context, accessTokens []string) (map[string]bool, error)
	// RecordBearerUse records the use of the token family of a token, as AuthenticateBearer does.
	RecordBearerUse(ctx context.Context, accessToken string)
}

// streamMonitor checks, once per heartbeat for every open stream of the process, that the access
// tokens are still valid and that the service closure lets the streams stay, with one query for
// the tokens and one read of the closure, and ends the streams that may not stay. It runs while
// streams are open.
type streamMonitor struct {
	s     *Server
	batch bearerBatch
	every time.Duration

	mu      sync.Mutex
	conns   map[*monitoredStream]struct{}
	running bool
	// used is when the use of each token was last recorded.
	used map[string]time.Time
}

// monitoredStream is one stream that the monitor checks: ended is closed when it must end.
type monitoredStream struct {
	token string
	ended chan struct{}
	once  sync.Once
}

func (c *monitoredStream) end() { c.once.Do(func() { close(c.ended) }) }

func newStreamMonitor(s *Server, batch bearerBatch, every time.Duration) *streamMonitor {
	return &streamMonitor{s: s, batch: batch, every: every, conns: map[*monitoredStream]struct{}{}, used: map[string]time.Time{}}
}

// add starts checking a stream of token, until remove; the token's use was just recorded by the
// request that opened the stream.
func (m *streamMonitor) add(token string) *monitoredStream {
	c := &monitoredStream{token: token, ended: make(chan struct{})}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conns[c] = struct{}{}
	if _, ok := m.used[token]; !ok {
		m.used[token] = time.Now()
	}
	if !m.running {
		m.running = true
		kernel.Go(context.Background(), "clientapi.stream_monitor", m.s.o.ErrorReporter, func(context.Context) { m.run() })
	}
	return c
}

func (m *streamMonitor) remove(c *monitoredStream) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.conns, c)
}

// run checks the open streams every m.every until none is left.
func (m *streamMonitor) run() {
	ticker := time.NewTicker(m.every)
	defer ticker.Stop()
	for range ticker.C {
		if !m.check() {
			return
		}
	}
}

// check checks every open stream once and reports whether any is left; the last one gone, it
// stops the monitor, which the next add starts again.
func (m *streamMonitor) check() bool {
	m.mu.Lock()
	if len(m.conns) == 0 {
		m.running = false
		m.used = map[string]time.Time{}
		m.mu.Unlock()
		return false
	}
	conns := make([]*monitoredStream, 0, len(m.conns))
	var tokens []string
	seen := map[string]bool{}
	for c := range m.conns {
		conns = append(conns, c)
		if !seen[c.token] {
			seen[c.token] = true
			tokens = append(tokens, c.token)
		}
	}
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), m.every)
	defer cancel()
	valid, err := m.batch.ValidBearers(ctx, tokens)
	if err != nil {
		return true // a failed read keeps the streams; the next heartbeat reads again
	}
	closed := m.s.streamClosed(ctx)
	now := time.Now()
	for _, c := range conns {
		if closed || !valid[c.token] {
			c.end() // revoked, expired, the account became unusable, or the server was closed
		}
	}
	// The use of a token that stays open is recorded as often as the requests record it.
	m.mu.Lock()
	var due []string
	for _, t := range tokens {
		if valid[t] && now.Sub(m.used[t]) >= appidentity.LastUsedInterval {
			m.used[t] = now
			due = append(due, t)
		}
	}
	m.mu.Unlock()
	for _, t := range due {
		m.batch.RecordBearerUse(ctx, t)
	}
	return true
}
