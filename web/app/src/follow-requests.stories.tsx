// SPDX-License-Identifier: AGPL-3.0-only

// The follow requests to the reader (FR-302), the screen the row over the notifications opens: ren, a locked account,
// asked by sam (the scenario's) and by marco and mio (`followRequests`), each with Decline and Approve; each state every
// screen is shown in (app.samples.tsx); and the notifications with the row of the requests above what waits for
// approval. The answer in a request's place, right after Approve or Decline, is the row's own state (the design
// system's Follow requests).
// More of the requests than a page holds, before and after Show more (pages.samples.ts, `more`).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";
import { manyFollowRequests } from "./pages.samples";

const at = "/notifications/follow_requests";
const viewer = "ren";
const server = { followRequests: { from: ["marco", "mio"] } };

const meta = {
  title: "Follow requests",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at, viewer, server }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
export const Empty: Story = { parameters: screen({ at, viewer, server, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at, viewer, server, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at, viewer, server, state: "error" }) };
export const LongText: Story = { parameters: screen({ at, viewer, server, state: "long" }) };
/** More requests than a page holds: Show more under the newest twenty. */
export const Many: Story = { parameters: screen({ at, viewer, server: manyFollowRequests }) };
/** The same after Show more: the older requests under the newest twenty, and no Show more. */
export const ManyMore: Story = { parameters: screen({ at, viewer, server: manyFollowRequests, more: true }) };
/** The notifications of ren, with the row of the follow requests over them. */
export const InNotifications: Story = { parameters: screen({ at: "/notifications", viewer, server }) };
