// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeThreadMutes keeps the statuses whose thread each account mutes.
type fakeThreadMutes struct {
	muted map[string]map[string]bool
	reads int
}

func (f *fakeThreadMutes) Mute(_ context.Context, accountID, statusID string) error {
	if f.muted[accountID] == nil {
		f.muted[accountID] = map[string]bool{}
	}
	f.muted[accountID][statusID] = true
	return nil
}

func (f *fakeThreadMutes) Unmute(_ context.Context, accountID, statusID string) error {
	delete(f.muted[accountID], statusID)
	return nil
}

func (f *fakeThreadMutes) MutedAmong(_ context.Context, accountIDs, statusIDs []string) (map[string]map[string]bool, error) {
	f.reads++
	out := map[string]map[string]bool{}
	for _, a := range accountIDs {
		for _, s := range statusIDs {
			if f.muted[a][s] {
				if out[a] == nil {
					out[a] = map[string]bool{}
				}
				out[a][s] = true
			}
		}
	}
	return out, nil
}

// viewerView is sampleView with the viewer's account in its relation, as the reader gives it.
func viewerView(v appcontent.Viewer, _ string) (appcontent.StatusView, error) {
	view := sampleView(v)
	if view.Relation != nil {
		view.Relation.AccountID = v.AccountID
	}
	return view, nil
}

// TestMuteStatusThread checks that POST and DELETE /statuses/{id}/mute mute and unmute the
// thread of a status the viewer sees, and answer the status with muted; that
// an anonymous status has no muted; and that a server without thread mutes answers 404.
func TestMuteStatusThread(t *testing.T) {
	mutes := &fakeThreadMutes{muted: map[string]map[string]bool{}}
	f := &fakeStatuses{view: viewerView}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Statuses, o.StatusReader, o.Interactions = f, f, f
		o.ThreadMutes = mutes
	})
	path := "/statuses/" + sampleStatusID + "/mute"

	res := e.send(t, http.MethodPost, path, "write", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST mute = %d", res.StatusCode)
	}
	if s := decodeStatus(t, res); s.Muted == nil || !*s.Muted {
		t.Errorf("muted after POST = %v, want true", s.Muted)
	}
	if !mutes.muted[aliceAccountID][sampleStatusID] {
		t.Errorf("mutes = %v, want the viewer's of the status", mutes.muted)
	}
	res = e.send(t, http.MethodGet, "/statuses/"+sampleStatusID, "read", "")
	if s := decodeStatus(t, res); s.Muted == nil || !*s.Muted {
		t.Errorf("muted of GET = %v, want true", s.Muted)
	}
	res = e.send(t, http.MethodGet, "/statuses/"+sampleStatusID, "", "")
	if s := decodeStatus(t, res); s.Muted != nil {
		t.Errorf("muted of an anonymous GET = %v, want none", *s.Muted)
	}

	res = e.send(t, http.MethodDelete, path, "write", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("DELETE mute = %d", res.StatusCode)
	}
	if s := decodeStatus(t, res); s.Muted == nil || *s.Muted {
		t.Errorf("muted after DELETE = %v, want false", s.Muted)
	}

	if res := e.send(t, http.MethodPost, path, "read", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("POST mute with read only = %d, want 403", res.StatusCode)
	}

	without := newStatusEnv(t, f)
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		if res := without.send(t, method, path, "write", ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s mute without thread mutes = %d, want 404", method, res.StatusCode)
		}
	}
}
