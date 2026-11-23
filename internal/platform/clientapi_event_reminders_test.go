// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestEventRemindersChangeOnlyWhenAsked checks that PUT /preferences/event_reminders without
// minutes_before and without defaults is a 422 that changes nothing, while an empty
// minutes_before turns the reminders off and defaults restores them.
func TestEventRemindersChangeOnlyWhenAsked(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice")
	put := func(body map[string]any) (string, clientapi.EventReminders) {
		t.Helper()
		var r clientapi.EventReminders
		res := e.send("alice", http.MethodPut, "/preferences/event_reminders", body, nil, &r)
		return e.outcome(res), r
	}
	get := func() clientapi.EventReminders {
		t.Helper()
		var r clientapi.EventReminders
		if res := e.send("alice", http.MethodGet, "/preferences/event_reminders", nil, nil, &r); res.StatusCode != http.StatusOK {
			t.Fatalf("GET = %s", e.outcome(res))
		}
		return r
	}
	before := get()
	for _, body := range []map[string]any{{}, {"defaults": false}} {
		if got, _ := put(body); got[:3] != "422" {
			t.Errorf("PUT %v = %s, want 422", body, got)
		}
	}
	if after := get(); !slices.Equal(after.MinutesBefore, before.MinutesBefore) || after.Defaults != before.Defaults {
		t.Errorf("the reminders after the refused changes = %+v, want %+v", after, before)
	}
	if got, r := put(map[string]any{"minutes_before": []int{}}); got != "200" || len(r.MinutesBefore) != 0 || r.Defaults {
		t.Errorf("PUT an empty minutes_before = %s %+v, want the reminders off", got, r)
	}
	if got, r := put(map[string]any{"defaults": true}); got != "200" || !r.Defaults || !slices.Equal(r.MinutesBefore, before.MinutesBefore) {
		t.Errorf("PUT defaults = %s %+v, want %+v", got, r, before)
	}
}
