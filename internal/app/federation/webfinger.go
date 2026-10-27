// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/federation"
)

// webFingerAccept is the Accept header of WebFinger requests (RFC 7033 §10.2).
const webFingerAccept = "application/jrd+json, application/json"

// errWebFingerUnanswered reports a WebFinger request that got no usable answer (a network
// error, 429, or a server error), as opposed to an answer that does not confirm the handle.
var errWebFingerUnanswered = errors.New("federation: WebFinger did not answer")

// jrd is the part of a JSON Resource Descriptor (RFC 7033 §4.4) that handle checks read.
type jrd struct {
	Subject string `json:"subject"`
	Links   []struct {
		Rel  string `json:"rel"`
		Type string `json:"type"`
		Href string `json:"href"`
	} `json:"links"`
}

// self returns the canonical href of the ActivityStreams self link, or "".
func (j jrd) self() federation.IRI {
	for _, l := range j.Links {
		if l.Rel != "self" || (l.Type != "" && !isActivityStreams(l.Type)) {
			continue
		}
		if iri, err := CanonicalIRI(l.Href); err == nil {
			return iri
		}
	}
	return ""
}

// checkHandle checks the handle of an actor with WebFinger (federation.md §6.2): the handle is
// acct:{preferredUsername}@{the actor's host} when WebFinger of it names the actor as self, and
// when its subject has another domain, WebFinger of the subject on that domain names the actor
// too. The result is a mismatch when an answer does not confirm the handle, and the zero
// HandleCheck when WebFinger did not answer, so that the stored handle stays.
func (r *RemoteActors) checkHandle(ctx context.Context, a *federation.Actor) appaccount.HandleCheck {
	host := a.ID.Host()
	username := a.PreferredUsername
	mismatch := appaccount.HandleCheck{Outcome: appaccount.HandleMismatch, Username: username, Domain: host}
	if username == "" || strings.ContainsAny(username, "@/?#: ") {
		return mismatch
	}
	first, err := r.webFinger(ctx, a.ID.Origin(), "acct:"+username+"@"+host)
	switch {
	case errors.Is(err, errWebFingerUnanswered):
		return appaccount.HandleCheck{}
	case err != nil || first.self() != a.ID:
		return mismatch
	}
	user, domain, ok := parseAcct(first.Subject)
	if !ok {
		user, domain = username, host
	}
	if domain == host {
		return appaccount.HandleCheck{Outcome: appaccount.HandleVerified, Username: user, Domain: domain}
	}
	// The handle names another domain (Mastodon's WEB_DOMAIN and LOCAL_DOMAIN): that domain
	// must name the actor too. The rules of fetching apply to it as to the actor's own server:
	// never this instance or a suspended server, which cannot confirm the handle, and not an
	// unavailable one.
	if r.d.Fetcher != nil {
		switch err := r.d.Fetcher.check(ctx, federation.IRI("https://"+domain+"/")); {
		case errors.Is(err, ErrFetchRefused):
			return appaccount.HandleCheck{Outcome: appaccount.HandleMismatch, Username: user, Domain: domain}
		case err != nil:
			return appaccount.HandleCheck{}
		}
	}
	second, err := r.webFinger(ctx, "https://"+domain, "acct:"+user+"@"+domain)
	switch {
	case errors.Is(err, errWebFingerUnanswered):
		return appaccount.HandleCheck{}
	case err != nil || second.self() != a.ID:
		return appaccount.HandleCheck{Outcome: appaccount.HandleMismatch, Username: user, Domain: domain}
	}
	return appaccount.HandleCheck{Outcome: appaccount.HandleVerified, Username: user, Domain: domain}
}

// parseAcct splits acct:user@domain, with the domain in canonical form.
func parseAcct(subject string) (user, domain string, ok bool) {
	rest, found := strings.CutPrefix(subject, "acct:")
	if !found {
		return "", "", false
	}
	i := strings.LastIndexByte(rest, '@')
	if i <= 0 || i == len(rest)-1 {
		return "", "", false
	}
	domain, err := CanonicalHost(rest[i+1:])
	if err != nil {
		return "", "", false
	}
	return rest[:i], domain, true
}

// webFinger looks resource up at origin's /.well-known/webfinger. It is not signed: WebFinger
// is public (FR-401). A redirect to this instance or to a suspended server fails with
// ErrFetchRefused.
func (r *RemoteActors) webFinger(ctx context.Context, origin, resource string) (jrd, error) {
	if err := r.identifies(ctx); err != nil {
		return jrd{}, fmt.Errorf("%w: %w", errWebFingerUnanswered, err)
	}
	u := origin + "/.well-known/webfinger?resource=" + url.QueryEscape(resource)
	resp, err := r.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodGet, URL: u, Header: http.Header{"Accept": {webFingerAccept}}, Purpose: kernel.HTTPPurposeJSON,
		CheckRedirect: r.d.Fetcher.redirectPolicy(),
	})
	switch {
	case errors.Is(err, ErrFetchRefused), errors.Is(err, kernel.ErrForbiddenDestination), errors.Is(err, kernel.ErrInvalidHTTPRequest),
		errors.Is(err, kernel.ErrTooManyRedirects), errors.Is(err, kernel.ErrResponseTooLarge):
		return jrd{}, err
	case err != nil:
		return jrd{}, fmt.Errorf("%w: %w", errWebFingerUnanswered, err)
	}
	body, err := resp.ReadAll()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return jrd{}, fmt.Errorf("%w: %s answered %d", errWebFingerUnanswered, u, resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return jrd{}, fmt.Errorf("federation: %s answered %d", u, resp.StatusCode)
	case errors.Is(err, kernel.ErrResponseTooLarge):
		return jrd{}, err
	case err != nil:
		return jrd{}, fmt.Errorf("%w: %w", errWebFingerUnanswered, err)
	}
	var out jrd
	if err := json.Unmarshal(body, &out); err != nil {
		return jrd{}, fmt.Errorf("federation: WebFinger of %s: %w", resource, err)
	}
	return out, nil
}
