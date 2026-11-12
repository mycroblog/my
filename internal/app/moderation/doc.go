// SPDX-License-Identifier: AGPL-3.0-only

// Package moderation holds the use cases and port definitions of the moderation module: reports, domain blocks, suspensions, and the audit log.
// It is the module's public API; other modules reach the moderation module only through this package.
//
// An action writes its event, the statuses it names, and its restriction rows in one transaction,
// under the lock of its account or domain, so that the actions and lifts on one target run one
// after another and each projects the restrictions that the others committed; the timeline's
// copy of the restrictions follows in the same transaction, and the notifications and the work on
// statuses go to jobs. AccountPolicy answers from the account's restrictions in effect and those
// of its domain and the domain's parents, read on every call; the lifecycle and the review of the
// registration are the other inputs of what an account may do, which their modules decide.
//
// A suspension severs the follows both ways and records, per local account that lost approved
// follows, how many. A remote account's suspension severs them in its own transaction, since it
// touches as many accounts as follow it here; a domain's has no bound, so a job severs them one
// local account per transaction, even when the action is lifted before the job runs.
//
// The audit log lists whether each target still exists with one query per kind of target: the
// module's own tables for reports, appeals, and sign-up blocks; the account module for accounts;
// and, through the AuditTargets port, the modules that write audit entries but cannot be imported
// here (roles, custom emoji and their requests, webhooks, relays). The other targets are never
// removed.
//
// The hourly purge removes the records past their retention in order: actions, then reports
// (a report that an action cites goes with the action), then sign-up blocks, then the audit log,
// in small batches and oldest first, rereading the retention every run and leaving the records
// of accounts under a hold. Severances, and the notifications that point at a purged record,
// stay.
package moderation
