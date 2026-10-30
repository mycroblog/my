// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/notification"
)

// NotificationPreferences reads and changes the viewer's notification settings (FR-210).
// *appnotification.Service implements it.
type NotificationPreferences interface {
	Settings(ctx context.Context, accountID string) ([]appnotification.GroupSettings, error)
	// UpdateSettings fails with notification.ErrInvalidSetting or notification.ErrFixedSetting
	// for a change the viewer cannot make.
	UpdateSettings(ctx context.Context, accountID string, choices []notification.Setting) ([]appnotification.GroupSettings, error)
}

func newNotificationSettings(rows []appnotification.GroupSettings) NotificationSettings {
	out := NotificationSettings{Rows: make([]NotificationSettingRow, len(rows))}
	for i, r := range rows {
		types := make([]NotificationType, len(r.Spec.Types))
		for j, t := range r.Spec.Types {
			types[j] = NotificationType(t)
		}
		out.Rows[i] = NotificationSettingRow{
			Group: NotificationGroup(r.Spec.Group), Section: NotificationSettingRowSection(r.Spec.Section), Types: types,
			InApp: NotificationSettingCell{Enabled: r.InApp.Enabled, Fixed: r.InApp.Fixed},
		}
		if !r.Email.Absent {
			out.Rows[i].Email = &NotificationSettingCell{Enabled: r.Email.Enabled, Fixed: r.Email.Fixed}
		}
	}
	return out
}

// GetNotificationSettings serves GET /api/v1/preferences/notifications.
func (s *Server) GetNotificationSettings(ctx context.Context, _ GetNotificationSettingsRequestObject) (GetNotificationSettingsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.NotificationPreferences == nil {
		body, headers := s.notFound(ctx)
		return GetNotificationSettingsdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	rows, err := s.o.NotificationPreferences.Settings(ctx, v.AccountID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the notification settings of %s: %w", v.AccountID, err)
	}
	cache := cacheNoStore
	return GetNotificationSettings200JSONResponse{
		Body: newNotificationSettings(rows), Headers: GetNotificationSettings200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// UpdateNotificationSettings serves PATCH /api/v1/preferences/notifications.
func (s *Server) UpdateNotificationSettings(ctx context.Context, req UpdateNotificationSettingsRequestObject) (UpdateNotificationSettingsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.NotificationPreferences == nil {
		body, headers := s.notFound(ctx)
		return UpdateNotificationSettingsdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	var choices []notification.Setting
	if req.Body != nil {
		for _, c := range req.Body.Changes {
			choices = append(choices, notification.Setting{
				Group: notification.Group(c.Group), Channel: notification.Channel(c.Channel), Enabled: c.Enabled,
			})
		}
	}
	rows, err := s.o.NotificationPreferences.UpdateSettings(ctx, v.AccountID, choices)
	if errors.Is(err, notification.ErrInvalidSetting) || errors.Is(err, notification.ErrFixedSetting) {
		l := s.Localizer(ctx)
		body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("changes"))
		return UpdateNotificationSettingsdefaultApplicationProblemPlusJSONResponse{
			Body: body, Headers: headers, StatusCode: http.StatusUnprocessableEntity,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: update the notification settings of %s: %w", v.AccountID, err)
	}
	cache := cacheNoStore
	return UpdateNotificationSettings200JSONResponse{
		Body: newNotificationSettings(rows), Headers: UpdateNotificationSettings200ResponseHeaders{CacheControl: &cache},
	}, nil
}
