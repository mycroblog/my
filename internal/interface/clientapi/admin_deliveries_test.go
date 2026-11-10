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

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const (
	deliveryOne   = "01900000-0000-7000-8000-00000000d001"
	deliveryTwo   = "01900000-0000-7000-8000-00000000d002"
	localSenderID = "01900000-0000-7000-8000-00000000a001"
	remoteActorID = "01900000-0000-7000-8000-00000000a002"
)

// fakeDeliveryLog stands in for appfederation.DeliveryLog: it keeps two deliveries to
// remote.example, the newer one masked, and records the queries and the reveals.
type fakeDeliveryLog struct {
	queries  []appfederation.DeliveryQuery
	revealed []string
}

func (f *fakeDeliveryLog) entries() []appfederation.DeliveryEntry {
	at := adminNow.Add(-time.Hour)
	return []appfederation.DeliveryEntry{
		{
			ID: deliveryTwo, Domain: "remote.example", ActivityType: federation.ActivityCreate, ActivityAt: at, CreatedAt: at,
			Mask: appfederation.DeliveryMaskDirect,
			Attempts: []federation.DeliveryAttempt{{
				ID: "01900000-0000-7000-8000-0000000000f1", DeliveryID: deliveryTwo, Outcome: federation.DeliveryRetryableFailure,
				Error: "connection refused", AttemptedAt: at,
			}},
		},
		{
			ID: deliveryOne, Domain: "remote.example", ActivityType: federation.ActivityAccept, ActivityAt: at, CreatedAt: at,
			SharedInbox: true, Sender: &appfederation.DeliverySender{AccountID: localSenderID, Username: "alice"},
			ActivityURI: "https://local.example/ap/activities/1", InboxURL: "https://remote.example/inbox",
			Attempts: []federation.DeliveryAttempt{{
				ID: "01900000-0000-7000-8000-0000000000f2", DeliveryID: deliveryOne, Outcome: federation.DeliverySucceeded,
				Scheme: "cavage", HTTPStatus: 202, AttemptedAt: at,
			}},
		},
	}
}

func (f *fakeDeliveryLog) List(_ context.Context, q appfederation.DeliveryQuery, _ string) ([]appfederation.DeliveryEntry, error) {
	f.queries = append(f.queries, q)
	switch q.ActorAccountID {
	case "", localSenderID:
	case remoteActorID:
		return nil, appfederation.ErrNotLocalAccount
	default:
		return nil, fmt.Errorf("account %s: %w", q.ActorAccountID, appaccount.ErrNotFound)
	}
	out := f.entries()
	if q.Before == deliveryTwo {
		out = out[1:]
	}
	if len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

func (f *fakeDeliveryLog) Reveal(_ context.Context, domain, id, _ string) (appfederation.RevealedDelivery, error) {
	if domain != "remote.example" || id != deliveryTwo {
		return appfederation.RevealedDelivery{}, fmt.Errorf("delivery %s: %w", id, appfederation.ErrNotFound)
	}
	f.revealed = append(f.revealed, id)
	return appfederation.RevealedDelivery{
		ID: id, Mask: appfederation.DeliveryMaskDirect, Sender: appfederation.DeliverySender{AccountID: localSenderID, Username: "alice"},
		ActivityURI: "https://local.example/ap/activities/2", InboxURL: "https://remote.example/users/bob/inbox",
	}, nil
}

func newDeliveryLogEnv(t *testing.T) (*authEnv, *fakeDeliveryLog) {
	t.Helper()
	log := &fakeDeliveryLog{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.DeliveryLog = log
	})
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, log
}

func TestListInstanceDeliveries(t *testing.T) {
	e, log := newDeliveryLogEnv(t)
	res := e.admin(t, http.MethodGet, "/admin/instances/Remote.Example/deliveries?limit=1", "admin-read", nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("status = %d, Cache-Control = %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	page := decodeJSON[clientapi.AdminDeliveryPage](t, res)
	if len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("first page = %+v", page)
	}
	masked := page.Items[0]
	if masked.Id != deliveryTwo || masked.Mask == nil || *masked.Mask != clientapi.AdminDeliveryMask("direct") ||
		masked.Sender != nil || masked.ActivityUri != nil || masked.InboxUrl != nil || len(masked.Attempts) != 1 ||
		masked.Attempts[0].Error == nil || masked.Attempts[0].StatusCode != nil {
		t.Errorf("masked delivery = %+v", masked)
	}
	if len(log.queries) != 1 || log.queries[0].Domain != "remote.example" || log.queries[0].Limit != 2 {
		t.Errorf("queries = %+v, want remote.example with one more than the limit", log.queries)
	}

	res = e.admin(t, http.MethodGet, "/admin/instances/remote.example/deliveries?limit=1&cursor="+*page.NextCursor, "admin-read", nil)
	rest := decodeJSON[clientapi.AdminDeliveryPage](t, res)
	if len(rest.Items) != 1 || rest.NextCursor != nil {
		t.Fatalf("second page = %+v", rest)
	}
	whole := rest.Items[0]
	if whole.Mask != nil || whole.Sender == nil || whole.Sender.AccountId != localSenderID || whole.Sender.Username == nil ||
		whole.ActivityUri == nil || whole.InboxUrl == nil || !whole.SharedInbox || len(whole.Attempts) != 1 ||
		whole.Attempts[0].StatusCode == nil || *whole.Attempts[0].StatusCode != 202 || whole.Attempts[0].SignatureScheme == nil {
		t.Errorf("delivery shown whole = %+v", whole)
	}

	res = e.admin(t, http.MethodGet, "/admin/instances/remote.example/deliveries?account_id="+localSenderID+"&activity_uri=https%3A%2F%2Flocal.example%2Fap%2Factivities%2F1", "admin-read", nil)
	if res.StatusCode != http.StatusOK {
		t.Errorf("filtered = %d", res.StatusCode)
	}
	if q := log.queries[len(log.queries)-1]; q.ActorAccountID != localSenderID || q.ActivityURI != "https://local.example/ap/activities/1" {
		t.Errorf("filtered query = %+v", q)
	}
}

// No response of the list or the reveal carries the body of an activity: the schemas have no
// field for it, and the handlers read none.
func TestInstanceDeliveriesCarryNoBody(t *testing.T) {
	e, _ := newDeliveryLogEnv(t)
	for _, op := range []struct{ method, path string }{
		{http.MethodGet, "/admin/instances/remote.example/deliveries"},
		{http.MethodPost, "/admin/instances/remote.example/deliveries/" + deliveryTwo + "/reveal"},
	} {
		res := e.admin(t, op.method, op.path, "admin-read", nil)
		b, err := io.ReadAll(res.Body)
		if err != nil || res.StatusCode != http.StatusOK {
			t.Fatalf("%s %s = %d, %v", op.method, op.path, res.StatusCode, err)
		}
		for _, key := range []string{`"document"`, `"body"`, `"object"`} {
			if strings.Contains(string(b), key) {
				t.Errorf("%s %s has %s: %s", op.method, op.path, key, b)
			}
		}
	}
}

func TestInstanceDeliveriesRefuse(t *testing.T) {
	e, log := newDeliveryLogEnv(t)
	for _, c := range []struct {
		name, method, path, token string
		status                    int
		detail                    string
	}{
		{"without manage_domains", http.MethodGet, "/admin/instances/remote.example/deliveries", "bob-admin", http.StatusForbidden, "manage_domains"},
		{"the reveal without manage_domains", http.MethodPost, "/admin/instances/remote.example/deliveries/" + deliveryTwo + "/reveal", "bob-admin", http.StatusForbidden, "manage_domains"},
		{"a remote account", http.MethodGet, "/admin/instances/remote.example/deliveries?account_id=" + remoteActorID, "admin-read", http.StatusBadRequest, "account_id"},
		{"an unknown account", http.MethodGet, "/admin/instances/remote.example/deliveries?account_id=01900000-0000-7000-8000-00000000a0ff", "admin-read", http.StatusNotFound, ""},
		{"a malformed domain", http.MethodGet, "/admin/instances/bad%20host/deliveries", "admin-read", http.StatusBadRequest, "domain"},
		{"a broken cursor", http.MethodGet, "/admin/instances/remote.example/deliveries?cursor=%25", "admin-read", http.StatusBadRequest, "cursor"},
		{"a delivery to another server", http.MethodPost, "/admin/instances/elsewhere.example/deliveries/" + deliveryTwo + "/reveal", "admin-read", http.StatusNotFound, ""},
		{"an unknown delivery", http.MethodPost, "/admin/instances/remote.example/deliveries/" + deliveryOne + "/reveal", "admin-read", http.StatusNotFound, ""},
	} {
		res := e.admin(t, c.method, c.path, c.token, map[string]string{"Accept-Language": "en"})
		p := decodeProblem(t, res)
		if res.StatusCode != c.status || c.detail != "" && (p.Detail == nil || !strings.Contains(*p.Detail, c.detail)) {
			t.Errorf("%s: %d %+v, want %d naming %q", c.name, res.StatusCode, p, c.status, c.detail)
		}
	}
	if len(log.revealed) != 0 {
		t.Errorf("revealed %v, want nothing", log.revealed)
	}
}

func TestRevealInstanceDelivery(t *testing.T) {
	e, log := newDeliveryLogEnv(t)
	res := e.admin(t, http.MethodPost, "/admin/instances/remote.example/deliveries/"+deliveryTwo+"/reveal", "admin-read", nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("status = %d", res.StatusCode)
	}
	got := decodeJSON[clientapi.AdminDeliveryReveal](t, res)
	if got.Id != deliveryTwo || got.Sender.AccountId != localSenderID || got.ActivityUri == "" || got.InboxUrl != "https://remote.example/users/bob/inbox" ||
		got.Mask == nil || *got.Mask != clientapi.AdminDeliveryMask("direct") {
		t.Errorf("revealed = %+v", got)
	}
	if len(log.revealed) != 1 {
		t.Errorf("revealed %v, want the delivery once", log.revealed)
	}
}
