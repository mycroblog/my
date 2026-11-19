// SPDX-License-Identifier: AGPL-3.0-only

package payments

import (
	"bytes"
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/mycroblog/my/internal/app/supporters"
)

// The payment pages of a provider without hosted ones (supporters.PageServer) are documents of
// the web client's payment entry (ADR-0074 §11, web/app/pay.html): a shell that loads the
// entry's scripts and stylesheets, with the page's data (supporters.PageData) in a JSON data
// block that the entry renders. html/template writes the shell, escaping every value for its
// context and encoding the data as JSON that cannot end the script element. The entry's scripts
// carry the Subresource Integrity of the build's inventory, and the Content-Security-Policy
// allows no script but this site's and payjp.js, which takes the card in PAY.JP's iframe (PCI DSS
// 6.4.3; no setting changes it).

// PageEntry is what the shell loads to start the payment entry: web.Handler.PayEntry.
type PageEntry struct {
	// Script is the entry module; "" leaves the pages blank.
	Script string
	// Stylesheets are the CSS of the entry and of its static imports, and Preloads those imports.
	Stylesheets, Preloads []string
	// Integrity is the Subresource Integrity of Script and of each of Preloads, by URL.
	Integrity map[string]string
	// ViteClient is the client script of a Vite dev server, and DevOrigin that server's origin,
	// which the pages' policy then allows; both empty in production.
	ViteClient, DevOrigin string
}

// Shell renders the payment pages' documents: its Render is supporters.PageEnv.Render.
type Shell struct {
	Entry PageEntry
	// ThemeStylesheet returns the URL of the instance theme's stylesheet (ADR-0059), "" for none;
	// nil links none.
	ThemeStylesheet func(context.Context) string
	// Server returns the server that the pages say is paid: its name, its logos, and its short
	// description; nil names none.
	Server func(context.Context) supporters.PageInstance
	// Logger logs a document that could not be written; nil means slog.Default().
	Logger *slog.Logger
}

// The sources of payjp.js (ADR-0074 §11): its script, its iframes (the card element and the 3-D
// Secure), and the API it calls.
const (
	payjpScripts = "https://js.pay.jp"
	payjpFrames  = "https://js.pay.jp https://api.pay.jp"
	payjpConnect = "https://api.pay.jp"
)

// pageShellData is what the shell writes.
type pageShellData struct {
	Lang, Dir, Title string
	Entry            PageEntry
	ThemeStylesheet  string
	// Page is a struct, not a string: html/template encodes it as JSON in the data block and
	// escapes what could end the script element.
	Page supporters.PageData
}

// pageShell is the document of every payment page. The data block sits between the comments that
// keep Cloudflare's Email Address Obfuscation from rewriting the address of the card form into
// "[email protected]"; html/template drops the comments of a template, so they come from functions.
var pageShell = template.Must(template.New("pay").Funcs(template.FuncMap{
	"integrity": func(m map[string]string, u string) string { return m[u] },
	"emailOff":  func() template.HTML { return "<!--email_off-->" },
	"emailOn":   func() template.HTML { return "<!--/email_off-->" },
}).Parse(`<!doctype html>
<html lang="{{.Lang}}" dir="{{.Dir}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="color-scheme" content="light dark">
<meta name="referrer" content="no-referrer">
<meta name="robots" content="noindex, nofollow">
<title>{{.Title}}</title>
{{- range .Entry.Stylesheets}}
<link rel="stylesheet" href="{{.}}">
{{- end}}
{{- with .ThemeStylesheet}}
<link rel="stylesheet" href="{{.}}">
{{- end}}
{{- $integrity := .Entry.Integrity}}
{{- range .Entry.Preloads}}
<link rel="modulepreload" href="{{.}}"{{with integrity $integrity .}} integrity="{{.}}"{{end}}>
{{- end}}
{{- with .Entry.ViteClient}}
<script type="module" src="{{.}}"></script>
{{- end}}
{{- with .Entry.Script}}
<script type="module" src="{{.}}"{{with integrity $integrity .}} integrity="{{.}}"{{end}}></script>
{{- end}}
</head>
<body>
{{emailOff}}<script type="application/json" id="pay">{{.Page}}</script>{{emailOn}}
<div id="root"></div>
</body>
</html>
`))

// Render answers with the document of a payment page; it implements supporters.PageRenderer.
func (s Shell) Render(w http.ResponseWriter, r *http.Request, status int, doc supporters.PageDocument) {
	ctx := r.Context()
	d := pageShellData{Lang: doc.Lang, Dir: doc.Dir, Title: doc.Title, Entry: s.Entry, Page: doc.Data}
	if s.ThemeStylesheet != nil {
		d.ThemeStylesheet = s.ThemeStylesheet(ctx)
	}
	if s.Server != nil {
		server := s.Server(ctx)
		d.Page.Server = &server
	}
	var body bytes.Buffer
	if err := pageShell.Execute(&body, d); err != nil {
		log := s.Logger
		if log == nil {
			log = slog.Default()
		}
		log.ErrorContext(ctx, "render a payment page", slog.String("screen", doc.Data.Screen), slog.Any("error", err))
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Language", doc.Lang)
	hd.Set("Content-Security-Policy", PageContentSecurityPolicy(s.Entry.DevOrigin))
	hd.Set("Cache-Control", "no-store")
	hd.Set("Referrer-Policy", "no-referrer")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Add("Vary", "Accept-Language")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}

// PageContentSecurityPolicy is the policy of every payment page (ADR-0074 §11): the entry's
// scripts, stylesheets, fonts, and images from this site, payjp.js and its iframes and API, forms
// to this site only, and nothing else. dev is the origin of a Vite dev server, whose scripts,
// styles, and hot module replacement the pages load in development, "" in production. It has no
// nonce: the data block is not executed, and no script is inline.
func PageContentSecurityPolicy(dev string) string {
	with := func(base string, more ...string) string {
		for _, m := range more {
			if m != "" {
				base += " " + m
			}
		}
		return base
	}
	style := "'self'"
	connect := payjpConnect
	if dev != "" {
		// The dev server's client injects <style> elements and talks to it over a WebSocket.
		style = "'self' 'unsafe-inline'"
		connect = with(connect, "'self'", dev, websocketOrigin(dev))
	}
	return strings.Join([]string{
		"default-src 'none'",
		with("script-src 'self'", dev, payjpScripts),
		with("style-src "+style, dev),
		"frame-src " + payjpFrames,
		"connect-src " + connect,
		with("img-src 'self' data:", dev),
		with("font-src 'self'", dev),
		"form-action 'self'",
		"frame-ancestors 'none'",
		"base-uri 'none'",
		"object-src 'none'",
	}, "; ")
}

// websocketOrigin is the ws: or wss: origin of an http: or https: origin.
func websocketOrigin(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	if u.Scheme == "https" {
		return "wss://" + u.Host
	}
	return "ws://" + u.Host
}
