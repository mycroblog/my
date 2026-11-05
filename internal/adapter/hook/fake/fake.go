// SPDX-License-Identifier: AGPL-3.0-only

// Package fake is an in-memory hook Plugin for tests: it answers what a test sets and counts its
// calls. Only tests link it (`scripts/go-build-tags.sh` keeps the fakes out of cmd/my).
package fake

import (
	"context"
	"sync"

	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/domain/hook"
)

// Plugin implements apphook.Plugin.
type Plugin struct {
	mu       sync.Mutex
	answer   apphook.Answer
	err      error
	requests []apphook.Request
}

var _ apphook.Plugin = (*Plugin)(nil)

// New returns a plugin that allows everything.
func New() *Plugin { return &Plugin{answer: apphook.Answer{Verdict: hook.Allow}} }

// Answer makes the plugin return a from now on.
func (p *Plugin) Answer(a apphook.Answer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.answer, p.err = a, nil
}

// Fail makes the plugin fail with err from now on.
func (p *Plugin) Fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
}

// Requests returns what reached the plugin.
func (p *Plugin) Requests() []apphook.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]apphook.Request(nil), p.requests...)
}

// Check implements apphook.Plugin.
func (p *Plugin) Check(ctx context.Context, r apphook.Request) (apphook.Answer, error) {
	if err := ctx.Err(); err != nil {
		return apphook.Answer{}, apphook.ErrPluginTimeout
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, r)
	if p.err != nil {
		return apphook.Answer{}, p.err
	}
	return p.answer, nil
}
