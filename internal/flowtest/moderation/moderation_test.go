// SPDX-License-Identifier: AGPL-3.0-only

// Package moderation_test runs the moderation flows (moderationtest.ModerationFlows, AuditLogFlows,
// AppealFlows, ModerationRetentionFlows, and SignupBlockFlows) once, on SQLite (ADR-0153 §1).
package moderation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/moderationtest"
)

func harness(t *testing.T, opts uow.Options) moderationtest.ModerationHarness {
	db := flowtest.SQLite(t, opts)
	return moderationtest.ModerationHarness{UnitOfWork: db, Moderation: db.ModerationRepository()}
}

func TestModerationFlows(t *testing.T) {
	t.Parallel()
	moderationtest.ModerationFlows(t, harness)
}

func TestAuditLogFlows(t *testing.T) {
	t.Parallel()
	moderationtest.AuditLogFlows(t, harness)
}

func TestAppealFlows(t *testing.T) {
	t.Parallel()
	moderationtest.AppealFlows(t, harness)
}

func TestModerationRetentionFlows(t *testing.T) {
	t.Parallel()
	moderationtest.ModerationRetentionFlows(t, harness)
}

func TestSignupBlockFlows(t *testing.T) {
	t.Parallel()
	moderationtest.SignupBlockFlows(t, harness)
}
