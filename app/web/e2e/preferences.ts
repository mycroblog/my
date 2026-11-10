// SPDX-License-Identifier: AGPL-3.0-only
// The signed-in user's preferences that specs answer GET /preferences with when they mock the API. Every
// required field of the schema is here, so that a new one is added in this one place.
import type { components } from "@mycroblog/mykit";

type Preferences = components["schemas"]["Preferences"];

/** A full `Preferences`: English, UTC (chosen), public posts that everyone may reply to, boost and quote. */
const preferencesFixture: Preferences = {
  locale: "en",
  locale_follows_browser: false,
  time_zone: "UTC",
  time_zone_chosen: true,
  time_zone_follows_browser: false,
  default_visibility: "public",
  default_interaction_policy: {
    reply: { automatic: "everyone" },
    announce: { automatic: "everyone" },
    quote: { automatic: "everyone" },
  },
  default_post_language: "und",
  require_media_description: false,
  timeline_auto_insert: false,
  link_card_size: "large",
  timeline_languages: [],
};

/** The full preferences, with the fields of `overrides` in place of their defaults; `preferencesOf({})` gives them as they are. */
export function preferencesOf(overrides: Partial<Preferences>): Preferences {
  return { ...preferencesFixture, ...overrides };
}
