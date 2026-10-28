// SPDX-License-Identifier: AGPL-3.0-only
// The timeline of a hashtag (/tags/{tag}; FR-203) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { type Schemas, statusPage } from "../../test/fixtures";
import { apiBase } from "../../test/msw/http";
import { haruka, longPost, pictureHandlers, post, variedTimeline } from "../../test/postStories";
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

/** GET /timelines/tag/{hashtag} answering `items`. */
function timeline(items: Schemas["Status"][]) {
  return http.get(`${apiBase}/timelines/tag/:hashtag`, () => HttpResponse.json(statusPage(items)));
}

/** The posts with #sourdough. */
const sourdough = [longPost, ...variedTimeline.filter((status) => status.poll !== undefined)];

const meta = {
  title: "Screens/Timeline/Hashtag",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: { path: "/tags/sourdough", signedIn: true, handlers: [...pictureHandlers, timeline(sourdough)] },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The posts with the hashtag. */
export const Normal: Story = {};

/** To someone who is not signed in. */
export const SignedOut: Story = { parameters: { screen: { signedIn: false } } };

/** A hashtag nobody used yet. */
export const Empty: Story = answering(timeline([]));

/** The first page has not arrived yet. */
export const Loading: Story = loadingOn("get", "/timelines/tag/sourdough");

/** The server failed to answer the first page. */
export const ServerError: Story = failingOn("get", "/timelines/tag/sourdough");

/** A long hashtag in Japanese, in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  parameters: {
    screen: {
      path: "/tags/週末のパン作り記録",
      handlers: [
        timeline([
          post(50, haruka, {
            content: "<p>今週はライ麦パンに挑戦しました。少し酸味が強かったけど、おいしかった！</p>",
            language: "ja",
            favourites_count: 8,
          }),
        ]),
      ],
    },
  },
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
