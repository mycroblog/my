// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestClientAPIMedia uploads an image through the client API on the real database and queue,
// waits for its processing, changes its description and focus, and checks that another
// account can neither read nor change it.
func TestClientAPIMedia(t *testing.T) {
	t.Parallel()
	// The inline queue processes the upload without a worker.
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h := app.Handler()
	spec := apitest.Load(t)

	users := map[string]string{}
	for _, name := range []string{"alice", "bob"} {
		r, err := app.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{
			Username: name, Email: name + "@example.com", Password: "correct horse battery staple",
		})
		if err != nil {
			t.Fatalf("CreateAdmin(%s): %v", name, err)
		}
		users[name] = issueTokens(t, app, r.User.ID, "").AccessToken
	}
	send := func(method, path, token, contentType string, body []byte) (*http.Response, clientapi.MediaAttachment) {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		res := spec.Serve(t, h, req)
		var m clientapi.MediaAttachment
		if res.StatusCode < 300 {
			if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
				t.Fatal(err)
			}
		}
		return res, m
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="photo.jpg"`)
	hdr.Set("Content-Type", "image/jpeg")
	part, err := w.CreatePart(hdr)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(testkit.FixtureJPEG(640, 480, 6))
	_ = w.WriteField("description", "A view from the window")
	_ = w.Close()

	res, up := send(http.MethodPost, "/media", users["alice"], w.FormDataContentType(), body.Bytes())
	if res.StatusCode != http.StatusAccepted || up.Description == nil || *up.Description != "A view from the window" {
		t.Fatalf("upload = %d %+v", res.StatusCode, up)
	}
	path := "/media/" + up.Id
	got := up
	testkit.Eventually(t, 10*time.Second, func() bool {
		if got.State != "processing" {
			return true
		}
		_, got = send(http.MethodGet, path, users["alice"], "", nil)
		return got.State != "processing"
	})
	if got.State != "ready" || got.Variants == nil || got.Blurhash == nil || got.Meta.Width == nil || *got.Meta.Width != 480 || *got.Meta.Height != 640 {
		t.Fatalf("processed = %+v", got)
	}
	prefix := "https://example.test/media/" + up.Id + "/"
	v := got.Variants
	if !strings.HasPrefix(v.Large.Url, prefix) || !strings.HasSuffix(v.Large.Url, "/large.jpg") || !strings.HasSuffix(v.Small.Url, "/small.webp") ||
		deref(v.Small.ContentType) != "image/webp" || v.Animated != nil || deref(got.Url) != v.Large.Url || deref(got.PreviewUrl) != v.Small.Url {
		t.Errorf("variants = %+v, want a large JPEG and a small WebP under %s", v, prefix)
	}

	res, got = send(http.MethodPatch, path, users["alice"], "application/json", []byte(`{"description": "A garden", "focus": {"x": -0.5, "y": 0.25}}`))
	if res.StatusCode != http.StatusOK || *got.Description != "A garden" || got.Meta.Focus != (clientapi.MediaFocus{X: -0.5, Y: 0.25}) {
		t.Errorf("update = %d %+v", res.StatusCode, got)
	}
	if _, again := send(http.MethodGet, path, users["alice"], "", nil); *again.Description != "A garden" || again.Meta.Focus.X != -0.5 {
		t.Errorf("after the update = %+v", again)
	}

	for _, req := range []struct{ method, contentType, body string }{{http.MethodGet, "", ""}, {http.MethodPatch, "application/json", `{"description": "mine"}`}} {
		if res, _ := send(req.method, path, users["bob"], req.contentType, []byte(req.body)); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s by another account = %d, want 404", req.method, res.StatusCode)
		}
	}

	res, _ = send(http.MethodPost, "/media", users["alice"], "multipart/form-data; boundary=x", []byte("--x\r\nContent-Disposition: form-data; name=\"file\"\r\n\r\n<svg/>\r\n--x--\r\n"))
	if res.StatusCode != http.StatusUnsupportedMediaType || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/problem+json") {
		t.Errorf("svg upload = %d %s, want 415", res.StatusCode, res.Header.Get("Content-Type"))
	}
}
