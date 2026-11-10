// SPDX-License-Identifier: AGPL-3.0-only
// The drafts page (/drafts; web-client/screens/drafts.md) in the states of ADR-0079 §13: scheduled posts and drafts,
// one whose schedule failed, nothing saved, loading, a failure, the schedule popover, Japanese, and narrow.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect, userEvent, within } from "storybook/test";
import { draftFixture, type Schemas } from "../../test/fixtures";
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

const drafts: Schemas["Draft"][] = [
  draftFixture(
    "d1",
    { text: "Good morning, everyone! The garden opens at nine today.", media: [{ id: "m1" }] },
    { scheduled_at: "2026-11-11T00:00:00Z" },
  ),
  draftFixture(
    "d2",
    {
      text: "",
      spoiler_text: "Spoilers for the final episode",
      poll: { options: ["Loved it", "Not for me"], expires_in: 86_400 },
    },
    { scheduled_at: "2026-11-12T12:30:00Z" },
  ),
  draftFixture(
    "d3",
    { text: "Replying to the thread about the bridge: it reopens next week.", in_reply_to_id: "s1" },
    {
      updated_at: "2026-11-09T07:00:00Z",
      last_schedule: {
        publish_at: "2026-11-09T08:00:00Z",
        outcome: "failed",
        reason: "reply_unavailable",
        occurred_at: "2026-11-09T08:00:04Z",
      },
    },
  ),
  draftFixture(
    "d4",
    { text: "Ideas for the next meetup:\n- a walk\n- a picnic" },
    { updated_at: "2026-11-08T21:15:00Z" },
  ),
];

/** GET /drafts answering the scheduled or the other drafts of `items`. */
function listing(items: Schemas["Draft"][]) {
  return http.get(`${apiBase}/drafts`, ({ request }) =>
    withViewer(request, () => {
      const scheduled = new URL(request.url).searchParams.get("scheduled") === "true";
      return HttpResponse.json({ items: items.filter((d) => (d.scheduled_at !== undefined) === scheduled) });
    }),
  );
}

const meta = {
  title: "Screens/Drafts",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/drafts", signedIn: true, handlers: [listing(drafts)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two scheduled posts, and two drafts, one of them after a schedule that failed. */
export const Normal: Story = {};

/** Nothing saved and nothing scheduled. */
export const Empty: Story = answering(listing([]));

/** The lists have not arrived yet. */
export const Loading: Story = loadingOn("get", "/drafts");

/** The server failed to answer the lists. */
export const ServerError: Story = failingOn("get", "/drafts");

/** The popover that moves a schedule, in the user's time zone. */
export const Reschedule: Story = {
  play: async (context) => {
    await settled(context);
    const [button] = await within(context.canvasElement).findAllByRole("button", { name: /^Change time: / });
    if (button === undefined) throw new Error("no Change time button");
    await userEvent.click(button);
    await expect(await within(document.body).findByRole("dialog", { name: "Schedule the post" })).toBeVisible();
  },
};

/** The page in Japanese. */
export const Japanese: Story = inLocale("ja");

/** The page on a phone. */
export const Narrow: Story = { ...narrow };
