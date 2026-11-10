// SPDX-License-Identifier: AGPL-3.0-only
// The hashtags feature: following a hashtag, whose posts then join the home timeline (ADR-0013 §24–§28), the list of
// the followed hashtags in the settings, and the hashtags a profile features (ADR-0006 §17), shown on the profile and
// chosen in the profile form. Routes and other features use it through this file only (web-client/README.md A3).
export { accountFeaturedTagsQueryOptions, featuredTagsQueryOptions, followedTagsQueryOptions } from "./api/tags";
export { FeaturedTags } from "./components/FeaturedTags";
export { FeaturedTagsEditor } from "./components/FeaturedTagsEditor";
export { FollowedTags } from "./components/FollowedTags";
export { TagFollowButton } from "./components/TagFollowButton";
export { useFeaturedTagsDraft } from "./hooks/useFeaturedTagsDraft";
