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

// ObjectQuotes is what object fetches read of quotes (ADR-0069): what Notes present, and the
// approvals this server gives. ContentQuotes implements it.
type ObjectQuotes interface {
	NoteQuotes
	FindQuoteAuthorization(ctx context.Context, id string) (appcontent.QuoteAuthorization, error)
}

// ContentQuotes joins the content module's services into ObjectQuotes.
type ContentQuotes struct {
	*appcontent.Service
	Interactions *appcontent.Interactions
}

// FindQuoteAuthorization implements ObjectQuotes.
func (c ContentQuotes) FindQuoteAuthorization(ctx context.Context, id string) (appcontent.QuoteAuthorization, error) {
	return c.Interactions.FindQuoteAuthorization(ctx, id)
}

// QuoteAuthorization answers a GET of the IRI of a QuoteAuthorization of this server
// (ADR-0069 §1): the approval of a quote of a local status by its author, to anyone, since the
// quoting post shows it already. A revoked one is a Tombstone, served with 410 Gone; a missing
// one, and one of a local-visibility status, whose IRI and author it would name (ADR-0018 §15), fail
// with ErrNotFound.
func (o *Objects) QuoteAuthorization(ctx context.Context, id string) (Object, error) {
	if o.d.Quotes == nil {
		return Object{}, fmt.Errorf("quote authorization %s: %w", id, ErrNotFound)
	}
	auth, err := o.d.Quotes.FindQuoteAuthorization(ctx, id)
	if errors.Is(err, appcontent.ErrNotFound) {
		return Object{}, fmt.Errorf("quote authorization %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return Object{}, err
	}
	uri := federation.IRI(appcontent.QuoteAuthorizationURI(o.d.Host, auth.ID))
	out := Object{Public: true}
	quoted, _, deleted, err := o.d.Statuses.ObjectStatus(ctx, auth.QuotedStatusID)
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		deleted = true
	case err != nil:
		return Object{}, err
	case quoted.Visibility == appcontent.VisibilityLocal:
		return Object{}, fmt.Errorf("quote authorization %s: %w", id, ErrNotFound)
	}
	if auth.RevokedAt != nil || deleted {
		out.Gone = true
		out.Document, err = json.Marshal(federation.TombstoneOf(uri).TopLevel())
		return out, err
	}
	author := federation.IRI(appaccount.ActorURI(o.d.Host, quoted.AuthorAccountID))
	out.Document, err = json.Marshal(federation.NewQuoteAuthorizationDocument(uri, author, federation.IRI(auth.QuotingURI), federation.IRI(quoted.URI)))
	return out, err
}
