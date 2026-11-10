// SPDX-License-Identifier: AGPL-3.0-only
// The keyword filters feature (FR-209, ADR-0157): the user's filters on their settings page, which fold or hide the
// posts that contain their words. The server matches the posts; the status feature folds the ones it marked. Routes
// and other features use it through this file only (web-client/README.md A3).
export { instanceQueryOptions, keywordFiltersQueryOptions } from "./api/filters";
export { KeywordFilters } from "./components/KeywordFilters";
