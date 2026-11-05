// SPDX-License-Identifier: AGPL-3.0-only

package contract_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/contract"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/pkg/testkit"
)

func run(t *testing.T, name string, d kind.Destination) map[string]contract.Result {
	t.Helper()
	k, ok := kind.ByName(name)
	if !ok {
		t.Fatalf("no kind %q", name)
	}
	conn, err := client.New(client.Options{Kind: k, Destination: d, KeepDetail: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	protocol := d.Protocol
	if protocol == "" {
		protocol = k.Protocol
	}
	var reported []contract.Result
	results := contract.Run(t.Context(), k, contract.Env{Conn: conn, Protocol: protocol}, func(r contract.Result) { reported = append(reported, r) })
	if len(reported) != len(results) {
		t.Errorf("reported %d of %d results", len(reported), len(results))
	}
	out := map[string]contract.Result{}
	for _, r := range results {
		out[r.Case] = r
	}
	return out
}

func TestAWellBehavedPluginPasses(t *testing.T) {
	connectFake, _ := fakeplugin.Connect(fakeplugin.Options{})
	jsonFake, _ := fakeplugin.JSON(fakeplugin.Options{})
	for _, tc := range []struct {
		name string
		h    http.Handler
		d    kind.Destination
	}{
		{"connect json", connectFake, kind.Destination{Protocol: kind.ProtocolJSON}},
		{"connect proto", connectFake, kind.Destination{Protocol: kind.ProtocolProto}},
		{"connect grpc", connectFake, kind.Destination{Protocol: kind.ProtocolGRPC}},
		{"net/http and JSON", jsonFake, kind.Destination{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.d.Address = fakeplugin.ServeUnix(t, tc.h)
			results := run(t, kind.Probe, tc.d)
			for _, name := range []string{"reachable", "echo", "echo_empty"} {
				if results[name].Outcome != contract.Pass {
					t.Errorf("%s: %+v", name, results[name])
				}
			}
		})
	}
}

func TestEachCaseReportsItsFailure(t *testing.T) {
	broken, _ := fakeplugin.JSON(fakeplugin.Options{Broken: true})
	results := run(t, kind.Probe, kind.Destination{Address: fakeplugin.ServeTCP(t, broken)})
	if results["reachable"].Outcome != contract.Pass {
		t.Errorf("reachable: %+v", results["reachable"])
	}
	for _, name := range []string{"echo", "echo_empty"} {
		if r := results[name]; r.Outcome != contract.Fail || r.Detail == "" {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

func TestTheWrongProtocolFails(t *testing.T) {
	jsonFake, _ := fakeplugin.JSON(fakeplugin.Options{})
	results := run(t, kind.Probe, kind.Destination{Address: fakeplugin.ServeTCP(t, jsonFake), Protocol: kind.ProtocolProto})
	if r := results["echo"]; r.Outcome != contract.Fail || r.Detail == "" {
		t.Errorf("echo: %+v", r)
	}
}

func TestAnUnreachablePluginEndsTheRun(t *testing.T) {
	results := run(t, kind.Probe, kind.Destination{Address: "unix:///does/not/exist.sock"})
	if len(results) != 1 || results["reachable"].Outcome != contract.Fail {
		t.Errorf("results = %+v", results)
	}
}

func TestRefusedCredentialsFail(t *testing.T) {
	guarded, _ := fakeplugin.Connect(fakeplugin.Options{Token: "right"})
	results := run(t, kind.Probe, kind.Destination{Address: fakeplugin.ServeUnix(t, guarded), Token: "wrong"})
	if r := results["reachable"]; r.Outcome != contract.Fail {
		t.Errorf("reachable: %+v", r)
	}
	results = run(t, kind.Probe, kind.Destination{Address: fakeplugin.ServeUnix(t, guarded), Token: "right"})
	if r := results["echo"]; r.Outcome != contract.Pass {
		t.Errorf("echo: %+v", r)
	}
}

func TestAKindWithoutCasesSaysSo(t *testing.T) {
	h, _ := fakeplugin.Connect(fakeplugin.Options{})
	results := run(t, kind.KeyManager, kind.Destination{Address: fakeplugin.ServeUnix(t, h)})
	if results["reachable"].Outcome != contract.Pass {
		t.Errorf("reachable: %+v", results["reachable"])
	}
	if r := results[kind.KeyManager]; r.Outcome != contract.Skip || r.Detail == "" {
		t.Errorf("%s: %+v", kind.KeyManager, r)
	}
}

func runBilling(t *testing.T, o fakeplugin.Options, billed bool) map[string]contract.Result {
	t.Helper()
	k, _ := kind.ByName(kind.Billing)
	h, _ := fakeplugin.JSON(o)
	conn, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, KeepDetail: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	out := map[string]contract.Result{}
	contract.Run(t.Context(), k, contract.Env{Conn: conn, Protocol: k.Protocol, Billed: billed}, func(r contract.Result) { out[r.Case] = r })
	return out
}

// The billing kind runs what may cost money only when asked, and a plugin's mistake fails its case.
func TestBillingCases(t *testing.T) {
	provider := testkit.NewFakeBillingSource(appfinance.SourcePlugin, time.UTC)
	month := finance.YearMonthOf(finance.DateOf(time.Now().UTC()))
	provider.SetMonth(month, testkit.FakeBillingMonth{Rows: []testkit.FakeBillingRow{
		{Service: "Hetzner Cloud", ChargeType: finance.ChargeUsage, Amount: "1.25", Currency: "USD"},
	}})

	results := runBilling(t, fakeplugin.Options{Billing: provider}, false)
	if r := results["fetch_without_requests"]; r.Outcome != contract.Pass {
		t.Errorf("fetch_without_requests: %+v", r)
	}
	if r := results["fetch_billed"]; r.Outcome != contract.Skip || !strings.Contains(r.Detail, "--billed") {
		t.Errorf("fetch_billed without --billed: %+v", r)
	}
	if n := provider.Calls(); n != 0 {
		t.Errorf("the default cases made %d billed requests", n)
	}

	results = runBilling(t, fakeplugin.Options{Billing: provider}, true)
	if r := results["fetch_billed"]; r.Outcome != contract.Pass {
		t.Errorf("fetch_billed: %+v", r)
	}
	if n := provider.Calls(); n != 1 {
		t.Errorf("fetch_billed made %d billed requests, want 1", n)
	}

	results = runBilling(t, fakeplugin.Options{Billing: provider, Broken: true}, true)
	for _, name := range []string{"fetch_without_requests", "fetch_billed"} {
		if r := results[name]; r.Outcome != contract.Fail || r.Detail == "" {
			t.Errorf("%s of a broken plugin: %+v", name, r)
		}
	}

	// A plugin without the RPC gets the hint about the protocol from both cases.
	results = runBilling(t, fakeplugin.Options{}, true)
	for _, name := range []string{"fetch_without_requests", "fetch_billed"} {
		if r := results[name]; r.Outcome != contract.Fail || !strings.Contains(r.Detail, "does the plugin serve this RPC") {
			t.Errorf("%s of a plugin without the kind: %+v", name, r)
		}
	}
}

func TestPassed(t *testing.T) {
	if !contract.Passed([]contract.Result{{Outcome: contract.Pass}, {Outcome: contract.Skip}}) {
		t.Error("a skip failed the run")
	}
	if contract.Passed([]contract.Result{{Outcome: contract.Pass}, {Outcome: contract.Fail}}) {
		t.Error("a failure passed")
	}
}

func TestAHookPluginPasses(t *testing.T) {
	connectFake, _ := fakeplugin.Connect(fakeplugin.Options{})
	jsonFake, _ := fakeplugin.JSON(fakeplugin.Options{})
	for _, h := range []http.Handler{connectFake, jsonFake} {
		// local.status refuses plain TCP, so this runs on a Unix socket like its sidecars.
		for _, name := range []string{kind.InboundStatus, kind.InboundFollow, kind.SignupCheck, kind.MediaCheck, kind.LocalStatus} {
			results := run(t, name, kind.Destination{Address: fakeplugin.ServeUnix(t, h)})
			if results["verdict"].Outcome != contract.Pass {
				t.Errorf("%s: %+v", name, results["verdict"])
			}
		}
	}
}

func TestAHookPluginWithAnEmptyAnswerFails(t *testing.T) {
	h, _ := fakeplugin.JSON(fakeplugin.Options{Broken: true})
	results := run(t, kind.InboundStatus, kind.Destination{Address: fakeplugin.ServeUnix(t, h)})
	if results["verdict"].Outcome != contract.Fail {
		t.Errorf("%+v", results["verdict"])
	}
}
