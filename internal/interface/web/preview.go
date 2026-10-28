// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"context"
	"errors"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rivo/uniseg"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
)

// This file serves the shells of profile pages (/@alice) and post pages (/@alice/status/{id}) with
// the meta tags of link previews: OGP, the X card, and fediverse:creator (ADR-0028).

// Previews is what the shells of profile and post pages read: profiles and statuses as an
// anonymous visitor sees them. *appcontent.Previews implements it. Both fail with an error
// wrapping appcontent.ErrNotFound for anything an anonymous visitor may not see.
type Previews interface {
	Profile(ctx context.Context, username string) (appcontent.ProfilePreview, error)
	Status(ctx context.Context, statusID string) (appcontent.StatusPreview, error)
}

// Image is an image of a link preview.
type Image struct {
	URL string
	// Width and Height are in pixels; zero when unknown.
	Width, Height int
	// Alt is the alt text, "" for none.
	Alt string
}

// Link preview limits.
const (
	// descriptionLength is the length of og:description in grapheme clusters, the ellipsis
	// included (ADR-0028: about 200 characters).
	descriptionLength = 200
	// Minimums of the X large image card (summary_large_image); smaller images get the
	// summary card.
	largeCardMinWidth  = 300
	largeCardMinHeight = 157
	// largeCardMinRatio is the smallest width / height of an image shown as a large card:
	// X crops it to about 2:1, so images up to 16:9 (1.78) still fit.
	largeCardMinRatio = 1.7
)

// Values of og:type and twitter:card.
const (
	ogTypeWebsite    = "website"
	ogTypeProfile    = "profile"
	ogTypeArticle    = "article"
	cardSummary      = "summary"
	cardSummaryLarge = "summary_large_image"
)

// previewMeta holds the values of the meta tags of a link preview. Every field is plain text
// or a URL, which the template escapes for its attribute.
type previewMeta struct {
	Type, SiteName, Title, URL, Description, Locale string
	Image                                           *Image
	// PublishedTime and AuthorURL are for posts (article:*), Username for profiles
	// (profile:username).
	PublishedTime, AuthorURL, Username string
	// Creator (fediverse:creator) and ActivityPubID (the ActivityPub alternate) are for profiles
	// and posts; the landing page has neither.
	Card, Creator, ActivityPubID string
}

// uuidPattern matches the canonical form of a UUIDv7, the only form of a status ID in a post
// page URL (ADR-0018).
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// previewRoute is a profile or post page path.
type previewRoute struct {
	// name is the path's username; for a post page, it only makes the URL readable.
	name     string
	statusID string // "" for a profile page
	// invalid is true for a path that cannot name a local account or status.
	invalid bool
}

// parsePreviewRoute parses /@{name} and /@{name}/status/{id}. A name with the account domain
// or the host (/@alice@example.org) is a local one; a name with another domain is a remote
// account's page, which has no preview. Under a profile, the first segment is one of the
// reserved words of the web client's pages (ADR-0053): only status is the server's, and the
// other paths are pages of the web client.
func (h *Handler) parsePreviewRoute(p string) (previewRoute, bool) {
	rest, ok := strings.CutPrefix(p, "/@")
	if !ok || rest == "" {
		return previewRoute{}, false
	}
	name, sub, nested := strings.Cut(rest, "/")
	if nested {
		word, statusID, ok := strings.Cut(sub, "/")
		if !ok || word != content.StatusPageSegment || strings.Contains(statusID, "/") {
			return previewRoute{}, false
		}
		return previewRoute{name: name, statusID: statusID, invalid: !uuidPattern.MatchString(statusID)}, true
	}
	if user, domain, ok := strings.Cut(name, "@"); ok {
		if !strings.EqualFold(domain, h.o.AccountDomain) && !strings.EqualFold(domain, h.o.Host) {
			return previewRoute{}, false
		}
		name = user
	}
	return previewRoute{name: name, invalid: account.ValidateUsername(name) != nil}, true
}

// servePreview serves a profile or a post page: its shell with the preview's meta tags, or
// the generic shell with 404 when an anonymous visitor may not see it.
func (h *Handler) servePreview(w http.ResponseWriter, r *http.Request, route previewRoute) {
	ctx := r.Context()
	var (
		doc document
		err error
	)
	switch {
	case route.invalid:
		err = appcontent.ErrNotFound
	case route.statusID == "":
		var p appcontent.ProfilePreview
		if p, err = h.o.Previews.Profile(ctx, route.name); err == nil {
			doc, err = h.renderProfile(ctx, p)
		}
	default:
		var s appcontent.StatusPreview
		if s, err = h.o.Previews.Status(ctx, route.statusID); err == nil {
			if s.Author.Username() != route.name {
				// The page is found by the status ID; the URL with the current username is the
				// canonical one (ADR-0053).
				h.shellHeaders(w.Header())
				http.Redirect(w, r, content.StatusURL(h.o.Host, s.Author.Username(), s.Post.Status.ID), http.StatusMovedPermanently)
				return
			}
			doc, err = h.renderStatus(ctx, s)
		}
	}
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		h.serveHidden(w, r)
	case err != nil:
		h.fail(w, r, err)
	default:
		h.shellHeaders(w.Header())
		doc.serve(w, r)
	}
}

// serveHidden answers a profile or post page that an anonymous visitor may not see, or that
// does not exist, with the generic shell and 404. The response is the same whatever the
// reason, so that it does not reveal which (ADR-0017, ADR-0018); a logged-in visitor's SPA
// still loads the page with its token.
func (h *Handler) serveHidden(w http.ResponseWriter, r *http.Request) {
	d, err := h.documents(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	hd := w.Header()
	h.shellHeaders(hd)
	// The generic shell is never indexed (documents).
	d.shell.crawlers.set(hd)
	hd.Set("Content-Type", d.shell.contentType)
	hd.Set("Content-Length", strconv.Itoa(len(d.shell.body)))
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		_, _ = w.Write(d.shell.body)
	}
}

// renderProfile renders the shell of a profile page.
func (h *Handler) renderProfile(ctx context.Context, p appcontent.ProfilePreview) (document, error) {
	lang := h.previewLanguage(ctx, p.Language)
	meta, version := h.authorMeta(ctx, lang, p)
	meta.Type = ogTypeProfile
	meta.URL = profileURL(h.o.Host, p.Username())
	meta.ActivityPubID = p.Actor.Account.URI
	meta.Username = p.Username() + "@" + h.o.AccountDomain
	if prof := p.Actor.Profile; prof != nil {
		meta.Description = description(plainText(prof.NoteHTML))
	}
	img, err := h.profileImage(ctx, lang, p, nil)
	if err != nil {
		return document{}, err
	}
	meta.Image = img
	meta.Card = card(meta.Image)
	version = append(version, "profile", imageVersion(meta.Image))
	policy, err := h.ownerPolicy(ctx, p)
	if err != nil {
		return document{}, err
	}
	return h.renderPreview(ctx, lang, meta, policy, version)
}

// renderStatus renders the shell of a post page.
func (h *Handler) renderStatus(ctx context.Context, s appcontent.StatusPreview) (document, error) {
	lang := h.previewLanguage(ctx, s.Language)
	meta, version := h.authorMeta(ctx, lang, s.Author)
	st, rev := s.Post.Status, s.Post.Revision
	meta.Type = ogTypeArticle
	meta.URL = content.StatusURL(h.o.Host, s.Author.Username(), st.ID)
	meta.ActivityPubID = st.URI
	meta.PublishedTime = st.PublishedAt.UTC().Format(time.RFC3339)
	meta.AuthorURL = profileURL(h.o.Host, s.Author.Username())
	if rev.SpoilerText != "" {
		// Behind a content warning, the preview shows only the warning.
		meta.Description = description(rev.SpoilerText)
	} else {
		meta.Description = description(plainText(bodyHTML(rev)))
	}
	img, err := h.profileImage(ctx, lang, s.Author, s.Image)
	if err != nil {
		return document{}, err
	}
	meta.Image = img
	meta.Card = card(meta.Image)
	policy, err := h.ownerPolicy(ctx, s.Author)
	if err != nil {
		return document{}, err
	}
	if s.Visibility == content.VisibilityUnlisted {
		// An unlisted post is for anyone with the link, not for search engines (ADR-0028).
		policy = policy.NoIndex()
	}
	version = append(version, "status", st.ID, rev.ID, string(s.Visibility), imageVersion(meta.Image))
	return h.renderPreview(ctx, lang, meta, policy, version)
}

// authorMeta fills the meta tags that a profile page and its posts share, and returns them
// with the parts of the ETag they depend on. The site is the instance's name in the preview's
// language; the handle keeps the account domain.
func (h *Handler) authorMeta(ctx context.Context, lang string, p appcontent.ProfilePreview) (previewMeta, []string) {
	username, profileRevision := p.Username(), ""
	if prof := p.Actor.Profile; prof != nil {
		profileRevision = prof.ID
	}
	handle := username + "@" + h.o.AccountDomain
	meta := previewMeta{
		SiteName: h.instanceName(ctx),
		Title:    h.o.Messages.Localizer(lang).ServerPreviewTitle(i18n.ServerPreviewTitleArgs{Handle: handle, Name: displayName(p)}),
		Locale:   ogLocale(lang),
		Creator:  "@" + handle,
	}
	return meta, []string{p.Actor.Account.ID, username, profileRevision, meta.SiteName}
}

// displayName returns the display name of a profile, or its username when it has none.
func displayName(p appcontent.ProfilePreview) string {
	if prof := p.Actor.Profile; prof != nil && strings.TrimSpace(prof.DisplayName) != "" {
		return prof.DisplayName
	}
	return p.Username()
}

// profileImage chooses the preview image (ADR-0028): the first attached image, then the
// profile's header image or avatar, then the instance's preview image, which is the built-in
// image when the instance has none.
func (h *Handler) profileImage(ctx context.Context, lang string, p appcontent.ProfilePreview, attached *appcontent.PreviewImage) (*Image, error) {
	name, l := displayName(p), h.o.Messages.Localizer(lang)
	switch {
	case attached != nil:
		return previewImage(attached, attached.Alt), nil
	case p.Header != nil:
		return previewImage(p.Header, l.ServerPreviewHeaderAlt(name)), nil
	case p.Avatar != nil:
		return previewImage(p.Avatar, l.ServerPreviewAvatarAlt(name)), nil
	}
	return h.instanceImage(ctx, lang)
}

// instanceImage returns the instance's preview image for a preview in language lang, the
// preview image drawn from its logo when it has none (ADR-0116), or the built-in image when it
// has neither. An image without alt text gets a generic one naming the instance.
func (h *Handler) instanceImage(ctx context.Context, lang string) (*Image, error) {
	l := h.o.Messages.Localizer(lang)
	var img *Image
	if h.o.DefaultImage != nil {
		var err error
		if img, err = h.o.DefaultImage(ctx); err != nil {
			return nil, err
		}
	}
	if img == nil || img.URL == "" {
		brand, err := h.currentBrand(ctx)
		if err != nil {
			return nil, err
		}
		img = brand.Preview
	}
	if img == nil || img.URL == "" {
		return h.builtInImage(l.ServerPreviewBuiltinImageAlt()), nil
	}
	out := *img
	if out.Alt == "" {
		out.Alt = l.ServerPreviewDefaultImageAlt(h.instanceName(ctx))
	}
	return &out, nil
}

func previewImage(p *appcontent.PreviewImage, alt string) *Image {
	return &Image{URL: p.URL, Width: p.Width, Height: p.Height, Alt: alt}
}

// imageVersion identifies the chosen image for the ETag.
func imageVersion(img *Image) string {
	if img == nil {
		return ""
	}
	return img.URL + "\x1f" + strconv.Itoa(img.Width) + "x" + strconv.Itoa(img.Height) + "\x1f" + img.Alt
}

// card returns the X card for an image: the large image card when the image is about 2:1 or
// wider and large enough, the summary card otherwise.
func card(img *Image) string {
	if img == nil || img.Width < largeCardMinWidth || img.Height < largeCardMinHeight ||
		float64(img.Width)/float64(img.Height) < largeCardMinRatio {
		return cardSummary
	}
	return cardSummaryLarge
}

// previewLanguage returns the language of a preview: the given BCP 47 tag, or the instance's
// default locale when it is undetermined. Accept-Language never changes it, so that shared
// caches keep one copy (ADR-0028).
func (h *Handler) previewLanguage(ctx context.Context, lang string) string {
	if lang = i18n.Canonicalize(lang); lang != "" && lang != appcontent.UndeterminedLanguage {
		return lang
	}
	return h.instanceLocale(ctx)
}

// ownerPolicy returns the crawler policy of the pages of a local account: the instance's,
// narrowed by the account's indexable and ai_opt_out (ADR-0057). An account without a profile
// is not indexable.
func (h *Handler) ownerPolicy(ctx context.Context, p appcontent.ProfilePreview) (instance.CrawlerPolicy, error) {
	settings, err := h.settings(ctx)
	if err != nil {
		return instance.CrawlerPolicy{}, err
	}
	indexable, aiOptOut := false, false
	if prof := p.Actor.Profile; prof != nil {
		indexable, aiOptOut = prof.Indexable, prof.AIOptOut
	}
	return settings.CrawlerPolicy().ForOwner(indexable, aiOptOut), nil
}

// renderPreview renders the shell with meta in language lang, stating policy to crawlers. Its
// ETag comes from the version of the template and the assets, the language, the policy, and the
// parts of the profile or post it shows (ADR-0028). lang sets only <html lang> and og:locale:
// the runtime config keeps the instance's default locale.
func (h *Handler) renderPreview(ctx context.Context, lang string, meta previewMeta, policy instance.CrawlerPolicy, version []string) (document, error) {
	locale := h.instanceLocale(ctx)
	name := h.instanceName(ctx)
	theme, err := h.currentTheme(ctx)
	if err != nil {
		return document{}, err
	}
	brand, err := h.currentBrand(ctx)
	if err != nil {
		return document{}, err
	}
	crawlers := directivesOf(policy)
	d, err := renderShell(shellData{
		Lang: lang, Dir: i18n.Direction(lang), Title: meta.Title, Theme: theme, Brand: brand.Icon, Crawlers: crawlers,
		HasIcon: h.hasIcon, HasAppleTouchIcon: h.hasAppleTouchIcon, HasThemeScript: h.hasThemeScript, ViteClient: viteClient(h.o.DevViteOrigin), Entry: h.tags,
		Config:  h.runtimeConfig(locale, name, brand),
		Preview: &meta,
	})
	if err != nil {
		return document{}, err
	}
	key := append([]string{h.previewVersion, lang, locale, name, theme.Stylesheet, theme.Light, theme.Dark, brand.Version, string(policy.Search), string(policy.AI)}, version...)
	d.etag = etag([]byte(strings.Join(key, "\x00")))
	d.crawlers = &crawlers
	return d, nil
}

// ogLocale formats a BCP 47 tag as og:locale (language_TERRITORY): "ja" -> "ja",
// "en-US" -> "en_US". Script and other subtags are dropped.
func ogLocale(tag string) string {
	parts := strings.Split(i18n.Canonicalize(tag), "-")
	out := parts[0]
	for _, p := range parts[1:] {
		if len(p) == 2 {
			return out + "_" + p
		}
	}
	return out
}

func profileURL(host, username string) string { return "https://" + host + "/@" + username }

// bodyHTML returns the HTML of a revision in its own language, or its first body.
func bodyHTML(rev content.Revision) string {
	for _, b := range rev.Bodies {
		if b.Language == rev.Language {
			return b.ContentHTML
		}
	}
	if len(rev.Bodies) > 0 {
		return rev.Bodies[0].ContentHTML
	}
	return ""
}

// blockTags are the elements whose boundaries separate words in plainText.
var blockTags = map[string]bool{
	"br": true, "p": true, "div": true, "li": true, "ul": true, "ol": true, "blockquote": true, "pre": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// plainText turns the sanitized HTML of a status or a profile note into plain text: tags are
// removed (block boundaries become spaces), character references are decoded, and runs of
// white space become one space. The result is text for html/template to escape, never HTML.
func plainText(s string) string {
	var b strings.Builder
	for s != "" {
		lt := strings.IndexByte(s, '<')
		if lt < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:lt])
		gt := strings.IndexByte(s[lt:], '>')
		if gt < 0 {
			break // an unterminated tag is dropped
		}
		tag := strings.TrimLeft(s[lt+1:lt+gt], "/")
		if end := strings.IndexFunc(tag, func(r rune) bool { return unicode.IsSpace(r) || r == '/' }); end >= 0 {
			tag = tag[:end]
		}
		if blockTags[strings.ToLower(tag)] {
			b.WriteByte(' ')
		}
		s = s[lt+gt+1:]
	}
	return strings.Join(strings.Fields(html.UnescapeString(b.String())), " ")
}

// description truncates text for og:description to descriptionLength grapheme clusters, so
// that no emoji or combining sequence is cut, ending a truncated text with an ellipsis.
func description(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if uniseg.GraphemeClusterCount(text) <= descriptionLength {
		return text
	}
	var b strings.Builder
	g := uniseg.NewGraphemes(text)
	for n := 0; n < descriptionLength-1 && g.Next(); n++ {
		b.WriteString(g.Str())
	}
	return strings.TrimRightFunc(b.String(), unicode.IsSpace) + "…"
}
