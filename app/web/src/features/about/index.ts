// SPDX-License-Identifier: AGPL-3.0-only
// The about feature (FR-718): the instance's information page (`/about`) that everyone may read, signed in or not,
// with its description, rules, the remote domains it moderates as far as its settings list them, and the links to
// its documents. Routes and other features use it through this file only (web-client/README.md A3).
export { instanceQueryOptions, publicDomainActionsQueryOptions } from "./api/about";
export { About, SourceCodeLink } from "./components/About";
