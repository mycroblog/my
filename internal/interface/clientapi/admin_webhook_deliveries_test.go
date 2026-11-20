// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/webhook"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const (
	hookID       = "01890a5d-ac96-774b-bcce-b302099a6101"
	reportHookID = "01890a5d-ac96-774b-bcce-b302099a6102"
	statusHookID = "01890a5d-ac96-774b-bcce-b302099a6103"
	testHookID   = "01890a5d-ac96-774b-bcce-b302099a6104"
)

// deliveryWebhooks is one endpoint with a delivery of a report, of a post, and of a test, each
// with its message's body; it implements only the operations on deliveries.
type deliveryWebhooks struct {
	clientapi.Webhooks
}

var hookDeliveries = []webhook.DeliveryRecord{
	{Delivery: webhook.Delivery{ID: testHookID, MessageID: testHookID, EndpointID: hookID, CreatedAt: adminNow},
		EventType: webhook.EventTest, Body: []byte(`{"type":"webhook.test"}`)},
	{Delivery: webhook.Delivery{ID: statusHookID, MessageID: statusHookID, EndpointID: hookID, CreatedAt: adminNow},
		EventType: webhook.EventStatusCreated, Body: []byte(`{"type":"status.created"}`)},
	{Delivery: webhook.Delivery{ID: reportHookID, MessageID: reportHookID, EndpointID: hookID, CreatedAt: adminNow},
		EventType: webhook.EventReportCreated, Body: []byte(`{"type":"report.created","comment":"spam"}`)},
}

func (deliveryWebhooks) Deliveries(context.Context, string, string, int) ([]webhook.DeliveryRecord, error) {
	return hookDeliveries, nil
}

func (deliveryWebhooks) Redeliver(_ context.Context, _, id string) (webhook.DeliveryRecord, error) {
	for _, d := range hookDeliveries {
		if d.Delivery.ID == id {
			return d, nil
		}
	}
	return webhook.DeliveryRecord{}, nil
}

func (deliveryWebhooks) Test(context.Context, string) (webhook.DeliveryRecord, error) {
	return hookDeliveries[0], nil
}

// bodies returns the request body of each delivery by its ID, "-" for one without it.
func bodies(items []clientapi.AdminWebhookDelivery) map[string]string {
	out := map[string]string{}
	for _, d := range items {
		out[d.Id] = "-"
		if d.RequestBody != nil {
			out[d.Id] = *d.RequestBody
		}
	}
	return out
}

func TestWebhookDeliveriesShowTheBodyToWhoMayReadItsEvent(t *testing.T) {
	report := `{"type":"report.created","comment":"spam"}`
	for _, tc := range []struct {
		name   string
		perms  []identity.Permission
		token  string
		report string
	}{
		{"every permission", identity.AllPermissions(), "carol-admin", report},
		{"without manage_reports", allPermissionsBut(identity.PermManageReports), "carol-admin", "-"},
		{"a token of admin:manage_webhooks", identity.AllPermissions(), "webhooks", "-"},
		{"a token of admin:manage_reports admin:manage_webhooks", identity.AllPermissions(), "webhooks-reports", report},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := newPermissionScopeEnv(t, tc.perms, func(o *clientapi.Options) { o.Webhooks = deliveryWebhooks{} })
			e.auth.tokens["carol-admin"] = bearer("admin", carolAccountID)
			want := map[string]string{testHookID: `{"type":"webhook.test"}`, statusHookID: `{"type":"status.created"}`, reportHookID: tc.report}

			res := e.admin(t, http.MethodGet, "/admin/webhooks/"+hookID+"/deliveries", tc.token, nil)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("list = %d %+v", res.StatusCode, decodeProblem(t, res))
			}
			got := bodies(decodeJSON[clientapi.AdminWebhookDeliveryPage](t, res).Items)
			for id, body := range want {
				if got[id] != body {
					t.Errorf("the body of %s in the list = %s, want %s", id, got[id], body)
				}
			}

			res = e.send(t, http.MethodPost, "/admin/webhooks/"+hookID+"/deliveries/"+reportHookID+"/redeliver", tc.token, "")
			if res.StatusCode != http.StatusAccepted {
				t.Fatalf("redeliver = %d %+v", res.StatusCode, decodeProblem(t, res))
			}
			if got := bodies([]clientapi.AdminWebhookDelivery{decodeJSON[clientapi.AdminWebhookDelivery](t, res)}); got[reportHookID] != tc.report {
				t.Errorf("the body of the delivery sent again = %s, want %s", got[reportHookID], tc.report)
			}

			res = e.send(t, http.MethodPost, "/admin/webhooks/"+hookID+"/test", tc.token, "")
			if res.StatusCode != http.StatusAccepted {
				t.Fatalf("test = %d %+v", res.StatusCode, decodeProblem(t, res))
			}
			if got := bodies([]clientapi.AdminWebhookDelivery{decodeJSON[clientapi.AdminWebhookDelivery](t, res)}); got[testHookID] != want[testHookID] {
				t.Errorf("the body of the test = %s, want %s", got[testHookID], want[testHookID])
			}
		})
	}
}
