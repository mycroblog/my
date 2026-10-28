// SPDX-License-Identifier: AGPL-3.0-only
// The follow requests of a locked account (/settings/follow-requests) in the
// states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
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
  viewerWith,
} from "../../test/stories";
import { listedAccounts } from "./test/storyFixtures";

/** GET /follow_requests answering `accounts`, with a next page when `more`. */
function requesting(accounts: Schemas["Account"][], more = false) {
  return http.get(`${apiBase}/follow_requests`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(more ? { accounts, next_cursor: "next" } : { accounts })),
  );
}

/** A locked account with ten requests. */
const locked = viewerWith({ locked: true, follow_requests_count: 10 });

const meta = {
  title: "Screens/Settings/FollowRequests",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: { path: "/settings/follow-requests", signedIn: true, handlers: [locked, requesting(listedAccounts, true)] },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Ten requests with long, right-to-left and plain names, and a next page. */
export const Normal: Story = {};

/** No one asked to follow. */
export const Empty: Story = answering(viewerWith({ locked: true }), requesting([]));

/** The requests have not arrived yet. */
export const Loading: Story = loadingOn("get", "/follow_requests");

/** The server failed to answer the requests. */
export const ServerError: Story = failingOn("get", "/follow_requests");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
