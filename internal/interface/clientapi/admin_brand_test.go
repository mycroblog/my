// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeBrands keeps the brand images with the checks of the domain, as appinstance.Service does;
// testkit.InstanceRepositoryContract covers the storage, the audit log, and the projection.
type fakeBrands struct {
	now     time.Time
	current instance.Brands
	sets    []appinstance.SetBrandInput
	clears  []string
}

func (f *fakeBrands) Brands(context.Context) (instance.Brands, error) { return f.current, nil }

func (f *fakeBrands) SanitizeBrand(kind instance.BrandKind, svg []byte) (instance.BrandSVG, error) {
	return instance.SanitizeBrandSVG(kind, svg)
}

func (f *fakeBrands) SetBrand(_ context.Context, in appinstance.SetBrandInput) (appinstance.SetBrandResult, error) {
	svg, err := instance.SanitizeBrandSVG(in.Kind, in.SVG)
	if err != nil {
		return appinstance.SetBrandResult{}, err
	}
	if in.Background != "" {
		if in.Background, err = instance.NormalizeBrandBackground(in.Background); err != nil {
			return appinstance.SetBrandResult{}, err
		}
	}
	rev := instance.BrandRevision{
		ID: fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099a7%03d", len(f.sets)+len(f.clears)+1), Kind: in.Kind, Action: instance.BrandSet,
		Background: in.Background, RevisedByUserID: in.RevisedByUserID, RevisedAt: f.now,
		Files: []instance.BrandFile{{Rendition: instance.RenditionSVG, Width: int(svg.ViewWidth), Height: int(svg.ViewHeight)}},
	}
	for _, r := range instance.BrandRenditions(in.Kind) {
		data, ok := in.Renditions[r]
		if !ok {
			return appinstance.SetBrandResult{}, &instance.BrandError{Code: instance.BrandMissingRendition, Rendition: r}
		}
		_, w, h, err := instance.CheckBrandPNG(r, data, svg.ViewWidth, svg.ViewHeight)
		if err != nil {
			return appinstance.SetBrandResult{}, err
		}
		rev.Files = append(rev.Files, instance.BrandFile{Rendition: r, Width: w, Height: h})
	}
	f.sets = append(f.sets, in)
	f.current.Set(&rev)
	return appinstance.SetBrandResult{Revision: rev, Dropped: svg.Dropped}, nil
}

func (f *fakeBrands) ClearBrand(_ context.Context, kind instance.BrandKind, by string) (instance.BrandRevision, error) {
	f.clears = append(f.clears, string(kind)+" "+by)
	switch kind {
	case instance.BrandIcon:
		f.current.Icon = nil
	case instance.BrandLogo:
		f.current.Logo = nil
	case instance.BrandLogoDark:
		f.current.LogoDark = nil
	}
	return instance.BrandRevision{Kind: kind, Action: instance.BrandClear}, nil
}

func newBrandEnv(t *testing.T) (*authEnv, *fakeBrands) {
	t.Helper()
	f := &fakeBrands{now: adminNow}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.BrandImages = f
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, f
}

func (e *authEnv) sendMultipart(t *testing.T, method, path, token string, parts ...part) *http.Response {
	t.Helper()
	body, contentType := multipartBody(t, parts...)
	req := httptest.NewRequest(method, clientapi.BasePath+path, body)
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept-Language", "en")
	return e.spec.Serve(t, e.h, req)
}

// brandParts returns the parts of a complete upload of kind.
func brandParts(kind instance.BrandKind) []part {
	in := testkit.BrandFixture(kind)
	parts := []part{{name: "svg", filename: "logo.svg", contentType: "image/svg+xml", data: in.SVG}}
	for _, r := range instance.BrandRenditions(kind) {
		parts = append(parts, part{name: strings.ReplaceAll(string(r), "-", "_"), filename: string(r) + ".png", contentType: "image/png", data: in.Renditions[r]})
	}
	return parts
}

func TestBrandOperationsNeedAPermission(t *testing.T) {
	e, f := newBrandEnv(t)
	for _, op := range []struct {
		method, path string
		parts        []part
	}{
		{http.MethodGet, "/admin/instance/brand", nil},
		{http.MethodPost, "/admin/instance/brand/sanitize", []part{{name: "kind", data: []byte("icon")}, {name: "svg", data: testkit.BrandSVGFixture(instance.BrandIcon)}}},
		{http.MethodPut, "/admin/instance/brand/icon", brandParts(instance.BrandIcon)},
		{http.MethodDelete, "/admin/instance/brand/icon", nil},
	} {
		var res *http.Response
		if op.parts != nil {
			res = e.sendMultipart(t, op.method, op.path, "bob-admin", op.parts...)
		} else {
			res = e.admin(t, op.method, op.path, "bob-admin", nil)
		}
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_settings") {
			t.Errorf("%s %s by bob = %d %+v, want 403 for manage_settings", op.method, op.path, res.StatusCode, p)
		}
	}
	if len(f.sets) != 0 || len(f.clears) != 0 {
		t.Error("a refused operation changed a brand image")
	}
}

func TestSanitizeBrand(t *testing.T) {
	e, f := newBrandEnv(t)
	res := e.sendMultipart(t, http.MethodPost, "/admin/instance/brand/sanitize", "admin",
		part{name: "kind", data: []byte("logo")}, part{name: "svg", filename: "logo.svg", data: testkit.BrandSVGFixture(instance.BrandLogo)})
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("sanitize = %d %s", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	got := decodeJSON[clientapi.BrandSanitized](t, res)
	if strings.Contains(got.Svg, "script") || !strings.HasPrefix(got.Svg, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 100">`) ||
		got.ViewBoxWidth != 400 || got.ViewBoxHeight != 100 {
		t.Errorf("sanitized = %+v", got)
	}
	if len(got.Dropped) != 1 || got.Dropped[0].Kind != "element" || got.Dropped[0].Name != "script" {
		t.Errorf("dropped = %+v", got.Dropped)
	}
	if want := []clientapi.BrandRenditionSize{{Rendition: "logo-mail", Width: 384, Height: 96}, {Rendition: "preview-1200x630", Width: 1200, Height: 630}}; !slices.Equal(got.Renditions, want) {
		t.Errorf("renditions = %+v, want %+v", got.Renditions, want)
	}
	if len(f.sets) != 0 {
		t.Error("sanitize saved the image")
	}

	for _, tt := range []struct {
		name  string
		parts []part
		code  clientapi.BrandErrorCode
	}{
		{"script entity", []part{{name: "kind", data: []byte("icon")}, {name: "svg", data: []byte(`<!DOCTYPE svg [<!ENTITY x "y">]><svg viewBox="0 0 1 1"/>`)}}, "doctype"},
		{"text", []part{{name: "kind", data: []byte("icon")}, {name: "svg", data: []byte(`<svg viewBox="0 0 1 1"><text>A</text></svg>`)}}, "text"},
		{"too large", []part{{name: "kind", data: []byte("icon")}, {name: "svg", data: make([]byte, instance.MaxBrandSVGInputBytes+1)}}, "too_large"},
	} {
		res := e.sendMultipart(t, http.MethodPost, "/admin/instance/brand/sanitize", "admin", tt.parts...)
		p := decodeJSON[clientapi.BrandProblem](t, res)
		if res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeInvalidBrand || p.Title == "" ||
			p.BrandError == nil || p.BrandError.Code != tt.code {
			t.Errorf("%s: sanitize = %d %+v", tt.name, res.StatusCode, p)
		}
	}
	for _, parts := range [][]part{
		{{name: "svg", data: testkit.BrandSVGFixture(instance.BrandIcon)}},
		{{name: "kind", data: []byte("banner")}, {name: "svg", data: testkit.BrandSVGFixture(instance.BrandIcon)}},
		{{name: "kind", data: []byte("icon")}},
		{{name: "kind", data: []byte("icon")}, {name: "svg", data: []byte("<svg/>")}, {name: "svg", data: []byte("<svg/>")}},
	} {
		if res := e.sendMultipart(t, http.MethodPost, "/admin/instance/brand/sanitize", "admin", parts...); res.StatusCode != http.StatusBadRequest {
			t.Errorf("sanitize %v = %d, want 400", parts, res.StatusCode)
		}
	}
}

func TestSetAndClearBrand(t *testing.T) {
	e, f := newBrandEnv(t)
	if got := decodeJSON[clientapi.AdminBrands](t, e.admin(t, http.MethodGet, "/admin/instance/brand", "admin-read", nil)); got != (clientapi.AdminBrands{}) {
		t.Errorf("brands before setting = %+v", got)
	}
	if inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil)); inst.Icon != nil || inst.Logo != nil || inst.LogoDark != nil {
		t.Errorf("Instance without brand images = %+v %+v %+v", inst.Icon, inst.Logo, inst.LogoDark)
	}

	parts := append(brandParts(instance.BrandIcon), part{name: "background", data: []byte("#FFEEDD")}, part{name: "ignored", data: []byte("x")})
	res := e.sendMultipart(t, http.MethodPut, "/admin/instance/brand/icon", "admin", parts...)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set = %d %s", res.StatusCode, mustRead(t, res.Body))
	}
	got := decodeJSON[clientapi.AdminBrandImage](t, res)
	base := "https://social.example/brand/" + got.Revision + "/"
	if got.SvgUrl != base+"svg.svg" || got.Width != 100 || got.Height != 100 || !got.RevisedAt.Equal(adminNow) ||
		got.Dropped == nil || len(*got.Dropped) != 1 || len(got.Renditions) != 6 {
		t.Fatalf("set = %+v", got)
	}
	if r := got.Renditions[2]; r.Rendition != "apple-touch-180" || r.Url != base+"apple-touch-180.png" || r.Width != 180 || r.Height != 180 {
		t.Errorf("rendition = %+v", r)
	}
	in := f.sets[0]
	if in.Kind != instance.BrandIcon || in.Background != "#ffeedd" || in.RevisedByUserID != userOf(aliceAccountID) || len(in.Renditions) != 6 {
		t.Errorf("SetBrand input = %+v", in)
	}
	inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil))
	if inst.Icon == nil || inst.Icon.Revision != got.Revision || inst.Icon.SvgUrl != got.SvgUrl || len(inst.Icon.Renditions) != 6 || inst.Logo != nil {
		t.Errorf("Instance.icon = %+v", inst.Icon)
	}
	admin := decodeJSON[clientapi.AdminBrands](t, e.admin(t, http.MethodGet, "/admin/instance/brand", "admin-read", nil))
	if admin.Icon == nil || admin.Icon.Background == nil || *admin.Icon.Background != "#ffeedd" || admin.Icon.Dropped != nil {
		t.Errorf("admin brands = %+v", admin.Icon)
	}

	res = e.admin(t, http.MethodDelete, "/admin/instance/brand/icon", "admin", nil)
	if res.StatusCode != http.StatusNoContent || len(f.clears) != 1 || f.clears[0] != "icon "+userOf(aliceAccountID) {
		t.Errorf("clear = %d %v", res.StatusCode, f.clears)
	}
	if inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil)); inst.Icon != nil {
		t.Errorf("Instance.icon after clearing = %+v", inst.Icon)
	}
}

func TestSetBrandProblems(t *testing.T) {
	e, f := newBrandEnv(t)
	icon := brandParts(instance.BrandIcon)
	withPart := func(name string, data []byte) []part {
		out := []part{}
		for _, p := range icon {
			if p.name != name {
				out = append(out, p)
			}
		}
		if data != nil {
			out = append(out, part{name: name, data: data})
		}
		return out
	}
	for _, tt := range []struct {
		name  string
		parts []part
		code  clientapi.BrandErrorCode
	}{
		{"wrong size", withPart("pwa_512", testkit.FixturePNG(512, 511, false)), "dimensions"},
		{"missing rendition", withPart("maskable_512", nil), "missing_rendition"},
		{"not a PNG", withPart("favicon_48", []byte("GIF89a")), "invalid_png"},
		{"rendition too large", withPart("favicon_32", make([]byte, instance.MaxBrandPNGBytes+1)), "too_large"},
		{"unsafe SVG", withPart("svg", []byte(`<svg viewBox="0 0 1 1"><style>@import "x.css";</style></svg>`)), "unsafe_css"},
	} {
		res := e.sendMultipart(t, http.MethodPut, "/admin/instance/brand/icon", "admin", tt.parts...)
		p := decodeJSON[clientapi.BrandProblem](t, res)
		if res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeInvalidBrand || p.BrandError == nil || p.BrandError.Code != tt.code {
			t.Errorf("%s: set = %d %+v", tt.name, res.StatusCode, p)
		}
	}
	if len(f.sets) != 0 || f.current != (instance.Brands{}) {
		t.Errorf("a rejected image was saved: %+v", f.current)
	}
	if res := e.sendMultipart(t, http.MethodPut, "/admin/instance/brand/banner", "admin", icon...); res.StatusCode != http.StatusBadRequest && res.StatusCode != http.StatusNotFound {
		t.Errorf("set an unknown kind = %d", res.StatusCode)
	}
	if res := e.sendMultipart(t, http.MethodPut, "/admin/instance/brand/icon", "admin", withPart("svg", nil)...); res.StatusCode != http.StatusBadRequest {
		t.Errorf("set without an SVG = %d", res.StatusCode)
	}
}

func mustRead(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
