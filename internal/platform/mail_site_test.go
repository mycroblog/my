// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
)

// TestMailSiteColorsFollowTheme checks that an email is in the colors of the design tokens' defaults
// until a theme is published, and then in the theme's colors, light and dark, with the defaults
// for what the theme leaves alone (ADR-0059 §2, ADR-0086 §2).
func TestMailSiteColorsFollowTheme(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	ctx := t.Context()
	def := domaininstance.DefaultThemeColor

	site := platform.MailSite(ctx, e.app)
	if want := kernel.MailColorsFrom(def); site.Colors != want {
		t.Fatalf("colors without a theme = %+v, want the defaults %+v", site.Colors, want)
	}

	if _, err := e.app.Instance.SaveThemeDraft(ctx, instance.SaveThemeDraftInput{Tokens: []domaininstance.ThemeTokenValue{
		{Token: "color-surface", Light: "#fdf6e3", Dark: "#002b36"},
	}}); err != nil {
		t.Fatal(err)
	}
	// A draft is not the theme yet.
	if site := platform.MailSite(ctx, e.app); site.Colors.Light.Surface != def("color-surface", false) {
		t.Errorf("the draft's surface reached the email: %+v", site.Colors)
	}
	if _, err := e.app.Instance.PublishTheme(ctx, ""); err != nil {
		t.Fatal(err)
	}
	site = platform.MailSite(ctx, e.app)
	if site.Colors.Light.Surface != "#fdf6e3" || site.Colors.Dark.Surface != "#002b36" {
		t.Errorf("surface = %q / %q, want the theme's #fdf6e3 / #002b36", site.Colors.Light.Surface, site.Colors.Dark.Surface)
	}
	if site.Colors.Light.Text != def("color-text", false) || site.Colors.Dark.Accent != def("color-accent", true) {
		t.Errorf("the colors the theme leaves alone are not the defaults: %+v", site.Colors)
	}

	m, err := kernel.MailComposer{Site: site}.Compose(kernel.MailDraft{
		Category: kernel.MailTransactional, To: "alice@example.com", Subject: "Hello",
		Paragraphs: []string{"Hello."}, Reason: "You get this email because you asked.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"background-color:#fdf6e3", "background-color: #002b36 !important"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("the HTML has no %q", want)
		}
	}
}
