// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
)

// This file serves the shells of the pages about the instance itself, the landing page (/), the
// information page (/about), and the pages of the instance's published documents, with the meta
// tags of a link preview of the instance (ADR-0028): its name, its one-paragraph summary, and its
// preview image. The pages are the same for every visitor; the web client shows the landing page
// to visitors without a session and the home timeline to signed-in users, and the other pages to
// everyone.

// Paths of the pages about the instance.
const (
	pathLanding = "/"
	pathAbout   = "/about"
)

// Paths of the web client's pages of the instance's documents, which GET /instance links to.
// The shell of a published document's page previews it under the document's name; the page of
// a document that is not published is the generic shell.
const (
	PathAboutTerms              = "/about/terms"
	PathAboutPrivacyPolicy      = "/about/privacy"
	PathAboutCommerceDisclosure = "/about/commerce"
)

// Documents reads which documents the instance has published; *appinstance.Service implements it.
type Documents interface {
	// HasDocument reports whether the document key is published.
	HasDocument(ctx context.Context, key instance.TextKey) (bool, error)
}

// documentPage is the page of a document: its text key and its name in link previews.
type documentPage struct {
	key  instance.TextKey
	name func(*i18n.Localizer) string
}

// documentPages are the pages of the instance's documents, by path.
var documentPages = map[string]documentPage{
	PathAboutTerms:              {instance.TextTerms, (*i18n.Localizer).ServerPreviewDocumentTerms},
	PathAboutPrivacyPolicy:      {instance.TextPrivacyPolicy, (*i18n.Localizer).ServerPreviewDocumentPrivacyPolicy},
	PathAboutCommerceDisclosure: {instance.TextCommerceDisclosure, (*i18n.Localizer).ServerPreviewDocumentCommerceDisclosure},
}

// serveDocumentPage serves the shell of the page of a document at path, a key of documentPages:
// a preview under the document's name when the instance has published it, and the generic shell
// otherwise. A failure to read whether it is published is logged and serves the generic shell,
// so that it never breaks the page.
func (h *Handler) serveDocumentPage(w http.ResponseWriter, r *http.Request, path string, page documentPage) {
	ok, err := h.o.Documents.HasDocument(r.Context(), page.key)
	if err != nil {
		h.o.Logger.WarnContext(r.Context(), "read an instance document", slog.String("key", string(page.key)), slog.Any("error", err))
	}
	if err != nil || !ok {
		h.serveShell(w, r)
		return
	}
	lang := h.instanceLocale(r.Context())
	l := h.o.Messages.Localizer(lang)
	h.serveInstancePage(w, r, path, l.ServerPreviewDocumentTitle(i18n.ServerPreviewDocumentTitleArgs{Document: page.name(l), Instance: h.instanceName(r.Context())}))
}

// serveInstancePage serves the shell of the page about the instance at path under title (see
// renderInstancePage).
func (h *Handler) serveInstancePage(w http.ResponseWriter, r *http.Request, path, title string) {
	doc, err := h.renderInstancePage(r.Context(), path, title)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.shellHeaders(w.Header())
	doc.serve(w, r)
}

// renderInstancePage renders the shell of the page about the instance at path in the instance's
// default locale, under title. An empty title titles the landing page with the instance's name
// and the information page "About {name}".
func (h *Handler) renderInstancePage(ctx context.Context, path, title string) (document, error) {
	lang := h.instanceLocale(ctx)
	name := h.instanceName(ctx)
	switch {
	case title != "":
	case path == pathAbout:
		title = h.o.Messages.Localizer(lang).ServerPreviewAboutTitle(name)
	default:
		title = name
	}
	summary := ""
	if h.o.InstanceDescription != nil {
		summary = description(strings.TrimSpace(h.o.InstanceDescription(ctx)))
	}
	img, err := h.instanceImage(ctx, lang)
	if err != nil {
		return document{}, err
	}
	meta := previewMeta{
		Type: ogTypeWebsite, SiteName: name, Title: title, URL: "https://" + h.o.Host + path,
		Description: summary, Locale: ogLocale(lang), Image: img, Card: card(img),
	}
	// The pages about the instance state the instance's own policy (ADR-0057).
	settings, err := h.settings(ctx)
	if err != nil {
		return document{}, err
	}
	return h.renderPreview(ctx, lang, meta, settings.CrawlerPolicy(), []string{"instance", path, title, summary, imageVersion(img)})
}
