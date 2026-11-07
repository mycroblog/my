// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// moderationProjectionEqualsRebuild runs random actions on accounts and domains, lifts, the
// expiry job, and the handling of reports while the clock advances, and checks after each round
// that appmoderation.RebuildProjections derives the restrictions and the report states that the
// commands wrote: the rows whose end has passed stay until the expiry job removes them.
func moderationProjectionEqualsRebuild(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(589, 20261020)) //nolint:gosec // deterministic test input, not a secret
	durations := []time.Duration{0, time.Second, 5 * time.Second, time.Minute}
	accountKinds := map[string][]moderation.ActionKind{
		e.remote: {moderation.ActionLimit, moderation.ActionSuspend, moderation.ActionForceSensitive},
		e.local:  {moderation.ActionWarn, moderation.ActionLimit, moderation.ActionRestrictPosting, moderation.ActionDisableLogin, moderation.ActionSuspend},
	}
	targets := []string{e.remote, e.local}
	domains := []string{"remote.example", "sub.remote.example", "other.example"}
	domainKinds := []moderation.DomainActionKind{moderation.DomainLimit, moderation.DomainSuspend, moderation.DomainRejectMedia}
	var actions, domainActions, reports []string
	pick := func(ids []string) string { return ids[rng.IntN(len(ids))] }
	for round := range 3 {
		for range 60 {
			e.clock.Advance(time.Duration(rng.IntN(3000)+1) * time.Millisecond)
			var err error
			switch rng.IntN(9) {
			case 0, 1:
				target := pick(targets)
				var a moderation.Action
				a, err = e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
					TargetAccountID: target, Kind: accountKinds[target][rng.IntN(len(accountKinds[target]))],
					ReasonCategory: moderation.CategoryOther, Duration: durations[rng.IntN(len(durations))],
				})
				actions = append(actions, a.ID)
			case 2:
				if len(actions) > 0 {
					err = e.svc.LiftAccountAction(ctx, pick(actions), "", "")
				}
			case 3:
				var a moderation.DomainAction
				a, err = e.svc.IssueDomainAction(ctx, appmoderation.DomainActionInput{
					Domain: pick(domains), Kind: domainKinds[rng.IntN(len(domainKinds))], Duration: durations[rng.IntN(len(durations))],
				})
				domainActions = append(domainActions, a.ID)
			case 4:
				if len(domainActions) > 0 {
					err = e.svc.LiftDomainAction(ctx, pick(domainActions), "")
				}
			case 5:
				if _, err = e.svc.ExpireAccountRestrictions(ctx); err == nil {
					_, err = e.svc.ExpireDomainRestrictions(ctx)
				}
			case 6:
				var r moderation.Report
				r, _, err = e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
					URI: fmt.Sprintf("https://remote.example/flags/%d", len(reports)), ReporterAccountID: e.reports, TargetAccountID: e.local,
				})
				reports = append(reports, r.ID)
			case 7:
				if len(reports) > 0 {
					_, err = e.svc.AssignReport(ctx, pick(reports), pick([]string{"", NamedID("u1"), NamedID("u2")}), "")
				}
			default:
				if len(reports) == 0 {
					continue
				}
				id := pick(reports)
				if rng.IntN(2) == 0 {
					_, err = e.svc.ResolveReport(ctx, id, []moderation.ReportOutcome{moderation.OutcomeActioned, moderation.OutcomeDismissed}[rng.IntN(2)], "")
				} else {
					_, err = e.svc.ReopenReport(ctx, id, "")
				}
			}
			if err != nil && !errors.Is(err, appmoderation.ErrNotLiftable) && !errors.Is(err, appmoderation.ErrReportNotResolved) {
				t.Fatalf("command = %v", err)
			}
		}
		incremental := snapshotModerationProjections(t, e, targets, domains, reports)
		if len(incremental.accounts) == 0 || len(incremental.domains) == 0 || len(incremental.reports) == 0 {
			t.Fatalf("round %d: projections are empty: %+v", round, incremental)
		}
		if err := appmoderation.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Moderation, e.clock); err != nil {
			t.Fatalf("RebuildProjections = %v", err)
		}
		rebuilt := snapshotModerationProjections(t, e, targets, domains, reports)
		if !reflect.DeepEqual(incremental, rebuilt) {
			t.Fatalf("round %d: incremental projections differ from a rebuild:\nincremental %+v\nrebuilt     %+v", round, incremental, rebuilt)
		}
	}
}

type moderationProjections struct {
	accounts []moderation.RestrictionRow
	domains  []moderation.RestrictionRow
	reports  []moderation.ReportStatus
}

func snapshotModerationProjections(t *testing.T, e *moderationEnv, accounts, domains, reports []string) moderationProjections {
	t.Helper()
	ctx := t.Context()
	var out moderationProjections
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		out = moderationProjections{}
		for _, id := range accounts {
			rows, err := e.h.Moderation.AccountRestrictionRows(ctx, tx, id)
			if err != nil {
				return err
			}
			out.accounts = append(out.accounts, normalizeRows(rows)...)
		}
		for _, d := range domains {
			rows, err := e.h.Moderation.DomainRestrictionRows(ctx, tx, d)
			if err != nil {
				return err
			}
			out.domains = append(out.domains, normalizeRows(rows)...)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("restriction rows = %v", err)
	}
	for _, id := range reports {
		st, err := e.h.Moderation.FindReportStatus(ctx, id)
		if err != nil {
			t.Fatalf("FindReportStatus = %v", err)
		}
		st.Since = st.Since.UTC()
		out.reports = append(out.reports, st)
	}
	return out
}

// normalizeRows puts the ends of rows in UTC, so that rows compare by their values.
func normalizeRows(rows []moderation.RestrictionRow) []moderation.RestrictionRow {
	for i, r := range rows {
		if r.EffectiveUntil != nil {
			u := r.EffectiveUntil.UTC()
			rows[i].EffectiveUntil = &u
		}
	}
	return rows
}
