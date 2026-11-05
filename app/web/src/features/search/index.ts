// SPDX-License-Identifier: AGPL-3.0-only
// The search feature: finding an account by its handle or URL, on this server or on its own (FR-305), and posts by
// their words on a server with a search engine (FR-309).
// Routes and other features use it through this file only (web-client/README.md A3).
export { AccountSearch } from "./components/AccountSearch";
export { SearchTabs, type SearchType } from "./components/SearchTabs";
export { maxSearchLength } from "./api/lookup";
