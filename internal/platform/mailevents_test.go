// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform"
)

// POST /mail/events/ses exists only with ses_sns_topic_arn (ADR-0086 §5).
func TestMailEventsEndpoint(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		topic string
		want  int
	}{
		{"without a topic", "", http.StatusNotFound},
		// A body that is no SNS message reaches the endpoint, which refuses it.
		{"with a topic", "arn:aws:sns:us-west-2:123456789012:my-ses", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"}
			if tc.topic != "" {
				env["MY_SES_SNS_TOPIC_ARN"] = tc.topic
			}
			app, err := platform.NewApp(t.Context(), loadConfig(t, env), platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
			if err != nil {
				t.Fatalf("NewApp: %v", err)
			}
			t.Cleanup(func() { _ = app.Close() })
			rec := httptest.NewRecorder()
			app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mail/events/ses", strings.NewReader("{}")))
			if rec.Code != tc.want {
				t.Errorf("POST /mail/events/ses = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
