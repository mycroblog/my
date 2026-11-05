// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// The notes and the reopenings of fakeModeration, with the semantics of the moderation module.

func (f *fakeModeration) ReportDetail(_ context.Context, id string) (moderation.ReportDetail, error) {
	i, err := f.report(id)
	if err != nil {
		return moderation.ReportDetail{}, err
	}
	notes := []moderation.ReportNote{}
	for _, n := range f.notes {
		if n.ReportID == id {
			notes = append(notes, n)
		}
	}
	return moderation.ReportDetail{
		ReportRecord: f.reports[i], Notes: notes,
		History: moderation.TargetHistory{Reports: int64(len(f.reports) - 1), Actions: 4},
	}, nil
}

func (f *fakeModeration) AddReportNote(_ context.Context, id, body, by string) (moderation.ReportNote, error) {
	if strings.TrimSpace(body) == "" || utf8.RuneCountInString(body) > moderation.MaxReportNote {
		return moderation.ReportNote{}, moderation.ErrInvalidReportNote
	}
	if _, err := f.report(id); err != nil {
		return moderation.ReportNote{}, err
	}
	n := moderation.ReportNote{ID: f.id(), ReportID: id, AuthorUserID: by, Body: body, WrittenAt: f.now}
	f.notes = append(f.notes, n)
	return n, nil
}

func (f *fakeModeration) ReopenReport(_ context.Context, id, _ string) (moderation.ReportRecord, error) {
	i, err := f.report(id)
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	st := &f.reports[i].Status
	if st.State != moderation.ReportResolved {
		return moderation.ReportRecord{}, fmt.Errorf("%w: %s", appmoderation.ErrReportNotResolved, id)
	}
	st.State, st.Outcome, st.Since = moderation.ReportOpen, "", f.now
	return f.reports[i], nil
}

// ForwardReport forwards a report filed on this server; the fake takes every target as remote.
func (f *fakeModeration) ForwardReport(_ context.Context, id, byUserID string) (moderation.ReportRecord, error) {
	i, err := f.report(id)
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	r := &f.reports[i].Report
	if r.SentToTarget() {
		return f.reports[i], nil
	}
	if !r.Forwardable(true) {
		return moderation.ReportRecord{}, fmt.Errorf("%w: %s", appmoderation.ErrReportNotForwardable, id)
	}
	r.Forward = &moderation.ReportForward{ID: "forward-" + id, ReportID: id, ForwardedByUserID: byUserID, ForwardedAt: f.now}
	return f.reports[i], nil
}

func TestReportNotes(t *testing.T) {
	e, f := newModerationEnv(t)
	res := e.sendJSON(t, http.MethodGet, "/admin/reports/"+reportTwo, "admin-read", "")
	got := decodeJSON[clientapi.AdminReportDetail](t, res)
	if res.StatusCode != http.StatusOK || got.Notes == nil || len(got.Notes) != 0 ||
		got.TargetHistory != (clientapi.AdminReportTargetHistory{ReportsCount: 2, ActionsCount: 4}) {
		t.Fatalf("detail = %d %+v, want no notes, 2 other reports, 4 actions", res.StatusCode, got)
	}

	res = e.sendJSON(t, http.MethodPost, "/admin/reports/"+reportTwo+"/notes", "admin", `{"body":"Looks like a bot."}`)
	got = decodeJSON[clientapi.AdminReportDetail](t, res)
	if res.StatusCode != http.StatusOK || len(got.Notes) != 1 || got.Notes[0].Body != "Looks like a bot." ||
		got.Notes[0].AuthorAccountId == nil || *got.Notes[0].AuthorAccountId != aliceAccountID || !got.Notes[0].CreatedAt.Equal(adminNow) {
		t.Fatalf("add note = %d %+v", res.StatusCode, got)
	}
	if f.notes[0].AuthorUserID != userOf(aliceAccountID) {
		t.Errorf("note = %+v, want by alice", f.notes[0])
	}
	// A note from the command line has no author.
	f.notes = append(f.notes, moderation.ReportNote{ID: f.id(), ReportID: reportTwo, Body: "from the CLI", WrittenAt: adminNow})
	res = e.sendJSON(t, http.MethodGet, "/admin/reports/"+reportTwo, "admin-read", "")
	if got := decodeJSON[clientapi.AdminReportDetail](t, res); len(got.Notes) != 2 || got.Notes[1].AuthorAccountId != nil {
		t.Errorf("detail = %+v, want the CLI's note last without an author", got.Notes)
	}
	// The list of reports does not carry the notes.
	res = e.sendJSON(t, http.MethodGet, "/admin/reports", "admin-read", "")
	if body, _ := io.ReadAll(res.Body); strings.Contains(string(body), "Looks like a bot.") {
		t.Errorf("the list of reports shows a note: %s", body)
	}

	for _, body := range []string{`{"body":"   "}`, `{"body":"` + strings.Repeat("x", moderation.MaxReportNote+1) + `"}`} {
		res := e.send(t, http.MethodPost, "/admin/reports/"+reportTwo+"/notes", "admin", body)
		if res.StatusCode != http.StatusBadRequest && res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("note %.20s = %d, want 400 or 422", body, res.StatusCode)
		}
	}
	res = e.sendJSON(t, http.MethodPost, "/admin/reports/"+reportTwo+"/notes", "admin", `{"body":"  "}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "body") {
		t.Errorf("blank note = %d %+v, want 422 about body", res.StatusCode, p)
	}
	res = e.sendJSON(t, http.MethodPost, "/admin/reports/01890a5d-ac96-774b-bcce-b302099a4999/notes", "admin", `{"body":"lost"}`)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("note on an unknown report = %d, want 404", res.StatusCode)
	}
	if len(f.notes) != 2 {
		t.Errorf("notes = %+v, want the refused ones left out", f.notes)
	}
}

func TestReopenReport(t *testing.T) {
	e, f := newModerationEnv(t)
	res := e.sendJSON(t, http.MethodPost, "/admin/reports/"+reportOne+"/reopen", "admin", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Detail == nil || *p.Detail == "" {
		t.Errorf("reopen an open report = %d %+v, want 409", res.StatusCode, p)
	}
	if _, err := f.ResolveReport(t.Context(), reportOne, moderation.OutcomeDismissed, ""); err != nil {
		t.Fatal(err)
	}
	f.now = adminNow.Add(time.Hour)
	res = e.sendJSON(t, http.MethodPost, "/admin/reports/"+reportOne+"/reopen", "admin", "")
	got := decodeJSON[clientapi.AdminReportDetail](t, res)
	if res.StatusCode != http.StatusOK || got.State != clientapi.AdminReportStateOpen || got.Outcome != nil || !got.StateChangedAt.Equal(f.now) {
		t.Errorf("reopen = %d %+v, want open since %v", res.StatusCode, got, f.now)
	}
	res = e.sendJSON(t, http.MethodPost, "/admin/reports/01890a5d-ac96-774b-bcce-b302099a4999/reopen", "admin", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("reopen an unknown report = %d, want 404", res.StatusCode)
	}
}
