// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const reportTargetID = "01890a5d-ac96-774b-bcce-b302099a8059"

// fakeReports files every report it is given, or fails with err.
type fakeReports struct {
	got []appmoderation.LocalReportInput
	err error
}

func (f *fakeReports) File(_ context.Context, in appmoderation.LocalReportInput) (moderation.Report, error) {
	f.got = append(f.got, in)
	if f.err != nil {
		return moderation.Report{}, f.err
	}
	return moderation.Report{
		ID: "01890a5d-ac96-774b-bcce-b302099a7001", ReporterAccountID: in.ReporterAccountID, TargetAccountID: in.TargetAccountID,
		Category: in.Category, Comment: in.Comment, Forwarded: in.Forward, FiledAt: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC),
		StatusIDs: append([]string{}, in.StatusIDs...), RuleIDs: append([]string{}, in.RuleIDs...), VoiceRoom: in.VoiceRoom,
	}, nil
}

func newReportsEnv(t *testing.T, reports *fakeReports) *authEnv {
	t.Helper()
	return newReportsEnvWith(t, reports, nil)
}

// newReportsEnvWith is newReportsEnv with the voice chat rooms of rooms, none when nil.
func newReportsEnvWith(t *testing.T, reports *fakeReports, rooms clientapi.VoiceModeration) *authEnv {
	t.Helper()
	return newAuthEnv(t, func(o *clientapi.Options) {
		o.VoiceModeration = rooms
		accounts := fakeAccounts{}
		for id, a := range o.Accounts.(fakeAccounts) {
			accounts[id] = a
		}
		accounts[reportTargetID] = appaccount.Actor{
			Account: account.Account{ID: reportTargetID, Domain: "remote.example", URI: "https://remote.example/users/bob", CreatedAt: createdAt},
			Handle:  &account.Handle{AccountID: reportTargetID, Username: "bob", Domain: "remote.example"},
		}
		o.Accounts = accounts
		o.Reports = reports
	})
}

func (e *authEnv) postReport(t *testing.T, token string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, clientapi.BasePath+"/reports", strings.NewReader(string(data)))
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return e.spec.Serve(t, e.h, req)
}

func TestCreateReport(t *testing.T) {
	reports := &fakeReports{}
	e := newReportsEnv(t, reports)
	res := e.postReport(t, "write", map[string]any{
		"account_id": reportTargetID, "status_ids": []string{"01890a5d-ac96-774b-bcce-b302099a7101"},
		"category": "violation", "rule_ids": []string{"01890a5d-ac96-774b-bcce-b302099a7201"}, "comment": "rude", "forward": true,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var got clientapi.Report
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.TargetAccount.Id != reportTargetID || got.TargetAccount.Acct != "bob@remote.example" || !got.Forwarded ||
		got.Category != clientapi.ReportCategoryViolation || got.Comment != "rude" ||
		!reflect.DeepEqual(got.StatusIds, []string{"01890a5d-ac96-774b-bcce-b302099a7101"}) ||
		!reflect.DeepEqual(got.RuleIds, []string{"01890a5d-ac96-774b-bcce-b302099a7201"}) {
		t.Errorf("report = %+v", got)
	}
	want := appmoderation.LocalReportInput{
		ReporterAccountID: aliceAccountID, TargetAccountID: reportTargetID, StatusIDs: []string{"01890a5d-ac96-774b-bcce-b302099a7101"},
		Category: moderation.CategoryViolation, RuleIDs: []string{"01890a5d-ac96-774b-bcce-b302099a7201"}, Comment: "rude", Forward: true,
	}
	if len(reports.got) != 1 || !reflect.DeepEqual(reports.got[0], want) {
		t.Errorf("filed %+v, want %+v", reports.got, want)
	}

	// The fewest fields: no statuses, rules, or comment.
	if res := e.postReport(t, "write", map[string]any{"account_id": reportTargetID, "category": "spam"}); res.StatusCode != http.StatusOK {
		t.Errorf("minimal report = %d, want 200", res.StatusCode)
	}
	if in := reports.got[1]; in.StatusIDs != nil || in.RuleIDs != nil || in.Comment != "" || in.Forward {
		t.Errorf("minimal report filed %+v", in)
	}
	if res := e.postReport(t, "read", map[string]any{"account_id": reportTargetID, "category": "spam"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("with the read scope = %d, want 403", res.StatusCode)
	}
}

func TestCreateReportProblems(t *testing.T) {
	for _, tc := range []struct {
		name, detail string
		err          error
	}{
		{"field", "status_ids の値が正しくありません。", &appmoderation.ReportFieldError{Field: "status_ids"}},
		{"count", "", &moderation.CountError{Field: "rule_ids", Max: 20}},
		{"too long", "", &moderation.TextError{Field: "comment", Max: 1000}},
		{"not text", "", &moderation.TextError{Field: "comment"}},
	} {
		e := newReportsEnv(t, &fakeReports{err: tc.err})
		res := e.postReport(t, "write", map[string]any{"account_id": reportTargetID, "category": "spam"})
		if res.StatusCode != http.StatusUnprocessableEntity || res.Header.Get("Content-Language") != "ja" {
			t.Errorf("%s: status = %d (%s), want 422 in ja", tc.name, res.StatusCode, res.Header.Get("Content-Language"))
			continue
		}
		p := decodeProblem(t, res)
		if p.Detail == nil || *p.Detail == "" || (tc.detail != "" && *p.Detail != tc.detail) {
			t.Errorf("%s: detail = %v, want %q", tc.name, p.Detail, tc.detail)
		}
	}
}

func TestCreateReportIsRateLimited(t *testing.T) {
	e := newReportsEnv(t, &fakeReports{})
	for i := range clientapi.ReportLimit.Max {
		if res := e.postReport(t, "write", map[string]any{"account_id": reportTargetID, "category": "spam"}); res.StatusCode != http.StatusOK {
			t.Fatalf("report %d = %d, want 200", i+1, res.StatusCode)
		}
	}
	res := e.postReport(t, "write", map[string]any{"account_id": reportTargetID, "category": "spam"})
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" {
		t.Errorf("one report too many = %d (Retry-After %q), want 429", res.StatusCode, res.Header.Get("Retry-After"))
	}
	e.clock.Advance(2 * clientapi.ReportLimit.Window)
	if res := e.postReport(t, "write", map[string]any{"account_id": reportTargetID, "category": "spam"}); res.StatusCode != http.StatusOK {
		t.Errorf("after the window = %d, want 200", res.StatusCode)
	}
}
