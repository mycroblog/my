// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"rsc.io/qr"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
)

// The viewer's two-factor authentication with TOTP (FR-005). The identity module generates and
// checks the secrets; the sign-in step that asks for a code is the auth screens' (authpages).

// Problem types of two-factor authentication.
const (
	// ProblemTypeInvalidTwoFactorCode is the problem of a wrong, used, or malformed code.
	ProblemTypeInvalidTwoFactorCode = "/api/v1/problems/invalid-two-factor-code"
	// ProblemTypeTOTPSetupExpired is the problem of an unknown or expired setup token.
	ProblemTypeTOTPSetupExpired = "/api/v1/problems/totp-setup-expired"
	// ProblemTypeTwoFactorEnabled is the problem of a setup for a user who has two-factor
	// authentication.
	ProblemTypeTwoFactorEnabled = "/api/v1/problems/two-factor-enabled"
	// ProblemTypeTwoFactorNotEnabled is the problem of an operation that needs two-factor
	// authentication on a user without it.
	ProblemTypeTwoFactorNotEnabled = "/api/v1/problems/two-factor-not-enabled"
)

// TwoFactorSettings runs the viewer's two-factor authentication. *appidentity.Service implements it.
type TwoFactorSettings interface {
	TwoFactorStatus(ctx context.Context, userID string) (appidentity.TwoFactorStatus, error)
	BeginTOTPSetup(ctx context.Context, userID string) (appidentity.TOTPSetup, error)
	EnableTOTP(ctx context.Context, userID, setupToken, code string) ([]string, error)
	DisableTOTP(ctx context.Context, userID, code string) error
	RegenerateRecoveryCodes(ctx context.Context, userID, code string) ([]string, error)
}

func newTwoFactor(st appidentity.TwoFactorStatus) TwoFactor {
	out := TwoFactor{Enabled: st.Enabled}
	if st.Enabled {
		at, n := st.EnrolledAt.UTC(), st.RecoveryCodesRemaining
		out.EnrolledAt, out.RecoveryCodesRemaining = &at, &n
	}
	return out
}

// twoFactorProblem returns the problem of an error of the identity module's two-factor use
// cases, or nil for another error.
func (s *Server) twoFactorProblem(ctx context.Context, err error) *problem {
	l := s.Localizer(ctx)
	var limited *kernel.RateLimitedError
	var p problem
	switch {
	case errors.As(err, &limited):
		retry := seconds(limited.RetryAfter)
		p = s.problem(ctx, http.StatusTooManyRequests, ProblemTypeBlank, l.ServerErrorsRateLimited(), l.ServerErrorsRateLimitedDetail(retry))
		p.Headers.RetryAfter = &retry
	case errors.Is(err, appidentity.ErrInvalidSecondFactor):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidTwoFactorCode, l.ServerErrorsTwoFactorCodeInvalid(), "")
	case errors.Is(err, appidentity.ErrInvalidTOTPSetup):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeTOTPSetupExpired, l.ServerErrorsTwoFactorSetupExpired(), "")
	case errors.Is(err, appidentity.ErrTwoFactorEnabled):
		p = s.problem(ctx, http.StatusConflict, ProblemTypeTwoFactorEnabled, l.ServerErrorsTwoFactorEnabled(), "")
	case errors.Is(err, appidentity.ErrTwoFactorNotEnabled):
		p = s.problem(ctx, http.StatusConflict, ProblemTypeTwoFactorNotEnabled, l.ServerErrorsTwoFactorNotEnabled(), "")
	default:
		return nil
	}
	return &p
}

// GetTwoFactor serves GET /api/v1/two_factor.
func (s *Server) GetTwoFactor(ctx context.Context, _ GetTwoFactorRequestObject) (GetTwoFactorResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.TwoFactor == nil {
		return GetTwoFactordefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	st, err := s.o.TwoFactor.TwoFactorStatus(ctx, v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the two-factor authentication of %s: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return GetTwoFactor200JSONResponse{Body: newTwoFactor(st), Headers: GetTwoFactor200ResponseHeaders{CacheControl: &cache}}, nil
}

// SetUpTotp serves POST /api/v1/two_factor/totp/setup.
func (s *Server) SetUpTotp(ctx context.Context, _ SetUpTotpRequestObject) (SetUpTotpResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.TwoFactor == nil {
		return SetUpTotpdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	setup, err := s.o.TwoFactor.BeginTOTPSetup(ctx, v.UserID)
	if p := s.twoFactorProblem(ctx, err); p != nil {
		return SetUpTotpdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: set up TOTP for %s: %w", v.UserID, err)
	}
	code, err := newQRCode(setup.URI)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the QR code of a TOTP setup: %w", err)
	}
	cache := cacheNoStore
	return SetUpTotp200JSONResponse{
		Body: TotpSetup{
			Secret: setup.Secret, Uri: setup.URI, QrCode: code, SetupToken: setup.Token, ExpiresAt: setup.ExpiresAt.UTC(),
		},
		Headers: SetUpTotp200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// EnableTotp serves POST /api/v1/two_factor/totp.
func (s *Server) EnableTotp(ctx context.Context, req EnableTotpRequestObject) (EnableTotpResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.TwoFactor == nil {
		return EnableTotpdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return EnableTotpdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "code")), nil
	}
	codes, err := s.o.TwoFactor.EnableTOTP(ctx, v.UserID, req.Body.SetupToken, req.Body.Code)
	if p := s.twoFactorProblem(ctx, err); p != nil {
		return EnableTotpdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: enable TOTP for %s: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return EnableTotp200JSONResponse{Body: RecoveryCodes{RecoveryCodes: codes}, Headers: EnableTotp200ResponseHeaders{CacheControl: &cache}}, nil
}

// DisableTotp serves POST /api/v1/two_factor/totp/disable.
func (s *Server) DisableTotp(ctx context.Context, req DisableTotpRequestObject) (DisableTotpResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.TwoFactor == nil {
		return DisableTotpdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return DisableTotpdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "code")), nil
	}
	err = s.o.TwoFactor.DisableTOTP(ctx, v.UserID, req.Body.Code)
	if p := s.twoFactorProblem(ctx, err); p != nil {
		return DisableTotpdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: disable TOTP for %s: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return DisableTotp200JSONResponse{Body: TwoFactor{Enabled: false}, Headers: DisableTotp200ResponseHeaders{CacheControl: &cache}}, nil
}

// RegenerateRecoveryCodes serves POST /api/v1/two_factor/recovery_codes.
func (s *Server) RegenerateRecoveryCodes(ctx context.Context, req RegenerateRecoveryCodesRequestObject) (RegenerateRecoveryCodesResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.TwoFactor == nil {
		return RegenerateRecoveryCodesdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return RegenerateRecoveryCodesdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "code")), nil
	}
	codes, err := s.o.TwoFactor.RegenerateRecoveryCodes(ctx, v.UserID, req.Body.Code)
	if p := s.twoFactorProblem(ctx, err); p != nil {
		return RegenerateRecoveryCodesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: regenerate the recovery codes of %s: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return RegenerateRecoveryCodes200JSONResponse{
		Body: RecoveryCodes{RecoveryCodes: codes}, Headers: RegenerateRecoveryCodes200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// qrQuietZone is the light border that QR readers need around a code, in modules.
const qrQuietZone = 4

// newQRCode encodes text as a QR code drawn by an SVG path, so that clients draw it without a
// QR library and without an image URL that the Content-Security-Policy would have to admit.
func newQRCode(text string) (QrCode, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return QrCode{}, err
	}
	var b strings.Builder
	for y := range c.Size {
		for x := 0; x < c.Size; {
			if !c.Black(x, y) {
				x++
				continue
			}
			start := x
			for x < c.Size && c.Black(x, y) {
				x++
			}
			// One rectangle per run of dark modules in a row.
			b.WriteString("M" + strconv.Itoa(start+qrQuietZone) + " " + strconv.Itoa(y+qrQuietZone) +
				"h" + strconv.Itoa(x-start) + "v1h-" + strconv.Itoa(x-start) + "z")
		}
	}
	return QrCode{Size: c.Size + 2*qrQuietZone, Path: b.String()}, nil
}
