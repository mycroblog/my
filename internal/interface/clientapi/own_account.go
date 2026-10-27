// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/app/blob"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The viewer's own account: its password (FR-002) and its removal (FR-004), its deletion, and
// the exports of its data (FR-008). The current password or a passkey of the viewer confirms
// them (passkeys.go).

// Problem types of the viewer's own account.
const (
	// ProblemTypeWrongPassword is the problem of a confirmation with a wrong password.
	ProblemTypeWrongPassword = "/api/v1/problems/wrong-password"
	// ProblemTypeExportInProgress is the problem of a request for an export while another one is
	// being built.
	ProblemTypeExportInProgress = "/api/v1/problems/export-in-progress"
	// ProblemTypeExportNotReady is the problem of a download of an export without an archive.
	ProblemTypeExportNotReady = "/api/v1/problems/export-not-ready"
)

var (
	// DeleteAccountLimit bounds the attempts to delete the own account, which check a password
	// or a passkey.
	DeleteAccountLimit = kernel.Limit{Name: "api.account.delete", Max: 5, Window: time.Hour}
	// ExportLimit bounds the requests for data exports.
	ExportLimit = kernel.Limit{Name: "api.export", Max: 3, Window: 24 * time.Hour}
)

// OwnAccount changes and removes the viewer's password and deletes the viewer's own account;
// *appidentity.Service implements it.
type OwnAccount interface {
	HasPassword(ctx context.Context, userID string) (bool, error)
	ChangePassword(ctx context.Context, in appidentity.ChangePasswordInput) error
	RemovePassword(ctx context.Context, in appidentity.RemovePasswordInput) error
	DeleteOwnAccount(ctx context.Context, userID string, c appidentity.Confirmation) error
}

// GetPasswordStatus serves GET /api/v1/accounts/password.
func (s *Server) GetPasswordStatus(ctx context.Context, _ GetPasswordStatusRequestObject) (GetPasswordStatusResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OwnAccount == nil {
		return GetPasswordStatusdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	has, err := s.o.OwnAccount.HasPassword(ctx, v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: whether user %s has a password: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return GetPasswordStatus200JSONResponse{
		Body: PasswordStatus{HasPassword: has}, Headers: GetPasswordStatus200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// RemovePassword serves POST /api/v1/accounts/password/remove. The grant of the request's token
// keeps working; every other one ends.
func (s *Server) RemovePassword(ctx context.Context, req RemovePasswordRequestObject) (RemovePasswordResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OwnAccount == nil {
		return RemovePassworddefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return RemovePassworddefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "passkey")), nil
	}
	c, param, ok := confirmationOf(nil, req.Body.Passkey, req.Body.ExternalConfirmation, "passkey")
	if !ok {
		return RemovePassworddefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, param)), nil
	}
	err = s.o.OwnAccount.RemovePassword(ctx, appidentity.RemovePasswordInput{
		UserID: v.UserID, Passkey: c.Passkey, External: c.External, KeepFamilyID: v.FamilyID,
	})
	if p := s.passkeyProblem(ctx, err); p != nil {
		return RemovePassworddefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: remove the password of user %s: %w", v.UserID, err)
	}
	return RemovePassword204Response{}, nil
}

// ChangePassword serves POST /api/v1/accounts/password. The grant of the request's token keeps
// working; every other one ends.
func (s *Server) ChangePassword(ctx context.Context, req ChangePasswordRequestObject) (ChangePasswordResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OwnAccount == nil {
		return ChangePassworddefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return ChangePassworddefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "current_password")), nil
	}
	c, param, ok := confirmationOf(req.Body.CurrentPassword, req.Body.Passkey, req.Body.ExternalConfirmation, "current_password")
	if !ok {
		return ChangePassworddefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, param)), nil
	}
	if identity.ValidatePassword(req.Body.NewPassword) != nil {
		return ChangePassworddefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "new_password")), nil
	}
	err = s.o.OwnAccount.ChangePassword(ctx, appidentity.ChangePasswordInput{
		UserID: v.UserID, Confirmation: c, NewPassword: req.Body.NewPassword, KeepFamilyID: v.FamilyID,
	})
	if p := s.passkeyProblem(ctx, err); p != nil {
		return ChangePassworddefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	switch {
	case errors.Is(err, identity.ErrInvalidPassword):
		return ChangePassworddefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "new_password")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: change the password of user %s: %w", v.UserID, err)
	}
	return ChangePassword204Response{}, nil
}

// Exports runs the viewer's data exports; *appidentity.Exports implements it.
type Exports interface {
	RequestExport(ctx context.Context, userID string) (identity.DataExport, error)
	ListExports(ctx context.Context, userID string) ([]identity.DataExport, error)
	OpenExport(ctx context.Context, userID, exportID string) (io.ReadCloser, blob.Info, error)
	Now() time.Time
}

// DeleteOwnAccount serves POST /api/v1/accounts/delete.
func (s *Server) DeleteOwnAccount(ctx context.Context, req DeleteOwnAccountRequestObject) (DeleteOwnAccountResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OwnAccount == nil {
		return DeleteOwnAccountdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if p, h, limited := s.LimitProblem(ctx, DeleteAccountLimit, v.AccountID); limited {
		return DeleteOwnAccountdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	if req.Body == nil {
		return DeleteOwnAccountdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "password")), nil
	}
	c, param, ok := confirmationOf(req.Body.Password, req.Body.Passkey, req.Body.ExternalConfirmation, "password")
	if !ok {
		return DeleteOwnAccountdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, param)), nil
	}
	err = s.o.OwnAccount.DeleteOwnAccount(ctx, v.UserID, c)
	if p := s.passkeyProblem(ctx, err); p != nil {
		return DeleteOwnAccountdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: delete the account of user %s: %w", v.UserID, err)
	}
	return DeleteOwnAccount204Response{}, nil
}

// ListDataExports serves GET /api/v1/exports.
func (s *Server) ListDataExports(ctx context.Context, _ ListDataExportsRequestObject) (ListDataExportsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Exports == nil {
		return ListDataExportsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	es, err := s.o.Exports.ListExports(ctx, v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the exports of user %s: %w", v.UserID, err)
	}
	now := s.o.Exports.Now()
	items := make([]DataExport, 0, len(es))
	for _, e := range es {
		if d, ok := newDataExport(e, now); ok {
			items = append(items, d)
		}
	}
	cache := cacheNoStore
	return ListDataExports200JSONResponse{
		Body: DataExportList{Items: items}, Headers: ListDataExports200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// RequestDataExport serves POST /api/v1/exports.
func (s *Server) RequestDataExport(ctx context.Context, _ RequestDataExportRequestObject) (RequestDataExportResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Exports == nil {
		return RequestDataExportdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if p, h, limited := s.LimitProblem(ctx, ExportLimit, v.AccountID); limited {
		return RequestDataExportdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	e, err := s.o.Exports.RequestExport(ctx, v.UserID)
	switch {
	case errors.Is(err, appidentity.ErrExportInProgress):
		l := s.Localizer(ctx)
		return RequestDataExportdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusConflict, ProblemTypeExportInProgress, l.ServerErrorsExportInProgress(), "")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: request an export of user %s: %w", v.UserID, err)
	}
	d, _ := newDataExport(e, s.o.Exports.Now())
	cache := cacheNoStore
	return RequestDataExport202JSONResponse{Body: d, Headers: RequestDataExport202ResponseHeaders{CacheControl: &cache}}, nil
}

// DownloadDataExport serves GET /api/v1/exports/{id}/archive.
func (s *Server) DownloadDataExport(ctx context.Context, req DownloadDataExportRequestObject) (DownloadDataExportResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.Exports == nil {
		return DownloadDataExportdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	r, info, err := s.o.Exports.OpenExport(ctx, v.UserID, req.Id)
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return DownloadDataExportdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appidentity.ErrExportNotReady):
		l := s.Localizer(ctx)
		return DownloadDataExportdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusConflict, ProblemTypeExportNotReady, l.ServerErrorsExportNotReady(), "")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: open export %s of user %s: %w", req.Id, v.UserID, err)
	}
	cache := cacheNoStore
	disposition := `attachment; filename="export-` + req.Id + `.zip"`
	return DownloadDataExport200ApplicationzipResponse{
		Body: r, ContentLength: info.Size,
		Headers: DownloadDataExport200ResponseHeaders{CacheControl: &cache, ContentDisposition: &disposition},
	}, nil
}

// newDataExport returns the DataExport of e at now; an expired export, which is being removed,
// is left out.
func newDataExport(e identity.DataExport, now time.Time) (DataExport, bool) {
	d := DataExport{Id: e.ID, RequestedAt: e.RequestedAt}
	switch e.State(now) {
	case identity.ExportStatePending:
		d.State = DataExportStatePending
		return d, true
	case identity.ExportStateReady:
		d.State = DataExportStateReady
		size := e.Result.FileSize
		d.FileSize = &size
	case identity.ExportStateFailed:
		d.State = DataExportStateFailed
	default:
		return DataExport{}, false
	}
	completed, expires := e.Result.CompletedAt, e.Result.ExpiresAt
	d.CompletedAt, d.ExpiresAt = &completed, &expires
	return d, true
}
