// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestInstanceAutoDeleteLimit checks that GET /instance publishes the server maximum of the
// automatic deletion while it is on, for the about page, and never the server default
// (ADR-0151 §8).
func TestInstanceAutoDeleteLimit(t *testing.T) {
	e, _ := newIncomeEnv(t)
	read := func() clientapi.Instance {
		t.Helper()
		return decode[clientapi.Instance](t, e.send(t, http.MethodGet, "/instance", "", ""))
	}
	setSetting(t, e, "auto_delete_default_enabled", "true")
	if got := read(); got.AutoDeleteLimit != nil {
		t.Errorf("without a maximum: auto_delete_limit = %+v, want absent", got.AutoDeleteLimit)
	}
	setSetting(t, e, "auto_delete_max_age_days", "90")
	setSetting(t, e, "auto_delete_max_kept_pins", "2")
	setSetting(t, e, "auto_delete_max_allow_media", "true")
	want := &clientapi.AutoDeleteLimit{MaxAgeDays: 90, AllowPinned: true, KeptPins: 2, AllowMedia: true}
	if got := read(); !reflect.DeepEqual(got.AutoDeleteLimit, want) {
		t.Errorf("auto_delete_limit = %+v, want %+v", got.AutoDeleteLimit, want)
	}
}
