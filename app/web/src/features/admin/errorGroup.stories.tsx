// SPDX-License-Identifier: AGPL-3.0-only
// One error group (/admin/errors/$id; web-client/screens/admin-errors.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { errorGroupDetailFixture } from "./test/handlers";
import {
  adminAnswering,
  adminAnsweringWithErrorPage,
  adminFailingOn,
  adminLoadingOn,
  adminScreen,
  withoutPermission,
} from "./test/stories";

const groupId = "0199f0e0-0000-7000-8000-0000000000e1";

/** Two days of hourly counts up to 2026-10-21T05:00Z, a spike on a new version, and none in the quiet hours. */
const hourlyCounts: Schemas["ErrorGroupHourCount"][] = Array.from({ length: 48 }, (_, index) => {
  const hour = new Date(Date.UTC(2026, 9, 19, 6 + index)).toISOString().replace(".000Z", "Z");
  const count = index > 30 ? (index % 5) * 7 + 3 : index % 6 === 0 ? 1 : 0;
  return { hour, count, last_version: index > 30 ? "0.3.1" : "0.3.0" };
}).filter((hour) => hour.count > 0);

/** A panic that was resolved, happened again on the next version, and was muted and unmuted. */
const regressed = errorGroupDetailFixture(groupId, {
  count_24h: 128,
  count_7d: 904,
  daily_counts: [0, 9, 0, 40, 120, 600, 128],
  hourly_counts: hourlyCounts,
  sample: {
    type: "runtime.Error",
    message: "runtime error: invalid memory address or nil pointer dereference",
    request_id: "01JB0000000000000000000000",
    trace_id: "4bf92f3577b34da6a3ce929d0e0e4736",
    version: "0.3.1",
    role: "all",
    occurred_at: "2026-10-21T05:30:00Z",
    stack: [
      "goroutine 1 [running]:",
      "github.com/mycroblog/my/internal/app/status.(*Service).Get(0xc000123450, {0x1a2b3c0, 0xc000456780}, {0xc0001a2b30, 0x24})",
      "\tgithub.com/mycroblog/my/internal/app/status/get.go:42 +0x32",
      "github.com/mycroblog/my/internal/interface/clientapi.(*Handler).GetStatus(0xc000789000, {0x1a2b3c0, 0xc000456780}, {0xc0001a2b30, 0x24})",
      "\tgithub.com/mycroblog/my/internal/interface/clientapi/statuses.go:118 +0x1a4",
      "net/http.HandlerFunc.ServeHTTP(0xc000abc000, {0x1a2d4e0, 0xc000def000}, 0xc000123000)",
      "\tnet/http/server.go:2220 +0x29",
    ].join("\n"),
  },
  events: [
    { kind: "resolved", actor: "user", version: "0.3.0", occurred_at: "2026-10-20T03:00:00Z" },
    { kind: "regressed", actor: "system", version: "0.3.1", occurred_at: "2026-10-20T12:00:00Z" },
    { kind: "muted", actor: "cli", occurred_at: "2026-10-20T13:00:00Z" },
    { kind: "unmuted", actor: "user", occurred_at: "2026-10-21T01:00:00Z" },
  ],
});

function answeringGroup(group: Schemas["ErrorGroupDetail"]) {
  return http.get(`${apiBase}/admin/error_groups/:id`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(group)),
  );
}

const meta = {
  title: "Screens/Admin/Error group",
  ...adminScreen(`/admin/errors/${groupId}`, [answeringGroup(regressed)]),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A panic that came back on a new version, with its latest stack, two days of counts and its history. */
export const Normal: Story = {};

/** A new group of the web client: one occurrence, no history (the default handler's group). */
export const Fresh: Story = adminAnswering(
  answeringGroup(
    errorGroupDetailFixture(groupId, {
      source: "web",
      type: "TypeError",
      where: "/@{$acct}",
      message: "Cannot read properties of undefined (reading 'display_name')",
      count_24h: 1,
      count_7d: 1,
      daily_counts: [0, 0, 0, 0, 0, 0, 1],
      hourly_counts: [{ hour: "2026-10-21T05:00:00Z", count: 1, last_version: "0.3.1" }],
    }),
  ),
);

/** A muted group. */
export const Muted: Story = adminAnswering(answeringGroup({ ...regressed, state: "muted" }));

/** A group that no longer exists. */
export const NotFound: Story = adminAnsweringWithErrorPage(
  http.get(`${apiBase}/admin/error_groups/:id`, () => problem(404, "about:blank", "Not Found")),
);

/** A moderator without manage_errors. */
export const MissingPermission: Story = withoutPermission;

/** The group has not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/error_groups/:id");

/** The server failed to answer the group. */
export const ServerError: Story = adminFailingOn("get", "/admin/error_groups/:id");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the stack scrolls on its own. */
export const Narrow: Story = narrow;
