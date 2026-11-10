// SPDX-License-Identifier: AGPL-3.0-only

package content

// This file holds CanView, the single visibility policy function (NFR-SEC-4, ADR-0017).
// Timelines, search, the client API, delivery, and ActivityPub object fetches all decide who may
// see a status through it. docs/architecture/visibility.md is the policy table it implements.

// ViewerKind says who is looking at a status.
type ViewerKind int

// Viewer kinds. The zero value is ViewerAnonymous, so an unset viewer gets the least access.
const (
	// ViewerAnonymous is a visitor that is not logged in, or an unsigned ActivityPub fetch.
	ViewerAnonymous ViewerKind = iota
	// ViewerLocal is a logged-in local account.
	ViewerLocal
	// ViewerRemote is a remote actor: a delivery recipient, or the signer of an ActivityPub
	// object fetch. For a fetch, the caller aggregates Relation over the signer's domain
	// (for example, Follow is FollowActive when any account on that domain actively follows the
	// author), because the remote server shows what it receives to all of its users.
	ViewerRemote
)

// Viewer identifies who is looking.
type Viewer struct {
	Kind ViewerKind
	// AccountID is the viewer's account; empty for ViewerAnonymous and for a fetch signed by a
	// server rather than an account.
	AccountID string
	// Moderator is true for a local account holding a moderation permission. It lets the viewer
	// see the posts of an author restricted by profile_view on the status and profile pages; it
	// never widens the visibility rules themselves.
	Moderator bool
	// DirectDenied hides every direct post from the viewer, the author's own included: the
	// viewer is a client API token that lacks the read:direct scope (ADR-0090 §4). It narrows
	// what the visibility rules allow and never widens them.
	DirectDenied bool
}

// FollowState is the viewer's follow of the author (follow_edges.state).
type FollowState int

// Follow states. Only an active (accepted) follow makes the viewer a follower.
const (
	FollowNone FollowState = iota
	FollowPending
	FollowActive
)

// Relation is what holds between the viewer and the status and its author.
type Relation struct {
	Follow FollowState // the viewer's follow of the author
	// Blocking is true when the viewer blocks the author; BlockedBy when the author blocks the viewer.
	Blocking  bool
	BlockedBy bool
	// Mentioned is true when the status mentions the viewer.
	Mentioned bool
}

// AuthorRestrictions are the author's moderation restrictions that bear on viewing
// (data-model §5.2; lifecycle states such as deleted map to ProfileView).
type AuthorRestrictions struct {
	ProfileView              bool // only the author and moderators see the profile and posts
	VisibilityCeilingPrivate bool // every post, past ones included, is followers-only (§5.4)
	PublicDiscovery          bool // kept off the local, federated, and hashtag timelines
}

// StatusFacts are the properties of a status that visibility depends on.
type StatusFacts struct {
	AuthorAccountID string
	AuthorIsLocal   bool
	Visibility      Visibility // the stored, original visibility
	// Unpublished is whether the status is a local reply held until the author of its parent
	// approves it (ADR-0155 §7): its author alone sees it.
	Unpublished bool
}

// ViewContext is where the status would be shown.
type ViewContext int

// View contexts. The zero value is invalid so that a forgotten context is denied.
const (
	_ ViewContext = iota
	// ContextStatus is fetching one status: the status page, a thread, or the client API.
	ContextStatus
	// ContextHome is the viewer's home timeline (FR-201).
	ContextHome
	// ContextLocalTimeline is the local public timeline (FR-202).
	ContextLocalTimeline
	// ContextFederatedTimeline is the federated public timeline (FR-202).
	ContextFederatedTimeline
	// ContextHashtag is a hashtag timeline (FR-203).
	ContextHashtag
	// ContextProfile is the author's profile timeline (FR-204).
	ContextProfile
	// ContextObjectFetch is a GET of the status's ActivityPub object IRI (ADR-0018).
	// A denial is answered with 404 so that existence does not leak.
	ContextObjectFetch
	// ContextDelivery is pushing the status to a remote actor's inbox (data-model §7).
	ContextDelivery
	// ContextSearch is a result of the full-text search of statuses (FR-309, ADR-0149 §8). It admits
	// only signed-in viewers, and nothing but public and local, not even for the author.
	ContextSearch
	// ContextDirectMessages is the viewer's direct messages by conversation (FR-212, ADR-0154
	// §6). It admits only signed-in viewers and only direct posts, whether or not the viewer
	// follows the author: the direct posts of others that the home timeline leaves out.
	ContextDirectMessages
)

// ViewContexts lists every view context.
func ViewContexts() []ViewContext {
	return []ViewContext{
		ContextStatus, ContextHome, ContextLocalTimeline, ContextFederatedTimeline,
		ContextHashtag, ContextProfile, ContextObjectFetch, ContextDelivery, ContextSearch, ContextDirectMessages,
	}
}

// String returns a short name for the context.
func (c ViewContext) String() string {
	switch c {
	case ContextStatus:
		return "status"
	case ContextHome:
		return "home"
	case ContextLocalTimeline:
		return "local timeline"
	case ContextFederatedTimeline:
		return "federated timeline"
	case ContextHashtag:
		return "hashtag"
	case ContextProfile:
		return "profile"
	case ContextObjectFetch:
		return "object fetch"
	case ContextDelivery:
		return "delivery"
	case ContextSearch:
		return "search"
	case ContextDirectMessages:
		return "direct messages"
	default:
		return "invalid"
	}
}

// CanView reports whether viewer may see status in context. It fails closed: an invalid
// visibility, viewer kind, or context, or a local visibility on a remote status, is denied. An
// unpublished status (a held local reply) is its author's alone.
func CanView(ctx ViewContext, viewer Viewer, status StatusFacts, author AuthorRestrictions, rel Relation) bool {
	if !status.Visibility.Valid() || (status.Visibility == VisibilityLocal && !status.AuthorIsLocal) {
		return false
	}
	if viewer.DirectDenied && status.Visibility == VisibilityDirect {
		return false
	}
	if !contextAdmits(ctx, viewer.Kind, status) {
		return false
	}
	isAuthor := viewer.Kind == ViewerLocal && viewer.AccountID != "" && viewer.AccountID == status.AuthorAccountID
	moderator := viewer.Kind == ViewerLocal && viewer.Moderator
	if status.Unpublished && !isAuthor {
		return false
	}

	// profile_view (suspension, deletion) hides everything except from the author and
	// moderators, and even they see it only on the status and profile pages.
	if author.ProfileView {
		privileged := isAuthor || moderator
		onOwnPages := ctx == ContextStatus || ctx == ContextProfile
		if !privileged || !onOwnPages {
			return false
		}
	}
	// Blocks either way always win over every grant below, except for the author's own view.
	if !isAuthor && (rel.Blocking || rel.BlockedBy) {
		return false
	}

	vis := EffectiveVisibility(status.Visibility, author)
	if !contextShows(ctx, vis, isAuthor, author, rel) {
		return false
	}
	return isAuthor || inAudience(ctx, vis, viewer.Kind, rel)
}

// contextAdmits reports whether the viewer kind and the status's origin make sense in ctx.
func contextAdmits(ctx ViewContext, kind ViewerKind, status StatusFacts) bool {
	switch ctx {
	case ContextStatus, ContextProfile, ContextFederatedTimeline, ContextHashtag:
		return kind == ViewerAnonymous || kind == ViewerLocal
	case ContextLocalTimeline:
		return (kind == ViewerAnonymous || kind == ViewerLocal) && status.AuthorIsLocal
	case ContextHome, ContextSearch, ContextDirectMessages:
		return kind == ViewerLocal
	case ContextObjectFetch:
		// This server serves only its own objects.
		return (kind == ViewerAnonymous || kind == ViewerRemote) && status.AuthorIsLocal
	case ContextDelivery:
		return kind == ViewerRemote && status.AuthorIsLocal
	default:
		return false
	}
}

// contextShows applies what each context lists, before the audience check.
func contextShows(ctx ViewContext, vis Visibility, isAuthor bool, author AuthorRestrictions, rel Relation) bool {
	switch ctx {
	case ContextHome:
		// Only the viewer's own posts and posts of accounts the viewer follows; direct posts from
		// others go to the conversation list instead.
		return isAuthor || rel.Follow == FollowActive
	case ContextLocalTimeline, ContextHashtag, ContextSearch:
		return !author.PublicDiscovery && (vis == VisibilityPublic || vis == VisibilityLocal)
	case ContextFederatedTimeline:
		return !author.PublicDiscovery && vis == VisibilityPublic
	case ContextDirectMessages:
		return vis == VisibilityDirect
	default:
		return true
	}
}

// inAudience reports whether a viewer other than the author is in the audience of vis.
func inAudience(ctx ViewContext, vis Visibility, kind ViewerKind, rel Relation) bool {
	follower := rel.Follow == FollowActive
	if ctx == ContextDelivery {
		// Push only to followers and mentioned actors; a local post reaches only followers,
		// never a mentioned remote actor that does not follow the author (ADR-0017).
		switch vis {
		case VisibilityPublic, VisibilityUnlisted, VisibilityPrivate:
			return follower || rel.Mentioned
		case VisibilityLocal:
			return follower
		case VisibilityDirect:
			return rel.Mentioned
		}
		return false
	}
	switch vis {
	case VisibilityPublic, VisibilityUnlisted:
		return true
	case VisibilityLocal:
		// Logged-in local accounts and followers, never anonymous visitors (ADR-0017).
		return kind == ViewerLocal || follower
	case VisibilityPrivate:
		return follower || rel.Mentioned
	case VisibilityDirect:
		return rel.Mentioned
	}
	return false
}
