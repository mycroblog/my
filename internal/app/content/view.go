// SPDX-License-Identifier: AGPL-3.0-only

package content

import "github.com/mycroblog/my/internal/domain/content"

// The visibility policy of statuses (docs/architecture/visibility.md), as part of this module's
// public API: the application layers of other modules, which do not import this module's
// domain, decide who sees a status only through CanView (NFR-SEC-4).
type (
	// Visibility is content.Visibility.
	Visibility = content.Visibility
	// Viewer is content.Viewer.
	Viewer = content.Viewer
	// Body is content.Body.
	Body = content.Body
	// ViewerKind is content.ViewerKind.
	ViewerKind = content.ViewerKind
	// ViewContext is content.ViewContext.
	ViewContext = content.ViewContext
	// StatusFacts is content.StatusFacts.
	StatusFacts = content.StatusFacts
	// AuthorRestrictions is content.AuthorRestrictions.
	AuthorRestrictions = content.AuthorRestrictions
	// Relation is content.Relation.
	Relation = content.Relation
	// FollowState is content.FollowState.
	FollowState = content.FollowState
)

// The values of the policy's types that other modules use.
const (
	VisibilityPublic   = content.VisibilityPublic
	VisibilityLocal    = content.VisibilityLocal
	VisibilityUnlisted = content.VisibilityUnlisted
	VisibilityPrivate  = content.VisibilityPrivate
	VisibilityDirect   = content.VisibilityDirect

	ViewerAnonymous = content.ViewerAnonymous
	ViewerLocal     = content.ViewerLocal

	ContextStatus            = content.ContextStatus
	ContextHome              = content.ContextHome
	ContextLocalTimeline     = content.ContextLocalTimeline
	ContextFederatedTimeline = content.ContextFederatedTimeline
	ContextHashtag           = content.ContextHashtag
	ContextProfile           = content.ContextProfile
	ContextSearch            = content.ContextSearch

	FollowNone   = content.FollowNone
	FollowActive = content.FollowActive
)

// CanView is content.CanView, the single visibility policy function.
func CanView(ctx ViewContext, viewer Viewer, status StatusFacts, author AuthorRestrictions, rel Relation) bool {
	return content.CanView(ctx, viewer, status, author, rel)
}
