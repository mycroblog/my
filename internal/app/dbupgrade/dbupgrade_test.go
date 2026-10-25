// SPDX-License-Identifier: AGPL-3.0-only

package dbupgrade_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mycroblog/my/internal/app/dbupgrade"
)

// failingRepository fails every read; a service that needs no read must not call it.
type failingRepository struct{ dbupgrade.Repository }

func (failingRepository) Runs(context.Context) ([]dbupgrade.Run, error) {
	return nil, errors.New("unreachable")
}

func TestNewServiceRefusesInvalidRegistrations(t *testing.T) {
	t.Parallel()
	rebuild := func(context.Context, string) error { return nil }
	for name, opts := range map[string]dbupgrade.Options{
		"a step without a key":        {Steps: []dbupgrade.Step{{}}},
		"a key registered twice":      {Steps: []dbupgrade.Step{{Key: "a"}, {Key: "a"}}},
		"projections without rebuild": {Steps: []dbupgrade.Step{{Key: "a", Projections: []string{"content"}}}},
		"an unregistered prerequisite": {
			Steps: []dbupgrade.Step{{Key: "a"}}, Prerequisites: []dbupgrade.Prerequisite{{Key: "b", Via: "v0.2.0"}},
			RebuildProjection: rebuild,
		},
		"a prerequisite without a release": {Steps: []dbupgrade.Step{{Key: "a"}}, Prerequisites: []dbupgrade.Prerequisite{{Key: "a"}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Error("NewService did not panic")
				}
			}()
			dbupgrade.NewService(opts)
		})
	}
}

func TestServiceWithoutStepsReadsNothing(t *testing.T) {
	t.Parallel()
	svc := dbupgrade.NewService(dbupgrade.Options{Repository: failingRepository{}})
	if err := svc.Ready(t.Context()); err != nil {
		t.Errorf("Ready = %v", err)
	}
	if r, err := svc.Check(t.Context()); err != nil || r.NewDatabase || len(r.Missing) != 0 {
		t.Errorf("Check = %+v, %v", r, err)
	}
	if done, err := svc.Upgrade(t.Context(), nil); err != nil || len(done) != 0 {
		t.Errorf("Upgrade = %v, %v", done, err)
	}
}
