// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_otel

package platform_test

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
)

// TestNoOTelBuildRefusesOTLPLogs checks that a build without OpenTelemetry refuses
// logs_exporter = otlp at startup, by the configuration or by NewApp, naming the setting.
func TestNoOTelBuildRefusesOTLPLogs(t *testing.T) {
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	env := map[string]string{
		"MY_HOST": "example.test", "MY_KEK": base64.StdEncoding.EncodeToString(kek), "MY_BLOB_FS_DIR": t.TempDir(),
		"MY_DATABASE_URL": sqlitetest.NewFile(t), "MY_LOGS_EXPORTER": config.LogsOTLP,
	}
	cfg, _, err := config.Load(config.Source{LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }})
	if err == nil {
		var a *platform.App
		a, err = platform.NewApp(t.Context(), cfg, platform.AppOptions{})
		if err == nil {
			_ = a.Close()
			t.Fatal("NewApp with logs_exporter = otlp succeeded in the no_otel build")
		}
	}
	if !strings.Contains(strings.ToLower(err.Error()), "otlp") {
		t.Errorf("the refusal %q does not name otlp", err)
	}
}
