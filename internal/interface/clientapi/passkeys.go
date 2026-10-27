// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The viewer's passkeys (FR-004): registering one after the password or another passkey,
// listing, and removing them, and the confirmations with a passkey that the operations on the
// viewer's own account take in place of the password. The identity module runs the WebAuthn
// ceremonies; signing in with a passkey is the server-rendered sign-in page's.

// Problem types of passkeys.
const (
	// ProblemTypeInvalidPasskey is the problem of a registration response that does not verify.
	ProblemTypeInvalidPasskey = "/api/v1/problems/invalid-passkey"
	// ProblemTypePasskeyRegistrationExpired is the problem of an unknown or expired registration.
	ProblemTypePasskeyRegistrationExpired = "/api/v1/problems/passkey-registration-expired"
	// ProblemTypeTooManyPasskeys is the problem of a registration by a user with the most
	// passkeys allowed.
	ProblemTypeTooManyPasskeys = "/api/v1/problems/too-many-passkeys"
	// ProblemTypePasskeyConfirmationExpired is the problem of an unknown, expired, or used
	// confirmation with a passkey.
	ProblemTypePasskeyConfirmationExpired = "/api/v1/problems/passkey-confirmation-expired"
	// ProblemTypeLastSignInMethod is the problem of a removal that would leave the viewer no way
	// to sign in.
	ProblemTypeLastSignInMethod = "/api/v1/problems/last-sign-in-method"
)

// PasskeySettings runs the viewer's passkeys. *appidentity.Service implements it.
type PasskeySettings interface {
	Passkeys(ctx context.Context, userID string) ([]identity.PasskeyCredential, error)
	BeginPasskeyRegistration(ctx context.Context, userID, name string, c appidentity.Confirmation) (appidentity.PasskeyRegistration, error)
	FinishPasskeyRegistration(ctx context.Context, userID, token string, response []byte) (identity.PasskeyCredential, error)
	RemovePasskey(ctx context.Context, userID, passkeyID string) error
	BeginPasskeyConfirmation(ctx context.Context, userID string) (appidentity.PasskeyConfirmation, error)
}

// confirmationOf returns the confirmation of a request with password, passkey, and external
// (external_confirmation), the members of its body that confirm it, or false with the name of
// the parameter to blame when it has none or a malformed passkey. A confirmation with a linked
// login provider takes the place of the others, and a passkey that of the password.
func confirmationOf(password *string, passkey *PasskeyAssertion, external *string, passwordName string) (appidentity.Confirmation, string, bool) {
	if external != nil {
		if *external == "" {
			return appidentity.Confirmation{}, "external_confirmation", false
		}
		return appidentity.Confirmation{External: *external}, "", true
	}
	if passkey != nil {
		if passkey.ConfirmationToken == "" || len(passkey.Credential) == 0 {
			return appidentity.Confirmation{}, "passkey", false
		}
		response, err := json.Marshal(passkey.Credential)
		if err != nil {
			return appidentity.Confirmation{}, "passkey", false
		}
		return appidentity.Confirmation{Passkey: appidentity.PasskeyAssertion{Token: passkey.ConfirmationToken, Response: response}}, "", true
	}
	if password == nil || *password == "" {
		return appidentity.Confirmation{}, passwordName, false
	}
	return appidentity.Confirmation{Password: *password}, "", true
}

func newPasskey(c identity.PasskeyCredential) Passkey {
	return Passkey{Id: c.ID, Name: c.Name, CreatedAt: c.CreatedAt.UTC()}
}

// passkeyProblem returns the problem of an error of the identity module's passkey use cases,
// or nil for another error.
func (s *Server) passkeyProblem(ctx context.Context, err error) *problem {
	l := s.Localizer(ctx)
	var limited *kernel.RateLimitedError
	var p problem
	switch {
	case errors.As(err, &limited):
		retry := seconds(limited.RetryAfter)
		p = s.problem(ctx, http.StatusTooManyRequests, ProblemTypeBlank, l.ServerErrorsRateLimited(), l.ServerErrorsRateLimitedDetail(retry))
		p.Headers.RetryAfter = &retry
	case errors.Is(err, identity.ErrInvalidPasskeyName):
		p = s.invalidParameterResponse(ctx, "name")
	case errors.Is(err, appidentity.ErrWrongPassword):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeWrongPassword, l.ServerErrorsWrongPassword(), "")
	case errors.Is(err, appidentity.ErrInvalidPasskey):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidPasskey, l.ServerErrorsPasskeyInvalid(), "")
	case errors.Is(err, appidentity.ErrPasskeyCeremonyExpired):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypePasskeyRegistrationExpired, l.ServerErrorsPasskeyRegistrationExpired(), "")
	case errors.Is(err, appidentity.ErrTooManyPasskeys):
		p = s.problem(ctx, http.StatusConflict, ProblemTypeTooManyPasskeys, l.ServerErrorsPasskeyTooMany(), "")
	case errors.Is(err, appidentity.ErrPasskeyConfirmationExpired):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypePasskeyConfirmationExpired, l.ServerErrorsPasskeyConfirmationExpired(), "")
	case errors.Is(err, appidentity.ErrLastSignInMethod):
		p = s.problem(ctx, http.StatusConflict, ProblemTypeLastSignInMethod, l.ServerErrorsPasskeyLastSignInMethod(), "")
	case errors.Is(err, appidentity.ErrExternalConfirmationExpired):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeExternalConfirmationExpired, l.ServerErrorsExternalConfirmationExpired(), "")
	default:
		return nil
	}
	return &p
}

// ListPasskeys serves GET /api/v1/passkeys.
func (s *Server) ListPasskeys(ctx context.Context, _ ListPasskeysRequestObject) (ListPasskeysResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Passkeys == nil {
		return ListPasskeysdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	list, err := s.o.Passkeys.Passkeys(ctx, v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the passkeys of %s: %w", v.UserID, err)
	}
	items := make([]Passkey, len(list))
	for i, c := range list {
		items[i] = newPasskey(c)
	}
	cache := cacheNoStore
	return ListPasskeys200JSONResponse{Body: PasskeyList{Items: items}, Headers: ListPasskeys200ResponseHeaders{CacheControl: &cache}}, nil
}

// BeginPasskeyRegistration serves POST /api/v1/passkeys/registration.
func (s *Server) BeginPasskeyRegistration(ctx context.Context, req BeginPasskeyRegistrationRequestObject) (BeginPasskeyRegistrationResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Passkeys == nil {
		return BeginPasskeyRegistrationdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return BeginPasskeyRegistrationdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "password")), nil
	}
	c, param, ok := confirmationOf(req.Body.Password, req.Body.Passkey, req.Body.ExternalConfirmation, "password")
	if !ok {
		return BeginPasskeyRegistrationdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, param)), nil
	}
	reg, err := s.o.Passkeys.BeginPasskeyRegistration(ctx, v.UserID, req.Body.Name, c)
	if p := s.passkeyProblem(ctx, err); p != nil {
		return BeginPasskeyRegistrationdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: begin a passkey registration for %s: %w", v.UserID, err)
	}
	var options map[string]any
	if err := json.Unmarshal(reg.Options, &options); err != nil {
		return nil, fmt.Errorf("clientapi: the options of a passkey registration: %w", err)
	}
	cache := cacheNoStore
	return BeginPasskeyRegistration200JSONResponse{
		Body:    PasskeyRegistration{Options: options, RegistrationToken: reg.Token, ExpiresAt: reg.ExpiresAt.UTC()},
		Headers: BeginPasskeyRegistration200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// CreatePasskey serves POST /api/v1/passkeys.
func (s *Server) CreatePasskey(ctx context.Context, req CreatePasskeyRequestObject) (CreatePasskeyResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Passkeys == nil {
		return CreatePasskeydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil || len(req.Body.Credential) == 0 {
		return CreatePasskeydefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "credential")), nil
	}
	response, err := json.Marshal(req.Body.Credential)
	if err != nil {
		return CreatePasskeydefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "credential")), nil
	}
	c, err := s.o.Passkeys.FinishPasskeyRegistration(ctx, v.UserID, req.Body.RegistrationToken, response)
	if p := s.passkeyProblem(ctx, err); p != nil {
		return CreatePasskeydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: register a passkey for %s: %w", v.UserID, err)
	}
	return CreatePasskey201JSONResponse(newPasskey(c)), nil
}

// DeletePasskey serves DELETE /api/v1/passkeys/{id}.
func (s *Server) DeletePasskey(ctx context.Context, req DeletePasskeyRequestObject) (DeletePasskeyResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Passkeys == nil {
		return DeletePasskeydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	err = s.o.Passkeys.RemovePasskey(ctx, v.UserID, req.Id)
	if p := s.passkeyProblem(ctx, err); p != nil {
		return DeletePasskeydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return DeletePasskeydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: remove passkey %s of %s: %w", req.Id, v.UserID, err)
	}
	return DeletePasskey204Response{}, nil
}

// BeginPasskeyConfirmation serves POST /api/v1/passkeys/confirmation.
func (s *Server) BeginPasskeyConfirmation(ctx context.Context, _ BeginPasskeyConfirmationRequestObject) (BeginPasskeyConfirmationResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Passkeys == nil {
		return BeginPasskeyConfirmationdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	c, err := s.o.Passkeys.BeginPasskeyConfirmation(ctx, v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: begin a passkey confirmation for %s: %w", v.UserID, err)
	}
	var options map[string]any
	if err := json.Unmarshal(c.Options, &options); err != nil {
		return nil, fmt.Errorf("clientapi: the options of a passkey confirmation: %w", err)
	}
	cache := cacheNoStore
	return BeginPasskeyConfirmation200JSONResponse{
		Body:    PasskeyConfirmation{Options: options, ConfirmationToken: c.Token, ExpiresAt: c.ExpiresAt.UTC()},
		Headers: BeginPasskeyConfirmation200ResponseHeaders{CacheControl: &cache},
	}, nil
}
