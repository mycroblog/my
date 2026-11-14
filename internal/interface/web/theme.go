// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
)

// The instance theme (ADR-0059 §5): the shell and the auth screens (authpages) load the current
// theme's stylesheet from a URL that names its revision, after the build's stylesheets, so the
// theme applies from the first paint without relaxing the CSP or varying the shell per visitor.

// PathTheme is the prefix of the theme stylesheets.
const PathTheme = "/theme/"

// ThemeStylesheetPath returns the path of the stylesheet of a theme revision. Its content never
// changes, so it is cached as immutable.
func ThemeStylesheetPath(revisionID string) string { return PathTheme + revisionID + ".css" }

// themeState is what the shell needs of the current theme: the path of its stylesheet ("" when
// the theme changes nothing) and the surface color in each color scheme for theme-color.
type themeState struct {
	Stylesheet  string
	Light, Dark string
}

// currentTheme returns the current theme, or the defaults without Options.Theme.
func (h *Handler) currentTheme(ctx context.Context) (themeState, error) {
	var (
		rev instance.ThemeRevision
		ok  bool
	)
	if h.o.Theme != nil {
		var err error
		if rev, ok, err = h.o.Theme(ctx); err != nil {
			return themeState{}, err
		}
	}
	if !ok {
		return defaultTheme(), nil
	}
	return themeState{
		Stylesheet: ThemeStylesheetPath(rev.ID),
		Light:      instance.ThemeColor(rev.Set.Tokens, themeColorToken, instance.SchemeLight),
		Dark:       instance.ThemeColor(rev.Set.Tokens, themeColorToken, instance.SchemeDark),
	}, nil
}

// defaultTheme is the theme state without a theme: no stylesheet, the default surface colors.
func defaultTheme() themeState {
	return themeState{
		Light: instance.ThemeColor(nil, themeColorToken, instance.SchemeLight),
		Dark:  instance.ThemeColor(nil, themeColorToken, instance.SchemeDark),
	}
}

// themeColorToken is the token whose colors are the shell's theme-color and the manifest's
// theme_color and background_color: the page background.
const themeColorToken = "color-surface"

// serveThemeStylesheet serves GET /theme/{file}: the stylesheet of the published revision
// {id}.css, as text/css that browsers never sniff as anything else.
func (h *Handler) serveThemeStylesheet(w http.ResponseWriter, r *http.Request) {
	id, ok := strings.CutSuffix(r.PathValue("file"), ".css")
	if !ok || id == "" || h.o.ThemeRevision == nil {
		h.o.NotFound(w, r)
		return
	}
	rev, found, err := h.o.ThemeRevision(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !found || rev.Empty() {
		h.o.NotFound(w, r)
		return
	}
	body := rev.Stylesheet()
	hd := w.Header()
	hd.Set("Content-Type", "text/css; charset=utf-8")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Cache-Control", cacheImmutable)
	hd.Set("ETag", etag(body))
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}
