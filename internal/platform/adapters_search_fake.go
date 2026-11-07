// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package platform

import (
	"context"
	"errors"
	"log/slog"

	searchfake "github.com/mycroblog/my/internal/adapter/search/fake"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/platform/config"
)

// This file is the only one outside the tests that imports the fake search backend, and only the
// test builds with the build tag e2e_fakes have it (ADR-0149 §3, ADR-0015 §6).
// scripts/go-build-tags.sh fails when cmd/my without the tag depends on the fake, the Dockerfile
// refuses the tag, and scripts/check-release-build.sh refuses a binary built with it.

func init() { searchIndexAdapters["fake"] = newFakeSearchIndex }

// newFakeSearchIndex returns the Index of search = fake, whose documents live in this process
// only. Like the configuration's check, it refuses any profile but development.
func newFakeSearchIndex(ctx context.Context, cfg *config.Config, logger *slog.Logger, _ kernel.Meter) (search.Index, error) {
	if cfg.Profile != "development" {
		return nil, errors.New("fake needs profile = development: it is an index in memory for tests")
	}
	logger.WarnContext(ctx, "this is a test build (build tag e2e_fakes): search = fake keeps its index in this process's memory")
	return searchfake.New(), nil
}
