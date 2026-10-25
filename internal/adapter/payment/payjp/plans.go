// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/mycroblog/my/internal/app/supporters"
)

// planRef returns the ID of the PAY.JP plan of a price of our plan (ADR-0099 §2): a new amount
// is a new plan, since PAY.JP cannot change a plan's amount.
func planRef(planID string, pr supporters.PriceSpec) supporters.PriceRef {
	return supporters.PriceRef(fmt.Sprintf("my_%s_%s_%d", planID, pr.Interval, pr.AmountMinor))
}

// EnsurePlan implements supporters.PaymentProvider. The product is a name of the plans of our
// plan, without an object of PAY.JP; a plan that spec no longer lists is archived by its
// metadata and never deleted, so that its subscriptions keep renewing on it.
func (p *Provider) EnsurePlan(ctx context.Context, spec supporters.PlanSpec) (supporters.PlanRefs, error) {
	if spec.PlanID == "" || spec.Name == "" || len(spec.Prices) == 0 {
		return supporters.PlanRefs{}, fmt.Errorf("payjp: %w: a plan needs an ID, a name, and prices", supporters.ErrPaymentInvalidRequest)
	}
	refs := supporters.PlanRefs{Product: supporters.ProductRef("my_" + spec.PlanID)}
	keep := map[supporters.PriceRef]bool{}
	for _, pr := range spec.Prices {
		if err := checkPrice(pr); err != nil {
			return supporters.PlanRefs{}, err
		}
		ref := planRef(spec.PlanID, pr)
		if err := p.ensurePlanPrice(ctx, spec, pr, ref); err != nil {
			return supporters.PlanRefs{}, err
		}
		keep[ref] = true
		refs.Prices = append(refs.Prices, supporters.PlanPrice{Spec: pr, Ref: ref})
	}
	plans, err := p.listPlans(ctx)
	if err != nil {
		return supporters.PlanRefs{}, err
	}
	for _, pl := range plans {
		ref := supporters.PriceRef(pl.ID)
		if pl.Metadata[metaPlanID] != spec.PlanID || keep[ref] || pl.Metadata[metaArchived] != "" {
			continue
		}
		form := url.Values{"metadata[" + metaArchived + "]": {"1"}}
		if err := p.post(ctx, "archive a plan", "/plans/"+url.PathEscape(pl.ID), form, nil); err != nil {
			return supporters.PlanRefs{}, err
		}
		refs.Archived = append(refs.Archived, ref)
	}
	return refs, nil
}

func checkPrice(pr supporters.PriceSpec) error {
	switch {
	case !strings.EqualFold(pr.Currency, currency):
		return fmt.Errorf("payjp: %w: PAY.JP takes JPY only, not %s", supporters.ErrPaymentInvalidRequest, pr.Currency)
	case pr.AmountMinor < MinimumAmount || pr.AmountMinor > MaximumAmount:
		return fmt.Errorf("payjp: %w: the amount %d is outside %d to %d yen", supporters.ErrPaymentInvalidRequest, pr.AmountMinor, MinimumAmount, MaximumAmount)
	case pr.Interval != supporters.IntervalMonth && pr.Interval != supporters.IntervalYear:
		return fmt.Errorf("payjp: %w: interval %q", supporters.ErrPaymentInvalidRequest, pr.Interval)
	}
	return nil
}

// ensurePlanPrice creates the plan of ref, or renames it and takes it out of the archive.
func (p *Provider) ensurePlanPrice(ctx context.Context, spec supporters.PlanSpec, pr supporters.PriceSpec, ref supporters.PriceRef) error {
	var pl plan
	err := p.get(ctx, "read a plan", "/plans/"+url.PathEscape(string(ref)), nil, &pl)
	switch {
	case errors.Is(err, supporters.ErrPaymentNotFound):
		form := url.Values{
			"id": {string(ref)}, "amount": {strconv.FormatInt(pr.AmountMinor, 10)}, "currency": {currency},
			"interval": {string(pr.Interval)}, "name": {spec.Name}, "metadata[" + metaPlanID + "]": {spec.PlanID},
		}
		return p.post(ctx, "create a plan", "/plans", form, nil)
	case err != nil:
		return err
	}
	form := url.Values{}
	if pl.Name != spec.Name {
		form.Set("name", spec.Name)
	}
	if pl.Metadata[metaArchived] != "" {
		form.Set("metadata["+metaArchived+"]", "")
	}
	if len(form) == 0 {
		return nil
	}
	return p.post(ctx, "update a plan", "/plans/"+url.PathEscape(string(ref)), form, nil)
}

// listPlans reads every plan of the account.
func (p *Provider) listPlans(ctx context.Context) ([]plan, error) {
	var out []plan
	for offset := 0; ; offset += supporters.PageSize {
		var page list[plan]
		q := url.Values{"limit": {strconv.Itoa(supporters.PageSize)}, "offset": {strconv.Itoa(offset)}}
		if err := p.get(ctx, "list the plans", "/plans", q, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Data...)
		if !page.HasMore || len(page.Data) == 0 {
			return out, nil
		}
	}
}

// getPlan reads a plan.
func (p *Provider) getPlan(ctx context.Context, ref supporters.PriceRef) (plan, error) {
	var pl plan
	err := p.get(ctx, "read a plan", "/plans/"+url.PathEscape(string(ref)), nil, &pl)
	return pl, err
}
