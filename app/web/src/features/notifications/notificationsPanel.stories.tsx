// SPDX-License-Identifier: AGPL-3.0-only
// The latest notifications in the third column of a timeline page on a desktop (xl; web-client/09-app-shell.md §9), in
// the states of ADR-0079 §13 The rows themselves are in the stories of the notifications page.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { bob, carol, dmitri, haruka, maximiliana, pictureHandlers, post, viewerPost } from "../../test/postStories";
import { answering, failingOn, inLocale, loadingOn, renderScreen, screenLoader, settled } from "../../test/stories";

type Notification = Schemas["Notification"];

/** Notification `n` of `type`, `n` × 13 minutes before the newest, with `fields`; a lower `n` is newer. */
function notification(n: number, type: Schemas["NotificationType"], fields: Partial<Notification> = {}): Notification {
  return {
    id: `0192f0c4-7a8e-7000-8000-0000000d${String(100 - n).padStart(4, "0")}`,
    type,
    created_at: new Date(Date.UTC(2026, 9, 28, 12, 0) - n * 13 * 60_000).toISOString(),
    ...fields,
  };
}

const latest: Notification[] = [
  notification(0, "mention", { account: maximiliana, status: post(90, maximiliana) }),
  notification(1, "favourite", { account: bob, status: viewerPost }),
  notification(2, "favourite", { account: carol, status: viewerPost }),
  notification(3, "follow", { account: haruka }),
  notification(4, "boost", { account: dmitri, status: viewerPost }),
];

function notifications(items: Notification[], unread: number) {
  return [
    http.get(`${apiBase}/notifications`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ items } satisfies Schemas["NotificationPage"])),
    ),
    http.get(`${apiBase}/notifications/marker`, ({ request }) =>
      withViewer(request, () =>
        HttpResponse.json({
          ...(items[unread] !== undefined && { last_read_id: items[unread].id }),
          unread_count: unread,
        } satisfies Schemas["NotificationMarker"]),
      ),
    ),
  ];
}

const meta = {
  title: "Screens/Notifications/Panel",
  render: renderScreen,
  loaders: [screenLoader],
  globals: { viewport: { value: "desktop", isRotated: false } },
  parameters: {
    screen: { path: "/home", signedIn: true, handlers: [...pictureHandlers, ...notifications(latest, 2)] },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The latest ones, the newest two unread, with the favourites of one post in one row. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    const panel = await context.canvas.findByRole("complementary", { name: /^Notifications/u });
    await expect(panel).toBeInTheDocument();
  },
};

/** None yet: it says so, and links to the page. */
export const Empty: Story = answering(...notifications([], 0));

/** The first read has not answered. */
export const Loading: Story = loadingOn("get", "/notifications");

/** The read failed: the panel says so and offers to try again, and the timeline stays. */
export const ServerError: Story = failingOn("get", "/notifications");

export const Japanese: Story = inLocale("ja");

export const RightToLeft: Story = inLocale("ar-XB");
