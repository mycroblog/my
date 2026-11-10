// SPDX-License-Identifier: AGPL-3.0-only

// Package activitypub serves the federation endpoints by calling application use cases:
// WebFinger and host-meta (FR-401), NodeInfo 2.1 (FR-402), the actor documents and collections
// of local accounts and the instance actor (FR-403), and the objects of local statuses, with the
// URIs of ADR-0018, and the inboxes (FR-405).
//
// Every lookup that finds nothing, or finds something that may not be shown (a remote account,
// a local account that is not active, a status hidden from the requester), answers with the same
// 404 response (ADR-0018). Objects (/ap/objects/{id} and its /activity) are judged for the
// requester that the request's HTTP signature names, through the federation module; a deleted
// status answers 410 with a Tombstone, and so does a deleted local actor. Responses that depend on the Accept header carry
// Vary: Accept, and only documents that anyone may fetch anonymously are cacheable
// (Cache-Control: public, max-age=60).
//
// The shared inbox (POST /ap/inbox) and the inbox of each active local actor (POST
// /ap/actors/{id}/inbox) check the content type and the body size, and leave the signature,
// the rate limits, and the record of the activity to the federation module (federation.md §3.1):
// they answer 202 Accepted, or the status of the step that refused the request. The outbox
// (the Create activities of public and unlisted statuses), followers, and following
// collections are paged (?page=true, then ?max_id=…&page=true); featured holds the pinned public
// and unlisted statuses, and featured-tags the hashtags that the actor features. Profile pages
// (/@alice) redirect ActivityPub requests to the actor and pass every other request to the web
// client's HTML handler (Options.HTML).
package activitypub
