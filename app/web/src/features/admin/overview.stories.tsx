// SPDX-License-Identifier: AGPL-3.0-only
// The server administration overview (/admin; web-client/screens/admin.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow, viewerWith } from "../../test/stories";
import { errorGroupFixture } from "./test/handlers";
import { adminFailingOn, adminLoadingOn, administrator, adminScreen, withoutPermission } from "./test/stories";

/** A queue whose oldest due job has waited for hours, with failed deliveries, and more open error groups than a page. */
const troubled = [
  http.get(`${apiBase}/admin/queue`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        adapter: "db",
        pending_listing: true,
        counts: [
          { state: "queued", type: "federation.deliver", count: 1284 },
          { state: "running", type: "media.process", count: 3 },
          { state: "dead", type: "federation.deliver", count: 41 },
          { state: "dead", type: "kv.cleanup", count: 2 },
        ],
        oldest_ready_at: new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString(),
      } satisfies Schemas["AdminQueue"]),
    ),
  ),
  http.get(`${apiBase}/admin/error_groups`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        items: Array.from({ length: 20 }, (_, index) =>
          errorGroupFixture(`0199f0e0-0000-7000-8000-0000000001${String(index).padStart(2, "0")}`),
        ),
        next_cursor: "more",
      }),
    ),
  ),
];

const meta = { title: "Screens/Admin/Overview", ...adminScreen("/admin", troubled) } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** An administrator's overview: failed jobs, a long wait, and more open error groups than a page. */
export const Normal: Story = {};

/** A server that runs well: no failed jobs, no wait, and no open errors (the default handlers). */
export const Healthy: Story = { parameters: { screen: { handlers: [administrator] } } };

/** A viewer whose roles watch nothing of the server's health, but who may change its settings and rules. */
export const NothingToWatch: Story = {
  parameters: { screen: { handlers: [viewerWith({ permissions: ["manage_settings", "manage_rules"] })] } },
};

/** A moderator, whose roles allow no administration screen. */
export const NoAdministration: Story = withoutPermission;

/** The queue has not answered yet: its tiles say they are loading. */
export const Loading: Story = adminLoadingOn("get", "/admin/queue");

/** The server failed to answer the error groups: their tile says so. */
export const ServerError: Story = adminFailingOn("get", "/admin/error_groups");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
