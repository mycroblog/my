// SPDX-License-Identifier: AGPL-3.0-only

package activitypub

import (
	"strings"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/account"
)

// apPrefix is the namespace of the ActivityPub ids and documents (ADR-0018).
const apPrefix = "/ap/"

// actorsPrefix is the path of local actor ids before the UUID (ADR-0018).
const actorsPrefix = "/ap/actors/"

// objectsPrefix is the path of local object ids before the UUID, and activitySegment the last
// segment of the IRI of a status's Create (ADR-0018).
const (
	objectsPrefix   = "/ap/objects/"
	activitySegment = "activity"
)

// quoteAuthorizationsPrefix is the path of the IRIs of the QuoteAuthorizations of this server
// before the UUID (ADR-0069 §1).
const quoteAuthorizationsPrefix = "/ap/quote_authorizations/"

// Collections of an actor, the last path segment under {actor}/ (ADR-0018). The inbox only
// takes POSTs.
const (
	collInbox       = appfederation.CollectionInbox
	collOutbox      = appfederation.CollectionOutbox
	collFollowers   = appfederation.CollectionFollowers
	collFollowing   = appfederation.CollectionFollowing
	collFeatured    = appfederation.CollectionFeatured
	collFeaturedTag = appfederation.CollectionFeaturedTags
)

// sharedInboxPath is the path of the instance's shared inbox.
const sharedInboxPath = "/ap/inbox"

// profileURL returns the human-readable profile URL of a local username on host.
func profileURL(host, username string) string { return appfederation.ProfileURL(host, username) }

// parseLocalURI returns the actor id or the username that a local https URI names: an actor
// id (https://{host}/ap/actors/{uuid}) or a profile URL (https://{host}/@{username}).
func parseLocalURI(host, uri string) (id, username string, ok bool) {
	origin := "https://" + host + "/"
	if len(uri) < len(origin) || !strings.EqualFold(uri[:len(origin)], origin) {
		return "", "", false
	}
	rest := uri[len(origin):]
	if strings.ContainsAny(rest, "?#") {
		return "", "", false
	}
	if s, found := strings.CutPrefix(rest, "ap/actors/"); found && kernel.ValidID(s) {
		return s, "", true
	}
	if s, found := strings.CutPrefix(rest, "@"); found && account.ValidateUsername(s) == nil {
		return "", s, true
	}
	return "", "", false
}
