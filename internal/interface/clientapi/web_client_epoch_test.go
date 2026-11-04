// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestWebClientEpochMatchesTheClient checks that the two constants of ADR-0028 §12 move together.
func TestWebClientEpochMatchesTheClient(t *testing.T) {
	src, err := os.ReadFile("../../../app/web/src/lib/webClientEpoch.ts")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("export const webClientEpoch = %d;", clientapi.WebClientEpoch)
	if !strings.Contains(string(src), want) {
		t.Errorf("webClientEpoch.ts does not have %q", want)
	}
	header := fmt.Sprintf("export const webClientEpochHeader = %q;", clientapi.HeaderWebClientEpoch)
	if !strings.Contains(string(src), header) {
		t.Errorf("webClientEpoch.ts does not have %q", header)
	}
}
