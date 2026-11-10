// SPDX-License-Identifier: AGPL-3.0-only

// Package gathering holds the rules of the events of the local users (ADR-0161): the settings of
// an event and their bounds, its date and places, the values of its visibility, the answers of
// its attendees, and the reasons its records carry, and the pure policy functions that decide who
// sees an event (CanSee), who sees each of its places (CanSeePlace), and who may organize one
// (CanOrganize). The code calls an event a gathering, so that it is not mistaken for the events
// of the data model (ADR-0020); the UI, the API, and the documents call it an event.
//
// The facts these functions read (follows, blocks, invitations, answers, approvals, the state of
// the accounts) are gathered by internal/app/gathering; this package knows nothing of the
// database or of the other modules.
package gathering
