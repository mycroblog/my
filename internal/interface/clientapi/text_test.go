// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"net/http"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
)

// TestBodyTextIsCheckedBeforeTheHandler checks that a NUL anywhere in a JSON body is a 422
// that names the property, before the handler looks the value up.
func TestBodyTextIsCheckedBeforeTheHandler(t *testing.T) {
	called := false
	e := newStatusEnv(t, &fakeStatuses{create: func(appcontent.CreateInput) (appcontent.Post, error) {
		called = true
		return appcontent.Post{}, nil
	}})
	for body, field := range map[string]string{
		`{"text":"hi","visibility":"public","in_reply_to_id":"\u0000"}`: "in_reply_to_id",
		`{"text":"hi","visibility":"public","media":[{"id":"01890a5d-ac96-774b-bcce-b302099a8301"},{"id":"01890a5d-ac96-774b-bcce-b302099a8302","description":"a\u0000"}]}`: "media[1].description",
	} {
		res := e.send(t, http.MethodPost, "/statuses", "write", body)
		p := decodeProblem(t, res)
		want := field + " の値が正しくありません。"
		if res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || *p.Detail != want {
			t.Errorf("POST %s = %d %+v, want 422 %q", body, res.StatusCode, p, want)
		}
	}
	if called {
		t.Error("the handler ran on a body with a NUL")
	}
}
