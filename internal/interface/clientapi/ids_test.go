// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// malformedIDs are forms of an ID that PostgreSQL's uuid type accepts or that are no UUID at
// all; none of them is the canonical UUIDv7 form (ADR-0078 §5).
var malformedIDs = map[string]string{
	"uppercase":  strings.ToUpper(notificationID(2)),
	"no hyphens": strings.ReplaceAll(notificationID(2), "-", ""),
	"braces":     "{" + notificationID(2) + "}",
	"urn":        "urn:uuid:" + notificationID(2),
	"uuidv4":     "3f2b8c1e-9d4a-4e6b-8a3c-1f2e3d4c5b6a",
	"garbage":    "not-an-id",
	"quote":      "x'--",
}

// usersOfAccounts remembers the account each userOf ID was derived from.
var usersOfAccounts sync.Map

// userOf returns the user ID of the local account accountID: a stable UUIDv7 per account.
func userOf(accountID string) string {
	userID := testkit.NamedID("user-" + accountID)
	usersOfAccounts.Store(userID, accountID)
	return userID
}

// accountOfUser is the inverse of userOf for the IDs it has handed out.
func accountOfUser(userID string) (string, bool) {
	accountID, ok := usersOfAccounts.Load(userID)
	if !ok {
		return "", false
	}
	return accountID.(string), true
}

// answer is what a client sees of a response.
type answer struct {
	status int
	header map[string]string
	body   string
}

func answerOf(t *testing.T, res *http.Response) answer {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	a := answer{status: res.StatusCode, header: map[string]string{}, body: string(b)}
	for _, h := range []string{"Content-Type", "Content-Language", "Cache-Control", "Vary"} {
		a.header[h] = strings.Join(res.Header.Values(h), ",")
	}
	return a
}

func (a answer) equal(b answer) bool {
	if a.status != b.status || a.body != b.body {
		return false
	}
	for k, v := range a.header {
		if b.header[k] != v {
			return false
		}
	}
	return true
}

// TestMalformedPathID: an ID in the path that is not canonical answers exactly as an ID of
// nothing does, with the same headers and body (ADR-0078 §5, ADR-0018).
func TestMalformedPathID(t *testing.T) {
	e, _ := newNotificationEnv(t)
	missing := answerOf(t, e.send(t, http.MethodPost, "/notifications/"+notificationID(9)+"/dismiss", "write", ""))
	if missing.status != http.StatusNotFound {
		t.Fatalf("missing notification = %d, want 404", missing.status)
	}
	for name, id := range malformedIDs {
		t.Run(name, func(t *testing.T) {
			got := answerOf(t, e.send(t, http.MethodPost, "/notifications/"+url.PathEscape(id)+"/dismiss", "write", ""))
			if !got.equal(missing) {
				t.Errorf("answer = %+v, want that of a missing notification %+v", got, missing)
			}
		})
	}
	// The scope is checked first, so a malformed ID shows no more than a missing one.
	missing = answerOf(t, e.send(t, http.MethodPost, "/notifications/"+notificationID(9)+"/dismiss", "read", ""))
	got := answerOf(t, e.send(t, http.MethodPost, "/notifications/NOT-AN-ID/dismiss", "read", ""))
	if missing.status != http.StatusForbidden || !got.equal(missing) {
		t.Errorf("without the scope: answer = %+v, want %+v", got, missing)
	}
}

// TestMalformedBodyID: an ID in the body that is not canonical answers as an ID of nothing.
func TestMalformedBodyID(t *testing.T) {
	e, _ := newNotificationEnv(t)
	body := func(id string) string { return `{"last_read_id":"` + id + `"}` }
	missing := answerOf(t, e.send(t, http.MethodPut, "/notifications/marker", "write", body(notificationID(9))))
	if missing.status != http.StatusNotFound {
		t.Fatalf("missing notification = %d, want 404", missing.status)
	}
	for name, id := range malformedIDs {
		t.Run(name, func(t *testing.T) {
			got := answerOf(t, e.send(t, http.MethodPut, "/notifications/marker", "write", body(id)))
			if !got.equal(missing) {
				t.Errorf("answer = %+v, want that of a missing notification %+v", got, missing)
			}
		})
	}
}

// TestMalformedReferenceIDs: the IDs a new post refers to (the post replied to, the media) reach
// the application layer in a form that names nothing when they are not canonical, so that no
// adapter sees them.
func TestMalformedReferenceIDs(t *testing.T) {
	const unknown = "00000000-0000-7000-8000-000000000000"
	for name, id := range malformedIDs {
		t.Run(name, func(t *testing.T) {
			var got appcontent.CreateInput
			f := &fakeStatuses{
				create: func(in appcontent.CreateInput) (appcontent.Post, error) {
					got = in
					return appcontent.Post{Status: content.Status{ID: sampleStatusID}}, nil
				},
				view: func(v appcontent.Viewer, _ string) (appcontent.StatusView, error) { return sampleView(v), nil },
			}
			e := newStatusEnv(t, f)
			body := `{"text":"hi","visibility":"local","in_reply_to_id":"` + id + `","media":[{"id":"` + id + `"},{"id":"` + sampleMediaID + `"}]}`
			if res := e.send(t, http.MethodPost, "/statuses", "write", body); res.StatusCode != http.StatusCreated {
				t.Fatalf("POST = %d", res.StatusCode)
			}
			if got.InReplyToID != unknown || len(got.Media) != 2 || got.Media[0].MediaID != unknown || got.Media[1].MediaID != sampleMediaID {
				t.Errorf("CreateInput = %+v, want the malformed IDs replaced by an ID of nothing", got)
			}
		})
	}
}

// TestMalformedCursor: a cursor whose content is not a canonical ID is a broken cursor, whether
// the cursor is the ID itself or the base64 of it.
func TestMalformedCursor(t *testing.T) {
	e, _ := newNotificationEnv(t)
	for name, id := range malformedIDs {
		t.Run(name, func(t *testing.T) {
			res := e.send(t, http.MethodGet, "/notifications?cursor="+url.QueryEscape(id), "read", "")
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", res.StatusCode)
			}
			if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeBlank || p.Detail == nil {
				t.Errorf("problem = %+v, want an invalid parameter", p)
			}
		})
	}
}
