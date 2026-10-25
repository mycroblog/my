// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_billing_gcp

package platform_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pubsubv2 "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	billinggcp "github.com/mycroblog/my/internal/adapter/billing/gcp"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestBillingImportOfABudget imports billing_gcp_method = budget from a Pub/Sub subscription on
// pstest (ADR-0068 §7): the same notification received again, by later imports, does not add to
// the month's total.
func TestBillingImportOfABudget(t *testing.T) {
	t.Parallel()
	const (
		topic = "projects/my-billing/topics/billing-budget"
		sub   = "projects/my-billing/subscriptions/my-billing-budget"
	)
	srv := pstest.NewServer()
	t.Cleanup(func() { _ = srv.Close() })
	opts := []option.ClientOption{
		option.WithEndpoint(srv.Addr), option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCConnectionPool(1), // one loopback connection per client, not up to four
	}
	ctx := t.Context()
	c, err := pubsubv2.NewClient(ctx, "my-billing", opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if _, err := c.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: topic}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{Name: sub, Topic: topic, AckDeadlineSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	pacific, err := billinggcp.Location()
	if err != nil {
		t.Fatal(err)
	}
	y, m, _ := time.Now().In(pacific).Date()
	start := time.Date(y, m, 1, 0, 0, 0, 0, pacific).UTC().Format(time.RFC3339)
	notify := func(amount string) {
		data, _ := json.Marshal(map[string]any{
			"budgetDisplayName": "my", "costAmount": json.Number(amount), "costIntervalStart": start,
			"budgetAmount": 100, "budgetAmountType": "SPECIFIED_AMOUNT", "currencyCode": "USD",
		})
		srv.Publish(topic, data, map[string]string{"billingAccountId": "012345-6789AB-CDEF01", "budgetId": "b1", "schemaVersion": "1.0"})
		time.Sleep(10 * time.Millisecond) // a later publish time
	}

	src, err := billinggcp.NewBudget(ctx, billinggcp.BudgetOptions{Subscription: sub, PullWait: 2 * time.Second}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline", "MY_BILLING_SOURCES": "gcp",
		"MY_BILLING_GCP_METHOD": "budget", "MY_BILLING_GCP_BUDGET_SUBSCRIPTION": sub,
	})
	app, err := platform.NewApp(ctx, cfg, platform.AppOptions{
		Logger: slog.New(slog.DiscardHandler), BillingSources: map[string]appfinance.BillingSource{"gcp": src},
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close(); _ = src.Close() })

	notify("10.00")
	notify("12.34")
	app.Finance.Import(ctx, "gcp", "run-1")
	notify("12.34") // the same notification again
	app.Finance.Import(ctx, "gcp", "run-2")
	app.Finance.Import(ctx, "gcp", "run-3") // nothing new: the latest notification is read again

	admin, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+"/admin/billing_imports", nil)
	req.Header.Set("Authorization", "Bearer "+issueAccessToken(t, app, admin.User.ID, "read write admin"))
	res := apitest.Load(t).Serve(t, app.Handler(), req)
	var list clientapi.BillingImportList
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil || res.StatusCode != http.StatusOK || len(list.Items) != 1 {
		t.Fatalf("GET /admin/billing_imports = %d %+v, %v", res.StatusCode, list, err)
	}
	st := list.Items[0]
	if st.LastRun == nil || st.LastRun.Outcome != clientapi.BillingImportRunOutcomeSucceeded {
		t.Errorf("last run = %+v, want succeeded", st.LastRun)
	}
	if len(st.Months) != 1 || st.Months[0].Kind != "estimate" || len(st.Months[0].Totals) != 1 ||
		st.Months[0].Totals[0] != (clientapi.Money{Amount: "12.34", Currency: "USD"}) {
		t.Errorf("months = %+v, want the estimate of 12.34 USD once", st.Months)
	}
}
