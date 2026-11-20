// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestInstanceLandingSettings checks that GET /instance says which parts of the landing page the
// operator shows, both by default, and follows each setting on its own (#3080).
func TestInstanceLandingSettings(t *testing.T) {
	e, _ := newIncomeEnv(t)
	read := func() clientapi.Instance {
		t.Helper()
		return decode[clientapi.Instance](t, e.send(t, http.MethodGet, "/instance", "", ""))
	}
	if got := read(); !got.LandingFediverseExplanation || !got.LandingFeatures {
		t.Errorf("by default: explanation %v, features %v, want true and true", got.LandingFediverseExplanation, got.LandingFeatures)
	}
	setSetting(t, e, "landing_fediverse_explanation", "false")
	if got := read(); got.LandingFediverseExplanation || !got.LandingFeatures {
		t.Errorf("explanation hidden: explanation %v, features %v, want false and true", got.LandingFediverseExplanation, got.LandingFeatures)
	}
	setSetting(t, e, "landing_fediverse_explanation", "true")
	setSetting(t, e, "landing_features", "false")
	if got := read(); !got.LandingFediverseExplanation || got.LandingFeatures {
		t.Errorf("features hidden: explanation %v, features %v, want true and false", got.LandingFediverseExplanation, got.LandingFeatures)
	}
}
