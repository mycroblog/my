// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/federation"
)

// ObjectQuotes is what object fetches read of quotes and of the approvals of interactions
// (ADR-0069, ADR-0155 §8): what Notes present, and the approvals this server gives.
// ContentQuotes implements it.
type ObjectQuotes interface {
	NoteQuotes
	FindInteractionAuthorization(ctx context.Context, kind appcontent.InteractionKind, id string) (appcontent.InteractionAuthorization, error)
}

// ContentQuotes joins the content module's services into ObjectQuotes.
type ContentQuotes struct {
	*appcontent.Service
	Interactions *appcontent.Interactions
}

// FindInteractionAuthorization implements ObjectQuotes.
func (c ContentQuotes) FindInteractionAuthorization(ctx context.Context, kind appcontent.InteractionKind, id string) (appcontent.InteractionAuthorization, error) {
	return c.Interactions.FindInteractionAuthorization(ctx, kind, id)
}

// authorizationTypes are the object types of the authorizations of each kind.
var authorizationTypes = map[appcontent.InteractionKind]federation.ObjectType{
	appcontent.InteractionReply:    federation.ObjectReplyAuthorization,
	appcontent.InteractionAnnounce: federation.ObjectAnnounceAuthorization,
	appcontent.InteractionQuote:    federation.ObjectQuoteAuthorization,
}

// Authorization answers a GET of the IRI of an authorization of kind of this server (ADR-0069
// §1, ADR-0155 §8): the approval of a reply, a boost, or a quote of a local status by its author,
// to anyone, since the interaction shows it already. A revoked one is a Tombstone, served with
// 410 Gone, as is one of a status that is gone; a missing one, and one of a local-visibility
// status, whose IRI and author it would name (ADR-0018 §15), fail with ErrNotFound.
func (o *Objects) Authorization(ctx context.Context, kind appcontent.InteractionKind, id string) (Object, error) {
	typ, ok := authorizationTypes[kind]
	if o.d.Quotes == nil || !ok {
		return Object{}, fmt.Errorf("%s authorization %s: %w", kind, id, ErrNotFound)
	}
	auth, err := o.d.Quotes.FindInteractionAuthorization(ctx, kind, id)
	if errors.Is(err, appcontent.ErrNotFound) {
		return Object{}, fmt.Errorf("%s authorization %s: %w", kind, id, ErrNotFound)
	}
	if err != nil {
		return Object{}, err
	}
	uri := federation.IRI(appcontent.InteractionAuthorizationURI(o.d.Host, kind, auth.ID))
	out := Object{Public: true}
	target, _, deleted, err := o.d.Statuses.ObjectStatus(ctx, auth.TargetStatusID)
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		deleted = true
	case err != nil:
		return Object{}, err
	case target.Visibility == appcontent.VisibilityLocal:
		return Object{}, fmt.Errorf("%s authorization %s: %w", kind, id, ErrNotFound)
	}
	if auth.RevokedAt != nil || deleted {
		out.Gone = true
		out.Document, err = json.Marshal(federation.TombstoneOf(uri).TopLevel())
		return out, err
	}
	author := federation.IRI(appaccount.ActorURI(o.d.Host, target.AuthorAccountID))
	out.Document, err = json.Marshal(federation.NewAuthorizationDocument(typ, uri, author, federation.IRI(auth.InteractingURI), federation.IRI(target.URI)))
	return out, err
}
