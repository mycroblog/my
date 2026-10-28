// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// ModerationHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the moderation repository on it.
type ModerationHarness struct {
	UnitOfWork uow.UnitOfWork
	Moderation appmoderation.Repository
}

// ModerationFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type ModerationFactory func(t *testing.T, opts uow.Options) ModerationHarness

// ModerationContract verifies the moderation repository of a database adapter through the
// moderation use cases, moderation.FederationPolicy (federation.md §11), and
// moderation.AccountPolicy (data-model §5.3): a domain action restricts the domain and its
// subdomains until it expires or is lifted, a lift ends only its own action's restrictions,
// actions on remote accounts restrict them, each kind of action on a local account takes away
// its capabilities until it ends and may name a report of the account only, the actions on
// statuses record and act on statuses of the account only and a ban schedules the deletion of
// the account and is never lifted, kinds that do not apply to an account are refused, invalid actions are refused, each action is lifted once,
// every change emits the restrictions in effect afterwards, a suspension and the lift or the
// expiry of the last one are told once and the expiry job removes the expired rows, a received report is filed once,
// open, with its statuses and its comment cut to the limit, a local report is filed open with
// its statuses and cited rules only when the reporter may see those statuses of the target and
// the rules are in effect, the actions on domains and on
// accounts list newest first with their lifts and filters, the public list keeps only the
// limits and suspensions in effect without their private notes, and moderators list, assign,
// and resolve reports, each resolution emitting the report with it, and a reporter lists only
// their own reports, newest first, with their states, a filed report tells whether it is the
// target's only open one, and the open reports are counted. Moderators also write notes on a
// report, reopen a resolved one, see its target's other reports and actions, and an action that
// follows from a report resolves it as actioned once, each audited. The accounts are in-memory
// stand-ins.
func ModerationContract(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *moderationEnv)
	}{
		{"DomainActionsCoverSubdomainsUntilTheyEnd", moderationDomainActions},
		{"LiftEndsOnlyItsOwnAction", moderationLifts},
		{"RemoteAccountActions", moderationAccountActions},
		{"LocalAccountActions", moderationLocalAccountActions},
		{"ActionsOnStatusesAndBans", moderationStatusesAndBans},
		{"ListsAccountActions", moderationListsAccountActions},
		{"RejectsInvalidActions", moderationRejectsInvalid},
		{"RecordsEachLiftOnce", moderationLiftsOnce},
		{"EmitsTheRestrictionsInEffect", moderationEmits},
		{"FilesARemoteReportOnce", moderationReports},
		{"FilesALocalReport", moderationLocalReports},
		{"ListsDomainActions", moderationListsDomainActions},
		{"ListsPublicDomainActions", moderationListsPublicDomainActions},
		{"HandlesReports", moderationHandlesReports},
		{"ListsTheReportsOfAReporter", moderationReportsOfReporter},
		{"TellsSuspensionsAndTheirEnds", moderationSuspensions},
		{"FlagsTheFirstOpenReportAndCounts", moderationFirstOpenReport},
		{"NotesReopensAndResolvesByAction", moderationNotesAndReopenings},
		{"RebuildAgreesWithTheCommands", moderationProjectionEqualsRebuild},
		{"ReadsInTheTransactionsOfItsEvents", moderationReadsInTransaction},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newModerationEnv(t, newHarness)) })
	}
}

type moderationEnv struct {
	h          ModerationHarness
	projectors *uow.Projectors
	clock      *Clock
	ids        *IDGenerator
	actors     *stubActors
	// statuses and lifecycles stand in for the content and account modules.
	statuses   *stubModerationStatuses
	lifecycles *stubLifecycles
	svc        *appmoderation.Service
	policy     *appmoderation.FederationPolicy
	// accounts is the AccountPolicy on the same repository.
	accounts *appmoderation.AccountPolicy
	events   *eventLog
	// suspensions logs the AccountSuspensionChanged events apart from events.
	suspensions *eventLog
	remote      string
	local       string
	reports     string
	// instance is the instance actor.
	instance string
}

func newModerationEnv(t *testing.T, newHarness ModerationFactory) *moderationEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	clock := NewClock(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC))
	ids := NewIDGenerator(clock)
	e := &moderationEnv{
		h: h, projectors: projectors, clock: clock, ids: ids, actors: &stubActors{actors: map[string]appaccount.Actor{}}, events: &eventLog{},
		statuses:   &stubModerationStatuses{live: map[string]appcontent.Status{}, deleted: map[string]string{}, marked: map[string][]string{}},
		lifecycles: &stubLifecycles{due: map[string]time.Time{}},
	}
	for _, typ := range []string{appmoderation.EventDomainRestrictionsChanged, appmoderation.EventAccountRestrictionsChanged, appmoderation.EventReportFiled, appmoderation.EventReportResolved} {
		projectors.Register(typ, e.events.record)
	}
	e.suspensions = &eventLog{}
	projectors.Register(appmoderation.EventAccountSuspensionChanged, e.suspensions.record)
	appmoderation.RegisterAuditProjector(projectors, h.Moderation)
	e.svc = appmoderation.NewService(appmoderation.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Moderation, Accounts: e.actors, Clock: clock, IDs: ids,
		Statuses: e.statuses, Lifecycles: e.lifecycles,
	})
	e.policy = appmoderation.NewFederationPolicy(h.Moderation, clock)
	e.accounts = appmoderation.NewAccountPolicy(h.Moderation, e.actors, clock)
	e.remote, e.local, e.reports, e.instance = ids.NewID(), ids.NewID(), ids.NewID(), ids.NewID()
	e.actors.set(appaccount.Actor{Account: account.Account{ID: e.instance, Domain: account.LocalDomain, ActorType: account.ActorApplication}})
	e.actors.set(appaccount.Actor{Account: account.Account{ID: e.remote, Domain: "remote.example", URI: "https://remote.example/users/bob"}})
	e.actors.set(appaccount.Actor{Account: account.Account{ID: e.local, Domain: account.LocalDomain}})
	e.actors.set(appaccount.Actor{Account: account.Account{ID: e.reports, Domain: "remote.example", URI: "https://remote.example/actor"}})
	return e
}

func (e *moderationEnv) domainAction(t *testing.T, domain string, kind moderation.DomainActionKind, d time.Duration) moderation.DomainAction {
	t.Helper()
	a, err := e.svc.IssueDomainAction(t.Context(), appmoderation.DomainActionInput{Domain: domain, Kind: kind, Duration: d})
	if err != nil {
		t.Fatalf("IssueDomainAction(%s, %s) = %v", domain, kind, err)
	}
	return a
}

// checkDomain checks every domain predicate of the policy for domain.
func (e *moderationEnv) checkDomain(t *testing.T, domain string, suspended, media, reports bool) {
	t.Helper()
	ctx := t.Context()
	for _, c := range []struct {
		name string
		f    func() (bool, error)
		want bool
	}{
		{"FederationSuspended", func() (bool, error) { return e.policy.FederationSuspended(ctx, domain) }, suspended},
		{"RejectsMedia", func() (bool, error) { return e.policy.RejectsMedia(ctx, domain) }, media},
		{"RejectsReports", func() (bool, error) { return e.policy.RejectsReports(ctx, domain) }, reports},
	} {
		got, err := c.f()
		if err != nil || got != c.want {
			t.Errorf("%s(%q) = %v, %v; want %v", c.name, domain, got, err, c.want)
		}
	}
}

func moderationDomainActions(t *testing.T, e *moderationEnv) {
	e.checkDomain(t, "bad.example", false, false, false)
	e.domainAction(t, "bad.example", moderation.DomainSuspend, 0)
	e.checkDomain(t, "bad.example", true, false, false)
	e.checkDomain(t, "sub.bad.example", true, false, false)
	e.checkDomain(t, "example", false, false, false)
	e.checkDomain(t, "notbad.example", false, false, false)
	e.checkDomain(t, "", false, false, false)

	// A parent domain's action covers its subdomains; an expiring one ends by itself.
	e.domainAction(t, "media.example", moderation.DomainRejectMedia, time.Hour)
	e.domainAction(t, "reports.example", moderation.DomainRejectReports, 0)
	e.checkDomain(t, "cdn.media.example", false, true, false)
	e.checkDomain(t, "reports.example", false, false, true)
	e.clock.Advance(time.Hour)
	e.checkDomain(t, "cdn.media.example", false, false, false)

	rs, err := e.policy.DomainRestrictions(t.Context(), "x.bad.example")
	if want := []moderation.Restriction{moderation.RestrictFederation, moderation.RestrictProfileView}; err != nil || !reflect.DeepEqual(rs, want) {
		t.Errorf("DomainRestrictions = %v, %v; want %v", rs, err, want)
	}
}

func moderationLifts(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	first := e.domainAction(t, "bad.example", moderation.DomainSuspend, 0)
	second := e.domainAction(t, "bad.example", moderation.DomainSuspend, 24*time.Hour)
	e.domainAction(t, "bad.example", moderation.DomainLimit, 0)
	if err := e.svc.LiftDomainAction(ctx, first.ID, ""); err != nil {
		t.Fatalf("LiftDomainAction = %v", err)
	}
	e.checkDomain(t, "bad.example", true, false, false)
	if err := e.svc.LiftDomainAction(ctx, second.ID, ""); err != nil {
		t.Fatalf("LiftDomainAction = %v", err)
	}
	e.checkDomain(t, "bad.example", false, false, false)
	// Lifting again changes nothing; the limit stays.
	if err := e.svc.LiftDomainAction(ctx, second.ID, ""); err != nil {
		t.Errorf("LiftDomainAction again = %v, want nil", err)
	}
	rs, err := e.policy.DomainRestrictions(ctx, "bad.example")
	if want := []moderation.Restriction{moderation.RestrictPublicDiscovery, moderation.RestrictUnsolicitedReach}; err != nil || !reflect.DeepEqual(rs, want) {
		t.Errorf("DomainRestrictions after the lifts = %v, %v; want %v", rs, err, want)
	}
	if err := e.svc.LiftDomainAction(ctx, NamedID("unknown-domain-action"), ""); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("LiftDomainAction(unknown) = %v, want ErrNotFound", err)
	}
}

func moderationAccountActions(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	suspended := func(id string, want bool) {
		t.Helper()
		got, err := e.policy.AccountFederationSuspended(ctx, id)
		if err != nil || got != want {
			t.Errorf("AccountFederationSuspended(%s) = %v, %v; want %v", id, got, err, want)
		}
	}
	suspended(e.remote, false)
	a, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.remote, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam, Duration: time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction = %v", err)
	}
	suspended(e.remote, true)
	suspended(e.local, false)
	e.clock.Advance(time.Hour)
	suspended(e.remote, false)

	a, err = e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.remote, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategoryOther,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction = %v", err)
	}
	suspended(e.remote, true)
	if err := e.svc.LiftAccountAction(ctx, a.ID, "", "appealed"); err != nil {
		t.Fatalf("LiftAccountAction = %v", err)
	}
	suspended(e.remote, false)
	if err := e.svc.LiftAccountAction(ctx, a.ID, "", ""); err != nil {
		t.Errorf("LiftAccountAction again = %v, want nil", err)
	}

	for _, in := range []appmoderation.AccountActionInput{
		{TargetAccountID: e.remote, Kind: moderation.ActionWarn, ReasonCategory: moderation.CategoryOther},
		{TargetAccountID: e.remote, Kind: moderation.ActionDisableLogin, ReasonCategory: moderation.CategoryOther},
		{TargetAccountID: e.remote, Kind: moderation.ActionBan, ReasonCategory: moderation.CategoryOther},
		{TargetAccountID: e.remote, Kind: moderation.ActionDeleteStatuses, ReasonCategory: moderation.CategoryOther, StatusIDs: []string{e.ids.NewID()}},
		{TargetAccountID: e.instance, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategoryOther},
	} {
		if _, err := e.svc.IssueAccountAction(ctx, in); !errors.Is(err, appmoderation.ErrUnsupportedAction) {
			t.Errorf("IssueAccountAction(%s of %s) = %v, want ErrUnsupportedAction", in.Kind, in.TargetAccountID, err)
		}
	}
	missing := appmoderation.AccountActionInput{TargetAccountID: e.ids.NewID(), Kind: moderation.ActionLimit, ReasonCategory: moderation.CategoryOther}
	if _, err := e.svc.IssueAccountAction(ctx, missing); !errors.Is(err, appmoderation.ErrAccountNotFound) {
		t.Errorf("IssueAccountAction(missing) = %v, want ErrAccountNotFound", err)
	}
}

// moderationLocalAccountActions checks the actions on a local account: each kind takes away
// the capabilities of data-model §5.2 through the AccountPolicy until it expires or is lifted,
// and an action may name a report of the account only.
func moderationLocalAccountActions(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	check := func(when string, login, interact, profile bool, content appcontent.Restrictions) {
		t.Helper()
		for _, c := range []struct {
			name string
			f    func(context.Context, string) (bool, error)
			want bool
		}{
			{"LoginRestricted", e.accounts.LoginRestricted, login},
			{"InteractionRestricted", e.accounts.InteractionRestricted, interact},
			{"ProfileHidden", e.accounts.ProfileHidden, profile},
		} {
			if got, err := c.f(ctx, e.local); err != nil || got != c.want {
				t.Errorf("%s: %s = %v, %v; want %v", when, c.name, got, err, c.want)
			}
		}
		got, err := e.accounts.Content().Restrictions(ctx, e.local)
		if err != nil || got != content {
			t.Errorf("%s: content restrictions = %+v, %v; want %+v", when, got, err, content)
		}
	}
	issue := func(kind moderation.ActionKind, d time.Duration) moderation.Action {
		t.Helper()
		a, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: e.local, Kind: kind, ReasonCategory: moderation.CategoryViolation, NotifyTarget: true, Duration: d,
		})
		if err != nil {
			t.Fatalf("IssueAccountAction(%s) = %v", kind, err)
		}
		if !a.NotifyTarget || a.Issuer != moderation.IssuerModerator {
			t.Errorf("IssueAccountAction(%s) = %+v, want a moderator's action that notifies the target", kind, a)
		}
		return a
	}
	lift := func(a moderation.Action) {
		t.Helper()
		if err := e.svc.LiftAccountAction(ctx, a.ID, NamedID("user-1"), ""); err != nil {
			t.Fatalf("LiftAccountAction(%s) = %v", a.Kind, err)
		}
	}
	none := appcontent.Restrictions{}
	check("before", false, false, false, none)

	w := issue(moderation.ActionWarn, 0)
	check("warn", false, false, false, none)
	lift(w)

	for _, c := range []struct {
		kind                     moderation.ActionKind
		login, interact, profile bool
		content                  appcontent.Restrictions
	}{
		{moderation.ActionForceSensitive, false, false, false, appcontent.Restrictions{MediaSensitive: true}},
		{moderation.ActionLimit, false, false, false, appcontent.Restrictions{Author: appcontent.AuthorRestrictions{VisibilityCeilingPrivate: true, PublicDiscovery: true}}},
		{moderation.ActionRestrictPosting, false, false, false, appcontent.Restrictions{Post: true}},
		{moderation.ActionRestrictInteraction, false, true, false, appcontent.Restrictions{Interact: true}},
		{moderation.ActionDisableLogin, true, false, false, none},
		{moderation.ActionSuspend, true, true, true, appcontent.Restrictions{Post: true, Interact: true, Author: appcontent.AuthorRestrictions{ProfileView: true}}},
	} {
		a := issue(c.kind, 0)
		check(string(c.kind), c.login, c.interact, c.profile, c.content)
		lift(a)
		check(string(c.kind)+" lifted", false, false, false, none)
	}

	// Actions add up, and an expiring one ends by itself while the others stay.
	posting := issue(moderation.ActionRestrictPosting, time.Hour)
	login := issue(moderation.ActionDisableLogin, 0)
	check("posting and login", true, false, false, appcontent.Restrictions{Post: true})
	e.clock.Advance(time.Hour)
	check("posting expired", true, false, false, none)
	lift(login)
	check("all ended", false, false, false, none)
	if v, err := e.svc.AccountAction(ctx, posting.ID); err != nil || v.State != moderation.ActionExpired {
		t.Errorf("AccountAction(expired) = %+v, %v", v, err)
	}

	// A limit keeps the account's notifications from the accounts that do not follow it, and so
	// does a limit on its domain.
	reach := func(when, accountID string, want bool) {
		t.Helper()
		if got, err := e.accounts.UnsolicitedReachRestricted(ctx, accountID); err != nil || got != want {
			t.Errorf("%s: UnsolicitedReachRestricted = %v, %v; want %v", when, got, err, want)
		}
	}
	limit := issue(moderation.ActionLimit, 0)
	reach("limit", e.local, true)
	lift(limit)
	reach("limit lifted", e.local, false)
	domainLimit := e.domainAction(t, "remote.example", moderation.DomainLimit, 0)
	reach("domain limit", e.remote, true)
	reach("local after a domain limit", e.local, false)
	if err := e.svc.LiftDomainAction(ctx, domainLimit.ID, NamedID("user-1")); err != nil {
		t.Fatalf("LiftDomainAction(limit) = %v", err)
	}
	reach("domain limit lifted", e.remote, false)

	// The restrictions of the account's domain apply to a remote account.
	e.domainAction(t, "remote.example", moderation.DomainSuspend, 0)
	if hidden, err := e.accounts.ProfileHidden(ctx, e.remote); err != nil || !hidden {
		t.Errorf("ProfileHidden(remote of a suspended domain) = %v, %v; want true", hidden, err)
	}
	if hidden, err := e.accounts.ProfileHidden(ctx, e.local); err != nil || hidden {
		t.Errorf("ProfileHidden(local) after a domain action = %v, %v; want false", hidden, err)
	}

	// An action may follow from a report of its account only.
	r, _, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/7", ReporterAccountID: e.reports, TargetAccountID: e.local,
	})
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.local, Kind: moderation.ActionWarn, ReasonCategory: moderation.CategorySpam, ReportID: r.ID,
	})
	if err != nil || a.ReportID != r.ID {
		t.Errorf("IssueAccountAction(with its report) = %+v, %v", a, err)
	}
	for _, id := range []string{e.ids.NewID(), r.ID} {
		in := appmoderation.AccountActionInput{TargetAccountID: e.remote, Kind: moderation.ActionLimit, ReasonCategory: moderation.CategorySpam, ReportID: id}
		if _, err := e.svc.IssueAccountAction(ctx, in); !errors.Is(err, appmoderation.ErrInvalidReportReference) {
			t.Errorf("IssueAccountAction(report %s of another account) = %v, want ErrInvalidReportReference", id, err)
		}
	}
	// A remote account is never notified.
	ra, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.remote, Kind: moderation.ActionLimit, ReasonCategory: moderation.CategorySpam, NotifyTarget: true,
	})
	if err != nil || ra.NotifyTarget {
		t.Errorf("IssueAccountAction(remote, notify) = %+v, %v; want no notice", ra, err)
	}
}

// moderationListsAccountActions checks the moderators' list of the actions on accounts.
func moderationListsAccountActions(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	issue := func(target string, kind moderation.ActionKind, d time.Duration) moderation.Action {
		t.Helper()
		a, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: target, Kind: kind, ReasonCategory: moderation.CategoryOther, PublicReason: "public",
			PrivateNote: "private", IssuedByUserID: NamedID("user-1"), Duration: d,
		})
		if err != nil {
			t.Fatal(err)
		}
		e.clock.Advance(time.Second)
		return a
	}
	first := issue(e.local, moderation.ActionLimit, 0)
	expiring := issue(e.local, moderation.ActionRestrictPosting, time.Hour)
	remote := issue(e.remote, moderation.ActionSuspend, 0)
	lifted := issue(e.local, moderation.ActionDisableLogin, 0)
	if err := e.svc.LiftAccountAction(ctx, lifted.ID, NamedID("user-2"), "appealed"); err != nil {
		t.Fatal(err)
	}
	liftedAt := e.clock.Now()
	ids := func(vs []appmoderation.AccountActionView) []string {
		out := []string{}
		for _, v := range vs {
			out = append(out, v.Action.ID)
		}
		return out
	}
	list := func(f appmoderation.AccountActionFilter, limit int, want ...string) []appmoderation.AccountActionView {
		t.Helper()
		got, err := e.svc.AccountActions(ctx, f, limit)
		if err != nil || !reflect.DeepEqual(ids(got), append([]string{}, want...)) {
			t.Errorf("AccountActions(%+v, %d) = %v, %v; want %v", f, limit, ids(got), err, want)
		}
		return got
	}
	all := list(appmoderation.AccountActionFilter{}, 10, lifted.ID, remote.ID, expiring.ID, first.ID)
	if len(all) == 4 {
		want := moderation.Lift{ActionID: lifted.ID, LiftedByUserID: NamedID("user-2"), Reason: "appealed", LiftedAt: liftedAt}
		if v := all[0]; v.State != moderation.ActionLifted || v.Lift == nil || !reflect.DeepEqual(*v.Lift, want) || !reflect.DeepEqual(v.Action, lifted) {
			t.Errorf("lifted action = %+v", v)
		}
		if v := all[2]; v.State != moderation.ActionActive || v.Lift != nil || !reflect.DeepEqual(v.Action, expiring) {
			t.Errorf("expiring action = %+v", v)
		}
	}
	list(appmoderation.AccountActionFilter{AccountID: e.local}, 10, lifted.ID, expiring.ID, first.ID)
	list(appmoderation.AccountActionFilter{AccountID: e.local}, 2, lifted.ID, expiring.ID)
	list(appmoderation.AccountActionFilter{AccountID: e.local, Before: expiring.ID}, 2, first.ID)
	list(appmoderation.AccountActionFilter{AccountID: e.local, Active: true}, 10, expiring.ID, first.ID)
	e.clock.Advance(time.Hour)
	list(appmoderation.AccountActionFilter{Active: true}, 10, remote.ID, first.ID)
	if _, err := e.svc.AccountAction(ctx, e.ids.NewID()); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("AccountAction(unknown) = %v, want ErrNotFound", err)
	}
}

// AccountDomain implements appmoderation.AccountDomains.
func (s *stubActors) AccountDomain(ctx context.Context, id string) (string, error) {
	a, err := s.FindByID(ctx, id)
	return a.Account.Domain, err
}

func moderationRejectsInvalid(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	for _, in := range []appmoderation.DomainActionInput{
		{Domain: "", Kind: moderation.DomainSuspend},
		{Domain: "https://bad.example", Kind: moderation.DomainSuspend},
		{Domain: "Bad.Example", Kind: moderation.DomainSuspend},
		{Domain: "bad..example", Kind: moderation.DomainSuspend},
		{Domain: "bad.example", Kind: "silence"},
		{Domain: "bad\x00.example", Kind: moderation.DomainSuspend},
		{Domain: "bad.example", Kind: moderation.DomainSuspend, PrivateNote: strings.Repeat("あ", moderation.MaxActionText+1)},
	} {
		if _, err := e.svc.IssueDomainAction(ctx, in); !errors.Is(err, moderation.ErrInvalidAction) {
			t.Errorf("IssueDomainAction(%q, %q) = %v, want ErrInvalidAction", in.Domain, in.Kind, err)
		}
	}
	// Text that a dialect cannot store as it is fails alike on every dialect (data-model §1.2),
	// naming its column.
	for _, tc := range []struct {
		field string
		in    appmoderation.DomainActionInput
	}{
		{"public_comment", appmoderation.DomainActionInput{PublicComment: "spam\x00"}},
		{"private_note", appmoderation.DomainActionInput{PrivateNote: "\x00"}},
		{"private_note", appmoderation.DomainActionInput{PrivateNote: "a\xffb"}},
	} {
		tc.in.Domain, tc.in.Kind = "bad.example", moderation.DomainSuspend
		_, err := e.svc.IssueDomainAction(ctx, tc.in)
		if te := (*moderation.TextError)(nil); !errors.Is(err, moderation.ErrInvalidAction) || !errors.As(err, &te) || te.Field != tc.field || te.Max != 0 {
			t.Errorf("IssueDomainAction(invalid %s) = %v, want ErrInvalidAction and a TextError of %s", tc.field, err, tc.field)
		}
	}
	for _, in := range []appmoderation.AccountActionInput{
		{ReasonCategory: "rude"},
		{ReasonCategory: moderation.CategorySpam, PublicReason: "spam\x00"},
		{ReasonCategory: moderation.CategorySpam, PrivateNote: "\xff"},
		{ReasonCategory: moderation.CategorySpam, PrivateNote: strings.Repeat("a", moderation.MaxActionText+1)},
	} {
		in.TargetAccountID, in.Kind = e.remote, moderation.ActionLimit
		if _, err := e.svc.IssueAccountAction(ctx, in); !errors.Is(err, moderation.ErrInvalidAction) {
			t.Errorf("IssueAccountAction(%+v) = %v, want ErrInvalidAction", in, err)
		}
	}
	e.checkDomain(t, "bad.example", false, false, false)
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rows, err := e.h.Moderation.AccountRestrictionRows(ctx, tx, e.remote)
		if err == nil && len(rows) != 0 {
			t.Errorf("AccountRestrictionRows after invalid actions = %+v, want none", rows)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// A lift with an invalid reason is refused and leaves the action in effect.
	a, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.remote, Kind: moderation.ActionLimit, ReasonCategory: moderation.CategorySpam,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"done\x00", "\xff", strings.Repeat("a", moderation.MaxActionText+1)} {
		if err := e.svc.LiftAccountAction(ctx, a.ID, "", reason); !errors.Is(err, moderation.ErrInvalidAction) {
			t.Errorf("LiftAccountAction(reason %q...) = %v, want ErrInvalidAction", reason[:1], err)
		}
	}
	if err := e.svc.LiftAccountAction(ctx, a.ID, "", "done"); err != nil {
		t.Errorf("LiftAccountAction(valid reason) after refused lifts = %v", err)
	}
}

func moderationLiftsOnce(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	d := e.domainAction(t, "bad.example", moderation.DomainSuspend, 0)
	a, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.remote, Kind: moderation.ActionLimit, ReasonCategory: moderation.CategoryOther,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return e.h.Moderation.InsertDomainActionLift(ctx, tx, moderation.DomainLift{ActionID: d.ID, LiftedAt: e.clock.Now()})
		})
		if dup := i == 1; dup != errors.Is(err, appmoderation.ErrAlreadyRecorded) {
			t.Errorf("domain lift #%d = %v; want ErrAlreadyRecorded only for the second", i+1, err)
		}
		err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return e.h.Moderation.InsertActionLift(ctx, tx, moderation.Lift{ActionID: a.ID, LiftedAt: e.clock.Now()})
		})
		if dup := i == 1; dup != errors.Is(err, appmoderation.ErrAlreadyRecorded) {
			t.Errorf("account lift #%d = %v; want ErrAlreadyRecorded only for the second", i+1, err)
		}
	}
	// The stored actions read back as issued.
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		got, err := e.h.Moderation.DomainAction(ctx, tx, d.ID)
		if err == nil && !reflect.DeepEqual(got, d) {
			t.Errorf("DomainAction = %+v, want %+v", got, d)
		}
		if err != nil {
			return err
		}
		gotA, err := e.h.Moderation.Action(ctx, tx, a.ID)
		if err == nil && !reflect.DeepEqual(gotA, a) {
			t.Errorf("Action = %+v, want %+v", gotA, a)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func moderationEmits(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	a := e.domainAction(t, "bad.example", moderation.DomainLimit, 2*time.Hour)
	e.domainAction(t, "bad.example", moderation.DomainLimit, time.Hour)
	until := e.clock.Now().Add(2 * time.Hour)
	events := e.events.take()
	last, ok := events[len(events)-1].(appmoderation.DomainRestrictionsChanged)
	want := appmoderation.DomainRestrictionsChanged{Domain: "bad.example", Effects: []moderation.Effect{
		{Restriction: moderation.RestrictPublicDiscovery, Until: &until},
		{Restriction: moderation.RestrictUnsolicitedReach, Until: &until},
	}}
	if len(events) != 2 || !ok || !reflect.DeepEqual(last, want) {
		t.Errorf("events = %+v, want 2 ending with %+v", events, want)
	}
	if err := e.svc.LiftDomainAction(ctx, a.ID, ""); err != nil {
		t.Fatal(err)
	}
	soon := e.clock.Now().Add(time.Hour)
	events = e.events.take()
	want.Effects = []moderation.Effect{
		{Restriction: moderation.RestrictPublicDiscovery, Until: &soon},
		{Restriction: moderation.RestrictUnsolicitedReach, Until: &soon},
	}
	if len(events) != 1 || !reflect.DeepEqual(events[0], want) {
		t.Errorf("events after the lift = %+v, want %+v", events, want)
	}

	s, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.remote, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategoryOther,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.LiftAccountAction(ctx, s.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	events = e.events.take()
	if len(events) != 2 {
		t.Fatalf("account events = %+v, want 2", events)
	}
	if ev, ok := events[0].(appmoderation.AccountRestrictionsChanged); !ok || ev.AccountID != e.remote || len(ev.Effects) != 5 {
		t.Errorf("account event = %+v, want the 5 restrictions of a suspension", events[0])
	}
	if ev, ok := events[1].(appmoderation.AccountRestrictionsChanged); !ok || len(ev.Effects) != 0 {
		t.Errorf("account event after the lift = %+v, want no restrictions", events[1])
	}
}

func moderationReports(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	in := appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/1", ReporterAccountID: e.reports, TargetAccountID: e.local,
		StatusIDs: []string{OrderedID(2), OrderedID(1), OrderedID(2)}, Comment: strings.Repeat("あ", moderation.MaxReportComment+10),
	}
	r, created, err := e.svc.FileRemoteReport(ctx, in)
	if err != nil || !created {
		t.Fatalf("FileRemoteReport = %v, %v; want created", created, err)
	}
	if r.Comment != strings.Repeat("あ", moderation.MaxReportComment) || !reflect.DeepEqual(r.StatusIDs, []string{OrderedID(1), OrderedID(2)}) || r.Category != moderation.CategoryOther {
		t.Errorf("report = %+v, want the comment cut to the limit and the statuses s1 and s2", r)
	}
	again, created, err := e.svc.FileRemoteReport(ctx, in)
	if err != nil || created || again.ID != r.ID {
		t.Errorf("FileRemoteReport again = %s, %v, %v; want the first report", again.ID, created, err)
	}
	got, err := e.svc.Report(ctx, r.ID)
	if err != nil || !reflect.DeepEqual(got, r) {
		t.Errorf("Report = %+v, %v; want %+v", got, err, r)
	}
	st, err := e.svc.ReportStatus(ctx, r.ID)
	if err != nil || st.State != moderation.ReportOpen || !st.Since.Equal(r.FiledAt) {
		t.Errorf("ReportStatus = %+v, %v; want open since filing", st, err)
	}
	// Another Flag without statuses is another report.
	second, created, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/2", ReporterAccountID: e.reports, TargetAccountID: e.local,
	})
	if err != nil || !created || len(second.StatusIDs) != 0 {
		t.Fatalf("FileRemoteReport(second) = %+v, %v, %v", second, created, err)
	}
	all, err := e.svc.ReportsOf(ctx, e.local)
	if err != nil || len(all) != 2 || all[0].ID != r.ID || all[1].ID != second.ID || len(all[1].StatusIDs) != 0 {
		t.Errorf("ReportsOf = %+v, %v; want the two reports in order", all, err)
	}
	if events := e.events.take(); len(events) != 2 {
		t.Errorf("events = %+v, want one ReportFiled per report", events)
	}
	if _, _, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{ReporterAccountID: e.reports, TargetAccountID: e.local}); !errors.Is(err, moderation.ErrInvalidReport) {
		t.Errorf("FileRemoteReport without a URI = %v, want ErrInvalidReport", err)
	}
}

func moderationListsDomainActions(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	first := e.domainAction(t, "bad.example", moderation.DomainSuspend, 0)
	e.clock.Advance(time.Second)
	expiring := e.domainAction(t, "media.example", moderation.DomainRejectMedia, time.Hour)
	e.clock.Advance(time.Second)
	lifted := e.domainAction(t, "bad.example", moderation.DomainLimit, 0)
	e.clock.Advance(time.Second)
	if err := e.svc.LiftDomainAction(ctx, lifted.ID, NamedID("user-1")); err != nil {
		t.Fatal(err)
	}
	liftedAt := e.clock.Now()
	ids := func(vs []appmoderation.DomainActionView) []string {
		out := []string{}
		for _, v := range vs {
			out = append(out, v.Action.ID)
		}
		return out
	}
	list := func(f appmoderation.DomainActionFilter, limit int, want ...string) []appmoderation.DomainActionView {
		t.Helper()
		got, err := e.svc.DomainActions(ctx, f, limit)
		if err != nil || !reflect.DeepEqual(ids(got), append([]string{}, want...)) {
			t.Errorf("DomainActions(%+v, %d) = %v, %v; want %v", f, limit, ids(got), err, want)
		}
		return got
	}
	all := list(appmoderation.DomainActionFilter{}, 10, lifted.ID, expiring.ID, first.ID)
	if len(all) == 3 {
		if v := all[0]; v.State != moderation.ActionLifted || v.Lift == nil || v.Lift.LiftedByUserID != NamedID("user-1") || !v.Lift.LiftedAt.Equal(liftedAt) || !reflect.DeepEqual(v.Action, lifted) {
			t.Errorf("lifted action = %+v", v)
		}
		if v := all[1]; v.State != moderation.ActionActive || v.Lift != nil || !reflect.DeepEqual(v.Action, expiring) {
			t.Errorf("expiring action = %+v", v)
		}
	}
	list(appmoderation.DomainActionFilter{}, 2, lifted.ID, expiring.ID)
	list(appmoderation.DomainActionFilter{Before: expiring.ID}, 2, first.ID)
	list(appmoderation.DomainActionFilter{Domain: "bad.example"}, 10, lifted.ID, first.ID)
	list(appmoderation.DomainActionFilter{Domain: "sub.bad.example"}, 10)
	list(appmoderation.DomainActionFilter{Active: true}, 10, expiring.ID, first.ID)
	e.clock.Advance(time.Hour)
	list(appmoderation.DomainActionFilter{Active: true}, 10, first.ID)

	v, err := e.svc.DomainAction(ctx, expiring.ID)
	if err != nil || v.State != moderation.ActionExpired || v.Lift != nil {
		t.Errorf("DomainAction(expired) = %+v, %v", v, err)
	}
	if _, err := e.svc.DomainAction(ctx, e.ids.NewID()); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("DomainAction(unknown) = %v, want ErrNotFound", err)
	}
}

func moderationListsPublicDomainActions(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	issue := func(in appmoderation.DomainActionInput) moderation.DomainAction {
		t.Helper()
		e.clock.Advance(time.Second)
		a, err := e.svc.IssueDomainAction(ctx, in)
		if err != nil {
			t.Fatalf("IssueDomainAction(%+v) = %v", in, err)
		}
		return a
	}
	suspended := issue(appmoderation.DomainActionInput{Domain: "bad.example", Kind: moderation.DomainSuspend, PublicComment: "spam", PrivateNote: "see #12"})
	issue(appmoderation.DomainActionInput{Domain: "media.example", Kind: moderation.DomainRejectMedia})
	expiring := issue(appmoderation.DomainActionInput{Domain: "brief.example", Kind: moderation.DomainLimit, Duration: time.Hour})
	lifted := issue(appmoderation.DomainActionInput{Domain: "lifted.example", Kind: moderation.DomainSuspend})
	if err := e.svc.LiftDomainAction(ctx, lifted.ID, ""); err != nil {
		t.Fatal(err)
	}
	limited := issue(appmoderation.DomainActionInput{Domain: "spam.example.com", Kind: moderation.DomainLimit, Obfuscate: true})
	issue(appmoderation.DomainActionInput{Domain: "reports.example", Kind: moderation.DomainRejectReports})

	list := func(before string, limit int, want ...moderation.DomainAction) {
		t.Helper()
		got, err := e.svc.PublicDomainActions(ctx, before, limit)
		wantPublic := []moderation.PublicDomainAction{}
		for _, a := range want {
			wantPublic = append(wantPublic, a.Public())
		}
		if err != nil || !reflect.DeepEqual(append([]moderation.PublicDomainAction{}, got...), wantPublic) {
			t.Errorf("PublicDomainActions(%q, %d) = %+v, %v; want %+v", before, limit, got, err, wantPublic)
		}
	}
	list("", 10, limited, expiring, suspended)
	list("", 2, limited, expiring)
	list(expiring.ID, 2, suspended)
	e.clock.Advance(time.Hour)
	list("", 10, limited, suspended)

	// The repository filters by kind for any query.
	rs, err := e.h.Moderation.ListDomainActions(ctx, appmoderation.DomainActionQuery{
		Kinds: []moderation.DomainActionKind{moderation.DomainRejectMedia, moderation.DomainRejectReports},
	}, 10)
	kinds := []moderation.DomainActionKind{}
	for _, r := range rs {
		kinds = append(kinds, r.Action.Kind)
	}
	if want := []moderation.DomainActionKind{moderation.DomainRejectReports, moderation.DomainRejectMedia}; err != nil || !reflect.DeepEqual(kinds, want) {
		t.Errorf("ListDomainActions(reject_media, reject_reports) = %v, %v; want %v", kinds, err, want)
	}
}

// reportedStatuses is a stand-in for the content module: the statuses by ID with their authors,
// and those that the viewer of hidden may not see.
type reportedStatuses struct {
	authors map[string]string
	hidden  map[string]bool
}

func (s reportedStatuses) Visible(_ context.Context, _ appcontent.Viewer, id string) (appcontent.Status, error) {
	author, ok := s.authors[id]
	if !ok || s.hidden[id] {
		return appcontent.Status{}, fmt.Errorf("status %s: %w", id, appcontent.ErrNotFound)
	}
	return appcontent.Status{ID: id, AuthorAccountID: author}, nil
}

// citableRules is a stand-in for the instance module: the rules in effect.
type citableRules map[string]bool

func (r citableRules) RuleInEffect(_ context.Context, id string) (bool, error) { return r[id], nil }

func moderationLocalReports(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	reporter := e.ids.NewID()
	e.actors.set(appaccount.Actor{Account: account.Account{ID: reporter, Domain: account.LocalDomain}})
	statuses := reportedStatuses{
		authors: map[string]string{OrderedID(1): e.remote, OrderedID(2): e.remote, NamedID("hidden"): e.remote, NamedID("mine"): reporter},
		hidden:  map[string]bool{NamedID("hidden"): true},
	}
	reports := appmoderation.NewLocalReports(appmoderation.LocalReportDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Moderation, Accounts: e.actors, Statuses: statuses,
		Rules: citableRules{OrderedID(11): true, OrderedID(12): true}, Clock: e.clock, IDs: e.ids,
	})
	r, err := reports.File(ctx, appmoderation.LocalReportInput{
		ReporterAccountID: reporter, TargetAccountID: e.remote, StatusIDs: []string{OrderedID(2), OrderedID(1), OrderedID(2)},
		Category: moderation.CategoryViolation, RuleIDs: []string{OrderedID(12), OrderedID(11)}, Comment: "rude", Forward: true,
	})
	if err != nil {
		t.Fatalf("File = %v", err)
	}
	if !reflect.DeepEqual(r.StatusIDs, []string{OrderedID(1), OrderedID(2)}) || !reflect.DeepEqual(r.RuleIDs, []string{OrderedID(11), OrderedID(12)}) || !r.Forwarded || r.URI != "" {
		t.Errorf("report = %+v, want the statuses r1 and r2, the rules rule-1 and rule-2, forwarded", r)
	}
	got, err := e.svc.ReportRecord(ctx, r.ID)
	if err != nil || !reflect.DeepEqual(got.Report, r) || got.Status.State != moderation.ReportOpen || !got.Status.Since.Equal(r.FiledAt) {
		t.Errorf("ReportRecord = %+v, %v; want %+v, open", got, err, r)
	}
	listed, err := e.svc.Reports(ctx, appmoderation.ReportQuery{State: moderation.ReportOpen}, 10)
	if err != nil || len(listed) != 1 || !reflect.DeepEqual(listed[0].Report, r) {
		t.Errorf("Reports(open) = %+v, %v; want the report", listed, err)
	}
	if events := e.events.take(); len(events) != 1 {
		t.Errorf("events = %+v, want one ReportFiled", events)
	}

	// A local target is never forwarded to; a report without statuses or rules is a report.
	local, err := reports.File(ctx, appmoderation.LocalReportInput{
		ReporterAccountID: reporter, TargetAccountID: e.local, Category: moderation.CategorySpam, Forward: true,
	})
	if err != nil || local.Forwarded || len(local.StatusIDs) != 0 || len(local.RuleIDs) != 0 {
		t.Errorf("File(local target) = %+v, %v; want a report that is not forwarded", local, err)
	}

	for _, tc := range []struct {
		name  string
		in    appmoderation.LocalReportInput
		field string
	}{
		{"a status the reporter may not see", appmoderation.LocalReportInput{TargetAccountID: e.remote, StatusIDs: []string{OrderedID(1), NamedID("hidden")}, Category: moderation.CategorySpam}, "status_ids"},
		{"another account's status", appmoderation.LocalReportInput{TargetAccountID: e.remote, StatusIDs: []string{NamedID("mine")}, Category: moderation.CategorySpam}, "status_ids"},
		{"an unknown status", appmoderation.LocalReportInput{TargetAccountID: e.remote, StatusIDs: []string{NamedID("gone")}, Category: moderation.CategorySpam}, "status_ids"},
		{"a rule of another category", appmoderation.LocalReportInput{TargetAccountID: e.remote, Category: moderation.CategorySpam, RuleIDs: []string{OrderedID(11)}}, "rule_ids"},
		{"an unknown category", appmoderation.LocalReportInput{TargetAccountID: e.remote, Category: "abuse"}, "category"},
		{"a rule not in effect", appmoderation.LocalReportInput{TargetAccountID: e.remote, Category: moderation.CategoryViolation, RuleIDs: []string{NamedID("retired")}}, "rule_ids"},
		{"the reporter", appmoderation.LocalReportInput{TargetAccountID: reporter, Category: moderation.CategorySpam}, "account_id"},
		{"an unknown account", appmoderation.LocalReportInput{TargetAccountID: e.ids.NewID(), Category: moderation.CategorySpam}, "account_id"},
	} {
		tc.in.ReporterAccountID = reporter
		_, err := reports.File(ctx, tc.in)
		var fieldErr *appmoderation.ReportFieldError
		if !errors.Is(err, moderation.ErrInvalidReport) || !errors.As(err, &fieldErr) || fieldErr.Field != tc.field {
			t.Errorf("File(%s) = %v, want a *ReportFieldError of %s", tc.name, err, tc.field)
		}
	}
	for _, in := range []appmoderation.LocalReportInput{
		{TargetAccountID: e.remote, Category: moderation.CategoryOther, Comment: strings.Repeat("a", moderation.MaxLocalReportComment+1)},
	} {
		in.ReporterAccountID = reporter
		if _, err := reports.File(ctx, in); !errors.Is(err, moderation.ErrInvalidReport) {
			t.Errorf("File(%+v) = %v, want ErrInvalidReport", in, err)
		}
	}
	if events := e.events.take(); len(events) != 1 {
		t.Errorf("events = %+v, want one ReportFiled for the report on the local account alone", events)
	}
	all, err := e.svc.ReportsOf(ctx, e.remote)
	if err != nil || len(all) != 1 {
		t.Errorf("ReportsOf(remote) = %+v, %v; want only the filed report", all, err)
	}

	// A report of a voice chat room, whose host the voice module names, points at the room and
	// its revision, no statuses, and is never forwarded (ADR-0100 §11). Erasing the report
	// erases the reference, and the room is no longer reported.
	room := &moderation.ReportVoiceRoom{RoomID: NamedID("room"), RevisionID: NamedID("revision")}
	in := appmoderation.LocalReportInput{
		ReporterAccountID: reporter, TargetAccountID: e.local, Category: moderation.CategorySpam, Forward: true, VoiceRoom: room,
	}
	in.StatusIDs = []string{OrderedID(1)}
	var fieldErr *appmoderation.ReportFieldError
	if _, err := reports.File(ctx, in); !errors.As(err, &fieldErr) || fieldErr.Field != "status_ids" {
		t.Errorf("File(a room with statuses) = %v, want a *ReportFieldError of status_ids", err)
	}
	in.StatusIDs = nil
	roomReport, err := reports.File(ctx, in)
	if err != nil || roomReport.Forwarded || roomReport.VoiceRoom == nil || *roomReport.VoiceRoom != *room {
		t.Fatalf("File(a room) = %+v, %v; want the room, not forwarded", roomReport, err)
	}
	if got, err := e.svc.ReportRecord(ctx, roomReport.ID); err != nil || got.Report.VoiceRoom == nil || *got.Report.VoiceRoom != *room {
		t.Errorf("ReportRecord(a room) = %+v, %v; want the room and its revision", got.Report, err)
	}
	if listed, err := e.svc.Reports(ctx, appmoderation.ReportQuery{}, 10); err != nil || len(listed) == 0 || listed[0].Report.VoiceRoom == nil {
		t.Errorf("Reports = %+v, %v; want the newest report with its room", listed, err)
	}
	reported, err := e.svc.ReportedVoiceRooms(ctx, []string{room.RoomID, NamedID("other room")})
	if err != nil || !reported[room.RoomID] || len(reported) != 1 {
		t.Errorf("ReportedVoiceRooms = %v, %v; want the room alone", reported, err)
	}
	e.events.take()
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.h.Moderation.PurgeReports(ctx, tx, []string{roomReport.ID}) }); err != nil {
		t.Fatal(err)
	}
	if reported, err := e.svc.ReportedVoiceRooms(ctx, []string{room.RoomID}); err != nil || len(reported) != 0 {
		t.Errorf("ReportedVoiceRooms after the report's purge = %v, %v; want none", reported, err)
	}
}

func moderationHandlesReports(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	file := func(n string) moderation.Report {
		t.Helper()
		e.clock.Advance(time.Second)
		r, _, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
			URI: "https://remote.example/flags/" + n, ReporterAccountID: e.reports, TargetAccountID: e.local, StatusIDs: []string{NamedID("s" + n)},
		})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	oldest := func(want *time.Time) {
		t.Helper()
		got, err := e.svc.OldestOpenReportFiledAt(ctx)
		if err != nil || (got == nil) != (want == nil) || (got != nil && !got.Equal(*want)) {
			t.Errorf("OldestOpenReportFiledAt = %v, %v; want %v", got, err, want)
		}
	}
	oldest(nil)
	a, b, c := file("1"), file("2"), file("3")
	oldest(&a.FiledAt)
	list := func(q appmoderation.ReportQuery, limit int, want ...string) []moderation.ReportRecord {
		t.Helper()
		got, err := e.svc.Reports(ctx, q, limit)
		ids := []string{}
		for _, r := range got {
			ids = append(ids, r.Report.ID)
		}
		if err != nil || !reflect.DeepEqual(ids, append([]string{}, want...)) {
			t.Errorf("Reports(%+v, %d) = %v, %v; want %v", q, limit, ids, err, want)
		}
		return got
	}
	all := list(appmoderation.ReportQuery{}, 10, c.ID, b.ID, a.ID)
	if len(all) == 3 && (!reflect.DeepEqual(all[2].Report, a) || all[2].Status.State != moderation.ReportOpen || all[2].Status.Outcome != "") {
		t.Errorf("listed report = %+v, want %+v, open", all[2], a)
	}
	list(appmoderation.ReportQuery{}, 2, c.ID, b.ID)
	list(appmoderation.ReportQuery{After: b.ID}, 2, a.ID)

	// Assigning records the assignee; assigning it again changes nothing.
	r, err := e.svc.AssignReport(ctx, b.ID, NamedID("mod-1"), NamedID("mod-1"))
	if err != nil || r.Status.AssigneeUserID != NamedID("mod-1") || r.Status.State != moderation.ReportOpen || !r.Status.Since.Equal(b.FiledAt) {
		t.Errorf("AssignReport = %+v, %v; want open, assigned to mod-1", r.Status, err)
	}
	if r, err = e.svc.AssignReport(ctx, b.ID, NamedID("mod-1"), ""); err != nil || r.Status.AssigneeUserID != NamedID("mod-1") {
		t.Errorf("AssignReport again = %+v, %v", r.Status, err)
	}
	if r, err = e.svc.AssignReport(ctx, b.ID, "", NamedID("mod-2")); err != nil || r.Status.AssigneeUserID != "" {
		t.Errorf("AssignReport(nobody) = %+v, %v", r.Status, err)
	}
	if r, err = e.svc.AssignReport(ctx, b.ID, NamedID("mod-2"), NamedID("mod-2")); err != nil || r.Status.AssigneeUserID != NamedID("mod-2") {
		t.Errorf("AssignReport(mod-2) = %+v, %v", r.Status, err)
	}

	// The list filters by assignee and category, and orders oldest first.
	list(appmoderation.ReportQuery{AssigneeUserID: NamedID("mod-2")}, 10, b.ID)
	list(appmoderation.ReportQuery{AssigneeUserID: NamedID("mod-1")}, 10)
	list(appmoderation.ReportQuery{Unassigned: true}, 10, c.ID, a.ID)
	list(appmoderation.ReportQuery{Category: a.Category}, 10, c.ID, b.ID, a.ID)
	list(appmoderation.ReportQuery{Category: moderation.CategorySpam}, 10)
	list(appmoderation.ReportQuery{OldestFirst: true}, 2, a.ID, b.ID)
	list(appmoderation.ReportQuery{OldestFirst: true, After: b.ID}, 2, c.ID)
	list(appmoderation.ReportQuery{OldestFirst: true, Unassigned: true, State: moderation.ReportOpen}, 10, a.ID, c.ID)
	for _, q := range []appmoderation.ReportQuery{{AssigneeUserID: NamedID("mod-1"), Unassigned: true}, {Category: "rude"}} {
		if _, err := e.svc.Reports(ctx, q, 10); !errors.Is(err, appmoderation.ErrInvalidQuery) {
			t.Errorf("Reports(%+v) = %v, want ErrInvalidQuery", q, err)
		}
	}

	// Resolving records the outcome; resolving again keeps it.
	e.clock.Advance(time.Minute)
	resolvedAt := e.clock.Now()
	r, err = e.svc.ResolveReport(ctx, b.ID, moderation.OutcomeDismissed, NamedID("mod-2"))
	if err != nil || r.Status.State != moderation.ReportResolved || r.Status.Outcome != moderation.OutcomeDismissed ||
		r.Status.AssigneeUserID != NamedID("mod-2") || !r.Status.Since.Equal(resolvedAt) || !reflect.DeepEqual(r.Report, b) {
		t.Errorf("ResolveReport = %+v, %v; want resolved (dismissed) at %v", r, err, resolvedAt)
	}
	e.clock.Advance(time.Minute)
	if r, err = e.svc.ResolveReport(ctx, b.ID, moderation.OutcomeActioned, ""); err != nil || r.Status.Outcome != moderation.OutcomeDismissed || !r.Status.Since.Equal(resolvedAt) {
		t.Errorf("ResolveReport again = %+v, %v; want it unchanged", r.Status, err)
	}
	if _, err := e.svc.ResolveReport(ctx, a.ID, moderation.OutcomeActioned, ""); err != nil {
		t.Fatal(err)
	}
	list(appmoderation.ReportQuery{State: moderation.ReportOpen}, 10, c.ID)
	oldest(&c.FiledAt)
	resolved := list(appmoderation.ReportQuery{State: moderation.ReportResolved}, 10, b.ID, a.ID)
	if len(resolved) == 2 && resolved[1].Status.Outcome != moderation.OutcomeActioned {
		t.Errorf("resolved report = %+v, want actioned", resolved[1].Status)
	}
	got, err := e.svc.ReportRecord(ctx, a.ID)
	if err != nil || got.Status.State != moderation.ReportResolved || !reflect.DeepEqual(got.Report, a) {
		t.Errorf("ReportRecord = %+v, %v", got, err)
	}

	unknown := e.ids.NewID()
	if _, err := e.svc.ReportRecord(ctx, unknown); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("ReportRecord(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.AssignReport(ctx, unknown, NamedID("mod-1"), ""); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("AssignReport(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.ResolveReport(ctx, unknown, moderation.OutcomeActioned, ""); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("ResolveReport(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.ResolveReport(ctx, c.ID, "ignored", ""); !errors.Is(err, moderation.ErrInvalidReport) {
		t.Errorf("ResolveReport(ignored) = %v, want ErrInvalidReport", err)
	}
	if _, err := e.svc.Reports(ctx, appmoderation.ReportQuery{State: "closed"}, 10); !errors.Is(err, appmoderation.ErrInvalidQuery) {
		t.Errorf("Reports(closed) = %v, want ErrInvalidQuery", err)
	}
}

func moderationReportsOfReporter(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	other := e.ids.NewID()
	e.actors.set(appaccount.Actor{Account: account.Account{ID: other, Domain: "other.example", URI: "https://other.example/actor"}})
	file := func(n, reporter string) moderation.Report {
		t.Helper()
		e.clock.Advance(time.Second)
		r, _, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
			URI: "https://remote.example/flags/" + n, ReporterAccountID: reporter, TargetAccountID: e.local, StatusIDs: []string{NamedID("s" + n)},
		})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	a, theirs, b, c := file("1", e.reports), file("2", other), file("3", e.reports), file("4", e.reports)
	e.events.take()
	list := func(before string, limit int, want ...string) []moderation.ReportRecord {
		t.Helper()
		got, err := e.svc.ReportsFiledBy(ctx, e.reports, before, limit)
		ids := []string{}
		for _, r := range got {
			ids = append(ids, r.Report.ID)
		}
		if err != nil || !reflect.DeepEqual(ids, append([]string{}, want...)) {
			t.Errorf("ReportsFiledBy(%q, %d) = %v, %v; want %v", before, limit, ids, err, want)
		}
		return got
	}
	list("", 2, c.ID, b.ID)
	list(b.ID, 2, a.ID)
	if got, err := e.svc.ReportsFiledBy(ctx, e.ids.NewID(), "", 10); err != nil || len(got) != 0 {
		t.Errorf("ReportsFiledBy(nobody) = %v, %v; want none", got, err)
	}

	// Resolving emits the report with its resolution, once.
	e.clock.Advance(time.Minute)
	resolvedAt := e.clock.Now()
	if _, err := e.svc.ResolveReport(ctx, b.ID, moderation.OutcomeActioned, NamedID("mod-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ResolveReport(ctx, b.ID, moderation.OutcomeDismissed, NamedID("mod-1")); err != nil {
		t.Fatal(err)
	}
	events := e.events.take()
	if len(events) != 1 {
		t.Fatalf("events = %+v, want one ReportResolved", events)
	}
	ev, ok := events[0].(appmoderation.ReportResolved)
	if !ok || !reflect.DeepEqual(ev.Report, b) || ev.Resolution.ReportID != b.ID || ev.Resolution.Outcome != moderation.OutcomeActioned ||
		ev.Resolution.ResolvedByUserID != NamedID("mod-1") || !ev.Resolution.ResolvedAt.Equal(resolvedAt) || ev.Resolution.ID == "" {
		t.Errorf("event = %+v, want b resolved as actioned at %v", events[0], resolvedAt)
	}
	all := list("", 10, c.ID, b.ID, a.ID)
	if len(all) == 3 && (all[1].Status.State != moderation.ReportResolved || all[0].Status.State != moderation.ReportOpen ||
		!reflect.DeepEqual(all[1].Report, b)) {
		t.Errorf("listed = %+v, want b resolved and c open", all)
	}

	if got, err := e.svc.ReportFiledBy(ctx, e.reports, b.ID); err != nil || got.Status.State != moderation.ReportResolved || !reflect.DeepEqual(got.Report, b) {
		t.Errorf("ReportFiledBy(b) = %+v, %v", got, err)
	}
	if _, err := e.svc.ReportFiledBy(ctx, e.reports, theirs.ID); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("ReportFiledBy(another reporter's) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.ReportFiledBy(ctx, e.reports, e.ids.NewID()); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("ReportFiledBy(unknown) = %v, want ErrNotFound", err)
	}
}

// moderationFirstOpenReport checks that ReportFiled tells whether no other report of the target
// was open, which decides whether the moderators are notified, and the count of open reports.
func moderationFirstOpenReport(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	file := func(n string) (moderation.Report, bool) {
		t.Helper()
		e.clock.Advance(time.Second)
		r, _, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
			URI: "https://remote.example/flags/first-" + n, ReporterAccountID: e.reports, TargetAccountID: e.local,
		})
		if err != nil {
			t.Fatal(err)
		}
		events := e.events.take()
		if len(events) != 1 {
			t.Fatalf("events of report %s = %+v, want one ReportFiled", n, events)
		}
		ev, ok := events[0].(appmoderation.ReportFiled)
		if !ok || ev.Report.ID != r.ID {
			t.Fatalf("event = %+v, want ReportFiled of %s", events[0], r.ID)
		}
		return r, ev.FirstOpen
	}
	count := func(want int64) {
		t.Helper()
		if n, err := e.svc.CountOpenReports(ctx); err != nil || n != want {
			t.Errorf("CountOpenReports = %d, %v; want %d", n, err, want)
		}
	}
	count(0)
	a, first := file("a")
	if !first {
		t.Error("the first report of the account is not FirstOpen")
	}
	b, first := file("b")
	if first {
		t.Error("a second open report of the account is FirstOpen")
	}
	count(2)
	for _, r := range []moderation.Report{a, b} {
		if _, err := e.svc.ResolveReport(ctx, r.ID, moderation.OutcomeDismissed, ""); err != nil {
			t.Fatal(err)
		}
	}
	count(0)
	e.events.take() // the resolutions
	if _, first := file("c"); !first {
		t.Error("a report after the others are resolved is not FirstOpen")
	}
	count(1)
}
