// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"strings"
	"testing"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestInstanceTextQueries checks that GET /api/v1/instance reads the instance texts with one
// query, and still shows each of them.
func TestInstanceTextQueries(t *testing.T) {
	t.Parallel()
	tracer := testkit.NewTracer()
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, "alice")
	for key, body := range map[string]string{"name": "Example", "short_description": "Short", "description": "Long", "terms": "Terms"} {
		if _, err := c.app.Instance.SetText(t.Context(), appinstance.SetTextInput{Key: instance.TextKey(key), Body: body}); err != nil {
			t.Fatalf("SetText(%s): %v", key, err)
		}
	}
	was := ops(tracer)
	var got clientapi.Instance
	if res := c.do("alice", http.MethodGet, "/instance", nil, &got); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /instance = %d", res.StatusCode)
	}
	texts := 0
	for op, n := range ops(tracer) {
		if strings.Contains(op, "Text") {
			texts += n - was[op]
		}
	}
	if texts != 1 {
		t.Errorf("GET /instance read the texts with %d queries, want 1", texts)
	}
	if got.Name != "Example" || got.ShortDescription == nil || *got.ShortDescription != "Short" || got.Description == nil || *got.Description != "Long" {
		t.Errorf("instance = %+v, want its texts", got)
	}
}
