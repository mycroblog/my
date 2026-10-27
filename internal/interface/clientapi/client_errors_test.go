// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// recordingReporter records the events reported to it.
type recordingReporter struct {
	mu     sync.Mutex
	events []kernel.ErrorEvent
}

func (r *recordingReporter) Report(_ context.Context, ev kernel.ErrorEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *recordingReporter) Recover(context.Context, string) {}

func (r *recordingReporter) take() []kernel.ErrorEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	evs := r.events
	r.events = nil
	return evs
}

// The stacks of the same error in the two forms of the browsers; the second frame is of a
// browser extension and the third of another site.
const (
	v8Stack = "TypeError: Cannot read properties of undefined (reading 'id')\n" +
		"    at Kc (https://social.example/assets/StatusPage-Dk2l_9a3.js:1:2345)\n" +
		"    at chrome-extension://abcdefghijklmnop/content.js:10:5\n" +
		"    at https://cdn.other.example/lib.js:3:4\n" +
		"    at async Object.load (https://social.example/assets/index-BxH3k2a9.js?v=1#x:2:10)\n" +
		"    at https://social.example/assets/index-BxH3k2a9.js:3:7"
	geckoStack = "Kc@https://social.example/assets/StatusPage-Dk2l_9a3.js:1:2345\n" +
		"handler@moz-extension://0f3a/content.js:10:5\n" +
		"@https://social.example/assets/index-BxH3k2a9.js:3:7\n"
	foreignStack = "Error: boom\n" +
		"    at x (chrome-extension://abcdefghijklmnop/content.js:10:5)\n" +
		"    at https://cdn.other.example/lib.js:3:4\n" +
		"    at eval (eval at f (https://social.example/assets/index-BxH3k2a9.js:1:1), <anonymous>:1:1)"
)

type clientErrorEnv struct {
	*authEnv
	reporter *recordingReporter
}

func newClientErrorEnv(t *testing.T) clientErrorEnv {
	t.Helper()
	rep := &recordingReporter{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Origin = "https://social.example"
		o.ErrorReporter = rep
	})
	return clientErrorEnv{authEnv: e, reporter: rep}
}

// post sends body to POST /client_errors from remote with contentType and the access token
// token ("" for none) and checks that the answer is a 204 that the spec allows.
func (e clientErrorEnv) post(t *testing.T, remote, contentType, token string, body io.Reader) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, clientapi.BasePath+"/client_errors", body)
	req.RemoteAddr = remote
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return e.spec.Serve(t, e.h, req)
}

func (e clientErrorEnv) report(t *testing.T, token string, report map[string]any) {
	t.Helper()
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if res := e.post(t, "192.0.2.1:1234", "application/json", token, bytes.NewReader(b)); res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
}

func clientReport(stack string) map[string]any {
	return map[string]any{
		"name": "TypeError", "message": "Cannot read properties of undefined (reading 'id')",
		"stack": stack, "release": "0.3.0", "route": "/_app/statuses/$id", "kind": "route_error",
	}
}

func TestReportClientErrorV8(t *testing.T) {
	e := newClientErrorEnv(t)
	e.report(t, "", clientReport(v8Stack))
	evs := e.reporter.take()
	if len(evs) != 1 {
		t.Fatalf("reported %d events, want 1", len(evs))
	}
	ev := evs[0]
	if ev.Source != kernel.ErrorSourceWeb || ev.Kind != kernel.ErrorKindError || ev.Type != "TypeError" ||
		ev.Where != "web /_app/statuses/$id" || ev.Version != "0.3.0" || !ev.Unauthenticated {
		t.Errorf("event = %+v", ev)
	}
	if ev.Message != "Cannot read properties of undefined (reading 'id')" {
		t.Errorf("Message = %q", ev.Message)
	}
	// Only the frames of the server's own scripts stay, without their query and fragment.
	wantStack := "    at Kc (/assets/StatusPage-Dk2l_9a3.js:1:2345)\n" +
		"    at async Object.load (/assets/index-BxH3k2a9.js:2:10)\n" +
		"    at /assets/index-BxH3k2a9.js:3:7"
	if ev.Stack != wantStack {
		t.Errorf("Stack =\n%s\nwant\n%s", ev.Stack, wantStack)
	}
	if len(ev.Fingerprint) != 32 {
		t.Errorf("Fingerprint = %q, want 32 hexadecimal digits", ev.Fingerprint)
	}
}

func TestReportClientErrorGecko(t *testing.T) {
	e := newClientErrorEnv(t)
	e.report(t, "write", clientReport(geckoStack))
	evs := e.reporter.take()
	if len(evs) != 1 {
		t.Fatalf("reported %d events, want 1", len(evs))
	}
	ev := evs[0]
	if ev.Unauthenticated {
		t.Error("a report with an access token is marked unauthenticated")
	}
	wantStack := "    at Kc (/assets/StatusPage-Dk2l_9a3.js:1:2345)\n    at /assets/index-BxH3k2a9.js:3:7"
	if ev.Stack != wantStack {
		t.Errorf("Stack =\n%s\nwant\n%s", ev.Stack, wantStack)
	}

	// The same error in the other browser, and in a later release whose minified names, chunk
	// hashes, and positions differ, has the same fingerprint.
	e.report(t, "", clientReport(v8Stack))
	later := clientReport("TypeError: x\n    at Qa (https://social.example/assets/StatusPage-Zz9_aa01.js:1:999)")
	later["release"] = "0.4.0"
	e.report(t, "", later)
	otherRoute := clientReport(v8Stack)
	otherRoute["route"] = "/_app/home"
	e.report(t, "", otherRoute)
	evs = e.reporter.take()
	if len(evs) != 3 {
		t.Fatalf("reported %d events, want 3", len(evs))
	}
	if evs[0].Fingerprint != ev.Fingerprint || evs[1].Fingerprint != ev.Fingerprint {
		t.Errorf("fingerprints %q, %q, want %q of the same error", evs[0].Fingerprint, evs[1].Fingerprint, ev.Fingerprint)
	}
	if evs[2].Fingerprint == ev.Fingerprint {
		t.Error("an error of another route has the same fingerprint")
	}
}

func TestReportClientErrorNormalizesMessage(t *testing.T) {
	e := newClientErrorEnv(t)
	r1 := clientReport(v8Stack)
	r1["message"] = "Failed to fetch https://social.example/api/v1/statuses/0190a5d0-ac96-774b-bcce-b302099a8058 for alice@example.com at 1729987200123"
	r2 := clientReport(v8Stack)
	r2["message"] = "Failed to fetch https://social.example/api/v1/statuses/0190a5d0-ac96-774b-bcce-b302099a8059 for bob@example.org at 1729987200999"
	e.report(t, "", r1)
	e.report(t, "", r2)
	evs := e.reporter.take()
	if len(evs) != 2 {
		t.Fatalf("reported %d events, want 2", len(evs))
	}
	if want := "Failed to fetch <url> for <email> at <n>"; evs[0].Message != want {
		t.Errorf("Message = %q, want %q", evs[0].Message, want)
	}
	if evs[0].Fingerprint != evs[1].Fingerprint {
		t.Error("messages that differ only in their URL, address, and number have different fingerprints")
	}
}

func TestReportClientErrorIgnored(t *testing.T) {
	big := clientReport(v8Stack)
	big["message"] = strings.Repeat("x", 17<<10)
	bigBody, err := json.Marshal(big)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := json.Marshal(clientReport(v8Stack))
	if err != nil {
		t.Fatal(err)
	}
	badKind := clientReport(v8Stack)
	badKind["kind"] = "console"
	badKindBody, err := json.Marshal(badKind)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := json.Marshal(clientReport(foreignStack))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, contentType string
		body              io.Reader
	}{
		{"text/plain", "text/plain", bytes.NewReader(ok)},
		{"no content type", "", bytes.NewReader(ok)},
		{"form", "application/x-www-form-urlencoded", bytes.NewReader(ok)},
		{"declared over 16 KiB", "application/json", bytes.NewReader(bigBody)},
		// A body without a length is cut off at the limit.
		{"over 16 KiB without a length", "application/json", io.MultiReader(bytes.NewReader(bigBody))},
		{"not JSON", "application/json", strings.NewReader("{")},
		{"unknown kind", "application/json", bytes.NewReader(badKindBody)},
		{"no frame of the server's scripts", "application/json", bytes.NewReader(foreign)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newClientErrorEnv(t)
			res := e.post(t, "192.0.2.1:1234", tc.contentType, "", tc.body)
			if res.StatusCode != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", res.StatusCode)
			}
			if evs := e.reporter.take(); len(evs) != 0 {
				t.Errorf("reported %+v, want nothing", evs)
			}
		})
	}

	// With a charset, the body is still JSON.
	e := newClientErrorEnv(t)
	if res := e.post(t, "192.0.2.1:1234", "application/json; charset=utf-8", "", bytes.NewReader(ok)); res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
	if evs := e.reporter.take(); len(evs) != 1 {
		t.Errorf("reported %d events, want 1", len(evs))
	}
}

func TestReportClientErrorRateLimit(t *testing.T) {
	e := newClientErrorEnv(t)
	body, err := json.Marshal(clientReport(v8Stack))
	if err != nil {
		t.Fatal(err)
	}
	send := func(remote, token string) {
		t.Helper()
		if res := e.post(t, remote, "application/json", token, bytes.NewReader(body)); res.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", res.StatusCode)
		}
	}
	// One IP has 10 reports a minute, signed in or not; the rest is a 204 that keeps nothing.
	for i := range 10 {
		token := ""
		if i%2 == 1 {
			token = "write"
		}
		send("192.0.2.1:1234", token)
	}
	if evs := e.reporter.take(); len(evs) != 10 {
		t.Fatalf("reported %d events, want 10", len(evs))
	}
	send("192.0.2.1:1234", "")
	send("192.0.2.1:1234", "write")
	if evs := e.reporter.take(); len(evs) != 0 {
		t.Fatalf("reported %d events past the limit, want 0", len(evs))
	}
	// Another IP has its own limit, and the count of an IP slides out of the window.
	send("192.0.2.2:1234", "")
	e.clock.Advance(2 * clientapi.ClientErrorLimit.Window)
	send("192.0.2.1:1234", "")
	if evs := e.reporter.take(); len(evs) != 2 {
		t.Fatalf("reported %d events, want 2", len(evs))
	}
}

func TestReportClientErrorInvalidToken(t *testing.T) {
	e := newClientErrorEnv(t)
	body, err := json.Marshal(clientReport(v8Stack))
	if err != nil {
		t.Fatal(err)
	}
	// A token that is sent is checked, as for every operation.
	if res := e.post(t, "192.0.2.1:1234", "application/json", "nope", bytes.NewReader(body)); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
	if evs := e.reporter.take(); len(evs) != 0 {
		t.Errorf("reported %+v, want nothing", evs)
	}
}
