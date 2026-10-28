// SPDX-License-Identifier: AGPL-3.0-only
// The service closure (/admin/closure; web-client/screens/admin-closure.md) in the states of ADR-0112 §1. The default
// handlers have no closure: the stories answer GET /admin/closure and its check themselves.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

type Closure = Schemas["AdminServiceClosure"];

const day = 86_400_000;

/** The check of a server that has users, deliveries, supporters and a migration still going. */
const check: Schemas["AdminServiceClosureCheck"] = {
  local_accounts: 1284,
  delivery_inboxes: 5321,
  estimated_deletes: 412_880,
  live_supporter_subscriptions: 17,
  pending_migrations: 2,
  retention_holds: 1,
};

/** GET /admin/closure answering `closure`, and its check. */
function closureHandlers(closure: Closure) {
  return [
    http.get(`${apiBase}/admin/closure`, ({ request }) => withViewer(request, () => HttpResponse.json(closure))),
    http.get(`${apiBase}/admin/closure/check`, ({ request }) => withViewer(request, () => HttpResponse.json(check))),
  ];
}

/** A closure announced two months ago that closes `closesIn` days from now. */
function announced(closesIn: number, short = false): Closure {
  return {
    state: "announced",
    closes_at: new Date(Date.now() + closesIn * day).toISOString(),
    export_until: new Date(Date.now() + (closesIn + 30) * day).toISOString(),
    message:
      "Thank you for the years we spent here together. Running the server costs more than the donations cover, so it closes at the end of the year. Export your posts and followers before then; you can move your account to another server at any time until the closing.",
    announced_at: new Date(Date.now() - 60 * day).toISOString(),
    short_notice: short,
  };
}

const meta = {
  title: "Screens/Admin/Closure",
  ...adminScreen("/admin/closure", closureHandlers(announced(-1))),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** An announced closure whose date has come: the check of what the closing does, and the button that closes. */
export const Normal: Story = {};

/** An announcement two months ahead: the closing waits for its date. */
export const Announced: Story = adminAnswering(...closureHandlers(announced(60)));

/** An announcement with less than 30 days of notice, which the screen warns of. */
export const ShortNotice: Story = adminAnswering(...closureHandlers(announced(10, true)));

/** A server whose closure is not announced: the form to announce it. */
export const Empty: Story = adminAnswering(...closureHandlers({ state: "none", short_notice: false }));

/** A closed server, whose deliveries of the Deletes are under way. */
export const Closed: Story = {
  ...adminAnswering(
    ...closureHandlers({
      state: "closed",
      closes_at: "2026-10-01T00:00:00Z",
      export_until: "2026-11-01T00:00:00Z",
      short_notice: false,
      progress: {
        accounts: 1284,
        fanouts_pending: 3,
        deliveries: { pending: 90_120, succeeded: 312_004, permanent_failure: 2_210, skipped: 5_400, dead: 37 },
      },
    }),
  ),
  // The progress bar of the deliveries is no spinner, which `settled` would wait to go.
  play: async ({ canvas }) => {
    const meter = await canvas.findByRole("progressbar", { name: "Deliveries ended" }, { timeout: 10_000 });
    await expect(meter).toHaveAttribute("aria-valuetext", "319,651 of 409,771 deliveries");
  },
};

/** A moderator without manage_service_closure. */
export const MissingPermission: Story = withoutPermission;

/** The closure has not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/closure");

/** The server failed to answer the closure. */
export const ServerError: Story = adminFailingOn("get", "/admin/closure");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
