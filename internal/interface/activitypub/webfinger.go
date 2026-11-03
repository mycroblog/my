// SPDX-License-Identifier: AGPL-3.0-only

package activitypub

import (
	"encoding/xml"
	"net/http"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
)

// WebFinger link relations (RFC 7033) that the JRD carries.
const (
	relSelf        = "self"
	relProfilePage = "http://webfinger.net/rel/profile-page"
	relLRDD        = "lrdd"
)

// jrd is a JSON Resource Descriptor (RFC 7033 §4.4).
type jrd struct {
	Subject string    `json:"subject"`
	Aliases []string  `json:"aliases,omitempty"`
	Links   []jrdLink `json:"links"`
}

type jrdLink struct {
	Rel      string `json:"rel"`
	Type     string `json:"type,omitempty"`
	Href     string `json:"href,omitempty"`
	Template string `json:"template,omitempty"`
}

// webFinger serves GET /.well-known/webfinger (RFC 7033, ADR-0018). The resource is an acct:
// URI with the account domain or the host (acct:alice@example.org), or the https URI of an
// actor id or a profile page. The subject is always the handle on the account domain.
func (h *Handler) webFinger(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	resource := q.Get("resource")
	if resource == "" {
		h.o.Problem(w, r, http.StatusBadRequest)
		return
	}
	a, err := h.resolveResource(r, resource)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	doc := h.descriptor(a)
	if rels := q["rel"]; len(rels) > 0 { // RFC 7033 §4.3
		links := doc.Links[:0:0]
		for _, l := range doc.Links {
			for _, rel := range rels {
				if l.Rel == rel {
					links = append(links, l)
					break
				}
			}
		}
		doc.Links = links
	}
	h.writeJSON(w, r, typeJRD, doc)
}

// resolveResource finds the visible local actor that a WebFinger resource names; it fails with
// errActorGone for a deleted one.
func (h *Handler) resolveResource(r *http.Request, resource string) (appaccount.Actor, error) {
	if strings.HasPrefix(resource, "https://") {
		uri := resource
		// parseLocalURI refuses a query or a fragment, so it reads such a resource as it came.
		if c, err := appfederation.CanonicalIRI(resource); err == nil && !strings.ContainsAny(resource, "?#") {
			uri = string(c)
		}
		id, username, ok := parseLocalURI(h.o.Host, uri)
		switch {
		case !ok:
			return appaccount.Actor{}, appaccount.ErrNotFound
		case id != "":
			return h.lookupResource(r, id)
		default:
			return h.lookupUsername(r, username)
		}
	}
	// acct:alice@example.org; Mastodon also sends the bare alice@example.org.
	acct := strings.TrimPrefix(resource, "acct:")
	acct = strings.TrimPrefix(acct, "@")
	at := strings.LastIndexByte(acct, '@')
	if at <= 0 {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	username, domain := acct[:at], acct[at+1:]
	if !sameHost(domain, h.o.AccountDomain) && !sameHost(domain, h.o.Host) {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	// The instance actor's username is the account domain, spelled in any form too.
	if sameHost(username, h.o.AccountDomain) {
		username = h.instanceActorUsername()
	}
	return h.lookupUsername(r, username)
}

// sameHost reports whether a domain from a request names the configured host (federation.md §1.2):
// U-label, A-label, and trailing-dot spellings are the same. A configured host with a port, which
// local development allows, has no canonical form and compares as it is written.
func sameHost(domain, host string) bool {
	if strings.EqualFold(domain, host) {
		return true
	}
	d, err := appfederation.CanonicalHost(domain)
	if err != nil {
		return false
	}
	c, err := appfederation.CanonicalHost(host)
	return err == nil && d == c
}

// descriptor builds the JRD of a visible local actor.
func (h *Handler) descriptor(a appaccount.Actor) jrd {
	id := a.Account.URI
	self := jrdLink{Rel: relSelf, Type: typeActivityJSON, Href: id}
	if a.Local == nil { // the instance actor
		return jrd{Subject: "acct:" + h.instanceActorUsername() + "@" + h.o.AccountDomain, Aliases: []string{id}, Links: []jrdLink{self}}
	}
	profile := profileURL(h.o.Host, a.Local.Username)
	return jrd{
		Subject: "acct:" + a.Local.Username + "@" + h.o.AccountDomain,
		Aliases: []string{profile, id},
		Links:   []jrdLink{{Rel: relProfilePage, Type: "text/html", Href: profile}, self},
	}
}

// lrddTemplate is the WebFinger URI template that host-meta advertises (RFC 6415).
func (h *Handler) lrddTemplate() string {
	return "https://" + h.o.Host + PathWebFinger + "?resource={uri}"
}

// xrd is the host-meta document in XRD form.
type xrd struct {
	XMLName xml.Name  `xml:"http://docs.oasis-open.org/ns/xri/xrd-1.0 XRD"`
	Links   []xrdLink `xml:"Link"`
}

type xrdLink struct {
	Rel      string `xml:"rel,attr"`
	Template string `xml:"template,attr"`
}

// hostMeta serves GET /.well-known/host-meta in XRD form (RFC 6415), which older
// implementations read to find WebFinger.
func (h *Handler) hostMeta(w http.ResponseWriter, r *http.Request) {
	body, err := xml.MarshalIndent(xrd{Links: []xrdLink{{Rel: relLRDD, Template: h.lrddTemplate()}}}, "", "  ")
	if err != nil {
		h.report(r.Context(), err)
		h.o.Problem(w, r, http.StatusInternalServerError)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", typeXRD)
	hd.Set("Cache-Control", cachePublic)
	hd.Set("Cross-Origin-Resource-Policy", "cross-origin")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(append(body, '\n'))
}

// hostMetaJSON serves GET /.well-known/host-meta.json, host-meta in JRD form (RFC 6415 §3).
func (h *Handler) hostMetaJSON(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, r, typeJRD, struct {
		Links []jrdLink `json:"links"`
	}{Links: []jrdLink{{Rel: relLRDD, Template: h.lrddTemplate()}}})
}
