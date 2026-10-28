// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A new instance that has never federated has no instance actor yet; the overview of the landing
// page still answers, with the instance's start as when its actor is made.
func TestInstanceOverviewBeforeTheInstanceActor(t *testing.T) {
	t.Parallel()
	h := newClientAPIHandler(t)

	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/instance/overview", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/instance/overview = %d: %s", res.Code, res.Body)
	}
	var overview struct {
		Counts *struct {
			StartedAt string `json:"started_at"`
		} `json:"counts"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if overview.Counts == nil || overview.Counts.StartedAt == "" {
		t.Errorf("overview = %s, want the counts with started_at", res.Body)
	}
}
