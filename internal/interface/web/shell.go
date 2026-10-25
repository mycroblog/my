// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"strings"

	"github.com/mycroblog/my/internal/i18n"
)

// runtimeConfig is the per-instance configuration of the web client, in the shell's JSON data
// block. The build is the same for every instance and holds none of it (ADR-0028).
type runtimeConfig struct {
	// APIOrigin is the origin of the client API (ADR-0016).
	APIOrigin string `json:"apiOrigin"`
	// InstanceName is the instance's display name in the default locale, or its host when it has
	// none.
	InstanceName string `json:"instanceName"`
	// AccountDomain is the domain of local handles (@alice@{accountDomain}).
	AccountDomain string `json:"accountDomain"`
	// DefaultLocale is the instance's default locale.
	DefaultLocale string `json:"defaultLocale"`
}

type shellData struct {
	Lang, Dir, Title string
	// Theme gives the theme colors and the theme's stylesheet (ADR-0059).
	Theme themeState
	// Crawlers state the page's crawler policy in its meta tags (ADR-0057).
	Crawlers                   crawlerDirectives
	HasIcon, HasAppleTouchIcon bool
	// HasThemeScript reports whether the build has theme.js, which applies the stored color theme.
	HasThemeScript bool
	// ViteClient is the Vite client script of a dev server, empty in production.
	ViteClient string
	Entry      entryTags
	// Config is a struct, not a string: html/template encodes it as JSON in the data block and
	// escapes what could end the script element.
	Config runtimeConfig
	// Preview holds the link preview meta tags of the landing page, a profile, or a post page,
	// nil for the generic shell.
	Preview *previewMeta
}

// shellTemplate is the shell of the SPA: the generic shell, or with Preview the shell of the
// landing page, a profile, or a post page. html/template escapes every value for its context; nothing is
// marked as safe HTML, and user data never becomes template.HTML (ADR-0028).
var shellTemplate = template.Must(template.New("shell").Parse(shellSource))

// shellSource is the source of shellTemplate. Its hash is part of the ETag of previews.
const shellSource = `<!doctype html>
<html lang="{{.Lang}}" dir="{{.Dir}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>{{.Title}}</title>
{{- with .Crawlers.Robots}}
<meta name="robots" content="{{.}}">
{{- end}}
{{- if .Crawlers.TDMReservation}}
<meta name="tdm-reservation" content="1">
{{- end}}
{{- with .Preview}}
<meta property="og:type" content="{{.Type}}">
<meta property="og:site_name" content="{{.SiteName}}">
<meta property="og:title" content="{{.Title}}">
<meta property="og:url" content="{{.URL}}">
{{- if .Description}}
<meta property="og:description" content="{{.Description}}">
{{- end}}
<meta property="og:locale" content="{{.Locale}}">
{{- with .Image}}
<meta property="og:image" content="{{.URL}}">
{{- if and .Width .Height}}
<meta property="og:image:width" content="{{.Width}}">
<meta property="og:image:height" content="{{.Height}}">
{{- end}}
{{- if .Alt}}
<meta property="og:image:alt" content="{{.Alt}}">
{{- end}}
{{- end}}
{{- if .PublishedTime}}
<meta property="article:published_time" content="{{.PublishedTime}}">
<meta property="article:author" content="{{.AuthorURL}}">
{{- end}}
{{- if .Username}}
<meta property="profile:username" content="{{.Username}}">
{{- end}}
<meta name="twitter:card" content="{{.Card}}">
{{- if .Creator}}
<meta name="fediverse:creator" content="{{.Creator}}">
{{- end}}
<link rel="canonical" href="{{.URL}}">
{{- if .ActivityPubID}}
<link rel="alternate" type="application/activity+json" href="{{.ActivityPubID}}">
{{- end}}
{{- end}}
<meta name="theme-color" media="(prefers-color-scheme: light)" content="{{.Theme.Light}}">
<meta name="theme-color" media="(prefers-color-scheme: dark)" content="{{.Theme.Dark}}">
<link rel="manifest" href="` + PathWebManifest + `">
{{- if .HasIcon}}
<link rel="icon" href="/` + iconPath + `" type="image/svg+xml">
{{- end}}
{{- if .HasAppleTouchIcon}}
<link rel="apple-touch-icon" href="/` + appleTouchIconPath + `">
{{- end}}
{{- if .HasThemeScript}}
<script src="/` + themeScriptPath + `"></script>
{{- end}}
{{- range .Entry.Stylesheet}}
<link rel="stylesheet" href="{{.}}">
{{- end}}
{{- with .Theme.Stylesheet}}
<link rel="stylesheet" href="{{.}}">
{{- end}}
{{- range .Entry.Preload}}
<link rel="modulepreload" href="{{.}}">
{{- end}}
<script type="application/json" id="config">{{.Config}}</script>
{{- if .ViteClient}}
<script type="module" src="{{.ViteClient}}"></script>
{{- end}}
{{- if .Entry.Script}}
<script type="module" src="{{.Entry.Script}}"></script>
{{- end}}
</head>
<body>
<div id="root"></div>
</body>
</html>
`

func renderShell(d shellData) (document, error) {
	var buf bytes.Buffer
	if err := shellTemplate.Execute(&buf, d); err != nil {
		return document{}, fmt.Errorf("web: render the shell: %w", err)
	}
	return document{body: buf.Bytes(), etag: etag(buf.Bytes()), contentType: "text/html; charset=utf-8"}, nil
}

// contentSecurityPolicy is the policy of the web client's documents (ADR-0019 §9). It has no
// nonce, so that the shell can be cached (ADR-0028). The client API is on the
// server's own origin. Trusted Types are required for every script sink, and `trusted-types`
// allows exactly the client's two policies, each created once (no 'allow-duplicates'):
// my-server-sanitized-html (@my/ui PostContent, the server-sanitized post HTML) and
// my-service-worker (app/web src/lib/pwa.ts, the worker's fixed URL). No other script can make a
// policy of its own. Over HTTPS it ends with upgradeInsecureRequests.
const contentSecurityPolicy = "default-src 'none'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self' blob:; " +
	"media-src 'self' blob:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"manifest-src 'self'; " +
	"worker-src 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'none'; " +
	"object-src 'none'; " +
	"require-trusted-types-for 'script'; " +
	"trusted-types my-server-sanitized-html my-service-worker"

// withMediaOrigin lets the policy's images and media come from the CDN origin of the media files
// (media_origin) as well; an empty origin leaves it unchanged.
func withMediaOrigin(csp, origin string) string {
	if origin == "" {
		return csp
	}
	return strings.NewReplacer(
		"img-src 'self' blob:", "img-src 'self' blob: "+origin,
		"media-src 'self' blob:", "media-src 'self' blob: "+origin,
	).Replace(csp)
}

// upgradeInsecureRequests ends the policy of a server served over HTTPS. A server served over plain
// HTTP on localhost leaves it out: WebKit upgrades even loopback requests, which then fail.
const upgradeInsecureRequests = "; upgrade-insecure-requests"

// serviceWorkerContentSecurityPolicy is the policy of the Service Worker (sw.js), which applies to the
// worker itself: it only fetches from its own origin (the build, the shell, public media, the pages
// it passes through) and loads no other script.
const serviceWorkerContentSecurityPolicy = "default-src 'none'; connect-src 'self'"

// devContentSecurityPolicy relaxes the policy for a Vite dev server at origin: its scripts and
// files, its hot module replacement WebSocket, and the <style> elements its client injects.
// Trusted Types and the upgrade to HTTPS are left out, since the dev server's client and the
// plain-HTTP setup do not satisfy them.
func devContentSecurityPolicy(origin *url.URL) string {
	o := origin.Scheme + "://" + origin.Host
	ws := strings.Replace(o, "http", "ws", 1)
	return "default-src 'none'; " +
		"script-src 'self' " + o + "; " +
		"style-src 'self' 'unsafe-inline' " + o + "; " +
		"img-src 'self' blob: " + o + "; " +
		"media-src 'self' blob: " + o + "; " +
		"font-src 'self' " + o + "; " +
		"connect-src 'self' " + o + " " + ws + "; " +
		"manifest-src 'self'; " +
		"worker-src 'self'; " +
		"form-action 'self'; " +
		"frame-ancestors 'none'; " +
		"base-uri 'none'; " +
		"object-src 'none'"
}

// webManifest is the Web App Manifest (https://www.w3.org/TR/appmanifest/).
type webManifest struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	ShortName       string         `json:"short_name"`
	StartURL        string         `json:"start_url"`
	Scope           string         `json:"scope"`
	Display         string         `json:"display"`
	Lang            string         `json:"lang"`
	Dir             string         `json:"dir"`
	ThemeColor      string         `json:"theme_color"`
	BackgroundColor string         `json:"background_color"`
	Icons           []manifestIcon `json:"icons"`
}

type manifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

// renderWebManifest generates the manifest from the instance's name in the default locale and its
// theme. A manifest has one color scheme, so both colors are the light surface (ADR-0059 §5).
func renderWebManifest(locale, name string, theme themeState, icons []manifestIcon) (document, error) {
	m := webManifest{
		ID: "/", Name: name, ShortName: name, StartURL: "/", Scope: "/", Display: "standalone",
		Lang: locale, Dir: i18n.Direction(locale), ThemeColor: theme.Light, BackgroundColor: theme.Light,
		Icons: icons,
	}
	body, err := json.Marshal(m)
	if err != nil {
		return document{}, fmt.Errorf("web: render the manifest: %w", err)
	}
	body = append(body, '\n')
	return document{body: body, etag: etag(body), contentType: "application/manifest+json"}, nil
}
