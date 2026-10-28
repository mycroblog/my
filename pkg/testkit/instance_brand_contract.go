// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// instanceBrandCases are the cases of InstanceRepositoryContract about the brand images
// (ADR-0116): nothing until one is set, revisions that keep their files and stay readable after a
// newer one or a clear, rejected uploads that write nothing, and the instance_brands projection
// equal to a rebuild.
var instanceBrandCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"HasNoBrandUntilSet", instanceHasNoBrand},
	{"KeepsBrandRevisions", instanceKeepsBrandRevisions},
	{"RejectsInvalidBrands", instanceRejectsInvalidBrands},
	{"BrandProjectionEqualsRebuild", instanceBrandProjectionEqualsRebuild},
}

// BrandSVGFixture is a valid SVG of kind: square for the icon, 4:1 for the logos.
func BrandSVGFixture(kind instance.BrandKind) []byte {
	vb := "0 0 100 100"
	if kind != instance.BrandIcon {
		vb = "0 0 400 100"
	}
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="` + vb + `"><script>alert(1)</script><path d="M0 0h10v10z" fill="#123456"/></svg>`)
}

// BrandFixture returns a complete SetBrandInput of kind: BrandSVGFixture and every PNG
// rendition at its size, opaque.
func BrandFixture(kind instance.BrandKind) appinstance.SetBrandInput {
	svg := BrandSVGFixture(kind)
	vw, vh := 100.0, 100.0
	if kind != instance.BrandIcon {
		vw = 400
	}
	in := appinstance.SetBrandInput{Kind: kind, SVG: svg, Renditions: map[instance.BrandRendition][]byte{}}
	for _, r := range instance.BrandRenditions(kind) {
		w, h := instance.BrandRenditionSize(r, vw, vh)
		in.Renditions[r] = FixturePNG(w, h, false)
	}
	return in
}

func (e *instanceEnv) setBrand(t *testing.T, kind instance.BrandKind) instance.BrandRevision {
	t.Helper()
	e.clock.Advance(time.Second)
	in := BrandFixture(kind)
	in.RevisedByUserID = ruleModerator
	res, err := e.service.SetBrand(t.Context(), in)
	if err != nil {
		t.Fatalf("SetBrand(%s) = %v", kind, err)
	}
	return res.Revision
}

func (e *instanceEnv) brands(t *testing.T) instance.Brands {
	t.Helper()
	b, err := e.service.Brands(t.Context())
	if err != nil {
		t.Fatalf("Brands = %v", err)
	}
	return b
}

func (e *instanceEnv) brandFile(t *testing.T, revisionID string, r instance.BrandRendition) ([]byte, error) {
	t.Helper()
	_, rc, err := e.service.BrandFile(t.Context(), revisionID, r)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

func instanceHasNoBrand(t *testing.T, e *instanceEnv) {
	if b := e.brands(t); b != (instance.Brands{}) {
		t.Errorf("a new instance has brand images: %+v", b)
	}
	if _, err := e.brandFile(t, "0199f0e0-0000-7000-8000-000000000000", instance.RenditionSVG); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("BrandFile(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := e.brandFile(t, "../../etc", instance.RenditionSVG); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("BrandFile(not an ID) = %v, want ErrNotFound", err)
	}
}

func instanceKeepsBrandRevisions(t *testing.T, e *instanceEnv) {
	rec := &auditRecorder{}
	e.projectors.Register(appmoderation.EventAuditRecorded, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		rec.add(ev.(appmoderation.AuditRecorded).Entry)
		return nil
	})
	icon := e.setBrand(t, instance.BrandIcon)
	logo := e.setBrand(t, instance.BrandLogo)
	b := e.brands(t)
	if b.Icon == nil || b.Icon.ID != icon.ID || b.Logo == nil || b.Logo.ID != logo.ID || b.LogoDark != nil {
		t.Fatalf("Brands = %+v", b)
	}
	if b.Icon.Action != instance.BrandSet || b.Icon.RevisedByUserID != ruleModerator || !b.Icon.RevisedAt.Equal(icon.RevisedAt) {
		t.Errorf("icon revision = %+v", b.Icon)
	}
	// The icon's background is the theme's light surface by default.
	if b.Icon.Background == "" || !strings.HasPrefix(b.Icon.Background, "#") || b.Logo.Background != "" {
		t.Errorf("backgrounds = %q, %q", b.Icon.Background, b.Logo.Background)
	}
	if len(b.Icon.Files) != 1+len(instance.BrandRenditions(instance.BrandIcon)) {
		t.Errorf("icon files = %+v", b.Icon.Files)
	}
	svg, err := e.brandFile(t, icon.ID, instance.RenditionSVG)
	if err != nil {
		t.Fatalf("BrandFile(svg) = %v", err)
	}
	if bytes.Contains(svg, []byte("script")) || !bytes.Contains(svg, []byte(`<path d="M0 0h10v10z"`)) {
		t.Errorf("stored SVG = %s", svg)
	}
	f, _ := b.Icon.File(instance.RenditionSVG)
	if f.ContentType != "image/svg+xml" || f.Width != 100 || f.Height != 100 || f.Size != int64(len(svg)) || len(f.SHA256) != 64 ||
		f.Key != instance.BrandBlobKey(icon.ID, instance.RenditionSVG) {
		t.Errorf("SVG file = %+v", f)
	}
	png, err := e.brandFile(t, icon.ID, instance.RenditionPWA192)
	if err != nil {
		t.Fatalf("BrandFile(pwa-192) = %v", err)
	}
	if bytes.Contains(png, []byte(MetadataMarker)) {
		t.Error("the stored PNG keeps the metadata of the upload")
	}
	if _, err := e.brandFile(t, logo.ID, instance.RenditionPWA192); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("BrandFile(a rendition of another kind) = %v, want ErrNotFound", err)
	}

	second := e.setBrand(t, instance.BrandIcon)
	if got := e.brands(t).Icon; got == nil || got.ID != second.ID {
		t.Errorf("icon after the second set = %+v", got)
	}
	e.clock.Advance(time.Second)
	cleared, err := e.service.ClearBrand(t.Context(), instance.BrandIcon, "")
	if err != nil || cleared.Action != instance.BrandClear {
		t.Fatalf("ClearBrand = %+v, %v", cleared, err)
	}
	if got := e.brands(t); got.Icon != nil || got.Logo == nil {
		t.Errorf("Brands after clearing the icon = %+v", got)
	}
	// Each change is a setting change of brand_<kind> in the audit log, naming its revision.
	var audited []string
	for _, a := range rec.all() {
		if a.Action != appmoderation.AuditSettingChanged || a.TargetKind != appmoderation.AuditTargetSetting || a.ActorUserID != ruleModerator && a.SourceEventID != cleared.ID {
			t.Errorf("audit entry = %+v", a)
		}
		audited = append(audited, a.TargetID+" "+a.SourceEventID)
	}
	want := []string{"brand_icon " + icon.ID, "brand_logo " + logo.ID, "brand_icon " + second.ID, "brand_icon " + cleared.ID}
	if !reflect.DeepEqual(audited, want) {
		t.Errorf("audited %v, want %v", audited, want)
	}
	// An earlier revision keeps its files: its URLs never change meaning.
	if _, err := e.brandFile(t, icon.ID, instance.RenditionSVG); err != nil {
		t.Errorf("BrandFile(first icon) after clearing = %v", err)
	}
}

func instanceRejectsInvalidBrands(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	cases := map[string]func(in *appinstance.SetBrandInput){
		"unsafe SVG":        func(in *appinstance.SetBrandInput) { in.SVG = []byte(`<!DOCTYPE svg><svg/>`) },
		"missing rendition": func(in *appinstance.SetBrandInput) { delete(in.Renditions, instance.RenditionMaskable512) },
		"wrong size": func(in *appinstance.SetBrandInput) {
			in.Renditions[instance.RenditionPWA512] = FixturePNG(511, 512, false)
		},
		"not a PNG": func(in *appinstance.SetBrandInput) { in.Renditions[instance.RenditionFavicon32] = []byte("GIF89a") },
		"transparent": func(in *appinstance.SetBrandInput) {
			in.Renditions[instance.RenditionAppleTouch] = FixturePNG(180, 180, true)
		},
		"unexpected": func(in *appinstance.SetBrandInput) {
			in.Renditions[instance.RenditionPreview] = FixturePNG(1200, 630, false)
		},
		"invalid background": func(in *appinstance.SetBrandInput) { in.Background = "red" },
		"icon not square":    func(in *appinstance.SetBrandInput) { in.SVG = BrandSVGFixture(instance.BrandLogo) },
	}
	for name, mutate := range cases {
		in := BrandFixture(instance.BrandIcon)
		mutate(&in)
		_, err := e.service.SetBrand(ctx, in)
		if !errors.Is(err, instance.ErrInvalidBrand) {
			t.Errorf("%s: SetBrand = %v, want ErrInvalidBrand", name, err)
		}
	}
	if b := e.brands(t); b != (instance.Brands{}) {
		t.Errorf("a rejected brand was saved: %+v", b)
	}
	if keys := e.blobs.Keys(); len(keys) != 0 {
		t.Errorf("a rejected brand left files: %v", keys)
	}
	logo := BrandFixture(instance.BrandLogo)
	logo.Background = "#ffffff"
	if _, err := e.service.SetBrand(ctx, logo); !errors.Is(err, instance.ErrInvalidBrand) {
		t.Errorf("SetBrand(logo with a background) = %v, want ErrInvalidBrand", err)
	}
}

func instanceBrandProjectionEqualsRebuild(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	e.setBrand(t, instance.BrandIcon)
	logo := e.setBrand(t, instance.BrandLogo)
	e.setBrand(t, instance.BrandLogoDark)
	icon := e.setBrand(t, instance.BrandIcon)
	e.clock.Advance(time.Second)
	if _, err := e.service.ClearBrand(ctx, instance.BrandLogoDark, ruleModerator); err != nil {
		t.Fatal(err)
	}
	before := e.brands(t)
	if err := appinstance.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Instance); err != nil {
		t.Fatal(err)
	}
	after := e.brands(t)
	if after.Icon == nil || after.Icon.ID != icon.ID || after.Logo == nil || after.Logo.ID != logo.ID || after.LogoDark != nil {
		t.Errorf("brands after rebuild = %+v", after)
	}
	if before.Version() != after.Version() {
		t.Errorf("rebuild changed the brands: %s, want %s", after.Version(), before.Version())
	}
}
