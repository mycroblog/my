// SPDX-License-Identifier: AGPL-3.0-only

package maintenance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/mycroblog/my/internal/i18n"
)

// ClosurePage is what the static closure page shows (ADR-0088 §6): the page that the server's
// domain serves once the server is stopped after the closure of the service. `my admin closure
// export-html` fills it from the database and the configuration and renders it with
// RenderClosure, in the template and CSS of the maintenance page.
type ClosurePage struct {
	// ServerName names the server: maintenance_server_name, the instance's name, or the host name.
	ServerName string
	// Message is the operator's message of the closure announcement in plain text; "" for none.
	Message string
	// ClosesAt is the closing date of the announcement; zero leaves it out.
	ClosesAt time.Time
	// StatusPageURL is the instance's status page (ADR-0085 §6); "" for none.
	StatusPageURL string
	// Theme holds the values of the tokens the page's CSS uses (TokenNames).
	Theme Theme
}

// RenderClosure writes the closure page in the locales of ls, one section each in that order;
// the first sets the document's language and title. The page always carries its
// Content-Security-Policy in a <meta> element, since no server of this project serves it.
func RenderClosure(p ClosurePage, ls []*i18n.Localizer) ([]byte, error) {
	if len(ls) == 0 {
		return nil, fmt.Errorf("maintenance: no locale")
	}
	sections := make([]section, 0, len(ls))
	for _, l := range ls {
		s := section{
			Lang: l.Locale(), Dir: l.Direction(), ServerName: p.ServerName,
			Heading: l.ServerClosureTitle(), Description: l.ServerClosureDescription(p.ServerName),
			NoticeHeading: l.ServerClosureNotice(), Message: p.Message,
			StatusPageURL: p.StatusPageURL, StatusPageText: l.ServerClosureStatusPage(),
		}
		sections = append(sections, s.withDate(l.ServerClosureClosedOn(), p.ClosesAt))
	}
	html, _, err := render(document{Kind: "closure", Title: ls[0].ServerClosureDocumentTitle(p.ServerName), Sections: sections}, p.Theme, RenderOptions{MetaCSP: true})
	return html, err
}

// ClosureGonePaths and ClosureGonePrefixes are the paths that the stopped server's domain answers
// with 410 Gone and no page (ADR-0088 §2 and §6): ActivityPub (the actors, objects, collections,
// and inboxes), WebFinger, host-meta, NodeInfo, and the profiles. They repeat the paths of the
// packages of the normal server, which this package does not import. Every other path serves
// the closure page.
var (
	ClosureGonePaths = []string{
		"/inbox", "/.well-known/webfinger", "/.well-known/host-meta", "/.well-known/host-meta.json", "/.well-known/nodeinfo",
	}
	ClosureGonePrefixes = []string{"/ap/", "/nodeinfo/", "/@"}
)

// ClosureConfigs are the names of the examples of front-end configuration that ClosureConfig
// writes, in order.
var ClosureConfigs = []string{"nginx.conf", "Caddyfile", "cloudfront-function.js"}

// closureConfigs are the templates of ClosureConfigs, templates/closure/<name>.tmpl. Each serves ClosureGonePaths and
// ClosureGonePrefixes with 410 and every other path with <locale>.html for the first language of
// Accept-Language that has a page, or index.html with every locale.
var closureConfigs = template.Must(template.New("").Funcs(template.FuncMap{
	"json": func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	},
	// caddyPaths lists the paths and the prefixes as the operands of Caddy's path matcher.
	"caddyPaths": func(paths, prefixes []string) string {
		all := append([]string(nil), paths...)
		for _, p := range prefixes {
			all = append(all, p+"*")
		}
		return strings.Join(all, " ")
	},
}).ParseFS(templateFS, "templates/closure/*"))

// closureConfigData is the data of the templates of ClosureConfigs.
type closureConfigData struct {
	Host     string
	Locales  []string
	Paths    []string
	Prefixes []string
	// Root is the directory of the pages in the examples, which the operator changes.
	Root string
}

// ClosureConfig writes the example of the front end's configuration name (one of ClosureConfigs)
// for host, choosing among the pages of locales.
func ClosureConfig(name, host string, locales []string) ([]byte, error) {
	d := closureConfigData{Host: host, Locales: locales, Paths: ClosureGonePaths, Prefixes: ClosureGonePrefixes, Root: "/srv/closure"}
	var buf bytes.Buffer
	if err := closureConfigs.ExecuteTemplate(&buf, name+".tmpl", d); err != nil {
		return nil, fmt.Errorf("maintenance: %s: %w", name, err)
	}
	return buf.Bytes(), nil
}
