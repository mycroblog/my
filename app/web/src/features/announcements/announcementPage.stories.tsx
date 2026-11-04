// SPDX-License-Identifier: AGPL-3.0-only
// The page of one of the server's announcements (/announcements/$id, web-client/screens/announcement.md), in the states
// of ADR-0079 §13
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

const id = "0199f0e0-0000-7000-8000-00000000b001";

/** An announcement that was read, with a long title and the end of its window. */
const announcement = announcementFixture(id, {
  title: "Maintenance on Sunday from 02:00 to 04:00 UTC: the server will be read-only",
  content:
    '<p>Maintenance on <strong>Sunday</strong> from 02:00 to 04:00 UTC. The server will be read-only while we move the database to a bigger disk. <a href="https://status.my.example/">Status page</a></p>',
  ends_at: "2026-10-28T00:00:00Z",
  read: true,
});

/** GET /announcements/{id} answering `found`, and marking it read. */
function answer(found: Schemas["Announcement"]) {
  return [
    http.get(`${apiBase}/announcements/${id}`, ({ request }) => withViewer(request, () => HttpResponse.json(found))),
    http.post(`${apiBase}/announcements/${id}/read`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ ...found, read: true })),
    ),
  ];
}

const meta = {
  title: "Screens/About/Announcement",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: `/announcements/${id}`, signedIn: true, handlers: answer(announcement) } },
  play: async (context) => {
    await settled(context);
    await expect(await context.canvas.findByRole("article")).toBeInTheDocument();
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** An announcement that shows now. */
export const Normal: Story = {};

/** One whose window has ended. */
export const Ended: Story = answering(...answer({ ...announcement, ended: true }));

/** The address of an announcement that was withdrawn, or of none: the page says so, with the way to the list. */
export const NotFound: Story = {
  ...answering(
    http.get(`${apiBase}/announcements/${id}`, ({ request }) =>
      withViewer(request, () =>
        HttpResponse.json({ type: "about:blank", title: "Not Found", status: 404 }, { status: 404 }),
      ),
    ),
  ),
  play: async (context) => {
    await settled(context);
    await expect(await context.canvas.findByRole("link", { name: "All announcements" })).toBeInTheDocument();
  },
};

/** While it loads. */
export const Loading: Story = { ...loadingOn("get", "/announcements/:id"), play: settled };

/** It could not be loaded: the page says why in its place, with a way to try again. */
export const ServerError: Story = { ...failingOn("get", "/announcements/:id"), play: settled };

/** In Japanese, with an announcement written in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...answering(
    ...answer(
      announcementFixture(id, {
        title: "日曜日のメンテナンス",
        content: "<p>日曜日の 2 時から 4 時までメンテナンスを行います。その間は投稿できません。</p>",
        read: true,
      }),
    ),
  ),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** On a narrow screen. */
export const Narrow: Story = narrow;
