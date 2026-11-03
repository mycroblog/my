// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"

	appsupporters "github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The payment provider (ADR-0074 §4): its state and the reconciliation by hand, the admin
// operations on /admin/payment_status and /admin/payment_reconciles (manage_supporters,
// ADR-0058 server administration).

// Payments reads the state of the payment provider and reconciles with it now.
// *appsupporters.Payments implements it.
type Payments interface {
	Status(ctx context.Context) (appsupporters.PaymentStatus, error)
	RequestReconcile(ctx context.Context, byUserID string) error
}

// requirePayments checks the manage_supporters permission and that a payment provider is
// configured; it returns the problem to answer otherwise, and the viewer.
func (s *Server) requirePayments(ctx context.Context) (Viewer, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageSupporters); err != nil || p != nil {
		return Viewer{}, p, err
	}
	if s.o.Payments == nil {
		p := s.notFoundResponse(ctx)
		return Viewer{}, &p, nil
	}
	v, err := requireViewer(ctx)
	return v, nil, err
}

// newPaymentStatus presents the state of the payment provider to the administrators.
func newPaymentStatus(st appsupporters.PaymentStatus) PaymentStatus {
	out := PaymentStatus{
		Provider: st.Provider, Mode: PaymentStatusMode(st.Mode), TimeZone: st.Location.String(),
		Warnings: []PaymentStatusWarnings{},
	}
	if st.LastWebhook != nil {
		at := st.LastWebhook.ReceivedAt.UTC()
		out.LastWebhookAt = &at
	}
	if r := st.LastRun; r != nil {
		out.LastReconcile = &PaymentReconcileRun{
			Outcome: PaymentReconcileRunOutcome(r.Outcome), ErrorKind: PaymentReconcileRunErrorKind(r.ErrorKind),
			Requests: r.Requests, CheckedSubscriptions: r.CheckedSubscriptions, CorrectedSubscriptions: r.CorrectedSubscriptions,
			OccurredAt: r.OccurredAt.UTC(),
		}
	}
	if st.NoWebhook() {
		out.Warnings = append(out.Warnings, PaymentStatusWarningsNoWebhook)
	}
	if st.ReconcileCorrected() {
		out.Warnings = append(out.Warnings, PaymentStatusWarningsReconcileCorrected)
	}
	return out
}

// GetPaymentStatus serves GET /api/v1/admin/payment_status.
func (s *Server) GetPaymentStatus(ctx context.Context, _ GetPaymentStatusRequestObject) (GetPaymentStatusResponseObject, error) {
	_, p, err := s.requirePayments(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return GetPaymentStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	st, err := s.o.Payments.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: payment status: %w", err)
	}
	cache := cacheNoStore
	return GetPaymentStatus200JSONResponse{Body: newPaymentStatus(st), Headers: GetPaymentStatus200ResponseHeaders{CacheControl: &cache}}, nil
}

// RunPaymentReconcile serves POST /api/v1/admin/payment_reconciles.
func (s *Server) RunPaymentReconcile(ctx context.Context, _ RunPaymentReconcileRequestObject) (RunPaymentReconcileResponseObject, error) {
	v, p, err := s.requirePayments(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return RunPaymentReconciledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	err = s.o.Payments.RequestReconcile(ctx, v.UserID)
	switch {
	case errors.Is(err, appsupporters.ErrReconcileTooSoon):
		return RunPaymentReconciledefaultApplicationProblemPlusJSONResponse(s.nextHourProblem(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: reconcile with the payment provider: %w", err)
	}
	return RunPaymentReconcile204Response{}, nil
}
