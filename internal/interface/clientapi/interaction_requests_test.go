// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"net/http"
	"testing"
)

// TestInteractionRequestsWithoutTheModule checks that a server without the interaction requests
// answers their list 404, as it answers the answers to them, rather than an empty page.
func TestInteractionRequestsWithoutTheModule(t *testing.T) {
	e := newAuthEnv(t, nil)
	if res := e.send(t, http.MethodGet, "/interaction_requests", "read", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET /interaction_requests = %d, want 404", res.StatusCode)
	}
	if res := e.send(t, http.MethodPost, "/interaction_requests/"+sampleStatusID+"/accept", "write", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("POST accept = %d, want 404", res.StatusCode)
	}
}
