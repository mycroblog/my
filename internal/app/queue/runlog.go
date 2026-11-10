// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"log/slog"
	"sync"
)

// RunAttrs collects the attributes that a handler adds to the log line of its run (ADR-0055 §5).
type RunAttrs struct {
	mu    sync.Mutex
	attrs []slog.Attr
}

type runAttrsKey struct{}

// WithRunAttrs returns a context in which AddRunAttrs collects into the returned RunAttrs. A
// runner calls it before it calls the handler.
func WithRunAttrs(ctx context.Context) (context.Context, *RunAttrs) {
	r := &RunAttrs{}
	return context.WithValue(ctx, runAttrsKey{}, r), r
}

// AddRunAttrs adds attrs to the log line of the job run that ctx belongs to. It does nothing
// outside a run. A handler adds only what the operator may see in the log: never a URL or an
// address that the admin screens mask.
func AddRunAttrs(ctx context.Context, attrs ...slog.Attr) {
	r, _ := ctx.Value(runAttrsKey{}).(*RunAttrs)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attrs = append(r.attrs, attrs...)
}

// Attrs returns the attributes collected so far.
func (r *RunAttrs) Attrs() []slog.Attr {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]slog.Attr(nil), r.attrs...)
}
