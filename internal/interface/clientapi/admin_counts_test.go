// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

type fakeCounts struct {
	reports, registrations int64
	oldest                 *time.Time
}

func (f fakeCounts) CountOpenReports(context.Context) (int64, error) { return f.reports, nil }

func (f fakeCounts) OldestOpenReportFiledAt(context.Context) (*time.Time, error) {
	return f.oldest, nil
}

func (f fakeCounts) CountPendingRegistrations(context.Context) (int64, error) {
	return f.registrations, nil
}

// TestGetAdminCounts checks that each count needs its own permission and that a count the
// viewer may not see is omitted, not refused.
func TestGetAdminCounts(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	oldest := time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		perms []identity.Permission
		want  clientapi.AdminCounts
	}{
		{"both", []identity.Permission{identity.PermManageReports, identity.PermManageAccounts}, clientapi.AdminCounts{OpenReports: n(3), OldestOpenReportFiledAt: &oldest, PendingRegistrations: n(0)}},
		{"reports", []identity.Permission{identity.PermManageReports, identity.PermViewAudit}, clientapi.AdminCounts{OpenReports: n(3), OldestOpenReportFiledAt: &oldest}},
		{"registrations", []identity.Permission{identity.PermManageAccounts}, clientapi.AdminCounts{PendingRegistrations: n(0)}},
		{"none", []identity.Permission{identity.PermManageDomains}, clientapi.AdminCounts{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newAuthEnv(t, func(o *clientapi.Options) {
				o.Permissions = fakePermissions{userOf(aliceAccountID): tc.perms}
				o.ReportCounts, o.RegistrationCounts = fakeCounts{reports: 3, oldest: &oldest}, fakeCounts{reports: 3}
			})
			e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
			res := e.admin(t, http.MethodGet, "/admin/counts", "admin-read", nil)
			if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("GET /admin/counts = %d (Cache-Control %q), want 200 no-store", res.StatusCode, res.Header.Get("Cache-Control"))
			}
			got, want := decodeJSON[clientapi.AdminCounts](t, res), tc.want
			if (got.OldestOpenReportFiledAt == nil) != (want.OldestOpenReportFiledAt == nil) ||
				(got.OldestOpenReportFiledAt != nil && !got.OldestOpenReportFiledAt.Equal(*want.OldestOpenReportFiledAt)) {
				t.Errorf("oldest_open_report_filed_at = %v, want %v", got.OldestOpenReportFiledAt, want.OldestOpenReportFiledAt)
			}
			got.OldestOpenReportFiledAt, want.OldestOpenReportFiledAt = nil, nil
			if !reflect.DeepEqual(got, want) {
				t.Errorf("counts = %+v, want %+v", got, want)
			}
		})
	}
}

// TestGetAdminCountsWithoutOpenReports checks that the oldest open report is omitted when no
// report is open.
func TestGetAdminCountsWithoutOpenReports(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): {identity.PermManageReports}}
		o.ReportCounts = fakeCounts{}
	})
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	res := e.admin(t, http.MethodGet, "/admin/counts", "admin-read", nil)
	if got := decodeJSON[clientapi.AdminCounts](t, res); res.StatusCode != http.StatusOK || got.OldestOpenReportFiledAt != nil {
		t.Errorf("GET /admin/counts = %d %+v, want 200 without the oldest open report", res.StatusCode, got)
	}
}

// TestGetAdminCountsNeedsTheAdminScope checks that a token without admin:read is refused.
func TestGetAdminCountsNeedsTheAdminScope(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.ReportCounts, o.RegistrationCounts = fakeCounts{}, fakeCounts{}
	})
	if res := e.admin(t, http.MethodGet, "/admin/counts", "read", nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("GET /admin/counts with read = %d, want 403", res.StatusCode)
	}
}
