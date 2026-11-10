// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
)

// ObjectConversations is the part of the content module's public API that the Notes and the
// collections of conversations read (ADR-0154 §5, §9). *appcontent.Service implements it.
type ObjectConversations interface {
	NoteConversations
	ResolveConversation(ctx context.Context, id string) (appcontent.ResolvedConversation, error)
	ConversationStatuses(ctx context.Context, conversationID, after string, limit int) (appcontent.ConversationPage, error)
	AuthorRestrictionsOf(ctx context.Context, accountIDs []string) (map[string]appcontent.AuthorRestrictions, error)
}

// ConversationCollection is the answer to a GET of the IRI of a conversation of this server.
type ConversationCollection struct {
	// RedirectTo is the IRI of the conversation of this server that the requested one was merged
	// into, "" unless the request is redirected there (308).
	RedirectTo string
	// URI is the conversation's IRI, the id of the collection.
	URI string
	// Items are the IRIs of the statuses of the page that the requester may see, oldest first.
	Items []string
	// Next is the cursor of the next page, "" on the last page.
	Next string
}

// Conversation returns the collection of a conversation of this server for req (GET
// /ap/contexts/{id}, ADR-0154 §9), or with paged its page after the cursor after ("" for the
// first): the statuses of its thread that are not deleted, oldest first, ConversationPageSize at
// a time, of which only those that req may see are listed. An item left out still moves the
// cursor, so a page may be short or empty and still have a next page. A conversation that was
// merged into another of this server is redirected there; one merged into a remote one, one
// with a remote IRI, and one that does not exist fail with an error wrapping ErrNotFound alike.
func (o *Objects) Conversation(ctx context.Context, req Requester, id string, paged bool, after string) (ConversationCollection, error) {
	if o.d.Conversations == nil {
		return ConversationCollection{}, fmt.Errorf("conversation %s: %w", id, ErrNotFound)
	}
	c, err := o.d.Conversations.ResolveConversation(ctx, id)
	if errors.Is(err, appcontent.ErrNotFound) {
		return ConversationCollection{}, fmt.Errorf("conversation %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return ConversationCollection{}, err
	}
	switch {
	case !c.Named.IsLocal || (c.Merged() && !c.Final.IsLocal):
		return ConversationCollection{}, fmt.Errorf("conversation %s: %w", id, ErrNotFound)
	case c.Merged():
		return ConversationCollection{RedirectTo: c.Final.URI}, nil
	}
	out := ConversationCollection{URI: c.Final.URI}
	if !paged {
		return out, nil
	}
	page, err := o.d.Conversations.ConversationStatuses(ctx, c.Final.ID, after, appcontent.ConversationPageSize)
	if err != nil {
		return ConversationCollection{}, err
	}
	out.Next = page.Next
	visible, err := o.visibleAmong(ctx, req, page)
	if err != nil {
		return ConversationCollection{}, err
	}
	out.Items = []string{}
	for _, st := range page.Statuses {
		if visible[st.ID] {
			out.Items = append(out.Items, st.URI)
		}
	}
	return out, nil
}

// visibleAmong decides with appcontent.CanView in ContextObjectFetch, as canView does for one
// status, which statuses of page req may see, with reads that grow with the authors of the page
// rather than with its statuses: their restrictions and the signer's relationships with them
// together, the mentioned accounts of the statuses that need them together, and whether an
// account on the signer's domain follows an author once per author, only when a status needs it.
// A remote status is listed only when an anonymous visitor may see it on its page
// (ContextStatus) and the signer and its author do not block each other, so that this server never passes on a remote status addressed
// to others.
func (o *Objects) visibleAmong(ctx context.Context, req Requester, page appcontent.ConversationPage) (map[string]bool, error) {
	out := make(map[string]bool, len(page.Statuses))
	if len(page.Statuses) == 0 {
		return out, nil
	}
	var authors []string
	for _, st := range page.Statuses {
		if !slices.Contains(authors, st.AuthorAccountID) {
			authors = append(authors, st.AuthorAccountID)
		}
	}
	restrictions, err := o.d.Conversations.AuthorRestrictionsOf(ctx, authors)
	if err != nil {
		return nil, err
	}
	rels := map[string]appcontent.Relation{}
	if req.AccountID != "" {
		if rels, err = o.blocksWith(ctx, req.AccountID, authors); err != nil {
			return nil, err
		}
	}
	anon := appcontent.Viewer{Kind: appcontent.ViewerAnonymous}
	viewer := anon
	if !req.Anonymous() {
		viewer = appcontent.Viewer{Kind: appcontent.ViewerRemote, AccountID: req.AccountID}
	}
	// The local statuses that the signer's blocks alone do not decide.
	var pending []appcontent.Status
	for _, st := range page.Statuses {
		facts := appcontent.StatusFacts{AuthorAccountID: st.AuthorAccountID, AuthorIsLocal: st.IsLocal, Visibility: st.Visibility}
		rel := rels[st.AuthorAccountID]
		if !st.IsLocal {
			// The object fetch serves only this server's statuses; a remote one is listed as its
			// page shows it to an anonymous visitor.
			out[st.ID] = !rel.Blocking && !rel.BlockedBy &&
				appcontent.CanView(appcontent.ContextStatus, anon, facts, restrictions[st.AuthorAccountID], appcontent.Relation{})
			continue
		}
		if appcontent.CanView(appcontent.ContextObjectFetch, viewer, facts, restrictions[st.AuthorAccountID], rel) {
			out[st.ID] = true
		} else if !req.Anonymous() {
			pending = append(pending, st)
		}
	}
	if len(pending) == 0 {
		return out, nil
	}
	var mentioned []string
	for _, st := range pending {
		for _, id := range page.Revisions[st.ID].MentionedAccountIDs {
			if !slices.Contains(mentioned, id) {
				mentioned = append(mentioned, id)
			}
		}
	}
	onDomain, err := o.accountsOnDomain(ctx, req.Domain, mentioned)
	if err != nil {
		return nil, err
	}
	follows := map[string]bool{}
	for _, st := range pending {
		facts := appcontent.StatusFacts{AuthorAccountID: st.AuthorAccountID, AuthorIsLocal: true, Visibility: st.Visibility}
		rel := rels[st.AuthorAccountID]
		rel.Mentioned = slices.ContainsFunc(page.Revisions[st.ID].MentionedAccountIDs, func(id string) bool { return onDomain[id] })
		if appcontent.CanView(appcontent.ContextObjectFetch, viewer, facts, restrictions[st.AuthorAccountID], rel) {
			out[st.ID] = true
			continue
		}
		f, ok := follows[st.AuthorAccountID]
		if !ok {
			if f, err = o.followerOnDomain(ctx, st.AuthorAccountID, req.Domain); err != nil {
				return nil, err
			}
			follows[st.AuthorAccountID] = f
		}
		if f {
			rel.Follow = appcontent.FollowActive
			out[st.ID] = appcontent.CanView(appcontent.ContextObjectFetch, viewer, facts, restrictions[st.AuthorAccountID], rel)
		}
	}
	return out, nil
}

// blocksWith returns the blocks between the signer's account and each of authors, by author:
// together when Graph can, one by one otherwise.
func (o *Objects) blocksWith(ctx context.Context, accountID string, authors []string) (map[string]appcontent.Relation, error) {
	rs := make(map[string]appgraph.Relationship, len(authors))
	if b, ok := o.d.Graph.(relationBatch); ok {
		var err error
		if rs, err = b.Relationships(ctx, accountID, authors); err != nil {
			return nil, err
		}
	} else {
		for _, id := range authors {
			r, err := o.d.Graph.Relationship(ctx, accountID, id)
			if err != nil {
				return nil, err
			}
			rs[id] = r
		}
	}
	out := make(map[string]appcontent.Relation, len(authors))
	for _, id := range authors {
		out[id] = appcontent.Relation{Blocking: rs[id].Blocking, BlockedBy: rs[id].BlockedBy}
	}
	return out, nil
}

// objectAccountBatch is an ObjectAccounts that reads many accounts at once, as
// *appaccount.Service does.
type objectAccountBatch interface {
	FindByIDs(ctx context.Context, ids []string) (map[string]appaccount.Actor, error)
}

// accountsOnDomain returns the accounts among ids that are on domain, as AnyOnDomain tells of
// them: with one read when Accounts can, one by one otherwise.
func (o *Objects) accountsOnDomain(ctx context.Context, domain string, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if b, ok := o.d.Accounts.(objectAccountBatch); ok {
		actors, err := b.FindByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		for id, a := range actors {
			out[id] = a.Account.Domain == domain
		}
		return out, nil
	}
	for _, id := range ids {
		found, err := o.d.Accounts.AnyOnDomain(ctx, domain, []string{id})
		if err != nil {
			return nil, err
		}
		out[id] = found
	}
	return out, nil
}
