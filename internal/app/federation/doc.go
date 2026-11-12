// SPDX-License-Identifier: AGPL-3.0-only

// Package federation holds the use cases and port definitions of the federation module: ActivityPub inbox processing, delivery, actor and object fetching, signatures, and remote domain health.
// It is the module's public API; other modules reach the federation module only through this package.
//
// The module writes only its own tables. What an inbox handler receives, a remote actor, post,
// boost, favourite, follow, block, or report, it writes through the application API of the module that owns it
// (account, content, graph, moderation), inside the handler's one short transaction. The other way, each domain
// event of a local command that goes out has one projector (RegisterOutboundProjectors and its Register*Projectors
// neighbours), which records the outbound activity and its fan-out job in the command's transaction.
//
// The document an outbound activity delivers is built once and every delivery and retry sends the
// same bytes. The fan-out job builds most documents; the activities whose source may be gone by
// then are built in the transaction that records them: the Delete of a status, Like and Undo
// Like (from the event's URIs, so an Undo Like can embed the Like after the favourite's row is
// deleted), and the relay activities, written with the relay's events, whose inbox travels in the
// fan-out job's payload so that they still arrive after the relay is removed. The Update and
// Delete of a status go to the inboxes of the deliveries of its Create.
//
// instance_health is a current value, not a projection. Each retryable failure on a new UTC day
// adds a failure day, and seven make the server unavailable; a success, a signed inbound request,
// or a successful probe resets the run, but is written only when there is a run to reset or the
// server is unavailable. Those thinned successes leave no event, and delivery attempts expire, so
// the table cannot be rebuilt from events.
package federation
