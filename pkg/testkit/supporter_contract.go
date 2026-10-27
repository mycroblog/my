// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	domain "github.com/mycroblog/my/internal/domain/supporters"
)

// SupporterHarness is what SupporterRepositoryContract needs from a database adapter: the
// harness of the identity and account repositories, the supporters repository, and a
// PaymentWorld for the provider's side.
type SupporterHarness struct {
	AccountHarness
	Supporters supporters.Repository
	NewWorld   func(o PaymentWorldOptions) PaymentWorld
}

// SupporterFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type SupporterFactory func(t *testing.T, opts uow.Options) SupporterHarness

// SupporterRepositoryContract verifies the supporters repository of a database adapter through
// the use cases of the supporter program (ADR-0072, FR-910 to FR-912) on the identity
// repository's roles and the fake provider: the program is disabled by default, needs a provider
// and email, and is not disabled while subscriptions go on, and nothing of it shows while it is
// disabled; a plan takes only a role that nobody holds, with member permissions below every
// staff role, which nobody grants, changes, or deletes by hand afterwards and below which no
// staff role goes; the plans keep their order; a subscription follows the provider's snapshots
// and our deadlines through every state with its role (the end of the period after a
// cancellation, the 7-day grace, refunds and chargebacks, changes of plan at once and at the end
// of the period); a user subscribes once at a time and only when eligible; new prices go to the
// new subscriptions only; retiring a plan ends its subscriptions and frees its role; a staff
// member cancels at once; the supporter list shows only the members who chose to be listed, by
// plan, until their subscription ends; and the projections rebuild to what the use cases wrote.
func SupporterRepositoryContract(t *testing.T, newHarness SupporterFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *supporterEnv)
	}{
		{"EnablesOnlyWithAProviderAndEmail", supporterProgram},
		{"ShowsNothingWhileDisabled", supporterDisabled},
		{"TakesOnlyFitRolesAndGuardsThem", supporterSafetyRail},
		{"KeepsTheOrderOfThePlans", supporterTiers},
		{"GrantsTheRoleToThePeriodEnd", supporterCancel},
		{"KeepsTheRoleThroughTheGrace", supporterGrace},
		{"RevokesOnFullRefundsAndChargebacks", supporterReversals},
		{"ChangesPlansAtOnceOrAtThePeriodEnd", supporterChanges},
		{"RetriesAChangeWithItsKey", supporterChangeRetry},
		{"AppliesNoSnapshotOlderThanTheApplied", supporterSnapshotOrder},
		{"AbandonsUnpaidCheckouts", supporterAbandon},
		{"SubscribesOnceAndOnlyWhenEligible", supporterEligibility},
		{"ChargesNewPricesToNewSubscriptions", supporterPrices},
		{"RetiresPlans", supporterRetire},
		{"CancelsByStaff", supporterStaffCancel},
		{"ListsOnlyTheMembersWhoChoseTo", supporterListings},
		{"RebuildsItsProjections", supporterRebuild},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newSupporterEnv(t, newHarness)) })
	}
}

// supporterUsers is the users of the contract, by ID.
type supporterUsers struct {
	mu    sync.Mutex
	users map[string]supporters.User
}

func (u *supporterUsers) SupporterUser(_ context.Context, id string) (supporters.User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	user, ok := u.users[id]
	if !ok {
		return supporters.User{}, fmt.Errorf("%w: user %s", supporters.ErrNotFound, id)
	}
	return user, nil
}

func (u *supporterUsers) AccountIDs(_ context.Context, ids []string) (map[string]string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := map[string]string{}
	for _, id := range ids {
		if user, ok := u.users[id]; ok {
			out[id] = user.AccountID
		}
	}
	return out, nil
}

func (u *supporterUsers) put(user supporters.User) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.users[user.ID] = user
}

// subscriptionWorld is the provider's side of the contract: the fake adapter.
type subscriptionWorld interface {
	PaymentWorld
	Subscription(ref supporters.SubscriptionRef) (supporters.SubscriptionSnapshot, bool)
}

type supporterEnv struct {
	*accountEnv
	h        SupporterHarness
	world    subscriptionWorld
	users    *supporterUsers
	jobs     *paymentJobs
	handlers *queue.JobHandlerRegistry
	audit    *auditRecorder
	svc      *supporters.Service
	deps     supporters.Deps
	seq      int
}

func newSupporterEnv(t *testing.T, newHarness SupporterFactory) *supporterEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	rec := &auditRecorder{}
	ae := newAccountEnvOn(h.AccountHarness, projectors, func(p *uow.Projectors) {
		p.Register(appmoderation.EventAuditRecorded, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
			rec.add(ev.(appmoderation.AuditRecorded).Entry)
			return nil
		})
	})
	world, ok := h.NewWorld(PaymentWorldOptions{Name: paymentProvider, Mode: supporters.ModeTest, Now: ae.clock.Now}).(subscriptionWorld)
	if !ok {
		t.Fatal("the payment world does not report its subscriptions")
	}
	e := &supporterEnv{
		accountEnv: ae, h: h, world: world, users: &supporterUsers{users: map[string]supporters.User{}},
		jobs: &paymentJobs{pending: map[string]bool{}}, handlers: &queue.JobHandlerRegistry{}, audit: rec,
	}
	e.deps = supporters.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Supporters, Roles: ae.identity, Users: e.users, Provider: world, SendsEmail: true,
		Outbox: queue.AfterCommit(projectors, e.jobs, nil), Clock: ae.clock, IDs: ae.ids, Logger: discardLogger(),
	}
	e.svc = supporters.NewService(e.deps)
	e.svc.RegisterJobs(e.handlers, nil)
	return e
}

// run runs the queued jobs, the ones they queue included, and fails the test on one that does
// not end done.
func (e *supporterEnv) run(t *testing.T) {
	t.Helper()
	for i := 0; ; i++ {
		job, ok := e.jobs.next()
		if !ok {
			return
		}
		h, _, found := e.handlers.Lookup(job.Type)
		if !found {
			t.Fatalf("no handler of %s", job.Type)
		}
		r := h(t.Context(), queue.Job{
			ID: fmt.Sprintf("job-%d", i), Type: job.Type, Payload: job.Payload, IdempotencyKey: job.IdempotencyKey,
			OrderingKey: job.OrderingKey, Attempt: 1, RunAt: job.RunAt, CreatedAt: e.clock.Now(),
		})
		if !r.IsDone() {
			t.Fatalf("job %s (%s) did not end done: %v", job.Type, job.IdempotencyKey, r.Err())
		}
		e.jobs.done(job)
	}
}

func (e *supporterEnv) enable(t *testing.T) {
	t.Helper()
	if _, err := e.svc.SetProgram(t.Context(), true, ""); err != nil {
		t.Fatalf("SetProgram(true) = %v", err)
	}
}

// role creates a role of the CLI.
func (e *supporterEnv) role(t *testing.T, key string, position int, perms ...identity.Permission) appidentity.RoleWithPermissions {
	t.Helper()
	r, err := e.identity.CreateRole(t.Context(), appidentity.CreateRoleInput{Key: key, Position: position, Permissions: perms})
	if err != nil {
		t.Fatalf("CreateRole(%s) = %v", key, err)
	}
	return r
}

func terms(name string, rank int, monthly int64) domain.PlanTerms {
	return domain.PlanTerms{Name: name, MonthlyAmountMinor: monthly, AnnualDiscountPercent: 10, EmojiLimit: 5, Rank: rank, Visibility: domain.VisibilityPublic}
}

// plan creates a plan on a new role and makes its prices.
func (e *supporterEnv) plan(t *testing.T, key string, rank, position int, monthly int64) supporters.PlanView {
	t.Helper()
	e.role(t, key, position)
	p, err := e.svc.CreatePlan(t.Context(), supporters.PlanInput{RoleKey: key, Currency: "JPY", Terms: terms(key, rank, monthly)})
	if err != nil {
		t.Fatalf("CreatePlan(%s) = %v", key, err)
	}
	e.run(t)
	return e.planOf(t, p.ID)
}

func (e *supporterEnv) planOf(t *testing.T, id string) supporters.PlanView {
	t.Helper()
	p, err := e.svc.Plan(t.Context(), id)
	if err != nil {
		t.Fatalf("Plan(%s) = %v", id, err)
	}
	return p
}

// user adds an eligible user.
func (e *supporterEnv) user(name string) string {
	id := e.ids.NewID()
	e.users.put(supporters.User{ID: id, AccountID: NamedID("acct-" + name), Handle: "@" + name, Email: name + "@example.com", EmailVerified: true, Locale: "en"})
	return id
}

// subscribe starts a subscription, completes its checkout at the provider with an active
// subscription on the plan's price, and applies it as the return from the checkout does. It
// returns our subscription and the provider's.
func (e *supporterEnv) subscribe(t *testing.T, userID string, plan supporters.PlanView, interval domain.Interval) (string, supporters.SubscriptionRef) {
	t.Helper()
	co, err := e.svc.StartCheckout(t.Context(), supporters.CheckoutInput{
		UserID: userID, PlanID: plan.ID, Interval: interval, SuccessURL: "https://" + contractHost + "/ok", CancelURL: "https://" + contractHost + "/no",
	})
	if err != nil {
		t.Fatalf("StartCheckout = %v", err)
	}
	sub, err := e.svc.Subscription(t.Context(), co.SubscriptionID)
	if err != nil || sub.Checkout == nil {
		t.Fatalf("the subscription after the checkout: %+v, %v", sub, err)
	}
	e.seq++
	ref := supporters.SubscriptionRef(fmt.Sprintf("sub_%d", e.seq))
	e.world.PutCheckout(supporters.CheckoutResult{
		Ref: sub.Checkout.Ref, Status: supporters.CheckoutCompleted, LocalID: co.SubscriptionID,
		Customer: supporters.CustomerRef(fmt.Sprintf("cus_%d", e.seq)), Subscription: ref,
	})
	e.world.PutSubscription(supporters.SubscriptionSnapshot{
		Ref: ref, Customer: supporters.CustomerRef(fmt.Sprintf("cus_%d", e.seq)), LocalID: co.SubscriptionID,
		Price: plan.Prices[interval].Ref, Status: supporters.StatusActive,
		PeriodStart: e.clock.Now(), PeriodEnd: e.clock.Now().Add(30 * 24 * time.Hour),
	})
	e.sync(t, co.SubscriptionID)
	return co.SubscriptionID, ref
}

// sync queues and runs the JobSync of a subscription, as a webhook or a deadline does.
func (e *supporterEnv) sync(t *testing.T, id string) {
	t.Helper()
	e.seq++
	if err := e.jobs.Enqueue(t.Context(), supporters.SyncJob(id, fmt.Sprintf("test-%d", e.seq))); err != nil {
		t.Fatal(err)
	}
	e.run(t)
}

// change changes the provider's subscription and syncs ours.
func (e *supporterEnv) change(t *testing.T, id string, ref supporters.SubscriptionRef, f func(*supporters.SubscriptionSnapshot)) {
	t.Helper()
	s, ok := e.world.Subscription(ref)
	if !ok {
		t.Fatalf("no subscription %s at the provider", ref)
	}
	f(&s)
	e.world.PutSubscription(s)
	e.sync(t, id)
}

func (e *supporterEnv) sub(t *testing.T, id string) supporters.SubscriptionView {
	t.Helper()
	v, err := e.svc.Subscription(t.Context(), id)
	if err != nil {
		t.Fatalf("Subscription(%s) = %v", id, err)
	}
	return v
}

// expect checks the state of a subscription and whether its user holds the plan's role.
func (e *supporterEnv) expect(t *testing.T, id string, want domain.State, plan supporters.PlanView, holds bool) supporters.SubscriptionView {
	t.Helper()
	v := e.sub(t, id)
	if v.Current.State != want {
		t.Fatalf("state %s, want %s", v.Current.State, want)
	}
	if got := e.holds(t, v.UserID, plan.RoleID); got != holds {
		t.Fatalf("holds the role of %s: %v, want %v (state %s)", plan.Revision.Name, got, holds, v.Current.State)
	}
	return v
}

func (e *supporterEnv) holds(t *testing.T, userID, roleID string) bool {
	t.Helper()
	ids, err := e.identity.UserRoleIDs(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return slices.Contains(ids, roleID)
}

func (e *supporterEnv) managed(t *testing.T, roleID string) bool {
	t.Helper()
	ids, err := e.identity.ManagedRoleIDs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return slices.Contains(ids, roleID)
}

func (e *supporterEnv) actions() []moderation.AuditAction {
	var out []moderation.AuditAction
	for _, a := range e.audit.all() {
		out = append(out, a.Action)
	}
	return out
}

func supporterProgram(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	p, err := e.svc.Program(ctx)
	if err != nil || p.Enabled || !p.CanEnable() || p.ConfiguredProvider != paymentProvider {
		t.Fatalf("the program by default: %+v, %v", p, err)
	}
	noProvider := e.deps
	noProvider.Provider = nil
	if _, err := supporters.NewService(noProvider).SetProgram(ctx, true, ""); !errors.Is(err, supporters.ErrNoProvider) {
		t.Errorf("enabled without a provider: %v", err)
	}
	noMail := e.deps
	noMail.SendsEmail = false
	if _, err := supporters.NewService(noMail).SetProgram(ctx, true, ""); !errors.Is(err, supporters.ErrNoMail) {
		t.Errorf("enabled without email: %v", err)
	}
	e.enable(t)
	e.enable(t) // again: nothing more
	if p, _ := e.svc.Program(ctx); !p.Enabled || p.Provider != paymentProvider || p.ProviderMismatch() {
		t.Fatalf("the enabled program: %+v", p)
	}
	plan := e.plan(t, "supporter", 1, 10, 500)
	id, _ := e.subscribe(t, e.user("alice"), plan, domain.IntervalMonth)
	if _, err := e.svc.SetProgram(ctx, false, ""); !errors.Is(err, supporters.ErrLiveSubscriptions) {
		t.Fatalf("disabled with a subscription: %v", err)
	}
	if err := e.svc.CancelByStaff(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	if p, err := e.svc.SetProgram(ctx, false, ""); err != nil || p.Enabled {
		t.Fatalf("disabled after the subscription ended: %+v, %v", p, err)
	}
	var program []moderation.AuditAction
	for _, a := range e.actions() {
		if t, _ := a.Target(); t == moderation.AuditTargetSupporterProgram {
			program = append(program, a)
		}
	}
	if !reflect.DeepEqual(program, []moderation.AuditAction{moderation.AuditSupporterProgramEnabled, moderation.AuditSupporterProgramDisabled}) {
		t.Errorf("audit log of the program: %v", program)
	}
	// Another provider than the program's refuses new subscriptions until it is enabled again.
	e.enable(t)
	other := e.deps
	other.Provider = renamedProvider{e.world, "other"}
	if err := mismatchCheckout(ctx, supporters.NewService(other), e.user("bob"), plan.ID); !errors.Is(err, supporters.ErrProviderMismatch) {
		t.Errorf("a checkout on another provider: %v", err)
	}
}

func mismatchCheckout(ctx context.Context, svc *supporters.Service, userID, planID string) error {
	_, err := svc.StartCheckout(ctx, supporters.CheckoutInput{UserID: userID, PlanID: planID, Interval: domain.IntervalMonth})
	return err
}

// renamedProvider is a provider under another name.
type renamedProvider struct {
	supporters.PaymentProvider
	name string
}

func (p renamedProvider) Name() string { return p.name }

func supporterDisabled(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	plan := e.plan(t, "supporter", 1, 10, 500)
	if !plan.Ready() {
		t.Fatalf("the plan has no prices after its job: %+v", plan.Prices)
	}
	if _, err := e.svc.OfferedPlans(ctx); !errors.Is(err, supporters.ErrDisabled) {
		t.Errorf("OfferedPlans while disabled: %v", err)
	}
	alice := e.user("alice")
	if _, err := e.svc.StartCheckout(ctx, supporters.CheckoutInput{UserID: alice, PlanID: plan.ID, Interval: domain.IntervalMonth}); !errors.Is(err, supporters.ErrDisabled) {
		t.Errorf("StartCheckout while disabled: %v", err)
	}
	if _, err := e.svc.OwnSubscription(ctx, alice); !errors.Is(err, supporters.ErrDisabled) {
		t.Errorf("OwnSubscription while disabled: %v", err)
	}
	if counts, err := e.svc.PlanCounts(ctx); err != nil || len(counts) != 0 {
		t.Errorf("PlanCounts while disabled: %v, %v", counts, err)
	}
	if _, limited, err := e.svc.EmojiLimit(ctx, alice); err != nil || limited {
		t.Errorf("EmojiLimit while disabled: %v, %v", limited, err)
	}
	e.enable(t)
	offered, err := e.svc.OfferedPlans(ctx)
	if err != nil || len(offered) != 1 || offered[0].ID != plan.ID {
		t.Fatalf("OfferedPlans: %v, %v", offered, err)
	}
	hidden := domain.VisibilityHidden
	if _, err := e.svc.RevisePlan(ctx, supporters.PlanUpdate{ID: plan.ID, Visibility: &hidden}); err != nil {
		t.Fatal(err)
	}
	if offered, _ := e.svc.OfferedPlans(ctx); len(offered) != 0 {
		t.Errorf("a hidden plan is offered: %v", offered)
	}
	if _, err := e.svc.StartCheckout(ctx, supporters.CheckoutInput{UserID: alice, PlanID: plan.ID, Interval: domain.IntervalMonth}); !errors.Is(err, supporters.ErrPlanUnavailable) {
		t.Errorf("StartCheckout of a hidden plan: %v", err)
	}
}

func supporterSafetyRail(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	// The first administrator brings the built-in roles.
	admin, err := e.identity.CreateAdmin(ctx, appidentity.CreateAdminInput{
		Username: "root", Email: "root@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	create := func(key string) error {
		_, err := e.svc.CreatePlan(ctx, supporters.PlanInput{RoleKey: key, Currency: "JPY", Terms: terms(key, len(key), 500)})
		return err
	}
	rule := func(err error) identity.ManageRule {
		var ue *appidentity.UnmanageableRoleError
		if errors.As(err, &ue) {
			return ue.Rule
		}
		return ""
	}
	if got := rule(create(identity.RoleModerator)); got != identity.RuleBuiltin {
		t.Errorf("moderator as a plan's role: %s", got)
	}
	e.role(t, "helper", 60, identity.PermManageReports)
	if got := rule(create("helper")); got != identity.RulePermissions {
		t.Errorf("a role with a staff permission: %s", got)
	}
	e.role(t, "high", 100)
	if got := rule(create("high")); got != identity.RulePosition {
		t.Errorf("a role at the moderator's position: %s", got)
	}
	e.role(t, "held", 5)
	if _, err := e.identity.GrantRole(ctx, appidentity.RoleChangeInput{UserID: admin.User.ID, RoleKey: "held"}); err != nil {
		t.Fatal(err)
	}
	if got := rule(create("held")); got != identity.RuleHeld {
		t.Errorf("a held role: %s", got)
	}
	if _, err := e.svc.CreatePlan(ctx, supporters.PlanInput{RoleKey: "missing", Currency: "JPY", Terms: terms("x", 1, 500)}); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("an unknown role: %v", err)
	}
	if plans, _ := e.svc.Plans(ctx); len(plans) != 0 {
		t.Fatalf("refused plans were recorded: %v", plans)
	}
	plan := e.plan(t, "supporter", 1, 10, 500)
	if !e.managed(t, plan.RoleID) {
		t.Fatal("the plan's role is not managed")
	}
	if got := rule(create("supporter")); got != identity.RuleManaged {
		t.Errorf("the role of another plan: %s", got)
	}
	// Nobody grants, revokes, changes, or deletes the plan's role by hand.
	dave := admin.User.ID
	if _, err := e.identity.GrantRole(ctx, appidentity.RoleChangeInput{UserID: dave, RoleKey: "supporter"}); !errors.Is(err, appidentity.ErrRoleManagedByPlan) {
		t.Errorf("GrantRole of a plan's role: %v", err)
	}
	if _, err := e.identity.RevokeRole(ctx, appidentity.RoleChangeInput{UserID: dave, RoleKey: "supporter"}); !errors.Is(err, appidentity.ErrRoleManagedByPlan) {
		t.Errorf("RevokeRole of a plan's role: %v", err)
	}
	pos := 20
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{Key: "supporter", Position: &pos}); !errors.Is(err, appidentity.ErrRoleManagedByPlan) {
		t.Errorf("UpdateRole of a plan's role: %v", err)
	}
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{
		Key: "supporter", SetPermissions: true, Permissions: []identity.Permission{identity.PermManageReports},
	}); !errors.Is(err, appidentity.ErrRoleManagedByPlan) {
		t.Errorf("a staff permission on a plan's role: %v", err)
	}
	if err := e.identity.DeleteRole(ctx, "supporter", ""); !errors.Is(err, appidentity.ErrRoleManagedByPlan) {
		t.Errorf("DeleteRole of a plan's role: %v", err)
	}
	// No staff role goes at or below the plan's role.
	low := 10
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{Key: identity.RoleModerator, Position: &low}); !errors.Is(err, appidentity.ErrBreaksManagedRoles) {
		t.Errorf("the moderator moved onto the plan's role: %v", err)
	}
	if _, err := e.identity.UpdateRole(ctx, appidentity.UpdateRoleInput{
		Key: "held", SetPermissions: true, Permissions: []identity.Permission{identity.PermViewAudit},
	}); !errors.Is(err, appidentity.ErrBreaksManagedRoles) {
		t.Errorf("a staff permission below the plan's role: %v", err)
	}
	if _, err := e.identity.CreateRole(ctx, appidentity.CreateRoleInput{Key: "lowstaff", Position: 3, Permissions: []identity.Permission{identity.PermManageJobs}}); !errors.Is(err, appidentity.ErrBreaksManagedRoles) {
		t.Errorf("a new staff role below the plan's role: %v", err)
	}
	// The plan form changes the role, within the rail.
	staff := []appidentity.Permission{identity.PermManageReports}
	if _, err := e.svc.RevisePlan(ctx, supporters.PlanUpdate{ID: plan.ID, SetRolePermissions: true, RolePermissions: staff}); rule(err) != identity.RulePermissions {
		t.Errorf("a staff permission from the plan form: %v", err)
	}
	high := 100
	if _, err := e.svc.RevisePlan(ctx, supporters.PlanUpdate{ID: plan.ID, RolePosition: &high}); rule(err) != identity.RulePosition {
		t.Errorf("the plan's role moved to the moderator's position: %v", err)
	}
	ok := 40
	if p, err := e.svc.RevisePlan(ctx, supporters.PlanUpdate{ID: plan.ID, RolePosition: &ok}); err != nil || p.Role.Position != 40 {
		t.Errorf("the plan's role moved within the rail: %+v, %v", p.Role, err)
	}
	if !slices.Contains(e.actions(), moderation.AuditRoleUpdated) || !slices.Contains(e.actions(), moderation.AuditSupporterPlanCreated) {
		t.Errorf("audit log: %v", e.actions())
	}
}

func supporterTiers(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	basic := e.plan(t, "basic", 1, 10, 500)
	e.role(t, "gold", 20)
	if _, err := e.svc.CreatePlan(ctx, supporters.PlanInput{RoleKey: "gold", Currency: "JPY", Terms: terms("gold", 1, 1000)}); !errors.Is(err, supporters.ErrTiersOutOfOrder) {
		t.Errorf("a plan of the same rank: %v", err)
	}
	e.role(t, "silver", 5)
	var te *domain.TierError
	if _, err := e.svc.CreatePlan(ctx, supporters.PlanInput{RoleKey: "silver", Currency: "JPY", Terms: terms("silver", 2, 800)}); !errors.As(err, &te) || te.Rule != domain.TierPosition {
		t.Errorf("a higher plan on a lower role: %v", err)
	}
	gold, err := e.svc.CreatePlan(ctx, supporters.PlanInput{RoleKey: "gold", Currency: "JPY", Terms: terms("gold", 2, 1000)})
	if err != nil {
		t.Fatal(err)
	}
	low := 15
	if _, err := e.svc.RevisePlan(ctx, supporters.PlanUpdate{ID: basic.ID, RolePosition: &low}); err != nil {
		t.Errorf("the lower plan's role below the higher one's: %v", err)
	}
	above := 30
	if _, err := e.svc.RevisePlan(ctx, supporters.PlanUpdate{ID: basic.ID, RolePosition: &above}); !errors.As(err, &te) || te.Rule != domain.TierPosition {
		t.Errorf("the lower plan's role above the higher one's: %v", err)
	}
	three := 3
	if _, err := e.svc.RevisePlan(ctx, supporters.PlanUpdate{ID: basic.ID, Rank: &three}); !errors.As(err, &te) || te.Rule != domain.TierPosition {
		t.Errorf("the lower plan ranked above the higher one: %v", err)
	}
	if _, err := e.svc.CreatePlan(ctx, supporters.PlanInput{RoleKey: "silver", Currency: "XXX", Terms: terms("silver", 3, 800)}); !errors.Is(err, supporters.ErrInvalidPlan) {
		t.Errorf("a currency the provider does not take: %v", err)
	}
	if _, err := e.svc.CreatePlan(ctx, supporters.PlanInput{RoleKey: "silver", Currency: "JPY", Terms: terms("silver", 3, 10)}); !errors.Is(err, supporters.ErrInvalidPlan) {
		t.Errorf("a price below the provider's minimum: %v", err)
	}
	plans, _ := e.svc.Plans(ctx)
	if len(plans) != 2 || plans[0].ID != basic.ID || plans[1].ID != gold.ID {
		t.Errorf("plans by rank: %v", plans)
	}
}

func supporterCancel(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	plan := e.plan(t, "supporter", 1, 10, 500)
	alice := e.user("alice")
	id, ref := e.subscribe(t, alice, plan, domain.IntervalMonth)
	v := e.expect(t, id, domain.StateActive, plan, true)
	if v.Link != ref || v.Price.AmountMinor != 500 {
		t.Fatalf("the active subscription: %+v", v)
	}
	if own, err := e.svc.OwnSubscription(ctx, alice); err != nil || own.ID != id {
		t.Fatalf("OwnSubscription = %+v, %v", own, err)
	}
	if counts, _ := e.svc.PlanCounts(ctx); len(counts) != 1 || counts[0].Members != 1 {
		t.Errorf("PlanCounts: %v", counts)
	}
	if users, _ := e.svc.SubscriptionUsers(ctx, []string{id, NamedID("unknown")}); len(users) != 1 || users[0].UserID != alice {
		t.Errorf("SubscriptionUsers: %v", users)
	}
	if _, err := e.svc.Cancel(ctx, alice); err != nil {
		t.Fatalf("Cancel = %v", err)
	}
	e.expect(t, id, domain.StateCanceling, plan, true)
	if _, err := e.svc.Resume(ctx, alice); err != nil {
		t.Fatalf("Resume = %v", err)
	}
	e.expect(t, id, domain.StateActive, plan, true)
	if _, err := e.svc.Cancel(ctx, alice); err != nil {
		t.Fatal(err)
	}
	// The same snapshot again changes nothing.
	before := e.sub(t, id).Current.ID
	e.sync(t, id)
	if e.sub(t, id).Current.ID != before {
		t.Error("the same snapshot recorded a transition")
	}
	// The period ends: the provider's subscription ends.
	e.clock.Advance(31 * 24 * time.Hour)
	e.change(t, id, ref, func(s *supporters.SubscriptionSnapshot) { s.Status = supporters.StatusCanceled })
	e.expect(t, id, domain.StateExpired, plan, false)
	if _, err := e.svc.OwnSubscription(ctx, alice); !errors.Is(err, supporters.ErrNoLiveSubscription) {
		t.Errorf("OwnSubscription after the end: %v", err)
	}
	var system []moderation.AuditAction
	for _, a := range e.audit.all() {
		if a.ActorKind == moderation.AuditActorSystem {
			system = append(system, a.Action)
		}
	}
	if !reflect.DeepEqual(system, []moderation.AuditAction{moderation.AuditRoleGranted, moderation.AuditRoleRevoked}) {
		t.Errorf("the system's audit log: %v", system)
	}
}

func supporterGrace(t *testing.T, e *supporterEnv) {
	e.enable(t)
	plan := e.plan(t, "supporter", 1, 10, 500)
	id, ref := e.subscribe(t, e.user("alice"), plan, domain.IntervalMonth)
	failed := e.clock.Advance(30 * 24 * time.Hour)
	e.change(t, id, ref, func(s *supporters.SubscriptionSnapshot) {
		s.Status, s.LatestPaymentFailed, s.PeriodEnd = supporters.StatusPastDue, failed, failed.Add(30*24*time.Hour)
	})
	v := e.expect(t, id, domain.StatePastDue, plan, true)
	if v.Current.Grace == nil || !v.Current.Grace.EndsAt.Equal(failed.Add(domain.GracePeriod).UTC().Truncate(time.Millisecond)) {
		t.Fatalf("grace %+v", v.Current.Grace)
	}
	// Six days later the deadline is not due.
	e.clock.Advance(6 * 24 * time.Hour)
	if err := e.svc.QueueDeadlines(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	e.expect(t, id, domain.StatePastDue, plan, true)
	// Seven days after the failure the Scheduler's deadline takes the role away; the provider
	// still retries.
	e.clock.Advance(24 * time.Hour)
	if err := e.svc.QueueDeadlines(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	v = e.expect(t, id, domain.StateLapsed, plan, false)
	if v.Current.Reason != domain.ReasonGraceExpired {
		t.Errorf("reason %s", v.Current.Reason)
	}
	// The provider's retry succeeds: the role comes back.
	e.change(t, id, ref, func(s *supporters.SubscriptionSnapshot) {
		s.Status, s.LatestPaymentFailed = supporters.StatusActive, time.Time{}
	})
	e.expect(t, id, domain.StateActive, plan, true)
}

func supporterReversals(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	plan := e.plan(t, "supporter", 1, 10, 500)
	id, ref := e.subscribe(t, e.user("alice"), plan, domain.IntervalMonth)
	refund := func(mid, gross, parent string, kind supporters.MoneyKind, sub supporters.SubscriptionRef) supporters.MoneyMovement {
		return supporters.MoneyMovement{
			ID: mid, Kind: kind, ParentID: "txn_pay", ParentGross: parent, Subscription: sub, OccurredAt: e.clock.Now(),
			Gross: gross, Fee: "0", Net: gross, Currency: "JPY", Final: true,
		}
	}
	if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{refund("txn_r1", "-200", "500", supporters.MoneyRefund, ref)}); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	e.expect(t, id, domain.StateActive, plan, true)
	known, err := e.svc.KnownMoney(ctx, paymentProvider, []string{"txn_r1", "txn_pay"})
	if err != nil || !known["txn_r1"] || known["txn_pay"] {
		t.Errorf("KnownMoney = %v, %v", known, err)
	}
	full := refund("txn_r2", "-500", "500", supporters.MoneyRefund, ref)
	for range 2 {
		if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{full}); err != nil {
			t.Fatal(err)
		}
	}
	e.run(t)
	v := e.expect(t, id, domain.StateRevoked, plan, false)
	if v.Current.Reason != domain.ReasonReversed {
		t.Errorf("reason %s", v.Current.Reason)
	}
	if s, _ := e.world.Subscription(ref); s.Status != supporters.StatusCanceled {
		t.Errorf("the provider's subscription after a full refund: %s", s.Status)
	}
	// A won dispute gives nothing back; a later snapshot changes nothing either.
	if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{refund("txn_r3", "500", "500", supporters.MoneyChargebackReversal, ref)}); err != nil {
		t.Fatal(err)
	}
	e.sync(t, id)
	e.expect(t, id, domain.StateRevoked, plan, false)
	// A chargeback revokes another subscription.
	bob := e.user("bob")
	id2, ref2 := e.subscribe(t, bob, plan, domain.IntervalYear)
	if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{refund("txn_c1", "-5400", "5400", supporters.MoneyChargeback, ref2)}); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	e.expect(t, id2, domain.StateRevoked, plan, false)
	// The user subscribes again.
	if _, err := e.svc.StartCheckout(ctx, supporters.CheckoutInput{UserID: bob, PlanID: plan.ID, Interval: domain.IntervalMonth}); err != nil {
		t.Errorf("StartCheckout after a revocation: %v", err)
	}
}

func supporterChanges(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	basic := e.plan(t, "basic", 1, 10, 500)
	gold := e.plan(t, "gold", 2, 20, 1000)
	alice := e.user("alice")
	id, ref := e.subscribe(t, alice, basic, domain.IntervalMonth)
	if _, err := e.svc.ChangePlan(ctx, alice, basic.ID, domain.IntervalMonth); !errors.Is(err, supporters.ErrSameChoice) {
		t.Errorf("a change to the same choice: %v", err)
	}
	// An upgrade applies at once: the role is replaced.
	if _, err := e.svc.ChangePlan(ctx, alice, gold.ID, domain.IntervalMonth); err != nil {
		t.Fatalf("ChangePlan up = %v", err)
	}
	v := e.expect(t, id, domain.StateActive, gold, true)
	if e.holds(t, alice, basic.RoleID) || v.Current.PlanID != gold.ID {
		t.Fatalf("after the upgrade: %+v, holds basic %v", v.Current, e.holds(t, alice, basic.RoleID))
	}
	// A downgrade waits for the end of the period.
	if _, err := e.svc.ChangePlan(ctx, alice, basic.ID, domain.IntervalMonth); err != nil {
		t.Fatalf("ChangePlan down = %v", err)
	}
	v = e.expect(t, id, domain.StateActive, gold, true)
	if v.Current.Scheduled == nil || v.ScheduledPrice == nil || v.ScheduledPrice.PlanID != basic.ID {
		t.Fatalf("the scheduled downgrade: %+v", v.Current.Scheduled)
	}
	if _, err := e.svc.CancelScheduledChange(ctx, alice); err != nil {
		t.Fatal(err)
	}
	if v = e.sub(t, id); v.Current.Scheduled != nil {
		t.Fatalf("the downgrade was not withdrawn: %+v", v.Current.Scheduled)
	}
	// Yearly to monthly also waits; monthly to yearly applies at once.
	if _, err := e.svc.ChangePlan(ctx, alice, gold.ID, domain.IntervalYear); err != nil {
		t.Fatal(err)
	}
	if v = e.sub(t, id); v.Price.Interval != domain.IntervalYear || v.Current.Scheduled != nil {
		t.Fatalf("monthly to yearly: %+v", v.Price)
	}
	if _, err := e.svc.ChangePlan(ctx, alice, gold.ID, domain.IntervalMonth); err != nil {
		t.Fatal(err)
	}
	if v = e.sub(t, id); v.Price.Interval != domain.IntervalYear || v.Current.Scheduled == nil {
		t.Fatalf("yearly to monthly: %+v %+v", v.Price, v.Current.Scheduled)
	}
	// The period ends: the provider switches to the scheduled price.
	e.change(t, id, ref, func(s *supporters.SubscriptionSnapshot) {
		s.Price, s.ScheduledPrice = gold.Prices[domain.IntervalMonth].Ref, ""
	})
	if v = e.expect(t, id, domain.StateActive, gold, true); v.Price.Interval != domain.IntervalMonth || v.Current.Scheduled != nil {
		t.Fatalf("after the scheduled change: %+v %+v", v.Price, v.Current.Scheduled)
	}
	// A downgrade to the lower plan takes effect at the period's end, with its role.
	if _, err := e.svc.ChangePlan(ctx, alice, basic.ID, domain.IntervalMonth); err != nil {
		t.Fatal(err)
	}
	e.expect(t, id, domain.StateActive, gold, true)
	e.change(t, id, ref, func(s *supporters.SubscriptionSnapshot) {
		s.Price, s.ScheduledPrice = basic.Prices[domain.IntervalMonth].Ref, ""
	})
	v = e.expect(t, id, domain.StateActive, basic, true)
	if e.holds(t, alice, gold.RoleID) {
		t.Error("the role of the higher plan stayed after the downgrade")
	}
	// A change needs an active subscription.
	if _, err := e.svc.Cancel(ctx, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ChangePlan(ctx, alice, gold.ID, domain.IntervalMonth); !errors.Is(err, supporters.ErrSubscriptionState) {
		t.Errorf("a change while canceling: %v", err)
	}
}

// supporterSnapshotOrder applies two snapshots read at the same time in the wrong order: the one
// read first, with the older state, does not replace the newer one.
func supporterSnapshotOrder(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	basic := e.plan(t, "basic", 1, 10, 500)
	gold := e.plan(t, "gold", 2, 20, 1000)
	alice := e.user("alice")
	id, ref := e.subscribe(t, alice, basic, domain.IntervalMonth)
	older, ok := e.world.Subscription(ref)
	if !ok {
		t.Fatalf("no subscription %s at the provider", ref)
	}
	e.clock.Advance(time.Minute)
	older.ReadAt = e.clock.Now()
	newer := older
	newer.Price = gold.Prices[domain.IntervalMonth].Ref
	newer.ReadAt = e.clock.Now().Add(time.Second)
	e.clock.Advance(time.Minute)
	if _, err := e.svc.SyncSubscription(ctx, paymentProvider, newer); err != nil {
		t.Fatal(err)
	}
	if changed, err := e.svc.SyncSubscription(ctx, paymentProvider, older); err != nil || changed {
		t.Fatalf("SyncSubscription of the older snapshot = %v, %v; want no change", changed, err)
	}
	if v := e.expect(t, id, domain.StateActive, gold, true); v.Current.PlanID != gold.ID {
		t.Fatalf("plan after the older snapshot = %s, want gold", v.Current.PlanID)
	}
	// A snapshot read later applies.
	later := older
	later.ReadAt = e.clock.Now()
	if changed, err := e.svc.SyncSubscription(ctx, paymentProvider, later); err != nil || !changed {
		t.Fatalf("SyncSubscription of a later snapshot = %v, %v; want a change", changed, err)
	}
	e.expect(t, id, domain.StateActive, basic, true)
}

// changeRecorder is a provider that records the keys of ChangePrice and fails the next call
// while fail is set, as a change that failed part of the way.
type changeRecorder struct {
	supporters.PaymentProvider
	keys []string
	fail bool
}

func (r *changeRecorder) ChangePrice(ctx context.Context, ref supporters.SubscriptionRef, to supporters.PriceRef, when supporters.ChangeTiming, key string) error {
	r.keys = append(r.keys, key)
	if r.fail {
		r.fail = false
		return fmt.Errorf("%w: the plan was not switched", supporters.ErrPaymentUnavailable)
	}
	return r.PaymentProvider.ChangePrice(ctx, ref, to, when, key)
}

// supporterChangeRetry sends a change again with the key of its failed attempt, so that the
// provider charges the difference once, and another change with another key.
func supporterChangeRetry(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	basic := e.plan(t, "basic", 1, 10, 500)
	gold := e.plan(t, "gold", 2, 20, 1000)
	alice := e.user("alice")
	id, _ := e.subscribe(t, alice, basic, domain.IntervalMonth)
	rec := &changeRecorder{PaymentProvider: e.world, fail: true}
	deps := e.deps
	deps.Provider = rec
	svc := supporters.NewService(deps)
	if _, err := svc.ChangePlan(ctx, alice, gold.ID, domain.IntervalMonth); !errors.Is(err, supporters.ErrPaymentUnavailable) {
		t.Fatalf("ChangePlan with a failure = %v", err)
	}
	if _, err := svc.ChangePlan(ctx, alice, gold.ID, domain.IntervalMonth); err != nil {
		t.Fatalf("ChangePlan again = %v", err)
	}
	if len(rec.keys) != 2 || rec.keys[0] != rec.keys[1] || rec.keys[0] == "" {
		t.Fatalf("keys of the attempts of one change = %q, want the same key twice", rec.keys)
	}
	e.expect(t, id, domain.StateActive, gold, true)
	if _, err := svc.ChangePlan(ctx, alice, gold.ID, domain.IntervalYear); err != nil {
		t.Fatal(err)
	}
	if len(rec.keys) != 3 || rec.keys[2] == rec.keys[0] {
		t.Errorf("key of another change = %q, want a new one", rec.keys)
	}
}

func supporterAbandon(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	plan := e.plan(t, "supporter", 1, 10, 500)
	alice := e.user("alice")
	co, err := e.svc.StartCheckout(ctx, supporters.CheckoutInput{UserID: alice, PlanID: plan.ID, Interval: domain.IntervalMonth})
	if err != nil || co.RedirectURL == "" {
		t.Fatalf("StartCheckout = %+v, %v", co, err)
	}
	// Starting again while pending makes a new checkout of the same subscription.
	again, err := e.svc.StartCheckout(ctx, supporters.CheckoutInput{UserID: alice, PlanID: plan.ID, Interval: domain.IntervalMonth})
	if err != nil || again.SubscriptionID != co.SubscriptionID {
		t.Fatalf("StartCheckout again = %+v, %v", again, err)
	}
	e.expect(t, co.SubscriptionID, domain.StatePending, plan, false)
	if refs, _ := e.svc.OpenCheckouts(ctx, paymentProvider, e.clock.Now()); len(refs) != 1 {
		t.Errorf("OpenCheckouts = %v", refs)
	}
	e.clock.Advance(domain.CheckoutTimeout)
	if err := e.svc.QueueDeadlines(ctx); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	e.expect(t, co.SubscriptionID, domain.StateAbandoned, plan, false)
	if _, err := e.svc.StartCheckout(ctx, supporters.CheckoutInput{UserID: alice, PlanID: plan.ID, Interval: domain.IntervalMonth}); err != nil {
		t.Errorf("StartCheckout after an abandoned one: %v", err)
	}
}

func supporterEligibility(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	plan := e.plan(t, "supporter", 1, 10, 500)
	start := func(userID string) error {
		_, err := e.svc.StartCheckout(ctx, supporters.CheckoutInput{UserID: userID, PlanID: plan.ID, Interval: domain.IntervalMonth})
		return err
	}
	reason := func(err error) supporters.Eligibility {
		var ie *supporters.IneligibleError
		if errors.As(err, &ie) {
			return ie.Reason
		}
		return ""
	}
	if got := reason(start(NamedID("nobody"))); got != supporters.IneligibleNotLocal {
		t.Errorf("an unknown user: %s", got)
	}
	e.users.put(supporters.User{ID: NamedID("unverified"), Handle: "@u", Email: "u@example.com"})
	if got := reason(start(NamedID("unverified"))); got != supporters.IneligibleEmail {
		t.Errorf("an unverified address: %s", got)
	}
	e.users.put(supporters.User{ID: NamedID("suspended"), Handle: "@s", Email: "s@example.com", EmailVerified: true, Suspended: true})
	if got := reason(start(NamedID("suspended"))); got != supporters.IneligibleRestricted {
		t.Errorf("a suspended account: %s", got)
	}
	alice := e.user("alice")
	e.subscribe(t, alice, plan, domain.IntervalMonth)
	if err := start(alice); !errors.Is(err, supporters.ErrLiveSubscription) {
		t.Errorf("a second subscription: %v", err)
	}
}

func supporterPrices(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	plan := e.plan(t, "supporter", 1, 10, 500)
	if y := plan.Prices[domain.IntervalYear]; y.AmountMinor != 5400 {
		t.Fatalf("the yearly price: %d", y.AmountMinor)
	}
	alice := e.user("alice")
	id, _ := e.subscribe(t, alice, plan, domain.IntervalMonth)
	monthly := int64(800)
	revised, err := e.svc.RevisePlan(ctx, supporters.PlanUpdate{ID: plan.ID, MonthlyAmountMinor: &monthly})
	if err != nil || revised.Ready() {
		t.Fatalf("RevisePlan = ready %v, %v", revised.Ready(), err)
	}
	e.run(t)
	now := e.planOf(t, plan.ID)
	if !now.Ready() || now.Prices[domain.IntervalMonth].AmountMinor != 800 || now.Prices[domain.IntervalYear].AmountMinor != 8640 {
		t.Fatalf("the new prices: %+v", now.Prices)
	}
	// The subscription keeps its price (V1); a new one takes the new one.
	e.sync(t, id)
	if v := e.expect(t, id, domain.StateActive, plan, true); v.Price.AmountMinor != 500 {
		t.Errorf("the subscription's price: %d", v.Price.AmountMinor)
	}
	bobID, _ := e.subscribe(t, e.user("bob"), now, domain.IntervalMonth)
	if v := e.sub(t, bobID); v.Price.AmountMinor != 800 {
		t.Errorf("the new subscription's price: %d", v.Price.AmountMinor)
	}
	if !slices.Contains(e.actions(), moderation.AuditSupporterPlanRevised) {
		t.Errorf("audit log: %v", e.actions())
	}
}

func supporterRetire(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	plan := e.plan(t, "supporter", 1, 10, 500)
	alice, bob := e.user("alice"), e.user("bob")
	id, ref := e.subscribe(t, alice, plan, domain.IntervalMonth)
	co, err := e.svc.StartCheckout(ctx, supporters.CheckoutInput{UserID: bob, PlanID: plan.ID, Interval: domain.IntervalMonth})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RetirePlan(ctx, plan.ID, ""); err != nil {
		t.Fatalf("RetirePlan = %v", err)
	}
	e.run(t)
	if err := e.svc.RetirePlan(ctx, plan.ID, ""); !errors.Is(err, supporters.ErrPlanRetired) {
		t.Errorf("RetirePlan again = %v", err)
	}
	e.expect(t, co.SubscriptionID, domain.StateAbandoned, plan, false)
	v := e.expect(t, id, domain.StateCanceling, plan, true)
	if v.Current.Reason != domain.ReasonPlanRetired {
		t.Errorf("reason %s", v.Current.Reason)
	}
	if offered, _ := e.svc.OfferedPlans(ctx); len(offered) != 0 {
		t.Errorf("a retired plan is offered: %v", offered)
	}
	if !e.managed(t, plan.RoleID) {
		t.Fatal("the role is not managed while held")
	}
	e.clock.Advance(31 * 24 * time.Hour)
	e.change(t, id, ref, func(s *supporters.SubscriptionSnapshot) { s.Status = supporters.StatusCanceled })
	e.expect(t, id, domain.StateExpired, plan, false)
	if e.managed(t, plan.RoleID) {
		t.Error("the role stays managed after the last subscription ended")
	}
	if err := e.identity.DeleteRole(ctx, "supporter", ""); err != nil {
		t.Errorf("DeleteRole after the plan retired = %v", err)
	}
	five := 5
	if _, err := e.svc.RevisePlan(ctx, supporters.PlanUpdate{ID: plan.ID, Rank: &five}); !errors.Is(err, supporters.ErrPlanRetired) {
		t.Errorf("RevisePlan of a retired plan = %v", err)
	}
}

func supporterStaffCancel(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	plan := e.plan(t, "supporter", 1, 10, 500)
	id, ref := e.subscribe(t, e.user("alice"), plan, domain.IntervalMonth)
	if err := e.svc.CancelByStaff(ctx, id, ""); err != nil {
		t.Fatalf("CancelByStaff = %v", err)
	}
	e.run(t)
	v := e.expect(t, id, domain.StateExpired, plan, false)
	if v.Current.Reason != domain.ReasonStaff {
		t.Errorf("reason %s", v.Current.Reason)
	}
	if s, _ := e.world.Subscription(ref); s.Status != supporters.StatusCanceled {
		t.Errorf("the provider's subscription: %s", s.Status)
	}
	if err := e.svc.CancelByStaff(ctx, id, ""); !errors.Is(err, supporters.ErrNoLiveSubscription) {
		t.Errorf("CancelByStaff again = %v", err)
	}
	if !slices.Contains(e.actions(), moderation.AuditSupporterSubscriptionCanceled) {
		t.Errorf("audit log: %v", e.actions())
	}
	page, err := e.svc.Subscriptions(ctx, supporters.SubscriptionFilter{State: domain.StateExpired, Limit: 10})
	if err != nil || len(page) != 1 || page[0].ID != id {
		t.Errorf("Subscriptions(expired) = %v, %v", page, err)
	}
	if page, _ := e.svc.Subscriptions(ctx, supporters.SubscriptionFilter{State: domain.StateActive, Limit: 10}); len(page) != 0 {
		t.Errorf("Subscriptions(active) = %v", page)
	}
}

func supporterRebuild(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	basic := e.plan(t, "basic", 1, 10, 500)
	gold := e.plan(t, "gold", 2, 20, 1000)
	ida, _ := e.subscribe(t, e.user("alice"), basic, domain.IntervalMonth)
	idb, refb := e.subscribe(t, e.user("bob"), gold, domain.IntervalYear)
	failed := e.clock.Now()
	e.change(t, idb, refb, func(s *supporters.SubscriptionSnapshot) {
		s.Status, s.LatestPaymentFailed = supporters.StatusPastDue, failed
	})
	co, err := e.svc.StartCheckout(ctx, supporters.CheckoutInput{UserID: e.user("carol"), PlanID: basic.ID, Interval: domain.IntervalMonth})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RetirePlan(ctx, gold.ID, ""); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	snapshot := func() []any {
		plans, err := e.svc.Plans(ctx)
		if err != nil {
			t.Fatal(err)
		}
		subs, err := e.svc.Subscriptions(ctx, supporters.SubscriptionFilter{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		due, err := e.h.Supporters.DueDeadlines(ctx, e.clock.Now().Add(365*24*time.Hour), 10)
		if err != nil {
			t.Fatal(err)
		}
		live, err := e.h.Supporters.CountLiveSubscriptions(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		var liveIDs []string
		if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			liveIDs, err = e.h.Supporters.LiveSubscriptionIDs(ctx, tx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if int64(len(liveIDs)) != live {
			t.Errorf("LiveSubscriptionIDs = %v, want the %d of CountLiveSubscriptions", liveIDs, live)
		}
		return []any{plans, subs, due, live, liveIDs}
	}
	before := snapshot()
	if err := supporters.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Supporters); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); !reflect.DeepEqual(before, after) {
		t.Errorf("the projections differ after the rebuild:\nbefore %+v\nafter  %+v", before, after)
	}
	_, _ = ida, co
}

// supporterListings checks the supporter list (ADR-0071 §4, §6): only a member chooses to be
// listed; the list shows the listed members of the plans that are not retired, by the plan's
// rank and newest first, page by page; and the end of a subscription takes its member off.
func supporterListings(t *testing.T, e *supporterEnv) {
	ctx := t.Context()
	e.enable(t)
	small, large := e.plan(t, "small", 1, 10, 500), e.plan(t, "large", 2, 11, 1000)
	alice, bob, carol, dave := e.user("alice"), e.user("bob"), e.user("carol"), e.user("dave")
	names := map[string]string{alice: "alice", bob: "bob", carol: "carol"}
	e.subscribe(t, alice, small, domain.IntervalMonth)
	e.subscribe(t, bob, small, domain.IntervalYear)
	carolID, carolRef := e.subscribe(t, carol, large, domain.IntervalMonth)
	if _, err := e.svc.StartCheckout(ctx, supporters.CheckoutInput{UserID: dave, PlanID: small.ID, Interval: domain.IntervalMonth}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{dave, NamedID("nobody")} {
		if _, err := e.svc.SetListed(ctx, id, true); !errors.Is(err, supporters.ErrNotMember) {
			t.Errorf("SetListed(%s) = %v, want ErrNotMember", id, err)
		}
		if _, err := e.svc.Listed(ctx, id); !errors.Is(err, supporters.ErrNotMember) {
			t.Errorf("Listed(%s) = %v, want ErrNotMember", id, err)
		}
	}
	if listed, err := e.svc.Listed(ctx, alice); err != nil || listed {
		t.Fatalf("Listed before choosing = %v, %v; nobody is listed by default", listed, err)
	}
	for _, id := range []string{alice, bob, carol, alice} {
		if listed, err := e.svc.SetListed(ctx, id, true); err != nil || !listed {
			t.Fatalf("SetListed(%s) = %v, %v", id, listed, err)
		}
		e.clock.Advance(time.Second)
	}
	page := func(f supporters.ListingFilter) ([]string, *supporters.Listing) {
		t.Helper()
		l, err := e.svc.SupporterList(ctx, f)
		if err != nil {
			t.Fatalf("SupporterList(%+v) = %v", f, err)
		}
		var out []string
		for _, m := range l.Members {
			if want := NamedID("acct-" + names[m.UserID]); m.AccountID != want {
				t.Errorf("the account of %s: %q, want %q", m.UserID, m.AccountID, want)
			}
			out = append(out, m.UserID)
		}
		return out, l.Last
	}
	if got, last := page(supporters.ListingFilter{Limit: 10}); !reflect.DeepEqual(got, []string{bob, alice, carol}) || last != nil {
		t.Errorf("the list: %v (last %v), want bob and alice of the small plan, newest first, then carol", got, last)
	}
	var walked []string
	var after *supporters.Listing
	for range 4 {
		got, last := page(supporters.ListingFilter{After: after, Limit: 1})
		walked = append(walked, got...)
		if last == nil {
			break
		}
		after = last
	}
	if !reflect.DeepEqual(walked, []string{bob, alice, carol}) {
		t.Errorf("the pages: %v", walked)
	}
	if got, _ := page(supporters.ListingFilter{PlanID: large.ID, Limit: 10}); !reflect.DeepEqual(got, []string{carol}) {
		t.Errorf("the large plan: %v", got)
	}
	if l, _ := e.svc.SupporterList(ctx, supporters.ListingFilter{Limit: 10}); len(l.Plans) != 2 || l.Plans[0].ID != small.ID || l.Plans[1].ID != large.ID {
		t.Errorf("the plans of the list: %+v", l.Plans)
	}
	if listed, err := e.svc.SetListed(ctx, bob, false); err != nil || listed {
		t.Fatalf("SetListed(bob, false) = %v, %v", listed, err)
	}
	if got, _ := page(supporters.ListingFilter{Limit: 10}); !reflect.DeepEqual(got, []string{alice, carol}) {
		t.Errorf("after bob left the list: %v", got)
	}
	// The end of a subscription takes its member off the list for good.
	if _, err := e.svc.Cancel(ctx, carol); err != nil {
		t.Fatal(err)
	}
	if got, _ := page(supporters.ListingFilter{Limit: 10}); !reflect.DeepEqual(got, []string{alice, carol}) {
		t.Errorf("a canceling member is a member until the period's end: %v", got)
	}
	e.clock.Advance(31 * 24 * time.Hour)
	e.change(t, carolID, carolRef, func(s *supporters.SubscriptionSnapshot) { s.Status = supporters.StatusCanceled })
	if got, _ := page(supporters.ListingFilter{Limit: 10}); !reflect.DeepEqual(got, []string{alice}) {
		t.Errorf("after carol's subscription ended: %v", got)
	}
	if listed, err := e.h.Supporters.HasListing(ctx, nil, carol); err != nil || listed {
		t.Errorf("carol's choice stays after the end: %v, %v", listed, err)
	}
	// A retired plan leaves the list with its members, who stay listed until the period's end.
	if err := e.svc.RetirePlan(ctx, small.ID, ""); err != nil {
		t.Fatal(err)
	}
	e.run(t)
	l, err := e.svc.SupporterList(ctx, supporters.ListingFilter{Limit: 10})
	if err != nil || len(l.Members) != 0 || len(l.Plans) != 1 || l.Plans[0].ID != large.ID {
		t.Errorf("the list after the small plan retired: %+v, %v", l, err)
	}
}
