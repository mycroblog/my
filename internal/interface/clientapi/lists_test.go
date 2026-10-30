// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/timeline"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// failingLists fails every operation with err.
type failingLists struct{ err error }

func (f failingLists) CreateList(context.Context, string, string) (apptimeline.List, error) {
	return apptimeline.List{}, f.err
}

func (f failingLists) RenameList(context.Context, string, string, string) (apptimeline.List, error) {
	return apptimeline.List{}, f.err
}
func (f failingLists) DeleteList(context.Context, string, string) error { return f.err }
func (f failingLists) List(context.Context, string, string) (apptimeline.List, error) {
	return apptimeline.List{}, f.err
}
func (f failingLists) Lists(context.Context, string) ([]apptimeline.List, error) { return nil, f.err }
func (f failingLists) ListsWithMember(context.Context, string, string) ([]apptimeline.List, error) {
	return nil, f.err
}
func (f failingLists) AddListMember(context.Context, string, string, string) error    { return f.err }
func (f failingLists) RemoveListMember(context.Context, string, string, string) error { return f.err }
func (f failingLists) ListMembers(context.Context, string, string, string, int) ([]string, error) {
	return nil, f.err
}

func (f failingLists) ListTimeline(context.Context, appcontent.Viewer, string, apptimeline.Query) (apptimeline.Page, error) {
	return apptimeline.Page{}, f.err
}

func TestListProblems(t *testing.T) {
	const list = "/lists/01890a5d-ac96-774b-bcce-b302099a8300"
	member := list + "/accounts/" + bobAccountID
	for _, tc := range []struct {
		err          error
		method, path string
		body         string
		status       int
		problem      string
	}{
		{timeline.ErrInvalidListName, http.MethodPost, "/lists", `{"name":"x"}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeBlank},
		{timeline.ErrTooManyLists, http.MethodPost, "/lists", `{"name":"x"}`, http.StatusUnprocessableEntity, clientapi.ProblemTypeTooManyLists},
		{timeline.ErrTooManyListMembers, http.MethodPost, member, "", http.StatusUnprocessableEntity, clientapi.ProblemTypeTooManyListMembers},
		{timeline.ErrNotFollowed, http.MethodPost, member, "", http.StatusUnprocessableEntity, clientapi.ProblemTypeNotFollowed},
		{apptimeline.ErrNotFound, http.MethodGet, list, "", http.StatusNotFound, clientapi.ProblemTypeBlank},
		{apptimeline.ErrNotFound, http.MethodPatch, list, `{"name":"x"}`, http.StatusNotFound, clientapi.ProblemTypeBlank},
		{apptimeline.ErrNotFound, http.MethodDelete, list, "", http.StatusNotFound, clientapi.ProblemTypeBlank},
		{apptimeline.ErrNotFound, http.MethodGet, list + "/accounts", "", http.StatusNotFound, clientapi.ProblemTypeBlank},
		{apptimeline.ErrNotFound, http.MethodDelete, member, "", http.StatusNotFound, clientapi.ProblemTypeBlank},
		{apptimeline.ErrNotFound, http.MethodGet, "/timelines/list/01890a5d-ac96-774b-bcce-b302099a8300", "", http.StatusNotFound, clientapi.ProblemTypeBlank},
	} {
		e := newAuthEnv(t, func(o *clientapi.Options) { o.Lists = failingLists{err: tc.err} })
		token := "write"
		if tc.method == http.MethodGet {
			token = "read"
		}
		res := e.sendJSON(t, tc.method, tc.path, token, tc.body)
		if res.StatusCode != tc.status {
			t.Errorf("%s %s with %v = %d, want %d", tc.method, tc.path, tc.err, res.StatusCode, tc.status)
			continue
		}
		if p := decodeProblem(t, res); p.Type != tc.problem {
			t.Errorf("%s %s with %v: type %q, want %q", tc.method, tc.path, tc.err, p.Type, tc.problem)
		}
	}
	// A path ID that is not a canonical UUIDv7 is a 404 before the handler.
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Lists = failingLists{} })
	if res := e.send(t, http.MethodPost, list+"/accounts/not-an-id", "write", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("a malformed account_id = %d, want 404", res.StatusCode)
	}
}
