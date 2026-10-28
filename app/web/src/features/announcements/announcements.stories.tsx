// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements as a user sees them, above the home timeline (/home),
// in the states of ADR-0112 §1. The home timeline itself is in its own stories.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { answering, failingOn, inLocale, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { announcementFixture } from "./test/handlers";

/** Two unread announcements, one long with an end, and two read ones. */
const four: Schemas["Announcement"][] = [
  announcementFixture("0199f0e0-0000-7000-8000-00000000b001", {
    content:
      '<p>Maintenance on <strong>Sunday</strong> from 02:00 to 04:00 UTC. The server will be read-only while we move the database to a bigger disk; posts you write in that time wait in your browser and go out once we are back. <a href="https://status.my.example/">Status page</a></p>',
    published_at: "2026-10-21T00:00:00Z",
    ends_at: "2026-10-28T00:00:00Z",
  }),
  announcementFixture("0199f0e0-0000-7000-8000-00000000b002", {
    content: "<p>Voice chat is here: start a room from the navigation.</p>",
    published_at: "2026-10-20T00:00:00Z",
  }),
  announcementFixture("0199f0e0-0000-7000-8000-00000000b003", {
    content: "<p>Welcome to everyone who joined this week!</p>",
    published_at: "2026-10-15T00:00:00Z",
    read: true,
  }),
  announcementFixture("0199f0e0-0000-7000-8000-00000000b004", {
    content: "<p>We updated the rules: please credit the recipes you share.</p>",
    published_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-03T00:00:00Z",
    read: true,
  }),
];

/** GET /announcements answering `items`, and marking one read answering it read. */
function announcing(items: Schemas["Announcement"][]) {
  return [
    http.get(`${apiBase}/announcements`, ({ request }) => withViewer(request, () => HttpResponse.json({ items }))),
    http.post(`${apiBase}/announcements/:id/read`, ({ request, params }) =>
      withViewer(request, () => {
        const found = items.find((item) => item.id === params["id"]) ?? announcementFixture(String(params["id"]));
        return HttpResponse.json({ ...found, read: true });
      }),
    ),
  ];
}

const meta = {
  title: "Screens/About/Announcements",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/home", signedIn: true, handlers: announcing(four) } },
  play: async (context) => {
    await settled(context);
    await expect(await context.canvas.findByRole("region", { name: /^Announcements/u })).toBeInTheDocument();
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two unread announcements, one long with an end date, and two read ones. */
export const Normal: Story = {};

/** One unread announcement. */
export const One: Story = answering(...announcing(four.slice(1, 2)));

/** No announcements: nothing shows above the home timeline. */
export const Empty: Story = { ...answering(...announcing([])), play: settled };

/** The server failed to answer the announcements: the home timeline shows without them. */
export const ServerError: Story = { ...failingOn("get", "/announcements"), play: settled };

/** In Japanese, with an announcement written in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...answering(
    ...announcing([
      announcementFixture("0199f0e0-0000-7000-8000-00000000b005", {
        content: "<p>日曜日の 2 時から 4 時までメンテナンスを行います。その間は投稿できません。</p>",
      }),
      ...four.slice(2),
    ]),
  ),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
