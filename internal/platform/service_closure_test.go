// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestServiceClosureAnnouncement announces the closure of the service through the admin API
// against a real database (FR-808, ADR-0088 §1, §2), changes its dates, and withdraws it:
// GET /instance and NodeInfo state the announcement and its dates at once, and every change is
// in the audit log.
func TestServiceClosureAnnouncement(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice")
	closes := time.Now().UTC().Add(90 * 24 * time.Hour).Truncate(time.Second)
	nodeClosure := func() map[string]any {
		t.Helper()
		code, doc := apGet(t, c, "/nodeinfo/2.1")
		meta, _ := doc["metadata"].(map[string]any)
		closure, _ := meta["serviceClosure"].(map[string]any)
		if code != http.StatusOK || closure == nil {
			t.Fatalf("GET /nodeinfo/2.1 = %d %v", code, doc)
		}
		return closure
	}
	if got := nodeClosure(); got["state"] != "none" || got["closesAt"] != nil {
		t.Errorf("NodeInfo serviceClosure before an announcement = %v", got)
	}

	var got clientapi.AdminServiceClosure
	if res := c.do("alice", http.MethodPut, "/admin/closure/announcement", map[string]any{"closes_at": closes, "message": "Thank you."}, &got); res.StatusCode != http.StatusOK ||
		got.State != clientapi.ServiceClosureStateAnnounced || got.ShortNotice {
		t.Fatalf("announce = %d %+v", res.StatusCode, got)
	}
	var inst clientapi.Instance
	if res := c.do("", http.MethodGet, "/instance", nil, &inst); res.StatusCode != http.StatusOK || inst.Closure == nil ||
		inst.Closure.State != clientapi.ServiceClosureStateAnnounced || !inst.Closure.ClosesAt.Equal(closes) ||
		!inst.Closure.ExportUntil.Equal(closes.Add(30*24*time.Hour)) || inst.Closure.Message == nil || *inst.Closure.Message != "Thank you." {
		t.Errorf("GET /instance closure = %d %+v", res.StatusCode, inst.Closure)
	}
	if got := nodeClosure(); got["state"] != "announced" || got["closesAt"] != closes.Format(time.RFC3339) ||
		got["exportUntil"] != closes.Add(30*24*time.Hour).Format(time.RFC3339) {
		t.Errorf("NodeInfo serviceClosure = %v", got)
	}

	// The change of the dates applies at once.
	sooner := closes.Add(-80 * 24 * time.Hour)
	if res := c.do("alice", http.MethodPut, "/admin/closure/announcement", map[string]any{"closes_at": sooner}, &got); res.StatusCode != http.StatusOK || !got.ShortNotice {
		t.Errorf("change of the dates = %d %+v", res.StatusCode, got)
	}
	if got := nodeClosure(); got["closesAt"] != sooner.Format(time.RFC3339) {
		t.Errorf("NodeInfo serviceClosure after the change = %v", got)
	}
	if res := c.do("alice", http.MethodDelete, "/admin/closure/announcement", nil, &got); res.StatusCode != http.StatusOK || got.State != clientapi.ServiceClosureStateNone {
		t.Errorf("withdraw = %d %+v", res.StatusCode, got)
	}
	inst = clientapi.Instance{}
	if res := c.do("", http.MethodGet, "/instance", nil, &inst); res.StatusCode != http.StatusOK || inst.Closure != nil {
		t.Errorf("GET /instance closure after the withdrawal = %d %+v", res.StatusCode, inst.Closure)
	}
	if got := nodeClosure(); got["state"] != "none" {
		t.Errorf("NodeInfo serviceClosure after the withdrawal = %v", got)
	}

	entries, err := c.app.Moderation.AuditLog(t.Context(), appmoderation.AuditQuery{TargetKind: moderation.AuditTargetInstance}, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []moderation.AuditAction{moderation.AuditServiceClosureWithdrawn, moderation.AuditServiceClosureAnnounced, moderation.AuditServiceClosureAnnounced}
	if len(entries) != len(want) {
		t.Fatalf("audit log of the closure = %+v, want %v", entries, want)
	}
	for i, e := range entries {
		if e.Action != want[i] || e.TargetID != "service_closure" || e.ActorKind != moderation.AuditActorUser || e.SourceEventID == "" {
			t.Errorf("audit entry %d = %+v, want %s by a user", i, e, want[i])
		}
	}
}
