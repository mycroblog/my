// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// singleUserOperations are the operations that a one-person server does not offer (ADR-0152 §4).
var singleUserOperations = map[string]bool{
	"getLocalTimeline": true, "getFederatedTimeline": true, "streamTimeline": true,
	"listAccountSuggestions": true,
	"listOwnInvites":         true, "createOwnInvite": true, "revokeOwnInvite": true,
	"listInvites": true, "createInvite": true, "revokeInvite": true,
	"listPendingRegistrations": true, "approveRegistration": true, "rejectRegistration": true,
	"listSignupBlocks": true, "createSignupBlock": true, "getSignupBlock": true, "liftSignupBlock": true,
	"importSignupBlocks": true,
}

func singleUserSetting(on bool) instance.CurrentSetting {
	v := "false"
	if on {
		v = "true"
	}
	return instance.CurrentSetting{Key: instance.SettingSingleUser, RevisionID: testkit.NamedID("revision-single-user"), Value: v}
}

// TestSingleUserModeRefusesOperations sends every operation of the spec to a one-person server:
// those it does not offer answer the 404 single-user-mode, and no other operation does.
func TestSingleUserModeRefusesOperations(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.RateLimiter, o.Voice = nil, &fakeVoiceRooms{}
		o.Instance = &fakeInstance{settings: []instance.CurrentSetting{singleUserSetting(true)}}
	})
	e.auth.tokens["all"] = bearer("read write follow push admin", aliceAccountID)
	doc := e.spec.Doc()
	seen := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			url := clientapi.BasePath + strings.NewReplacer("{id}", postID, "{timeline}", "local", "{", "", "}", "").Replace(path)
			if !singleUserOperations[op.OperationID] && strings.HasPrefix(path, "/streaming") {
				continue // an open stream does not end
			}
			req := httptest.NewRequest(method, url, strings.NewReader("{}"))
			req.Header.Set("Authorization", "Bearer all")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			// The handler of an operation that is served may panic on the fakes of this env.
			func() {
				defer func() { _ = recover() }()
				e.h.ServeHTTP(rec, req)
			}()
			refused := rec.Code == http.StatusNotFound && strings.Contains(rec.Body.String(), clientapi.ProblemTypeSingleUserMode)
			if singleUserOperations[op.OperationID] {
				seen[op.OperationID] = true
				if !refused {
					t.Errorf("%s %s (%s) on a one-person server = %d %s, want the 404 single-user-mode", method, path, op.OperationID, rec.Code, rec.Body)
				}
			} else if refused {
				t.Errorf("%s %s (%s) on a one-person server = the 404 single-user-mode, want it served", method, path, op.OperationID)
			}
		}
	}
	for id := range singleUserOperations {
		if !seen[id] {
			t.Errorf("operation %s is not in the spec", id)
		}
	}
}

// TestSingleUserModeTimelines checks the answers of the public timelines on a one-person server, in
// the spec, and that they work again once the mode is off, the stored settings applying again.
func TestSingleUserModeTimelines(t *testing.T) {
	everyone := []instance.CurrentSetting{
		{Key: instance.SettingLocalTimelineVisibility, RevisionID: testkit.NamedID("revision-1"), Value: "everyone"},
		{Key: instance.SettingFederatedTimelineVisibility, RevisionID: testkit.NamedID("revision-2"), Value: "everyone"},
	}
	on := timelineAccessEnv(t, append(everyone, singleUserSetting(true))...)
	for _, path := range []string{"/timelines/local", "/timelines/federated"} {
		for _, auth := range []map[string]string{{"Accept-Language": "ja"}, {"Authorization": "Bearer read"}} {
			res := on.get(t, path, auth)
			if res.StatusCode != http.StatusNotFound {
				t.Fatalf("%s on a one-person server = %d, want 404", path, res.StatusCode)
			}
			if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeSingleUserMode || p.Title != "このサーバーは一人のためのサーバーで、この機能はありません。" {
				t.Errorf("%s problem = %+v", path, p)
			}
		}
	}
	// The home and hashtag timelines stay.
	decodePage(t, on.get(t, "/timelines/home", map[string]string{"Authorization": "Bearer read"}))
	if len(on.timelines.calls) != 1 {
		t.Errorf("calls = %+v, want only the home timeline", on.timelines.calls)
	}

	off := timelineAccessEnv(t, append(everyone, singleUserSetting(false))...)
	decodePage(t, off.get(t, "/timelines/local", nil))
	decodePage(t, off.get(t, "/timelines/federated", nil))
}

// TestGetInstanceOnSingleUserServer checks what GET /instance says of a one-person server: the
// effective registration mode and timelines, and single_user, so that clients that do not know
// the field hide the timelines and the registration too (ADR-0152 §6).
func TestGetInstanceOnSingleUserServer(t *testing.T) {
	stored := []instance.CurrentSetting{
		{Key: instance.SettingRegistrationMode, RevisionID: testkit.NamedID("revision-1"), Value: "open"},
		{Key: instance.SettingLocalTimelineVisibility, RevisionID: testkit.NamedID("revision-2"), Value: "everyone"},
	}
	get := func(t *testing.T, settings ...instance.CurrentSetting) clientapi.Instance {
		t.Helper()
		e := newAuthEnv(t, func(o *clientapi.Options) { o.Instance = &fakeInstance{settings: settings} })
		res := e.get(t, "/instance", nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		var got clientapi.Instance
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	on := get(t, append(stored, singleUserSetting(true))...)
	if !on.SingleUser || on.Registrations.Mode != clientapi.InstanceRegistrationsModeClosed ||
		on.Timelines != (clientapi.InstanceTimelines{Local: clientapi.InstanceAudienceOff, Federated: clientapi.InstanceAudienceOff}) {
		t.Errorf("one-person server = single_user %v, registrations %+v, timelines %+v", on.SingleUser, on.Registrations, on.Timelines)
	}
	off := get(t, append(stored, singleUserSetting(false))...)
	if off.SingleUser || off.Registrations.Mode != clientapi.InstanceRegistrationsModeOpen || off.Timelines.Local != clientapi.InstanceAudienceEveryone {
		t.Errorf("after the mode = single_user %v, registrations %+v, timelines %+v, want the stored settings", off.SingleUser, off.Registrations, off.Timelines)
	}
}
