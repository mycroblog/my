// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"strings"
	"testing"
	"time"
)

// TestLoadMaintenance checks that the maintenance mode starts with adapter settings that are wrong or missing,
// reading only its own settings (ADR-0084 §2).
func TestLoadMaintenance(t *testing.T) {
	env := map[string]string{
		"MY_HOST": "social.example.org", "MY_MAINTENANCE": "true", "MY_STATUS_PAGE_URL": "https://status.example.org",
		"MY_MAINTENANCE_MESSAGE": "Upgrading the database.", "MY_MAINTENANCE_UNTIL": "2026-10-24T12:00:00+09:00",
		// Wrong or unreachable adapter settings, and no kek.
		"MY_DB": "nosuchdb", "MY_DATABASE_URL": "postgres://unreachable.invalid/my", "MY_QUEUE": "nosuchqueue",
		"MY_BLOB": "nosuchblob", "MY_KV": "valkey", "MY_MAIL": "smtp",
	}
	if _, _, err := Load(source(env, nil)); err == nil {
		t.Fatal("Load accepted the wrong adapter settings")
	}
	cfg, settings, err := LoadMaintenance(source(env, nil))
	if err != nil {
		t.Fatalf("LoadMaintenance: %v", err)
	}
	if !cfg.Maintenance || cfg.Host != "social.example.org" || cfg.WebOrigin != "https://social.example.org" || cfg.Listen != ":8080" ||
		cfg.StatusPageURL != "https://status.example.org" || cfg.MaintenanceMessage != "Upgrading the database." ||
		!cfg.MaintenanceUntil.Equal(time.Date(2026, 10, 24, 3, 0, 0, 0, time.UTC)) || cfg.MaintenanceRetryAfter != 600*time.Second {
		t.Errorf("cfg = %+v", cfg)
	}
	for _, s := range settings {
		if !maintenanceKeys[s.Key] {
			t.Errorf("LoadMaintenance read %s", s.Key)
		}
	}
	if cfg.DB != "" || cfg.DatabaseURL != "" || cfg.Queue != "" {
		t.Errorf("LoadMaintenance set adapter settings: %+v", cfg)
	}
}

// TestMaintenanceSettings checks the forms that every load validates, the normal one included.
func TestMaintenanceSettings(t *testing.T) {
	for _, tt := range []struct{ env, value, want string }{
		{"MY_MAINTENANCE", "yes", "maintenance (MY_MAINTENANCE)"},
		{"MY_MAINTENANCE_SERVER_NAME", "  ", "maintenance_server_name"},
		{"MY_MAINTENANCE_SERVER_NAME", "two\nlines", "maintenance_server_name"},
		{"MY_MAINTENANCE_MESSAGE", strings.Repeat("あ", MaxMaintenanceMessage+1), "maintenance_message"},
		{"MY_MAINTENANCE_UNTIL", "2026-10-24 03:00", "maintenance_until"},
		{"MY_MAINTENANCE_RETRY_AFTER", "0", "maintenance_retry_after"},
		{"MY_MAINTENANCE_RETRY_AFTER", "86401", "maintenance_retry_after"},
		{"MY_STATUS_PAGE_URL", "http://status.example.org", "status_page_url"},
	} {
		for name, load := range map[string]func(Source) (*Config, []Setting, error){"Load": Load, "LoadMaintenance": LoadMaintenance} {
			if _, _, err := load(source(with(minimal(), tt.env, tt.value), nil)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("%s with %s=%q: err = %v, want a %s error", name, tt.env, tt.value, err, tt.want)
			}
		}
	}
	cfg, _, err := Load(source(with(minimal(), "MY_MAINTENANCE_MESSAGE", strings.Repeat("あ", MaxMaintenanceMessage), "MY_MAINTENANCE_RETRY_AFTER", "86400"), nil))
	if err != nil || cfg.Maintenance || cfg.MaintenanceRetryAfter != 86400*time.Second {
		t.Errorf("Load = %+v, %v", cfg, err)
	}
}

func TestMaintenanceRequested(t *testing.T) {
	for _, tt := range []struct {
		env   map[string]string
		files map[string]string
		want  bool
	}{
		{map[string]string{}, nil, false},
		{map[string]string{"MY_MAINTENANCE": "true", "MY_DB": "nosuchdb"}, nil, true},
		{map[string]string{"MY_CONFIG": "my.toml"}, map[string]string{"my.toml": "maintenance = \"true\"\n"}, true},
		{map[string]string{"MY_CONFIG": "my.toml", "MY_MAINTENANCE": "false"}, map[string]string{"my.toml": "maintenance = \"true\"\n"}, false},
	} {
		got, err := MaintenanceRequested(source(tt.env, tt.files))
		if err != nil || got != tt.want {
			t.Errorf("%v %v: MaintenanceRequested = %t, %v; want %t", tt.env, tt.files, got, err, tt.want)
		}
	}
	if _, err := MaintenanceRequested(source(map[string]string{"MY_MAINTENANCE": "on"}, nil)); err == nil {
		t.Error("MaintenanceRequested accepted on")
	}
}

func TestProfileRequested(t *testing.T) {
	for _, tt := range []struct {
		env   map[string]string
		files map[string]string
		want  string
	}{
		{map[string]string{}, nil, "production"},
		{map[string]string{"MY_PROFILE": "development", "MY_DB": "nosuchdb"}, nil, "development"},
		{map[string]string{"MY_CONFIG": "my.toml"}, map[string]string{"my.toml": "profile = \"development\"\n"}, "development"},
		{map[string]string{"MY_CONFIG": "my.toml", "MY_PROFILE": "production"}, map[string]string{"my.toml": "profile = \"development\"\n"}, "production"},
	} {
		got, err := ProfileRequested(source(tt.env, tt.files))
		if err != nil || got != tt.want {
			t.Errorf("%v %v: ProfileRequested = %q, %v; want %q", tt.env, tt.files, got, err, tt.want)
		}
	}
	if _, err := ProfileRequested(source(map[string]string{"MY_PROFILE": "staging"}, nil)); err == nil {
		t.Error("ProfileRequested accepted staging")
	}
}
