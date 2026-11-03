// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform"
)

// everyJobType returns the job types of a server with every setting that adds some.
func everyJobType(t *testing.T) []string {
	t.Helper()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true),
		"MY_MAIL":         "smtp", "MY_MAIL_FROM": "noreply@example.test", "MY_SMTP_HOST": "smtp.example.test",
		"MY_PAYMENTS_PROVIDER": "stripe", "MY_PAYMENTS_STRIPE_SECRET_KEY": "rk_test_x", "MY_PAYMENTS_STRIPE_WEBHOOK_SECRET": "whsec_x",
		"MY_VAPID_PRIVATE_KEY": "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app.Handlers.Types()
}

// TestJobTypeNames keeps one word for each act in the job types: purge deletes past a limit, and
// expire ends what has lapsed. The older words stay out of new types.
func TestJobTypeNames(t *testing.T) {
	types := everyJobType(t)
	for _, want := range []string{
		"federation.purge_records", "federation.update_poll", "federation.refresh_stale_actors", "federation.refetch_walk",
		"media.purge_unattached", "media.purge_remote_cache", "content.purge_link_cards", "graph.expire_mutes", "kv.purge_expired",
		"notification.purge_error_group_notices", "diagnostics.purge_error_counts", "identity.purge_unused_oauth_clients",
		"supporters.purge_payment_receipts", "supporters.reconcile_payments", "supporters.apply_payment_event",
		"supporters.sync_due", "supporters.cancel_for_closure", "identity.reject_for_closure", "identity.queue_closure_reminders",
		"instance.closure_delete_actors", "instance.check_closure_delivery", "notification.closure_fanout",
		"notification.closure_due", "notification.post_subscribers",
	} {
		if !slices.Contains(types, want) {
			t.Errorf("no job type %s", want)
		}
	}
	for _, typ := range types {
		_, name, _ := strings.Cut(typ, ".")
		for _, word := range []string{"cleanup", "prune"} {
			if strings.Contains(name, word) && typ != "account.prune_remote_profiles" {
				t.Errorf("job type %s uses %q; say purge or expire", typ, word)
			}
		}
	}
}

// TestRunningGuideLayoutNamesRealJobTypes checks the job types of the layout for email and
// notifications in the operators' guide: a prefix is a code module, so the layout names each type.
func TestRunningGuideLayoutNamesRealJobTypes(t *testing.T) {
	types := everyJobType(t)
	guide, err := os.ReadFile("../../docs/operations/running.md")
	if err != nil {
		t.Fatal(err)
	}
	_, after, found := strings.Cut(string(guide), "| Keep email and notifications quick | `worker_job_types = ")
	if !found {
		t.Fatal("the guide has no layout for email and notifications")
	}
	list, _, _ := strings.Cut(after, "`")
	for typ := range strings.SplitSeq(list, ",") {
		if strings.HasSuffix(typ, ".*") {
			t.Errorf("the layout uses the prefix %s, which also takes jobs that are not email or notifications", typ)
		}
		if !slices.Contains(types, typ) {
			t.Errorf("the layout names %s, which is not a job type", typ)
		}
	}
}
