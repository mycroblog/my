// SPDX-License-Identifier: AGPL-3.0-only
// One webhook and its deliveries (/admin/webhooks/$id; web-client/screens/admin-webhooks.md) in the states of
// ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { webhookDeliveryFixture, webhookFixture } from "./test/handlers";
import {
  adminAnswering,
  adminAnsweringWithErrorPage,
  adminFailingOn,
  adminLoadingOn,
  adminScreen,
  withoutPermission,
} from "./test/stories";

const webhookId = "0199f0e0-0000-7000-8000-0000000000b1";

const webhook = webhookFixture(webhookId, {
  description: "Moderators' chat room",
  events: ["report.created", "report.resolved", "appeal.created"],
});

/** Deliveries of every status and attempt outcome, newest first. */
const deliveries: Schemas["AdminWebhookDelivery"][] = [
  webhookDeliveryFixture("9", { event_type: "report.created", created_at: "2026-10-21T09:00:00Z" }),
  webhookDeliveryFixture("8", {
    event_type: "appeal.created",
    status: "failing",
    attempts: 3,
    created_at: "2026-10-21T08:00:00Z",
    last_attempt: { outcome: "http_error", status_code: 503, duration_ms: 120, attempted_at: "2026-10-21T08:30:00Z" },
  }),
  webhookDeliveryFixture("7", {
    event_type: "report.resolved",
    status: "failing",
    attempts: 5,
    created_at: "2026-10-21T07:00:00Z",
    last_attempt: { outcome: "timeout", duration_ms: 15000, attempted_at: "2026-10-21T07:45:00Z" },
  }),
  webhookDeliveryFixture("6", {
    event_type: "report.created",
    status: "failing",
    attempts: 1,
    created_at: "2026-10-21T06:00:00Z",
    last_attempt: { outcome: "connection_error", duration_ms: 3, attempted_at: "2026-10-21T06:00:01Z" },
  }),
  ...Array.from({ length: 5 }, (_, index) =>
    webhookDeliveryFixture(String(5 - index), {
      event_type: index === 4 ? "webhook.test" : "report.created",
      status: "succeeded",
      attempts: 1,
      created_at: `2026-10-20T0${String(5 - index)}:00:00Z`,
      last_attempt: {
        outcome: "succeeded",
        status_code: 204,
        duration_ms: 80 + index * 13,
        attempted_at: `2026-10-20T0${String(5 - index)}:00:01Z`,
      },
    }),
  ),
];

function answering(hook: Schemas["AdminWebhook"], items: Schemas["AdminWebhookDelivery"][]) {
  return [
    http.get(`${apiBase}/admin/webhooks/:id`, ({ request }) => withViewer(request, () => HttpResponse.json(hook))),
    http.get(`${apiBase}/admin/webhooks/:id/deliveries`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ items })),
    ),
  ];
}

const meta = {
  title: "Screens/Admin/Webhook",
  ...adminScreen(`/admin/webhooks/${webhookId}`, answering(webhook, deliveries)),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A webhook with deliveries that succeeded, one on its way, and failures of each kind. */
export const Normal: Story = {};

/** A webhook that the server disabled after its deliveries kept failing. */
export const DisabledForFailing: Story = adminAnswering(
  ...answering(
    {
      ...webhook,
      state: "disabled",
      state_reason: "failing",
      failure: { consecutive_failures: 20, first_failed_at: "2026-10-17T00:00:00Z" },
    },
    deliveries.slice(1, 4),
  ),
);

/** A new webhook that has not delivered anything yet. */
export const Empty: Story = adminAnswering(...answering(webhook, []));

/** A webhook that no longer exists. */
export const NotFound: Story = adminAnsweringWithErrorPage(
  http.get(`${apiBase}/admin/webhooks/:id`, () => problem(404, "about:blank", "Not Found")),
);

/** A moderator without manage_webhooks. */
export const MissingPermission: Story = withoutPermission;

/** The deliveries have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/webhooks/:id/deliveries", ...answering(webhook, []));

/** The server failed to answer the webhook. */
export const ServerError: Story = adminFailingOn("get", "/admin/webhooks/:id");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
