// SPDX-License-Identifier: AGPL-3.0-only
// A follow request notification on the notifications page (/notifications?filter=follows; FR-302) in each state of its
// answers: waiting, approved, declined, no longer waiting, answered elsewhere (a 404), and failed.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { delay, http, HttpResponse, type RequestHandler } from "msw";
import { expect, within } from "storybook/test";
import { relationshipFixture, type Schemas } from "../../test/fixtures";
import { apiBase, problem, withViewer } from "../../test/msw/http";
import { bob, pictureHandlers } from "../../test/postStories";
import { answering, inLocale, narrow, renderScreen, screenLoader, settled, viewerWith } from "../../test/stories";
import type { ScreenPlayContext } from "../../test/stories";

/** Bob's follow request, the only notification. */
const items: Schemas["Notification"][] = [
  {
    id: "0192f0c4-7a8e-7000-8000-0000000e0002",
    type: "follow_request",
    created_at: "2026-10-28T11:47:00Z",
    account: { ...bob, locked: false },
  },
];

/** GET /notifications answering the request, all of it read. */
const notifications = [
  http.get(`${apiBase}/notifications`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items } satisfies Schemas["NotificationPage"])),
  ),
  http.get(`${apiBase}/notifications/marker`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({ last_read_id: items[0]?.id ?? "", unread_count: 0 } satisfies Schemas["NotificationMarker"]),
    ),
  ),
];

/** GET /accounts/relationships with Bob's request waiting, or not. */
function relationships(waiting: boolean) {
  return http.get(`${apiBase}/accounts/relationships`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        items: new URL(request.url).searchParams
          .getAll("id")
          .map((id) => relationshipFixture(id, { requested_by: waiting })),
      } satisfies Schemas["RelationshipList"]),
    ),
  );
}

/** POST /follow_requests/{id}/accept failing with `status`. */
function acceptFailing(status: number, type: string, title: string) {
  return http.post(`${apiBase}/follow_requests/:id/accept`, ({ request }) =>
    withViewer(request, () => problem(status, type, title)),
  );
}

/** The row of Bob's request, once its answers are there. */
async function requestRow({ canvasElement }: ScreenPlayContext) {
  const canvas = within(canvasElement);
  const row = await canvas.findByRole("article", { name: /Bob requested to follow you/ }, { timeout: 10_000 });
  await within(row).findByRole("button", { name: /^Approve/ }, { timeout: 10_000 });
  return row;
}

/** Presses the answer named `name` of Bob's row, and waits for `shown` in the row. */
function pressing(name: RegExp, shown: string): NonNullable<Story["play"]> {
  return async (context) => {
    await settled(context);
    const row = await requestRow(context);
    await context.userEvent.click(within(row).getByRole("button", { name }));
    await expect(await within(row).findByText(shown, {}, { timeout: 10_000 })).toBeInTheDocument();
  };
}

/** The handlers of every story: a locked viewer with Bob's request waiting. */
const handlers = [
  ...pictureHandlers,
  ...notifications,
  relationships(true),
  viewerWith({ locked: true, follow_requests_count: 1 }),
];

/** A story whose server answers with `changes` before the handlers of every story (a story's handlers replace the meta's). */
function changing(...changes: RequestHandler[]) {
  return answering(...changes, ...handlers);
}

const meta = {
  title: "Screens/Notifications/FollowRequest",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/notifications?filter=follows", signedIn: true, handlers } },
  play: async (context) => {
    await settled(context);
    await requestRow(context);
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The request waits: Approve and Decline in the row. */
export const Waiting: Story = {};

/** An answer on its way: the pressed button is pending and the other is disabled. */
export const Answering: Story = {
  ...changing(
    http.post(`${apiBase}/follow_requests/:id/accept`, async () => {
      await delay("infinite");
      return new HttpResponse(null, { status: 204 });
    }),
  ),
  play: async (context) => {
    await settled(context);
    const row = await requestRow(context);
    await context.userEvent.click(within(row).getByRole("button", { name: /^Approve/ }));
    await expect(within(row).getByRole("button", { name: /^Decline/ })).toBeDisabled();
  },
};

/** Approved: the row says so in place of the answers. */
export const Approved: Story = { play: pressing(/^Approve/, "Approved") };

/** Declined: the row says so in place of the answers. */
export const Declined: Story = { play: pressing(/^Decline/, "Declined") };

/** The request no longer waits (answered on another device, or withdrawn): no answers. */
export const NoLongerWaiting: Story = {
  ...changing(relationships(false)),
  play: async (context) => {
    await settled(context);
    const row = await within(context.canvasElement).findByRole("article", { name: /Bob requested to follow you/ });
    await expect(within(row).queryByRole("button")).not.toBeInTheDocument();
  },
};

/** Answered elsewhere: the server has no request any more (a 404), which the row shows as that, not as an error. */
export const AnsweredElsewhere: Story = {
  ...changing(acceptFailing(404, "/api/v1/problems/not-found", "Not Found")),
  play: pressing(/^Approve/, "This request was already answered or withdrawn."),
};

/** The answer failed: a toast says why, and the answers stay. */
export const Failed: Story = {
  ...changing(acceptFailing(503, "about:blank", "Service Unavailable")),
  play: async (context) => {
    await settled(context);
    const row = await requestRow(context);
    await context.userEvent.click(within(row).getByRole("button", { name: /^Approve/ }));
    await within(context.canvasElement.ownerDocument.body).findAllByText(
      "Could not answer the follow request: Service Unavailable",
      {},
      { timeout: 10_000 },
    );
    await expect(within(row).getByRole("button", { name: /^Decline/ })).toBeEnabled();
  },
};

/** In Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  play: async (context) => {
    await settled(context);
    const row = await within(context.canvasElement).findByRole("article", { name: /フォローリクエスト/ });
    await within(row).findByRole("button", { name: /^承認/ }, { timeout: 10_000 });
  },
};

/** On a narrow screen. */
export const Narrow: Story = narrow;
