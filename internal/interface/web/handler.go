// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
)

// Options configure a Handler.
type Options struct {
	// Build is the Vite build of app/web; Embedded() in production.
	Build fs.FS
	// DevViteOrigin, when set, is the origin of a Vite dev server (http://localhost:5173).
	// The shell then loads the client from it with hot module replacement, and its
	// Content-Security-Policy is relaxed for that origin. Development only.
	DevViteOrigin string

	// Host is the serving domain of the instance (ADR-0018).
	Host string
	// WebOrigin is the configured origin of the web client (web_origin). When it is on Host, the
	// server serves the client there, so the shell's client calls the API at it too: plain
	// http://localhost:8080 in a local run. Otherwise, and when empty, the API is at https://Host.
	WebOrigin string
	// MediaOrigin is the origin of the media files' URLs (media_origin) when they are served from a
	// CDN; empty when they are on the server's own origin. The Content-Security-Policy then lets
	// images and media load from it.
	MediaOrigin string
	// AccountDomain is the domain of handles; empty means Host.
	AccountDomain string
	// InstanceName returns the instance's display name, the same in every locale, which the
	// shell, the Web App Manifest, and link previews show; nil or "" names the instance by Host.
	// It is called per request, so that a change of the name applies without a restart. Handles
	// and URLs always use the domains.
	InstanceName func(ctx context.Context) string
	// InstanceDescription returns the instance's one-paragraph summary, as plain text, which the
	// link preview of the landing page (/) shows; nil or "" leaves the preview without a
	// description. It is called per request, like InstanceName.
	InstanceDescription func(ctx context.Context) string
	// DefaultLocale returns the instance's current default locale, for <html lang dir>, the
	// runtime config, and the manifest; nil means the source locale. It is read per request so
	// that a change of the setting applies without a restart. The HTML never depends on
	// Accept-Language, so that shared caches keep one copy (ADR-0028).
	DefaultLocale func(ctx context.Context) string
	// Theme returns the current instance theme (ADR-0059), and false when it changes nothing. The
	// shell and the manifest take their theme colors from its surface token, and the shell loads its
	// stylesheet. It is called per request, so that a saved theme applies without a restart; nil
	// means the default theme.
	Theme func(ctx context.Context) (instance.ThemeRevision, bool, error)
	// ThemeRevision returns a theme revision, current or not, for its stylesheet at
	// ThemeStylesheetPath, and false when there is none; nil serves no theme stylesheets.
	ThemeRevision func(ctx context.Context, id string) (instance.ThemeRevision, bool, error)

	// Previews reads the profiles and statuses that the shells of profile pages (/@alice) and
	// post pages (/@alice/status/{id}) describe in their link preview meta tags; nil serves the
	// generic shell there.
	Previews Previews
	// Documents reads which of the instance's documents are published, for the link previews of
	// their pages (/about/terms, /about/privacy, /about/commerce); nil serves the generic shell
	// there. It is called per request, so that publishing a document applies at once.
	Documents Documents
	// VoiceRooms reads the summaries of the voice chat rooms for the link previews of their pages
	// (/voice/{room_id}); nil serves the generic shell there. It is called per request.
	VoiceRooms VoiceRooms
	// Messages formats the texts of link previews; nil loads the embedded catalogs.
	Messages *i18n.Bundle
	// DefaultImage returns the instance's preview image for profiles and posts without images,
	// for a preview in language lang (ADR-0028), or nil for the built-in image at
	// PathDefaultImage. An image without alt text gets a generic one naming the instance.
	// It is called per request, so that a change of the setting applies without a restart; nil
	// means the built-in image.
	DefaultImage func(ctx context.Context) (*Image, error)

	// Settings returns the current instance settings, whose search_indexing, ai_usage, and
	// security_contact decide what /robots.txt, the pages, and /.well-known/ state to crawlers
	// (ADR-0057). It is called per request, so that a change applies without a restart; nil
	// reads every setting as its default, which denies search engines and AI.
	Settings func(ctx context.Context) (instance.Settings, error)
	// Now returns the current time, for the expiry of security.txt; nil means time.Now.
	Now func() time.Time

	// NotFound writes the 404 response of the server, for paths the web client does not own.
	NotFound func(w http.ResponseWriter, r *http.Request)
	// Logger logs warnings at start; nil means slog.Default().
	Logger *slog.Logger
}

// Router registers handlers for http.ServeMux patterns.
type Router interface {
	Handle(pattern string, h http.Handler)
}

// Paths that the Handler serves besides the SPA routes.
const (
	PathAssets      = "/" + assetsDir + "/"
	PathWebManifest = "/manifest.webmanifest"
	PathLicenses    = "/" + licensesDir + "/"
)

// reservedPrefixes are the paths of the server's other endpoints. The SPA has no routes under
// them, so a request there that no endpoint serves is the server's 404, never the shell.
// /uploads is the blob prefix of media as received, which is never served (it may carry EXIF).
var reservedPrefixes = []string{"/api", "/oauth", "/auth", "/mail", "/.well-known", "/nodeinfo", "/ap", "/media", "/uploads", "/theme", "/" + licensesDir, PathRobots}

// Cache-Control values (ADR-0028).
const (
	// cacheShell lets shared caches keep the shell briefly; it is the same for every visitor.
	cacheShell = "public, max-age=60, stale-while-revalidate=300"
	// cacheImmutable is for content-hashed files, whose URL changes with their content.
	cacheImmutable = "public, max-age=31536000, immutable"
	// cacheRevalidate is for files at fixed URLs, such as sw.js and icon.svg.
	cacheRevalidate = "no-cache"
	cacheDev        = "no-store"
)

// Handler serves the web client. Its ServeHTTP serves the shell at SPA routes, and the
// files at the root of the build (sw.js, icon.svg).
type Handler struct {
	o         Options
	tags      entryTags
	apiOrigin string
	hasIcon   bool
	// hasAppleTouchIcon reports whether the build has apple-touch-icon.png.
	hasAppleTouchIcon bool
	// hasThemeScript reports whether the build has theme.js.
	hasThemeScript bool
	// icons are the manifest icons whose files the build has.
	icons []manifestIcon
	csp   string
	// localized holds the shell and the manifest of each default locale seen, rendered once
	// for the instance name they show.
	localized sync.Map // locale -> *localizedDocuments
	// files are the servable files of the build, by path, with their ETags.
	files map[string]string
	// previewVersion identifies the template, the assets, and the instance settings that
	// previews render, for their ETags.
	previewVersion string
}

// localizedDocuments are the documents that depend on the default locale, and the instance
// name, the theme, and the instance's crawler policy they show.
type localizedDocuments struct {
	name            string
	theme           themeState
	policy          instance.CrawlerPolicy
	shell, manifest document
}

// document is a response rendered once.
type document struct {
	body        []byte
	etag        string
	contentType string
	// crawlers are the headers that state the document's crawler policy; nil for none.
	crawlers *crawlerDirectives
}

// New returns a Handler on o. A build without a Vite manifest is not an error: the shell then
// loads no client, and New logs a warning.
func New(o Options) (*Handler, error) {
	if o.Build == nil {
		return nil, errors.New("web: no build")
	}
	if o.AccountDomain == "" {
		o.AccountDomain = o.Host
	}
	if o.DefaultLocale == nil {
		o.DefaultLocale = func(context.Context) string { return i18n.SourceLocale }
	}
	if o.NotFound == nil {
		o.NotFound = func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		}
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Messages == nil {
		m, err := i18n.Load()
		if err != nil {
			return nil, err
		}
		o.Messages = m
	}
	h := &Handler{o: o}

	files, err := buildFiles(o.Build)
	if err != nil {
		return nil, err
	}
	h.files = files
	_, h.hasIcon = files[iconPath]
	_, h.hasAppleTouchIcon = files[appleTouchIconPath]
	_, h.hasThemeScript = files[themeScriptPath]
	h.icons = []manifestIcon{}
	for _, icon := range appIcons {
		if _, ok := files[strings.TrimPrefix(icon.Src, "/")]; ok {
			h.icons = append(h.icons, icon)
		}
	}

	var tags entryTags
	apiOrigin, err := serverOrigin(o.Host, o.WebOrigin)
	if err != nil {
		return nil, err
	}
	mediaOrigin := ""
	if o.MediaOrigin != "" {
		u, err := url.Parse(o.MediaOrigin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("web: media origin %q is not an http(s) origin", o.MediaOrigin)
		}
		// On the server's own origin, 'self' already allows it.
		if origin := u.Scheme + "://" + u.Host; origin != apiOrigin {
			mediaOrigin = origin
		}
	}
	if o.DevViteOrigin != "" {
		dev, err := devOrigin(o.DevViteOrigin)
		if err != nil {
			return nil, err
		}
		h.o.DevViteOrigin = dev.String()
		tags = entryTags{Script: h.o.DevViteOrigin + "/src/main.tsx"}
		h.csp = withMediaOrigin(devContentSecurityPolicy(dev), mediaOrigin)
		// The dev server setup is plain HTTP on localhost.
		apiOrigin = "http://" + o.Host
	} else {
		tags, err = readEntry(o.Build)
		if errors.Is(err, errNoBuild) {
			o.Logger.Warn("the web client is not built into this binary; the shell loads no client", slog.Any("error", err))
		} else if err != nil {
			return nil, err
		}
		h.csp = withMediaOrigin(contentSecurityPolicy, mediaOrigin)
		if strings.HasPrefix(apiOrigin, "https://") {
			h.csp += upgradeInsecureRequests
		}
	}

	h.tags, h.apiOrigin = tags, apiOrigin
	version := []string{
		shellSource, h.o.DevViteOrigin, h.apiOrigin, h.o.Host, h.o.AccountDomain,
		strconv.FormatBool(h.hasIcon), strconv.FormatBool(h.hasAppleTouchIcon), tags.Script,
	}
	version = append(append(version, tags.Stylesheet...), tags.Preload...)
	h.previewVersion = etag([]byte(strings.Join(version, "\x00")))
	// Render once now, so that a template or encoding error fails the start.
	if _, err := h.render(i18n.SourceLocale, o.Host, defaultTheme(), instance.DenyAll); err != nil {
		return nil, err
	}
	return h, nil
}

// serverOrigin returns the origin at which the server serves the shell and the client API: webOrigin
// when it is on host, https://host otherwise. An origin on another host is a client served elsewhere
// (a development server, say), which says nothing about the server's own scheme.
func serverOrigin(host, webOrigin string) (string, error) {
	if webOrigin != "" {
		u, err := url.Parse(webOrigin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", fmt.Errorf("web: web origin %q is not an http(s) origin", webOrigin)
		}
		if strings.EqualFold(u.Host, host) {
			return u.Scheme + "://" + host, nil
		}
	}
	return "https://" + host, nil
}

// instanceLocale returns the instance's current default locale.
func (h *Handler) instanceLocale(ctx context.Context) string {
	if locale := h.o.DefaultLocale(ctx); locale != "" {
		return locale
	}
	return i18n.SourceLocale
}

// instanceName returns the instance's display name, or its host when it has none.
func (h *Handler) instanceName(ctx context.Context) string {
	if h.o.InstanceName != nil {
		if name := h.o.InstanceName(ctx); name != "" {
			return name
		}
	}
	return h.o.Host
}

// documents returns the shell and the manifest for the current default locale, the instance's
// name in it, the current theme, and the instance's crawler policy. The generic shell is never
// indexed: it is the shell of the app's screens, of remote accounts' pages, and of pages that
// are not found (ADR-0057).
func (h *Handler) documents(ctx context.Context) (*localizedDocuments, error) {
	locale := h.instanceLocale(ctx)
	name := h.instanceName(ctx)
	theme, err := h.currentTheme(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := h.settings(ctx)
	if err != nil {
		return nil, err
	}
	policy := settings.CrawlerPolicy().NoIndex()
	if d, ok := h.localized.Load(locale); ok {
		if d := d.(*localizedDocuments); d.name == name && d.theme == theme && d.policy == policy {
			return d, nil
		}
	}
	return h.render(locale, name, theme, policy)
}

// render renders the documents of a default locale that show the instance name, the theme, and
// the crawler policy, and caches them in place of those of an earlier name, theme, or policy.
func (h *Handler) render(locale, name string, theme themeState, policy instance.CrawlerPolicy) (*localizedDocuments, error) {
	crawlers := directivesOf(policy)
	shell, err := renderShell(shellData{
		Lang: locale, Dir: i18n.Direction(locale), Title: name, Theme: theme, Crawlers: crawlers,
		HasIcon: h.hasIcon, HasAppleTouchIcon: h.hasAppleTouchIcon, HasThemeScript: h.hasThemeScript, ViteClient: viteClient(h.o.DevViteOrigin), Entry: h.tags,
		Config: h.runtimeConfig(locale, name),
	})
	if err != nil {
		return nil, err
	}
	shell.crawlers = &crawlers
	// The headers state more than the meta tags (Content-Usage tells deny_training from deny), so
	// the ETag covers the policy too.
	shell.etag = etag([]byte(string(shell.body) + "\x00" + string(policy.Search) + "\x00" + string(policy.AI)))
	manifest, err := renderWebManifest(locale, name, theme, h.icons)
	if err != nil {
		return nil, err
	}
	d := &localizedDocuments{name: name, theme: theme, policy: policy, shell: shell, manifest: manifest}
	h.localized.Store(locale, d)
	return d, nil
}

// runtimeConfig returns the runtime config of the shell for the default locale and the
// instance's name in it.
func (h *Handler) runtimeConfig(locale, name string) runtimeConfig {
	return runtimeConfig{
		APIOrigin: h.apiOrigin, InstanceName: name, AccountDomain: h.o.AccountDomain, DefaultLocale: locale,
	}
}

func viteClient(origin string) string {
	if origin == "" {
		return ""
	}
	return origin + "/@vite/client"
}

// devOrigin parses the origin of a Vite dev server.
func devOrigin(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" || s != u.Scheme+"://"+u.Host {
		return nil, fmt.Errorf("web: dev Vite origin %q must be an origin such as http://localhost:5173, without path or trailing slash", s)
	}
	return u, nil
}

// buildFiles lists the files of the build that are served as they are, with their ETags:
// every file except the Vite manifest and other hidden files, and the index.html that Vite
// writes (the shell replaces it).
func buildFiles(build fs.FS) (map[string]string, error) {
	files := map[string]string{}
	err := fs.WalkDir(build, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != "." && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || p == entryKey {
			return nil
		}
		data, err := fs.ReadFile(build, p)
		if err != nil {
			return err
		}
		files[p] = etag(data)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("web: list the build: %w", err)
	}
	return files, nil
}

// etag is a strong entity tag of content.
func etag(content []byte) string {
	sum := sha256.Sum256(content)
	return `"` + base64.RawURLEncoding.EncodeToString(sum[:18]) + `"`
}

// Register registers the hashed assets, the Web App Manifest, and the built-in preview image on r. The shell and the
// files at the root of the build are served by ServeHTTP, which the caller routes to.
func (h *Handler) Register(r Router) {
	r.Handle("GET "+PathAssets+"{path...}", http.HandlerFunc(h.asset))
	r.Handle("GET "+PathWebManifest, http.HandlerFunc(h.webManifest))
	r.Handle("GET "+PathLicenses+"{file}", http.HandlerFunc(h.license))
	r.Handle("GET "+PathDefaultImage, http.HandlerFunc(h.serveDefaultImage))
	r.Handle("GET "+PathTheme+"{file}", http.HandlerFunc(h.serveThemeStylesheet))
	r.Handle("GET "+PathRobots, http.HandlerFunc(h.serveRobots))
	r.Handle("GET "+PathTDMRep, http.HandlerFunc(h.serveTDMRep))
	r.Handle("GET "+PathSecurityTxt, http.HandlerFunc(h.serveSecurityTxt))
}

// ServeHTTP serves a GET request for an SPA route: a file at the root of the build when one
// has the requested name, the server's 404 under the paths of other endpoints, the shell with
// link preview meta tags at the pages about the instance (/ and /about), at the pages of its
// published documents, and at profile and post pages, and the generic shell otherwise.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) || reserved(p) {
		h.o.NotFound(w, r)
		return
	}
	if name := strings.TrimPrefix(p, "/"); name != "" && !strings.Contains(name, "/") {
		if _, ok := h.files[name]; ok {
			if name == serviceWorkerPath {
				// A worker runs under the CSP of its own script's response, not the page's; the server's
				// default for non-documents (default-src 'none') would block its fetches.
				w.Header().Set("Content-Security-Policy", serviceWorkerContentSecurityPolicy)
			}
			h.serveFile(w, r, name, cacheRevalidate)
			return
		}
	}
	if p == pathLanding || p == pathAbout {
		h.serveInstancePage(w, r, p, "")
		return
	}
	if page, ok := documentPages[p]; ok && h.o.Documents != nil {
		h.serveDocumentPage(w, r, p, page)
		return
	}
	if h.o.Previews != nil {
		if route, ok := h.parsePreviewRoute(p); ok {
			h.servePreview(w, r, route)
			return
		}
	}
	if h.o.VoiceRooms != nil {
		if id, ok := voiceRoomID(p); ok {
			h.serveVoiceRoom(w, r, id)
			return
		}
	}
	h.serveShell(w, r)
}

// reserved reports whether p is under the path of another endpoint of the server.
func reserved(p string) bool {
	for _, prefix := range reservedPrefixes {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

func (h *Handler) serveShell(w http.ResponseWriter, r *http.Request) {
	d, err := h.documents(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.shellHeaders(w.Header())
	d.shell.serve(w, r)
}

// shellHeaders sets the headers that every shell carries: Vary: Accept, since the same URL
// may be an ActivityPub document, the Content-Security-Policy, and short public caching.
func (h *Handler) shellHeaders(hd http.Header) {
	addVary(hd, "Accept")
	hd.Set("Content-Security-Policy", h.csp)
	if h.o.DevViteOrigin != "" {
		hd.Set("Cache-Control", cacheDev)
	} else {
		hd.Set("Cache-Control", cacheShell)
	}
}

func (h *Handler) webManifest(w http.ResponseWriter, r *http.Request) {
	d, err := h.documents(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", cacheShell)
	d.manifest.serve(w, r)
}

// fail answers a document that cannot be rendered with 500 after logging it.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.o.Logger.ErrorContext(r.Context(), "web: render failed", slog.Any("error", err))
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// asset serves GET /assets/{path...}: content-hashed files, cached for a year.
func (h *Handler) asset(w http.ResponseWriter, r *http.Request) {
	// Only clean paths are in files, so "..", "." and empty segments find nothing.
	name := assetsDir + "/" + r.PathValue("path")
	if _, ok := h.files[name]; !ok {
		h.o.NotFound(w, r)
		return
	}
	h.serveFile(w, r, name, cacheImmutable)
}

// license serves GET /licenses/{file}: a license notice of the build, as plain text whatever its extension
// (the npm packages' notices are Markdown), so that the browser shows it as it is. Its URL is fixed, so it is
// revalidated.
func (h *Handler) license(w http.ResponseWriter, r *http.Request) {
	name := licensesDir + "/" + r.PathValue("file")
	if _, ok := h.files[name]; !ok {
		h.o.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	h.serveFile(w, r, name, cacheRevalidate)
}

// serveFile serves a file of the build with its ETag, answering conditional requests.
func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, name, cacheControl string) {
	f, err := h.o.Build.Open(name)
	if err != nil {
		h.o.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	content, ok := f.(io.ReadSeeker)
	if !ok {
		data, err := io.ReadAll(f)
		if err != nil {
			h.o.NotFound(w, r)
			return
		}
		content = bytes.NewReader(data)
	}
	hd := w.Header()
	hd.Set("Cache-Control", cacheControl)
	hd.Set("ETag", h.files[name])
	http.ServeContent(w, r, name, time.Time{}, content)
}

// serve writes the document with its ETag and its crawler headers, answering conditional
// requests.
func (d document) serve(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	if d.crawlers != nil {
		d.crawlers.set(hd)
	}
	hd.Set("Content-Type", d.contentType)
	hd.Set("ETag", d.etag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(d.body))
}

// addVary adds value to the Vary header unless it is already there.
func addVary(h http.Header, value string) {
	if !slices.Contains(h.Values("Vary"), value) {
		h.Add("Vary", value)
	}
}

// Stylesheet returns the URL of the stylesheet that the build makes for the server-rendered
// pages from the client's design tokens (app/web/src/server-pages.css, ADR-0030), or "" when the
// build has none (no build, or a Vite dev server).
func (h *Handler) Stylesheet() string { return h.tags.ServerPages }

// PasskeySignInScript returns the URL of the module script that signs in with a passkey on the
// server-rendered sign-in page (app/web/src/server-pages/passkey-sign-in.ts), or "" when the
// build has none.
func (h *Handler) PasskeySignInScript() string { return h.tags.PasskeySignIn }
