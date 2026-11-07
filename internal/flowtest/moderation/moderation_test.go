// SPDX-License-Identifier: AGPL-3.0-only

// Package moderation_test runs the moderation flows (testkit.ModerationFlows, AuditLogFlows,
// AppealFlows, ModerationRetentionFlows, and SignupBlockFlows) once, on SQLite (ADR-0153 §1).
package moderation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func harness(t *testing.T, opts uow.Options) testkit.ModerationHarness {
	db := flowtest.SQLite(t, opts)
	return testkit.ModerationHarness{UnitOfWork: db, Moderation: db.ModerationRepository()}
}

func TestModerationFlows(t *testing.T) {
	t.Parallel()
	testkit.ModerationFlows(t, harness)
}

func TestAuditLogFlows(t *testing.T) {
	t.Parallel()
	testkit.AuditLogFlows(t, harness)
}

func TestAppealFlows(t *testing.T) {
	t.Parallel()
	testkit.AppealFlows(t, harness)
}

func TestModerationRetentionFlows(t *testing.T) {
	t.Parallel()
	testkit.ModerationRetentionFlows(t, harness)
}

func TestSignupBlockFlows(t *testing.T) {
	t.Parallel()
	testkit.SignupBlockFlows(t, harness)
}
