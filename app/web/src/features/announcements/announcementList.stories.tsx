// SPDX-License-Identifier: AGPL-3.0-only
// The list page of the server's announcements (/announcements, web-client/screens/announcements.md), with the ones
// that show now and the ended ones, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
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
import { announcementFixture } from "./test/handlers";

/** An unread announcement with a long title, a read one, and an ended one. */
const history: Schemas["Announcement"][] = [
  announcementFixture("0199f0e0-0000-7000-8000-00000000b001", {
    title: "Maintenance on Sunday from 02:00 to 04:00 UTC: the server will be read-only",
    content:
      '<p>Maintenance on <strong>Sunday</strong> from 02:00 to 04:00 UTC. The server will be read-only while we move the database to a bigger disk. <a href="https://status.my.example/">Status page</a></p>',
    published_at: "2026-10-21T00:00:00Z",
    ends_at: "2026-10-28T00:00:00Z",
  }),
  announcementFixture("0199f0e0-0000-7000-8000-00000000b002", {
    title: "Voice chat is here",
    content: "<p>Voice chat is here: start a room from the navigation.</p>",
    published_at: "2026-10-20T00:00:00Z",
    read: true,
  }),
  announcementFixture("0199f0e0-0000-7000-8000-00000000b003", {
    title: "New rules",
    content: "<p>We updated the rules: please credit the recipes you share.</p>",
    published_at: "2026-09-01T00:00:00Z",
    ends_at: "2026-09-08T00:00:00Z",
    ended: true,
  }),
];

/** GET /announcements/history answering `items` (with a next page when `more`), and marking one read. */
function listing(items: Schemas["Announcement"][], more = false) {
  return [
    http.get(`${apiBase}/announcements/history`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ items, ...(more && { next_cursor: "next" }) })),
    ),
    http.post(`${apiBase}/announcements/:id/read`, ({ request, params }) =>
      withViewer(request, () => {
        const found = items.find((item) => item.id === params["id"]) ?? announcementFixture(String(params["id"]));
        return HttpResponse.json({ ...found, read: true });
      }),
    ),
  ];
}

const meta = {
  title: "Screens/About/Announcements list",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/announcements", signedIn: true, handlers: listing(history) } },
  play: async (context) => {
    await settled(context);
    await expect(await context.canvas.findByRole("heading", { level: 1, name: "Announcements" })).toBeInTheDocument();
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** One unread announcement that shows now, a read one, and an ended one. */
export const Normal: Story = {};

/** More announcements to load at the end of the list. */
export const MoreToLoad: Story = answering(...listing(history, true));

/** Opened from the notification of an announcement that was withdrawn: the top of the list says so. */
export const Withdrawn: Story = {
  parameters: { screen: { path: "/announcements?id=0199f0e0-0000-7000-8000-00000000b0ff" } },
};

/** No announcement has shown yet. */
export const Empty: Story = answering(...listing([]));

/** While the first page loads. */
export const Loading: Story = { ...loadingOn("get", "/announcements/history"), play: settled };

/** The list could not be loaded: the page says why in its place, with a way to try again. */
export const ServerError: Story = failingOn("get", "/announcements/history");

/** In Japanese, with an announcement written in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...answering(
    ...listing([
      announcementFixture("0199f0e0-0000-7000-8000-00000000b005", {
        title: "日曜日のメンテナンス",
        content: "<p>日曜日の 2 時から 4 時までメンテナンスを行います。その間は投稿できません。</p>",
      }),
      ...history.slice(1),
    ]),
  ),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
