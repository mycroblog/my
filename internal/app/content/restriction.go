// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/domain/content"
)

// Restrictions are the current moderation restrictions of an account that bear on its statuses.
type Restrictions struct {
	// Post forbids posting, replying, boosting, and editing (the post restriction).
	Post bool
	// Interact forbids favouriting, boosting, and replying (the interact restriction).
	Interact bool
	// MediaSensitive shows every attachment of the account's statuses as sensitive (the
	// media_sensitive restriction).
	MediaSensitive bool
	// Author are the restrictions that bear on who sees the account's statuses, the visibility
	// ceiling among them.
	Author content.AuthorRestrictions
}

// CanPost reports whether the account may post, reply, and edit.
func (r Restrictions) CanPost() bool { return !r.Post }

// CanInteract reports whether the account may favourite, boost, and reply.
func (r Restrictions) CanInteract() bool { return !r.Interact }

// VisibilityCeiling returns the widest visibility the account may post with (ADR-0058 §11).
func (r Restrictions) VisibilityCeiling() content.Visibility { return r.Author.VisibilityCeiling() }

// PostingLimits are what an account's current restrictions let it post (ADR-0058 §11).
type PostingLimits struct {
	// VisibilityCeiling is the widest visibility a new status gets.
	VisibilityCeiling content.Visibility
	// Visibilities are the visibilities a new status keeps as asked for, widest first.
	Visibilities []content.Visibility
}

// RestrictionPolicy is the port through which the content module reads an account's effective
// restrictions. Use cases decide posting rights and the visibility ceiling
// only through it, never from the moderation tables.
type RestrictionPolicy interface {
	// Restrictions returns the restrictions of an account that are in effect now.
	Restrictions(ctx context.Context, accountID string) (Restrictions, error)
}

// NoRestrictions is the RestrictionPolicy that restricts nobody, for the services and tests
// built without the moderation module's policy (moderation.AccountPolicy.Content).
type NoRestrictions struct{}

// Restrictions implements RestrictionPolicy.
func (NoRestrictions) Restrictions(context.Context, string) (Restrictions, error) {
	return Restrictions{}, nil
}
