// SPDX-License-Identifier: AGPL-3.0-only

package config

import "testing"

// TestLoadSchemaApply checks that my schema apply needs only the database's settings: no host, no
// kek, and wrong settings of other adapters.
func TestLoadSchemaApply(t *testing.T) {
	env := map[string]string{
		"MY_DB": "postgres", "MY_DATABASE_URL": "postgres://db/my", "MY_DATABASE_DEV_URL": "postgres://db/my_dev",
		"MY_QUEUE": "nosuchqueue", "MY_BLOB": "nosuchblob",
	}
	skipExcluded(t, env)
	if _, _, err := Load(source(env, nil)); err == nil {
		t.Fatal("Load accepted a configuration without host and kek")
	}
	cfg, settings, err := LoadSchemaApply(source(env, nil))
	if err != nil {
		t.Fatalf("LoadSchemaApply: %v", err)
	}
	if cfg.DB != "postgres" || cfg.DatabaseURL != "postgres://db/my" || cfg.DatabaseDevURL != "postgres://db/my_dev" {
		t.Errorf("cfg = %+v", cfg)
	}
	for _, s := range settings {
		if !schemaApplyKeys[s.Key] {
			t.Errorf("LoadSchemaApply read %s", s.Key)
		}
	}
	if _, _, err := LoadSchemaApply(source(map[string]string{"MY_DB": "sqlite"}, nil)); err == nil {
		t.Error("LoadSchemaApply accepted a configuration without database_url")
	}
}
