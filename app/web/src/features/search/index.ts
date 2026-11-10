// SPDX-License-Identifier: AGPL-3.0-only
// The search feature: finding an account by its handle or URL, on this server or on its own (FR-305), and posts by
// their words on a server with a search engine (FR-309), and the page that the follow and reply buttons of other
// servers open (the Activity Intents of WebFinger, FEP-3b86).
// Routes and other features use it through this file only (web-client/README.md A3).
export { AccountSearch } from "./components/AccountSearch";
export { SearchTabs, type SearchType } from "./components/SearchTabs";
export { maxSearchLength } from "./api/lookup";
export { ActivityIntent } from "./components/ActivityIntent";
export { type ActivityIntentKind } from "./hooks/useActivityIntent";
