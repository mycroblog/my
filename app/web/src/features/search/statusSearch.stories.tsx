// SPDX-License-Identifier: AGPL-3.0-only
// The search of posts (/search?type=posts; FR-309, web-client/screens/search.md) in the states of ADR-0079 §13, on a
// server with a search engine.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { instanceFixture, type Schemas, statusPage } from "../../test/fixtures";
import { apiBase, problem } from "../../test/msw/http";
import { longPost, pictureHandlers, variedTimeline } from "../../test/postStories";
import { answering, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled } from "../../test/stories";

/** GET /instance of a server whose search covers `visibilities`. */
function searchOn(visibilities: string[]) {
  return http.get(`${apiBase}/instance`, () =>
    HttpResponse.json({ ...instanceFixture, full_text_search: { available: true, visibilities } }),
  );
}

/** GET /statuses/search answering `items`. */
function searchAnswers(items: Schemas["Status"][]) {
  return http.get(`${apiBase}/statuses/search`, () => HttpResponse.json(statusPage(items)));
}

const meta = {
  title: "Screens/Search/Posts",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/search?type=posts&q=bread",
      signedIn: true,
      handlers: [...pictureHandlers, searchOn(["public"]), searchAnswers([longPost, ...variedTimeline])],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The posts found, drawn as in a timeline, with their content warnings and sensitive media. */
export const Normal: Story = {};

/** Before anything is sent. */
export const Idle: Story = { parameters: { screen: { path: "/search?type=posts" } } };

/** No post found, on a server whose search covers public posts. */
export const Empty: Story = answering(searchAnswers([]));

/** No post found, on a server whose search also covers the posts for this server. */
export const EmptyWithLocal: Story = answering(searchOn(["public", "local"]), searchAnswers([]));

/** The first page has not arrived yet. */
export const Loading: Story = loadingOn("get", "/statuses/search");

/** The search engine failed. */
export const SearchFailed: Story = answering(
  http.get(`${apiBase}/statuses/search`, () =>
    problem(503, "/api/v1/problems/search-failed", "The search is not working right now."),
  ),
);

/** Over the rate limit of the search. */
export const RateLimited: Story = answering(
  http.get(`${apiBase}/statuses/search`, () =>
    problem(429, "/api/v1/problems/rate-limited", "Too many requests", { headers: { "Retry-After": "120" } }),
  ),
);

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
