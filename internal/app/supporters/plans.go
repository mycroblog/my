// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/supporters"
)

// The plans (ADR-0072 §3, §4, §5, §9): an administrator with manage_supporters creates a plan
// on a role that nobody holds, which becomes managed; changes its terms and its role's
// permissions and position from the plan form; hides it; and retires it. Every change keeps the
// higher plans above the lower ones, and every change of the terms queues JobEnsurePrices, which
// makes the prices at the provider: a plan takes subscriptions once it has both.

// PermCreateCustomEmojis is the member permission that lets its holders create custom emoji
// (ADR-0070 §1, §4), whose limit a plan sets: the identity module's PermCreateCustomEmojis.
const PermCreateCustomEmojis = "create_custom_emojis"

// JobEnsurePrices makes the prices of a plan's current terms at the provider and archives the
// ones it no longer uses. Its idempotency key is "ensure_prices:<revision ID>".
const JobEnsurePrices = "supporters.ensure_prices"

// The errors of the plans.
var (
	// ErrPlanRetired: the plan is retired; it cannot change.
	ErrPlanRetired = errors.New("supporters: the plan is retired")
	// ErrPlanUnavailable: the plan is hidden, retired, or has no prices yet; it takes no new
	// subscriptions nor changes to it.
	ErrPlanUnavailable = errors.New("supporters: the plan takes no subscriptions")
)

// The errors of the plans from the domain.
var (
	ErrInvalidPlan     = supporters.ErrInvalidPlan
	ErrTiersOutOfOrder = supporters.ErrTiersOutOfOrder
)

// PlanView is a plan as the administrators and the users see it.
type PlanView struct {
	PlanRecord
	// Role is the plan's role; its ID is PlanRecord.RoleID.
	Role appidentity.RoleWithPermissions
	// Members counts the subscriptions of the plan in a member state.
	Members int64
}

// Ready reports whether the plan has the prices of its current terms, which new subscriptions
// need (ADR-0072 §3: "準備中" until then).
func (p PlanRecord) Ready() bool {
	for _, i := range supporters.Intervals() {
		price, ok := p.Prices[i]
		if !ok || price.AmountMinor != p.Revision.Amount(i) {
			return false
		}
	}
	return true
}

// Offered reports whether the plan takes new subscriptions and changes to it: public, not
// retired, and ready.
func (p PlanRecord) Offered() bool {
	return !p.Retired && p.Revision.Visibility == supporters.VisibilityPublic && p.Ready()
}

// Plans returns every plan, retired ones too, by rank, with its role and its member count.
func (s *Service) Plans(ctx context.Context) ([]PlanView, error) {
	plans, err := s.d.Repository.ListPlans(ctx, nil)
	if err != nil {
		return nil, err
	}
	roles, err := s.d.Roles.Roles(ctx)
	if err != nil {
		return nil, err
	}
	members, err := s.d.Repository.CountMembers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PlanView, len(plans))
	for i, p := range plans {
		out[i] = PlanView{PlanRecord: p, Members: members[p.ID]}
		if j := slices.IndexFunc(roles, func(r appidentity.RoleWithPermissions) bool { return r.ID == p.RoleID }); j >= 0 {
			out[i].Role = roles[j]
		}
	}
	return out, nil
}

// Plan returns one plan as Plans does.
func (s *Service) Plan(ctx context.Context, id string) (PlanView, error) {
	plans, err := s.Plans(ctx)
	if err != nil {
		return PlanView{}, err
	}
	for _, p := range plans {
		if p.ID == id {
			return p, nil
		}
	}
	return PlanView{}, fmt.Errorf("%w: plan %s", ErrNotFound, id)
}

// OfferedPlans returns the plans that take subscriptions, by rank (GET /instance/supporter_plans,
// ADR-0072 §18); ErrDisabled while the program is disabled.
func (s *Service) OfferedPlans(ctx context.Context) ([]PlanView, error) {
	enabled, err := s.Enabled(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, ErrDisabled
	}
	plans, err := s.Plans(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(plans, func(p PlanView) bool { return !p.Offered() }), nil
}

// PlanInput is a new plan.
type PlanInput struct {
	// RoleKey is the role of the plan (ADR-0072 §5).
	RoleKey  string
	Currency string
	Terms    supporters.PlanTerms
	// ByUserID is the administrator, "" for the CLI.
	ByUserID string
}

// checkAmounts checks the monthly and yearly prices of terms against the provider's minimum
// charge in currency; a currency the provider does not take is a PlanError of the currency.
func (s *Service) checkAmounts(currency string, terms supporters.PlanTerms) error {
	minimum, ok := s.d.Provider.MinimumAmount(currency)
	if !ok {
		return &supporters.PlanError{Field: supporters.FieldCurrency}
	}
	for _, i := range supporters.Intervals() {
		if terms.Amount(i) < minimum {
			return &supporters.PlanError{Field: supporters.FieldAmount}
		}
	}
	return nil
}

// CreatePlan creates a plan and returns it (ADR-0072 §3, §5). It fails with ErrNoProvider
// without a provider; with a *supporters.PlanError for terms that break a rule, a currency the
// provider does not take, or a price below its minimum; with ErrNotFound for an unknown role;
// with an *appidentity.UnmanageableRoleError for a role that a plan cannot have; with
// ErrPlanRoleTaken for the role of another plan; and with a *supporters.TierError when the
// plan breaks the order of the plans (a rank of another plan included). Audited.
func (s *Service) CreatePlan(ctx context.Context, in PlanInput) (PlanView, error) {
	if s.d.Provider == nil {
		return PlanView{}, ErrNoProvider
	}
	if err := supporters.ValidateCurrency(in.Currency); err != nil {
		return PlanView{}, err
	}
	if err := in.Terms.Validate(); err != nil {
		return PlanView{}, err
	}
	if err := s.checkAmounts(in.Currency, in.Terms); err != nil {
		return PlanView{}, err
	}
	now := s.now()
	plan := supporters.Plan{ID: s.d.IDs.NewID(), Currency: in.Currency, CreatedByUserID: in.ByUserID, CreatedAt: now}
	rev := supporters.PlanRevision{ID: s.d.IDs.NewID(), PlanID: plan.ID, PlanTerms: in.Terms, RevisedByUserID: in.ByUserID, RevisedAt: now}
	auditID := s.d.IDs.NewID()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		role, err := s.d.Roles.ManageRole(ctx, tx, in.RoleKey, appidentity.ManagerSupporters, now)
		if err != nil {
			return err
		}
		plan.RoleID = role.ID
		if err := s.d.Repository.InsertPlan(ctx, tx, plan); err != nil {
			return err
		}
		if err := s.insertRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := s.checkTiers(ctx, tx); err != nil {
			return err
		}
		if err := s.d.Outbox.EnqueueTx(ctx, tx, ensurePricesJob(plan.ID, rev.ID)); err != nil {
			return err
		}
		return tx.Emit(ctx, audit(auditID, in.ByUserID, appmoderation.AuditSupporterPlanCreated, appmoderation.AuditTargetSupporterPlan,
			plan.ID, rev.Name, rev.ID, now))
	})
	if err != nil {
		return PlanView{}, err
	}
	return s.Plan(ctx, plan.ID)
}

// insertRevision records a revision; a rank that another plan has breaks the order of the plans.
func (s *Service) insertRevision(ctx context.Context, tx uow.Tx, rev supporters.PlanRevision) error {
	err := s.d.Repository.InsertPlanRevision(ctx, tx, rev)
	if errors.Is(err, ErrRankTaken) {
		return &supporters.TierError{Rule: supporters.TierRank, Lower: rev.PlanID, Higher: rev.PlanID}
	}
	return err
}

// checkTiers checks inside tx that the plans that are not retired keep their order (§4).
func (s *Service) checkTiers(ctx context.Context, tx uow.Tx) error {
	plans, err := s.d.Repository.ListPlans(ctx, tx)
	if err != nil {
		return err
	}
	roles, err := s.d.Roles.RolesTx(ctx, tx)
	if err != nil {
		return err
	}
	var tiers []supporters.Tier
	for _, p := range plans {
		if p.Retired {
			continue
		}
		t := supporters.Tier{PlanID: p.ID, Rank: p.Revision.Rank, EmojiLimit: p.Revision.EmojiLimit}
		if i := slices.IndexFunc(roles, func(r appidentity.ManagedRoleView) bool { return r.ID == p.RoleID }); i >= 0 {
			t.Position = roles[i].Position
			for _, perm := range roles[i].Permissions {
				t.Permissions = append(t.Permissions, string(perm))
			}
			t.CreatesEmojis = slices.Contains(t.Permissions, PermCreateCustomEmojis)
		}
		tiers = append(tiers, t)
	}
	return supporters.CheckTiers(tiers)
}

// PlanUpdate changes a plan: each field that is not nil changes, and RolePermissions changes
// when SetRolePermissions is true.
type PlanUpdate struct {
	ID                    string
	Name, Description     *string
	MonthlyAmountMinor    *int64
	AnnualDiscountPercent *int
	EmojiLimit            *int
	Rank                  *int
	Visibility            *supporters.Visibility
	// RolePosition and RolePermissions change the plan's role (ADR-0072 §5).
	RolePosition       *int
	RolePermissions    []appidentity.Permission
	SetRolePermissions bool
	ByUserID           string
}

// RevisePlan changes a plan and returns it (ADR-0072 §3, §9): new terms are a new revision
// (and new prices for a new amount, which the subscriptions on the old ones keep, V1), and a
// change of the role goes through the identity module, which keeps it to member permissions
// below every staff role. It fails with ErrNotFound for an unknown plan, with ErrPlanRetired for
// a retired one, and as CreatePlan does for the terms, the role, and the order. A change to what
// the plan has records nothing. Audited.
func (s *Service) RevisePlan(ctx context.Context, in PlanUpdate) (PlanView, error) {
	if s.d.Provider == nil {
		return PlanView{}, ErrNoProvider
	}
	now := s.now()
	revID, auditID := s.d.IDs.NewID(), s.d.IDs.NewID()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		p, err := s.d.Repository.FindPlan(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if p.Retired {
			return ErrPlanRetired
		}
		terms := p.Revision.PlanTerms
		set := func(dst *string, v *string) {
			if v != nil {
				*dst = *v
			}
		}
		set(&terms.Name, in.Name)
		set(&terms.Description, in.Description)
		if in.MonthlyAmountMinor != nil {
			terms.MonthlyAmountMinor = *in.MonthlyAmountMinor
		}
		for _, f := range []struct {
			dst *int
			v   *int
		}{{&terms.AnnualDiscountPercent, in.AnnualDiscountPercent}, {&terms.EmojiLimit, in.EmojiLimit}, {&terms.Rank, in.Rank}} {
			if f.v != nil {
				*f.dst = *f.v
			}
		}
		if in.Visibility != nil {
			terms.Visibility = *in.Visibility
		}
		if err := terms.Validate(); err != nil {
			return err
		}
		if err := s.checkAmounts(p.Currency, terms); err != nil {
			return err
		}
		changed := false
		if terms != p.Revision.PlanTerms {
			rev := supporters.PlanRevision{ID: revID, PlanID: p.ID, PlanTerms: terms, RevisedByUserID: in.ByUserID, RevisedAt: now}
			if err := s.insertRevision(ctx, tx, rev); err != nil {
				return err
			}
			if err := s.d.Outbox.EnqueueTx(ctx, tx, ensurePricesJob(p.ID, rev.ID)); err != nil {
				return err
			}
			changed = true
		}
		if in.RolePosition != nil || in.SetRolePermissions {
			_, roleChanged, err := s.d.Roles.UpdateManagedRole(ctx, tx, appidentity.ManagedRoleUpdate{
				RoleID: p.RoleID, Position: in.RolePosition, Permissions: in.RolePermissions, SetPermissions: in.SetRolePermissions,
				ByUserID: in.ByUserID, At: now,
			})
			if err != nil {
				return err
			}
			changed = changed || roleChanged
		}
		if !changed {
			return nil
		}
		if err := s.checkTiers(ctx, tx); err != nil {
			return err
		}
		return tx.Emit(ctx, audit(auditID, in.ByUserID, appmoderation.AuditSupporterPlanRevised, appmoderation.AuditTargetSupporterPlan,
			p.ID, terms.Name, revID, now))
	})
	if err != nil {
		return PlanView{}, err
	}
	return s.Plan(ctx, in.ID)
}

// RetirePlan retires a plan for good (ADR-0072 §9): it leaves the offer and the lists, and
// every live subscription of it ends: a pending one at once, a lapsed one at once at the
// provider, and the others at the end of their period (their scheduled change withdrawn), when
// the role goes. The role becomes an ordinary one again once nobody holds it. PlanRetired tells
// the subscribers who had paid (ADR-0072 §16). It fails with
// ErrNotFound for an unknown plan and with ErrPlanRetired for a retired one. Audited.
func (s *Service) RetirePlan(ctx context.Context, id, byUserID string) error {
	now := s.now()
	auditID := s.d.IDs.NewID()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		p, err := s.d.Repository.FindPlan(ctx, tx, id)
		if err != nil {
			return err
		}
		if p.Retired {
			return ErrPlanRetired
		}
		if err := s.d.Repository.InsertPlanRetirement(ctx, tx, id, byUserID, now); err != nil {
			return err
		}
		subs, err := s.d.Repository.LiveSubscriptionIDsOfPlan(ctx, tx, id)
		if err != nil {
			return err
		}
		retired := PlanRetired{PlanID: id, PlanName: p.Revision.Name, At: now}
		for _, subID := range subs {
			rec, err := s.d.Repository.LockSubscription(ctx, tx, subID)
			if err != nil {
				return err
			}
			if rec.Current.State != supporters.StatePending {
				accountID, err := s.accountOf(ctx, tx, rec.UserID)
				if err != nil {
					return err
				}
				retired.Subscriptions = append(retired.Subscriptions, RetiredSubscription{SubscriptionID: rec.ID, UserID: rec.UserID, AccountID: accountID})
			}
			switch rec.Current.State {
			case supporters.StatePending:
				if err := s.close(ctx, tx, rec, supporters.ReasonPlanRetired, now); err != nil {
					return err
				}
			case supporters.StateLapsed:
				if err := s.enqueueCommand(ctx, tx, rec.ID, CommandCancelNow, supporters.ReasonPlanRetired, "retire:"+id); err != nil {
					return err
				}
			default:
				if err := s.enqueueCommand(ctx, tx, rec.ID, CommandCancelAtPeriodEnd, supporters.ReasonPlanRetired, "retire:"+id); err != nil {
					return err
				}
			}
		}
		if _, err := s.d.Roles.UnmanageRole(ctx, tx, p.RoleID); err != nil {
			return err
		}
		if err := tx.Emit(ctx, retired); err != nil {
			return err
		}
		return tx.Emit(ctx, audit(auditID, byUserID, appmoderation.AuditSupporterPlanRetired, appmoderation.AuditTargetSupporterPlan,
			id, p.Revision.Name, id, now))
	})
}

// ensurePricesPayload is the payload of JobEnsurePrices.
type ensurePricesPayload struct {
	PlanID string `json:"plan_id"`
}

func ensurePricesJob(planID, revisionID string) queue.NewJob {
	b, err := json.Marshal(ensurePricesPayload{PlanID: planID})
	if err != nil {
		panic(err) // a string always marshals
	}
	return queue.NewJob{Type: JobEnsurePrices, Payload: b, IdempotencyKey: "ensure_prices:" + revisionID, OrderingKey: "supporter_plan:" + planID}
}

// EnsurePrices is the work of JobEnsurePrices: it asks the provider for the monthly and yearly
// prices of the plan's current terms (EnsurePlan, idempotent), records them as the plan's
// current prices, and records the prices the provider archived. A retired plan needs nothing.
func (s *Service) EnsurePrices(ctx context.Context, planID string) queue.Result {
	if s.d.Provider == nil {
		return queue.Done()
	}
	p, err := s.d.Repository.FindPlan(ctx, nil, planID)
	switch {
	case errors.Is(err, ErrNotFound):
		return queue.Done()
	case err != nil:
		return queue.Retry(0, err)
	case p.Retired:
		return queue.Done()
	}
	spec := PlanSpec{PlanID: p.ID, Name: p.Revision.Name, Description: p.Revision.Description}
	for _, i := range supporters.Intervals() {
		spec.Prices = append(spec.Prices, PriceSpec{Interval: Interval(i), AmountMinor: p.Revision.Amount(i), Currency: p.Currency})
	}
	refs, err := s.d.Provider.EnsurePlan(ctx, spec)
	if err != nil {
		return s.providerFailure(ctx, "ensure the prices of plan "+planID, err)
	}
	name, now := s.d.Provider.Name(), s.now()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		for _, pp := range refs.Prices {
			price := Price{
				ID: s.d.IDs.NewID(), PlanID: p.ID, Interval: supporters.Interval(pp.Spec.Interval), AmountMinor: pp.Spec.AmountMinor,
				Currency: pp.Spec.Currency, Provider: name, Ref: pp.Ref, CreatedAt: now,
			}
			if err := s.d.Repository.InsertPrice(ctx, tx, price); err != nil {
				return err
			}
			// A price recorded already (EnsurePlan again for the same terms) keeps its row; an
			// amount the plan had before comes back as a new price beside the old one.
			stored, err := s.d.Repository.FindPriceByRef(ctx, tx, name, pp.Ref)
			if err != nil {
				return err
			}
			if err := s.d.Repository.SetPriceHead(ctx, tx, p.ID, stored.Interval, stored.ID); err != nil {
				return err
			}
		}
		for _, ref := range refs.Archived {
			old, err := s.d.Repository.FindPriceByRef(ctx, tx, name, ref)
			switch {
			case errors.Is(err, ErrNotFound):
				continue
			case err != nil:
				return err
			}
			if err := s.d.Repository.InsertPriceArchival(ctx, tx, old.ID, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// providerFailure returns the result of a job whose call of the provider failed: a retry for a
// failure that passes, and done with a warning otherwise (the admin screen shows the state).
func (s *Service) providerFailure(ctx context.Context, what string, err error) queue.Result {
	kind := ErrorKindOf(err)
	if kind.Retried() {
		return queue.Retry(0, fmt.Errorf("supporters: %s: %w", what, err))
	}
	s.d.Logger.WarnContext(ctx, "payment provider failed", slog.String("call", what), slog.String("error_kind", string(kind)), slog.Any("error", err))
	return queue.Done()
}
