// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package platform

import (
	"context"
	"errors"
	"log/slog"

	mailfake "github.com/mycroblog/my/internal/adapter/mail/fake"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// This file is the only one outside the tests that imports the fake mailer, and only the test
// builds with the build tag e2e_fakes have it (ADR-0015 §6). scripts/go-fakes.sh fails when
// cmd/my without the tag depends on the fake, the Dockerfile refuses the tag, and
// scripts/check-release-build.sh refuses a binary built with it.

func init() { testBuildMailers["fake"] = newFakeMailer }

// newFakeMailer returns the Mailer of mail = fake, which writes each message whole to the log
// instead of sending it. Like the configuration's check, it refuses any profile but development,
// so that no production instance counts on mail that reaches nobody or logs the links of its
// users.
func newFakeMailer(ctx context.Context, cfg *config.Config, logger *slog.Logger) (kernel.Mailer, error) {
	if cfg.Profile != "development" {
		return nil, errors.New("mail = fake needs profile = development: it sends nothing and writes each message to the log")
	}
	logger.WarnContext(ctx, "this is a test build (build tag e2e_fakes): mail = fake sends nothing and writes each message to the log")
	return mailfake.New(logger), nil
}
