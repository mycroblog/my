// SPDX-License-Identifier: AGPL-3.0-only
// The error groups (/admin/errors; web-client/screens/admin-errors.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { errorGroupFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** Error groups of both sources and every state: panics and errors of the server, and errors of the web client. */
const errorGroups: Schemas["ErrorGroup"][] = [
  errorGroupFixture("0199f0e0-0000-7000-8000-0000000000e1", { count_24h: 128, count_7d: 904 }),
  errorGroupFixture("0199f0e0-0000-7000-8000-0000000000e2", {
    source: "web",
    kind: "error",
    type: "TypeError",
    where: "/@{$acct}",
    message: "Cannot read properties of undefined (reading 'display_name')",
    count_24h: 1,
    count_7d: 1,
    daily_counts: [0, 0, 0, 0, 0, 0, 1],
  }),
  errorGroupFixture("0199f0e0-0000-7000-8000-0000000000e3", {
    kind: "error",
    where: "job federation.deliver",
    type: "*url.Error",
    message:
      'Post "https://an-instance-with-a-really-long-domain-name.social.example/users/someone/inbox": context deadline exceeded (Client.Timeout exceeded while awaiting headers)',
    count_24h: 0,
    count_7d: 37,
    daily_counts: [12, 8, 9, 5, 3, 0, 0],
    state: "resolved",
    resolved_version: "0.3.1",
  }),
  errorGroupFixture("0199f0e0-0000-7000-8000-0000000000e4", {
    source: "web",
    kind: "error",
    type: "ChunkLoadError",
    where: "/settings/profile",
    message: "Loading chunk 42 failed.",
    count_24h: 4,
    count_7d: 19,
    daily_counts: [2, 3, 4, 2, 2, 2, 4],
    state: "muted",
  }),
];

/** The groups that the query's state and source filters keep. */
function groupsOf(url: URL): Schemas["ErrorGroup"][] {
  const state = url.searchParams.get("state") ?? "open";
  const source = url.searchParams.get("source");
  return errorGroups.filter(
    (group) => (state === "all" || group.state === state) && (source === null || group.source === source),
  );
}

const groups = http.get(`${apiBase}/admin/error_groups`, ({ request }) =>
  withViewer(request, () => HttpResponse.json({ items: groupsOf(new URL(request.url)) })),
);

const meta = { title: "Screens/Admin/Errors", ...adminScreen("/admin/errors", [groups]) } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The open groups of both sources. */
export const Normal: Story = {};

/** Every group, whatever its state. */
export const AllStates: Story = { parameters: { screen: { path: "/admin/errors?state=all" } } };

/** No open group (the default handlers). */
export const Empty: Story = adminAnswering();

/** A moderator without manage_errors. */
export const MissingPermission: Story = withoutPermission;

/** The groups have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/error_groups");

/** The server failed to answer the groups. */
export const ServerError: Story = adminFailingOn("get", "/admin/error_groups");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the table scrolls on its own. */
export const Narrow: Story = narrow;
