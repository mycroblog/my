// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
)

func TestInstanceBrand(t *testing.T) {
	newAdminEnv(t)
	out := mustInstance(t, "", "brand", "show")
	for _, kind := range []string{"icon", "logo", "logo_dark"} {
		if !strings.Contains(out, kind+" ") || strings.Count(out, "(built-in)") != 3 {
			t.Errorf("show without brand images = %q", out)
		}
	}
	if out := mustInstance(t, "", "brand", "clear", "logo"); !strings.Contains(out, "logo: the built-in image is in use") {
		t.Errorf("clear = %q", out)
	}
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"brand"}, 2, "Usage:"},
		{[]string{"brand", "set", "icon"}, 2, "unknown subcommand"},
		{[]string{"brand", "clear"}, 2, "want 1 argument"},
		{[]string{"brand", "clear", "banner"}, 2, "unknown kind"},
	} {
		code, _, stderr := runInstanceCmd(t, "", tc.args...)
		if code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("my instance %q = %d, %q; want %d and %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}

func TestPrintBrands(t *testing.T) {
	rev := &instance.BrandRevision{
		ID: "0199f0e0-0000-7000-8000-000000000001", Kind: instance.BrandIcon, Action: instance.BrandSet, Background: "#ffffff",
		RevisedAt: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC),
		Files:     []instance.BrandFile{{Rendition: instance.RenditionSVG}},
	}
	var b bytes.Buffer
	printBrands(&b, "https://social.example", instance.Brands{Icon: rev})
	out := b.String()
	for _, want := range []string{
		"icon       " + rev.ID, "2026-10-28T09:00:00Z", "(cli)", "#ffffff", "https://social.example/brand/" + rev.ID + "/svg.svg",
		"logo       (built-in)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("printBrands lacks %q:\n%s", want, out)
		}
	}
}
