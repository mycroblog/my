// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/dbupgrade"
	"github.com/mycroblog/my/internal/platform"
)

func TestReadinessRequiresTheDataUpgradeSteps(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	s := startApp(t, cfg, platform.AppOptions{DataUpgrades: &platform.DataUpgrades{
		Steps: []dbupgrade.Step{{Key: "test.backfill", Description: "test step", Projections: []string{"account"}}},
	}}, platform.ServeOptions{}, nil)

	if resp, _ := s.do(t, http.MethodGet, platform.PathReadiness, nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("readiness before my db upgrade = %d, want 503", resp.StatusCode)
	}
	if logs := s.logs.String(); !strings.Contains(logs, "not ready") || !strings.Contains(logs, "missing test.backfill") {
		t.Errorf("logs = %q, want the missing step", logs)
	}
	if _, err := s.app.DataUpgrades.Upgrade(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if resp, body := s.do(t, http.MethodGet, platform.PathReadiness, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("readiness after my db upgrade = %d %q, want 200", resp.StatusCode, body)
	}
}

func TestNewAppRefusesAStepOfAnUnknownProjection(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	_, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{DataUpgrades: &platform.DataUpgrades{
		Steps: []dbupgrade.Step{{Key: "test.backfill", Projections: []string{"no_such_projection"}}},
	}})
	if err == nil || !strings.Contains(err.Error(), "data upgrade step test.backfill") {
		t.Errorf("NewApp = %v, want the unknown projection refused", err)
	}
}
