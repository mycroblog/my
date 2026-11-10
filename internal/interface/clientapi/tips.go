// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
	appsupporters "github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The tips (ADR-0162 §17, FR-918): the public amounts (GET /instance/tip_amounts), the users' own
// tips (/supporter/tips, tag supporters), and the administrators' settings
// (/admin/supporter_tips, manage_supporters).

// The problem types of the tips.
const (
	ProblemTypeSupporterTipAmount  = "/api/v1/problems/supporter-tip-amount"
	ProblemTypeSupporterTipPending = "/api/v1/problems/supporter-tip-pending"
	ProblemTypeTipsUnavailable     = "/api/v1/problems/tips-unavailable"
	ProblemTypeInvalidTipAmounts   = "/api/v1/problems/invalid-tip-amounts"
)

// supporterTipReturnPath is the screen of the web client that the checkout of a tip returns to.
const supporterTipReturnPath = "/settings/supporter/tip"

// Tips runs the tips: *appsupporters.Service implements it.
type Tips interface {
	TipsEnabled(ctx context.Context) (bool, error)
	TipAmounts(ctx context.Context) (string, []int64, error)
	TipSettings(ctx context.Context) (appsupporters.TipSettings, error)
	SetTips(ctx context.Context, in appsupporters.TipSettingsInput, byUserID string) (appsupporters.TipSettings, error)
	StartTip(ctx context.Context, in appsupporters.TipInput) (appsupporters.StartedTip, error)
	OwnTip(ctx context.Context, userID, tipID string) (appsupporters.TipView, error)
}

// tips returns the use cases, or the 404 of a server without them.
func (s *Server) tips(ctx context.Context) (Tips, *problem) {
	if s.o.Tips == nil {
		p := s.notFoundResponse(ctx)
		return nil, &p
	}
	return s.o.Tips, nil
}

// tipsEnabled reports whether the server takes tips; false without the use cases.
func (s *Server) tipsEnabled(ctx context.Context) (bool, error) {
	if s.o.Tips == nil {
		return false, nil
	}
	on, err := s.o.Tips.TipsEnabled(ctx)
	if err != nil {
		return false, fmt.Errorf("clientapi: the tips: %w", err)
	}
	return on, nil
}

// tipProblem maps the errors of the tips; it reports false for any other error.
func (s *Server) tipProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	var limited *kernel.RateLimitedError
	switch {
	case errors.Is(err, appsupporters.ErrTipsDisabled):
		return s.notFoundResponse(ctx), true
	case errors.Is(err, appsupporters.ErrTipAmount):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeSupporterTipAmount, l.ServerErrorsSupportersTipAmount(),
			l.ServerErrorsSupportersTipAmountDetail()), true
	case errors.Is(err, appsupporters.ErrOpenTip):
		return s.problem(ctx, http.StatusConflict, ProblemTypeSupporterTipPending, l.ServerErrorsSupportersTipPending(),
			l.ServerErrorsSupportersTipPendingDetail()), true
	case errors.Is(err, appsupporters.ErrTipsUnavailable):
		return s.problem(ctx, http.StatusConflict, ProblemTypeTipsUnavailable, l.ServerErrorsSupportersTipsUnavailable(),
			l.ServerErrorsSupportersTipsUnavailableDetail()), true
	case errors.As(err, &limited):
		retry := seconds(limited.RetryAfter)
		p := s.problem(ctx, http.StatusTooManyRequests, ProblemTypeBlank, l.ServerErrorsRateLimited(), l.ServerErrorsRateLimitedDetail(retry))
		p.Headers.RetryAfter = &retry
		return p, true
	}
	return s.ownSupporterProblem(ctx, err)
}

func tipMoneys(currency string, amounts []int64) []Money {
	out := make([]Money, 0, len(amounts))
	for _, a := range amounts {
		out = append(out, minorMoney(a, currency))
	}
	return out
}

// GetInstanceTipAmounts serves GET /api/v1/instance/tip_amounts.
func (s *Server) GetInstanceTipAmounts(ctx context.Context, _ GetInstanceTipAmountsRequestObject) (GetInstanceTipAmountsResponseObject, error) {
	tips, p := s.tips(ctx)
	if p != nil {
		return GetInstanceTipAmountsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	currency, amounts, err := tips.TipAmounts(ctx)
	if errors.Is(err, appsupporters.ErrTipsDisabled) {
		return GetInstanceTipAmountsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the amounts of the tips: %w", err)
	}
	cache := cachePublic
	if _, ok := ViewerFrom(ctx); ok {
		cache = cachePrivate
	}
	return GetInstanceTipAmounts200JSONResponse{
		Body: TipAmounts{Amounts: tipMoneys(currency, amounts)}, Headers: GetInstanceTipAmounts200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// instanceName returns the display name of the instance, its host without one.
func (s *Server) instanceName(ctx context.Context) (string, error) {
	if s.o.Instance == nil {
		return s.o.Host, nil
	}
	texts, err := s.instanceTexts(ctx, instance.TextName)
	if err != nil {
		return "", err
	}
	if t, ok := texts[instance.TextName]; ok && strings.TrimSpace(t.Body) != "" {
		return t.Body, nil
	}
	return s.o.Host, nil
}

// StartSupporterTip serves POST /api/v1/supporter/tips.
func (s *Server) StartSupporterTip(ctx context.Context, req StartSupporterTipRequestObject) (StartSupporterTipResponseObject, error) {
	tips, p := s.tips(ctx)
	if p != nil {
		return StartSupporterTipdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return StartSupporterTipdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "amount")), nil
	}
	amount, err := finance.ParseMoney(req.Body.Amount.Amount, req.Body.Amount.Currency)
	if err != nil || amount.Minor <= 0 {
		return StartSupporterTipdefaultApplicationProblemPlusJSONResponse(s.unprocessableValue(ctx, "amount")), nil
	}
	name, err := s.instanceName(ctx)
	if err != nil {
		return nil, err
	}
	st, err := tips.StartTip(ctx, appsupporters.TipInput{
		UserID: v.UserID, AmountMinor: amount.Minor, Currency: string(amount.Currency),
		Description: s.Localizer(ctx).ServerPaymentsTipDescription(name), ReturnURL: s.webOrigin() + supporterTipReturnPath,
	})
	if pr, ok := s.tipProblem(ctx, err); ok {
		return StartSupporterTipdefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: start a tip of %s: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return StartSupporterTip200JSONResponse{
		Body: SupporterTipCheckout{TipId: st.TipID, RedirectUrl: st.RedirectURL}, Headers: StartSupporterTip200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// GetOwnSupporterTip serves GET /api/v1/supporter/tips/{id}.
func (s *Server) GetOwnSupporterTip(ctx context.Context, req GetOwnSupporterTipRequestObject) (GetOwnSupporterTipResponseObject, error) {
	tips, p := s.tips(ctx)
	if p != nil {
		return GetOwnSupporterTipdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	tip, err := tips.OwnTip(ctx, v.UserID, knownID(req.Id))
	if errors.Is(err, appsupporters.ErrNotFound) {
		return GetOwnSupporterTipdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the tip %s: %w", req.Id, err)
	}
	out := SupporterTip{
		Id: tip.ID, State: SupporterTipState(tip.State), Amount: minorMoney(tip.AmountMinor, tip.Currency), CreatedAt: tip.CreatedAt.UTC(),
	}
	if tip.Payment != nil {
		out.PaidAt = utcPtr(tip.Payment.PaidAt)
	}
	cache := cacheNoStore
	return GetOwnSupporterTip200JSONResponse{Body: out, Headers: GetOwnSupporterTip200ResponseHeaders{CacheControl: &cache}}, nil
}

func newAdminSupporterTips(t appsupporters.TipSettings) AdminSupporterTips {
	out := AdminSupporterTips{
		Enabled: t.Enabled, Accepting: t.Accepting, OneTimePayments: t.OneTimePayments, Amounts: tipMoneys(t.Currency, t.Amounts),
		Limits: make([]AdminSupporterTipLimits, 0, len(t.Limits)), ServerLimitReached: t.ServerLimitReached,
	}
	if !t.Enabled {
		out.Amounts = []Money{}
	}
	for _, l := range t.Limits {
		out.Limits = append(out.Limits, AdminSupporterTipLimits{Minimum: minorMoney(l.Minimum, l.Currency), Maximum: minorMoney(l.Maximum, l.Currency)})
	}
	if !t.ChangedAt.IsZero() {
		out.ChangedAt = utcPtr(t.ChangedAt)
	}
	return out
}

func adminSupporterTipsResponse(t appsupporters.TipSettings) AdminSupporterTipsJSONResponse {
	cache := cacheNoStore
	return AdminSupporterTipsJSONResponse{Body: newAdminSupporterTips(t), Headers: AdminSupporterTipsResponseHeaders{CacheControl: &cache}}
}

// GetSupporterTips serves GET /api/v1/admin/supporter_tips.
func (s *Server) GetSupporterTips(ctx context.Context, _ GetSupporterTipsRequestObject) (GetSupporterTipsResponseObject, error) {
	_, p, err := s.requireSupporters(ctx)
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return GetSupporterTipsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	tips, p := s.tips(ctx)
	if p != nil {
		return GetSupporterTipsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	t, err := tips.TipSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the tips: %w", err)
	}
	return GetSupporterTips200JSONResponse{adminSupporterTipsResponse(t)}, nil
}

// SetSupporterTips serves PUT /api/v1/admin/supporter_tips.
func (s *Server) SetSupporterTips(ctx context.Context, req SetSupporterTipsRequestObject) (SetSupporterTipsResponseObject, error) {
	v, p, err := s.requireSupporters(ctx)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return SetSupporterTipsdefaultApplicationProblemPlusJSONResponse(*p), nil
	case req.Body == nil:
		return SetSupporterTipsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	tips, p := s.tips(ctx)
	if p != nil {
		return SetSupporterTipsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	in := appsupporters.TipSettingsInput{Enabled: req.Body.Enabled}
	if in.Enabled && req.Body.Amounts != nil {
		for _, m := range *req.Body.Amounts {
			amount, err := finance.ParseMoney(m.Amount, m.Currency)
			if err != nil || amount.Minor <= 0 || (in.Currency != "" && string(amount.Currency) != in.Currency) {
				return SetSupporterTips422ApplicationProblemPlusJSONResponse{s.tipAmountsProblem(ctx, TipAmountsProblemReasonCurrency)}, nil
			}
			in.Currency = string(amount.Currency)
			in.Amounts = append(in.Amounts, amount.Minor)
		}
	}
	t, err := tips.SetTips(ctx, in, v.UserID)
	var ae *appsupporters.TipAmountsError
	if errors.As(err, &ae) {
		return SetSupporterTips422ApplicationProblemPlusJSONResponse{s.tipAmountsProblem(ctx, TipAmountsProblemReason(ae.Rule))}, nil
	}
	if pr, ok := s.tipProblem(ctx, err); ok {
		return SetSupporterTipsdefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: set the tips: %w", err)
	}
	return SetSupporterTips200JSONResponse{adminSupporterTipsResponse(t)}, nil
}

// tipAmountsProblem is the 422 of amounts of the tips that break a rule.
func (s *Server) tipAmountsProblem(ctx context.Context, reason TipAmountsProblemReason) TipAmountsProblemApplicationProblemPlusJSONResponse {
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidTipAmounts,
		l.ServerErrorsSupportersTipAmountsInvalid(), l.ServerErrorsSupportersTipAmountsInvalidDetail(string(reason)))
	return TipAmountsProblemApplicationProblemPlusJSONResponse{
		Body: TipAmountsProblem{
			Type: body.Type, Title: body.Title, Status: body.Status, Detail: body.Detail, RequestId: body.RequestId, Reason: &reason,
		},
		Headers: TipAmountsProblemResponseHeaders{CacheControl: headers.CacheControl, ContentLanguage: headers.ContentLanguage},
	}
}
