// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestInstanceLinkCardSettings checks that GET /instance says what the link card crawler does,
// for the page that tells site operators about it (ADR-0131), and follows the settings at once.
func TestInstanceLinkCardSettings(t *testing.T) {
	e, _ := newIncomeEnv(t)
	read := func() clientapi.Instance {
		t.Helper()
		return decode[clientapi.Instance](t, e.send(t, http.MethodGet, "/instance", "", ""))
	}
	if got := read(); !got.LinkCardsEnabled || got.LinkCardsIgnoreRobotsTxt {
		t.Errorf("by default: enabled %v, ignore robots.txt %v, want true and false", got.LinkCardsEnabled, got.LinkCardsIgnoreRobotsTxt)
	}
	setSetting(t, e, "link_cards_ignore_robots_txt", "true")
	if got := read(); !got.LinkCardsEnabled || !got.LinkCardsIgnoreRobotsTxt {
		t.Errorf("ignoring robots.txt: enabled %v, ignore robots.txt %v, want true and true", got.LinkCardsEnabled, got.LinkCardsIgnoreRobotsTxt)
	}
	setSetting(t, e, "link_cards_enabled", "false")
	if got := read(); got.LinkCardsEnabled {
		t.Errorf("link cards off: enabled %v, want false", got.LinkCardsEnabled)
	}
}
