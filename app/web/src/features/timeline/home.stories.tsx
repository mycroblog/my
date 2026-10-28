// SPDX-License-Identifier: AGPL-3.0-only
// The home timeline (/home; FR-201, web-client/screens/home-first-run.md) in the states of ADR-0112 §1, with the
// first run of someone who follows nobody yet.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect, waitFor } from "storybook/test";
import { type Schemas, statusPage } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { bob, carol, haruka, maximiliana, newsBot, pictureHandlers, variedTimeline } from "../../test/postStories";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
  viewerWith,
} from "../../test/stories";

/** GET /timelines/home answering `items`. */
function homeTimeline(items: Schemas["Status"][]) {
  return http.get(`${apiBase}/timelines/home`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(statusPage(items))),
  );
}

/** GET /accounts/suggestions answering `suggestions`. */
function suggestions(items: Schemas["AccountSuggestion"][]) {
  return http.get(`${apiBase}/accounts/suggestions`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(items)),
  );
}

/** The follow of a suggested account, which the server accepts at once. */
const follow = http.post(`${apiBase}/accounts/:id/follow`, ({ params, request }) =>
  withViewer(request, () =>
    HttpResponse.json({
      id: String(params["id"]),
      following: true,
      followed_by: false,
      requested: false,
      requested_by: false,
      show_boosts: true,
      notify: false,
      blocking: false,
      muting: false,
      muting_notifications: false,
    } satisfies Schemas["Relationship"]),
  ),
);

/** A new user: no display name, no post, following nobody, with accounts suggested to follow. */
const firstRun = [
  viewerWith({ display_name: "", statuses_count: 0, following_count: 0, followers_count: 0 }),
  homeTimeline([]),
  suggestions([
    { source: "featured", account: { ...bob, note: "<p>I bake bread and write about it.</p>" } },
    { source: "featured", account: maximiliana },
    { source: "recently_active", account: carol },
    { source: "recently_active", account: haruka },
    { source: "recently_active", account: newsBot },
  ]),
  follow,
];

const meta = {
  title: "Screens/Timeline/Home",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: { path: "/home", signedIn: true, handlers: [...pictureHandlers, homeTimeline(variedTimeline)] },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Posts of every kind: a poll, many pictures, a content warning, a link card, a quote, a boost, and a reply. */
export const Normal: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    // The worker draws the pictures: the first avatar has loaded.
    await waitFor(async () => {
      const avatar = context.canvasElement.querySelector<HTMLImageElement>('main img[src*="/media/avatars/"]');
      await expect(avatar?.naturalWidth).toBeGreaterThan(0);
    });
    const warned = await canvas.findByRole("button", { name: "Show content", expanded: false });
    await userEvent.click(warned);
    await expect(warned).toHaveAttribute("aria-expanded", "true");
  },
};

/** Someone who follows nobody yet: the timeline is empty. */
export const Empty: Story = answering(homeTimeline([]));

/** The first run of a new user: the three first steps and the accounts suggested to follow; a follow counts. */
export const FirstRun: Story = {
  ...answering(...firstRun),
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    const checklist = await canvas.findByRole("region", { name: "Get started" });
    await expect(checklist).toBeInTheDocument();
    const region = await canvas.findByRole("region", { name: "Suggested accounts" });
    const [first] = await canvas.findAllByRole("button", { name: "Follow" });
    if (first === undefined) throw new Error("no suggestion to follow");
    await userEvent.click(first);
    await expect(region).toBeInTheDocument();
  },
};

/** The first run in Japanese. */
export const FirstRunJapanese: Story = { ...inLocale("ja"), ...answering(...firstRun) };

/** The first run at 320 CSS px. */
export const FirstRunNarrow: Story = { ...narrow, ...answering(...firstRun) };

/** The first page has not arrived yet. */
export const Loading: Story = loadingOn("get", "/timelines/home");

/** The server failed to answer the first page. */
export const ServerError: Story = failingOn("get", "/timelines/home");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, with the floating new-post button. */
export const Narrow: Story = narrow;
