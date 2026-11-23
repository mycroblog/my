// SPDX-License-Identifier: AGPL-3.0-only

package kernel_test

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

// recordHandler keeps the attributes of the records it handles, in their order.
type recordHandler struct {
	mu    sync.Mutex
	lines [][]slog.Attr
}

func (*recordHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordHandler) Handle(_ context.Context, r slog.Record) error {
	var attrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lines = append(h.lines, attrs)
	return nil
}

func (h *recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordHandler) WithGroup(string) slog.Handler      { return h }

func keys(attrs []slog.Attr) []string {
	out := make([]string, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, a.Key)
	}
	return out
}

// logUnit writes the line of a unit with ctx and returns the keys of its attributes, in order.
func logUnit(ctx context.Context, t *testing.T, attrs ...slog.Attr) []string {
	t.Helper()
	h := &recordHandler{}
	kernel.LogUnit(ctx, slog.New(h), slog.LevelInfo, "unit", kernel.NopMeter{}, attrs)
	if len(h.lines) != 1 {
		t.Fatalf("LogUnit wrote %d lines, want 1", len(h.lines))
	}
	return keys(h.lines[0])
}

// TestUnitAttrs checks that the work of a unit adds attributes to the line that ends it, after
// the caller's, that a nested unit collects its own, and that adding outside a unit does nothing
// (ADR-0055 §5).
func TestUnitAttrs(t *testing.T) {
	t.Run("outside a unit", func(t *testing.T) {
		ctx := t.Context()
		kernel.AddUnitAttrs(ctx, slog.String("lost", "x"))
		if got := logUnit(ctx, t, slog.String("a", "1")); !slices.Equal(got, []string{"a"}) {
			t.Errorf("line keys = %v, want [a]", got)
		}
	})

	t.Run("after the caller's attributes", func(t *testing.T) {
		ctx, u := kernel.WithUnitAttrs(t.Context())
		if got := u.Attrs(); len(got) != 0 {
			t.Errorf("Attrs() of a new unit = %v, want none", got)
		}
		kernel.AddUnitAttrs(ctx, slog.String("x", "1"))
		kernel.AddUnitAttrs(ctx, slog.String("y", "2"), slog.Int("z", 3))
		if got := keys(u.Attrs()); !slices.Equal(got, []string{"x", "y", "z"}) {
			t.Errorf("Attrs() keys = %v, want [x y z]", got)
		}
		if got := logUnit(ctx, t, slog.String("a", "1"), slog.String("b", "2")); !slices.Equal(got, []string{"a", "b", "x", "y", "z"}) {
			t.Errorf("line keys = %v, want [a b x y z]", got)
		}
	})

	t.Run("nested unit", func(t *testing.T) {
		outer, ou := kernel.WithUnitAttrs(t.Context())
		inner, iu := kernel.WithUnitAttrs(outer)
		kernel.AddUnitAttrs(outer, slog.String("outer", "1"))
		kernel.AddUnitAttrs(inner, slog.String("inner", "1"))
		if got := keys(ou.Attrs()); !slices.Equal(got, []string{"outer"}) {
			t.Errorf("outer Attrs() keys = %v, want [outer]", got)
		}
		if got := keys(iu.Attrs()); !slices.Equal(got, []string{"inner"}) {
			t.Errorf("inner Attrs() keys = %v, want [inner]", got)
		}
		if got := logUnit(inner, t, slog.String("a", "1")); !slices.Equal(got, []string{"a", "inner"}) {
			t.Errorf("inner line keys = %v, want [a inner]", got)
		}
		if got := logUnit(outer, t, slog.String("a", "1")); !slices.Equal(got, []string{"a", "outer"}) {
			t.Errorf("outer line keys = %v, want [a outer]", got)
		}
	})
}

// TestUnitAttrsConcurrentAdds adds from many goroutines of one unit; go test -race reports a
// data race.
func TestUnitAttrsConcurrentAdds(t *testing.T) {
	const n = 64
	ctx, u := kernel.WithUnitAttrs(t.Context())
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			kernel.AddUnitAttrs(ctx, slog.Int("i", i))
			_ = u.Attrs()
		}()
	}
	wg.Wait()
	got := u.Attrs()
	if len(got) != n {
		t.Fatalf("Attrs() has %d attributes, want %d", len(got), n)
	}
	seen := map[int64]bool{}
	for _, a := range got {
		seen[a.Value.Int64()] = true
	}
	if len(seen) != n {
		t.Errorf("Attrs() has %d distinct values, want %d", len(seen), n)
	}
}
