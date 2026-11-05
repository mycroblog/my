// SPDX-License-Identifier: AGPL-3.0-only

package translation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
)

type limiterRepository struct {
	Repository
	months []string
	over   bool
}

func (r *limiterRepository) AddUsageWithin(_ context.Context, _ uow.Tx, _, month string, _ int64, _ Limits) (bool, error) {
	r.months = append(r.months, month)
	return !r.over, nil
}

type limiterUnitOfWork struct{ uow.UnitOfWork }

func (limiterUnitOfWork) Tx(_ context.Context, fn func(uow.Tx) error) error { return fn(nil) }

type limiterClock struct{ now time.Time }

func (c limiterClock) Now() time.Time { return c.now }

func TestUsageLimiter(t *testing.T) {
	t.Parallel()
	// 2026-12-01 08:00 in Tokyo is still November in UTC.
	tokyo := time.FixedZone("JST", 9*60*60)
	repo := &limiterRepository{}
	limits := Limits{Account: 10, Server: 10}
	l := UsageLimiter{
		UnitOfWork: limiterUnitOfWork{}, Repository: repo,
		Clock:  limiterClock{time.Date(2026, 12, 1, 8, 0, 0, 0, tokyo)},
		Limits: func(context.Context) (Limits, error) { return limits, nil },
	}
	if err := l.Reserve(t.Context(), "alice", 1); err != nil {
		t.Fatal(err)
	}
	if len(repo.months) != 1 || repo.months[0] != "2026-11" {
		t.Errorf("months = %v, want the UTC month", repo.months)
	}

	repo.over = true
	if err := l.Reserve(t.Context(), "alice", 1); !errors.Is(err, ErrLimitReached) {
		t.Errorf("over a limit = %v, want ErrLimitReached", err)
	}

	// A limit of 0 refuses before the database, even for no characters.
	repo.over, repo.months = false, nil
	for _, zero := range []Limits{{Account: 0, Server: 10}, {Account: 10, Server: 0}} {
		limits = zero
		if err := l.Reserve(t.Context(), "alice", 0); !errors.Is(err, ErrLimitReached) {
			t.Errorf("limits %+v = %v, want ErrLimitReached", zero, err)
		}
	}
	if len(repo.months) != 0 {
		t.Errorf("a limit of 0 reached the repository: %v", repo.months)
	}
}

func TestConfiguredProviders(t *testing.T) {
	t.Parallel()
	settings := ProviderSettings{}
	c := ConfiguredProviders{
		Registered: []Provider{{ID: "one"}, {ID: "two"}},
		Settings:   func(context.Context) (ProviderSettings, error) { return settings, nil },
	}
	if got, err := c.Providers(t.Context()); err != nil || len(got) != 0 {
		t.Errorf("with none enabled = %v, %v", got, err)
	}
	settings = ProviderSettings{
		Enabled:      []string{"gone", "two", "one"},
		Visibilities: map[string][]string{"one": {}, "two": {"direct"}},
	}
	got, err := c.Providers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "two" || got[1].ID != "one" {
		t.Fatalf("providers = %+v", got)
	}
	if !got[0].Allows("direct") || got[0].Allows("public") {
		t.Errorf("two allows %v", got[0].Allowed())
	}
	if len(got[1].Allowed()) != 0 {
		t.Errorf("one, named with nothing, allows %v", got[1].Allowed())
	}
	settings.Visibilities = nil
	got, err = c.Providers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].Allows("local") || got[0].Allows("private") || got[0].Allows("direct") {
		t.Errorf("not named, two allows %v, want the default", got[0].Allowed())
	}
}
