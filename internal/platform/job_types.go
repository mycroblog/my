// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"slices"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// jobTypes lists every job type of this release, so that a command can check the settings that
// name job types without building the App. A test keeps it equal to the registered handlers.
var jobTypes = []string{
	"account.erase_deleted",
	"account.execute_deletions",
	"account.prune_remote_profiles",
	"content.auto_delete_statuses",
	"content.close_poll",
	"content.fetch_link_card",
	"content.purge_link_cards",
	"content.purge_relayed_statuses",
	"content.purge_remote_emojis",
	"content.purge_remote_statuses",
	"diagnostics.purge_error_counts",
	"hook.purge_decisions",
	"federation.deliver",
	"federation.fanout",
	"federation.fetch_featured",
	"federation.fetch_object",
	"federation.fetch_outbox",
	"federation.inbox",
	"federation.observe_instance",
	"federation.probe_instances",
	"federation.purge_records",
	"federation.refetch_status",
	"federation.refetch_walk",
	"federation.refresh_actor",
	"federation.refresh_stale_actors",
	"federation.update_poll",
	"federation.verify_quote",
	"finance.import_billing",
	"graph.expire_mutes",
	"graph.migrate_follows",
	"identity.build_export",
	"identity.closure_email",
	"identity.closure_email_fanout",
	"identity.deactivation_email",
	"identity.expire_exports",
	"identity.expire_imports",
	"identity.password_notice_email",
	"identity.password_reset_email",
	"identity.process_import",
	"identity.purge_auth_records",
	"identity.purge_origins",
	"identity.purge_unused_oauth_clients",
	"identity.queue_closure_reminders",
	"identity.reject_for_closure",
	"identity.verification_email",
	"instance.check_closure_delivery",
	"instance.closure_delete_actors",
	"instance.notify_announcement",
	"kv.purge_expired",
	"media.process",
	"media.purge_remote_cache",
	"media.purge_replaced",
	"media.purge_unattached",
	"media.reprocess",
	"media.reprocess_walk",
	"metrics.snapshot",
	"moderation.expire_restrictions",
	"moderation.purge_records",
	"moderation.sever_domain",
	"notification.closure_due",
	"notification.closure_fanout",
	"notification.email",
	"notification.moderator_email",
	"notification.post_subscribers",
	"notification.purge_error_group_notices",
	"notification.push",
	"notification.supporter_email",
	"projection.verify",
	"queue.purge_dead_jobs",
	"search.narrow",
	"search.rebuild",
	"search.sync_author",
	"search.sync_domain",
	"search.sync_status",
	"stream.sweep",
	"supporters.apply_payment_event",
	"supporters.cancel_for_closure",
	"supporters.command",
	"supporters.ensure_prices",
	"supporters.purge_payment_receipts",
	"supporters.reconcile_payments",
	"supporters.sync",
	"supporters.sync_due",
	"voice.apply",
	"voice.apply_media_event",
	"voice.check_account",
	"voice.check_block",
	"voice.check_hosts",
	"voice.notify_started",
	"voice.purge_records",
	"voice.sweep",
	"webhook.deliver",
	"webhook.purge_messages",
}

// JobTypes returns every job type of this release, sorted.
func JobTypes() []string { return slices.Clone(jobTypes) }

// jobTypesOf returns the job types that a process of cfg registers: some exist only with their
// setting.
func jobTypesOf(cfg *config.Config) []string {
	absent := map[string]bool{}
	if !sendsEmail(cfg) {
		absent["notification.email"], absent["notification.moderator_email"], absent["notification.supporter_email"] = true, true, true
	}
	if cfg.VAPIDPrivateKey == "" {
		absent["notification.push"] = true
	}
	if cfg.MetricsExporter == "" || cfg.MetricsExporter == "none" {
		absent["metrics.snapshot"] = true
	}
	if cfg.Stream != "sns" && cfg.Stream != "pubsub" {
		absent["stream.sweep"] = true
	}
	if cfg.PaymentsProvider == "" || cfg.PaymentsProvider == "none" {
		absent["supporters.apply_payment_event"], absent["supporters.reconcile_payments"], absent["supporters.purge_payment_receipts"] = true, true, true
	}
	return slices.DeleteFunc(JobTypes(), func(t string) bool { return absent[t] })
}

// JobTypeFindings checks the settings that name job types against the job types of cfg, which
// the start would refuse: worker_job_types, worker_job_types_except, and the routes of the queue.
func JobTypeFindings(cfg *config.Config) []config.Finding {
	var findings []config.Finding
	types := jobTypesOf(cfg)
	if len(cfg.WorkerJobTypes) > 0 || len(cfg.WorkerJobTypesExcept) > 0 {
		key := "worker_job_types"
		if len(cfg.WorkerJobTypes) == 0 {
			key = "worker_job_types_except"
		}
		if _, _, err := queue.SelectJobTypes(types, cfg.WorkerJobTypes, cfg.WorkerJobTypesExcept); err != nil {
			findings = append(findings, config.Finding{Severity: config.SeverityError, Key: key, Message: "cannot run: " + err.Error()})
		}
	}
	if routes := cfg.QueueRoutes(); len(routes) > 0 {
		patterns := make([]string, len(routes))
		for i, r := range routes {
			patterns[i] = r.Pattern
		}
		if err := queue.CheckJobTypePatterns(types, patterns); err != nil {
			findings = append(findings, config.Finding{Severity: config.SeverityError, Key: "queue_" + cfg.Queue + "_routes", Message: "cannot route: " + err.Error()})
		}
	}
	return findings
}
