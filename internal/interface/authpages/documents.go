// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
)

// The instance's documents: the terms of service and the privacy policy that administrators
// write in Markdown (instance texts terms and privacy_policy). The web client draws them
// (GET /instance/documents/{kind}); this package only links to them, from the footer of every
// page and from the registration page that asks applicants to agree to them. A document that
// the instance has not published has no link.

// Paths of the web client's pages of the documents (web.PathAboutTerms and
// web.PathAboutPrivacyPolicy; the interface packages do not import each other).
const (
	PathTerms         = "/about/terms"
	PathPrivacyPolicy = "/about/privacy"
)

// Documents reads which documents the instance has published; *appinstance.Service implements it.
type Documents interface {
	// HasDocument reports whether the document key is published.
	HasDocument(ctx context.Context, key instance.TextKey) (bool, error)
}

// documentPages are the documents that the footer and the registration page link to: the text
// key, the path, and the title.
var documentPages = []struct {
	key   instance.TextKey
	path  string
	title func(*i18n.Localizer) string
}{
	{instance.TextTerms, PathTerms, (*i18n.Localizer).ServerAuthDocumentTermsTitle},
	{instance.TextPrivacyPolicy, PathPrivacyPolicy, (*i18n.Localizer).ServerAuthDocumentPrivacyTitle},
}

// documentLink is a link to a document page in the footer and on the registration screen.
type documentLink struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// documentLinks returns the links to the documents that the instance has published, titled in
// the page's locale.
// A failure to read them is logged and leaves them out, so that it never breaks a page.
func (h *Handler) documentLinks(r *http.Request, l *i18n.Localizer) []documentLink {
	if h.o.Documents == nil {
		return nil
	}
	var out []documentLink
	for _, d := range documentPages {
		ok, err := h.o.Documents.HasDocument(r.Context(), d.key)
		if err != nil {
			h.o.Logger.WarnContext(r.Context(), "read an instance document", slog.String("key", string(d.key)), slog.Any("error", err))
			continue
		}
		if ok {
			out = append(out, documentLink{Path: d.path, Title: d.title(l)})
		}
	}
	return out
}
