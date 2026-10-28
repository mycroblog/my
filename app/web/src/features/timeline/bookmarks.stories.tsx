// SPDX-License-Identifier: AGPL-3.0-only
// The posts the viewer bookmarked (/bookmarks; FR-113) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { type Schemas, statusPage } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { pictureHandlers, variedTimeline } from "../../test/postStories";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";

/** GET /bookmarks answering `items`, each bookmarked. */
function bookmarks(items: Schemas["Status"][]) {
  return http.get(`${apiBase}/bookmarks`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(statusPage(items.map((status) => ({ ...status, bookmarked: true }))))),
  );
}

const meta = {
  title: "Screens/Timeline/Bookmarks",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/bookmarks",
      signedIn: true,
      handlers: [...pictureHandlers, bookmarks(variedTimeline.filter((status) => status.boost === undefined))],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Bookmarked posts of every kind. */
export const Normal: Story = {};

/** Nothing bookmarked yet. */
export const Empty: Story = answering(bookmarks([]));

/** The first page has not arrived yet. */
export const Loading: Story = loadingOn("get", "/bookmarks");

/** The server failed to answer the first page. */
export const ServerError: Story = failingOn("get", "/bookmarks");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
