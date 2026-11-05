// SPDX-License-Identifier: AGPL-3.0-only

// Package search is the module of the full-text search of statuses (ADR-0149): the port of the
// search engine an operator chooses with the setting search, and the job types of its index.
//
// The database is the source of truth: an index can be thrown away and rebuilt. A backend returns
// status IDs only, and never text, HTML, highlights, or scores, so that nothing it holds is shown
// without being read again from the database. The search of accounts and hashtags (FR-306,
// FR-307) does not go through this module.
//
// A backend's index follows the database through jobs that the facts enqueue in their transactions,
// each carrying an ID and reading the current state when it runs (ADR-0149 §7). Eligible is the one
// rule of what the index may hold.
//
// Service.Search answers a query: the backend narrows the candidates, and each is read again from the
// database and passes Eligible and the single visibility policy (ADR-0149 §8).
//
// With search = none there is no backend: the server opens no connection to an engine, starts no
// worker for it, and enqueues no search job (ADR-0149 §2).
package search
