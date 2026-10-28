// SPDX-License-Identifier: AGPL-3.0-only
// The webhooks (/admin/webhooks; web-client/screens/admin-webhooks.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { webhookFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** Webhooks in every state: enabled, disabled by an administrator, gone, and disabled for failing. */
const webhooks: Schemas["AdminWebhook"][] = [
  webhookFixture("0199f0e0-0000-7000-8000-0000000000b1", {
    description: "Moderators' chat room",
    events: ["report.created", "report.resolved", "report.reopened", "appeal.created", "registration.pending"],
  }),
  webhookFixture("0199f0e0-0000-7000-8000-0000000000b2", {
    url: "https://automation.a-very-long-subdomain-of-an-integration-service.example/hooks/0123456789abcdef0123456789abcdef",
    description:
      "The automation that copies every new account and every moderation action into the operators' spreadsheet",
    events: ["account.created", "account.updated", "moderation_action.created", "moderation_action.lifted"],
  }),
  webhookFixture("0199f0e0-0000-7000-8000-0000000000b3", {
    url: "https://errors.example/in",
    description: "",
    events: ["error_group.created", "error_group.regressed"],
    state: "disabled",
    state_reason: "admin",
  }),
  webhookFixture("0199f0e0-0000-7000-8000-0000000000b4", {
    url: "https://old-bot.example/webhook",
    description: "Emoji bot",
    events: ["custom_emoji.created", "custom_emoji.updated", "custom_emoji.deleted"],
    state: "disabled",
    state_reason: "failing",
    failure: { consecutive_failures: 20, first_failed_at: "2026-10-17T00:00:00Z" },
  }),
  webhookFixture("0199f0e0-0000-7000-8000-0000000000b5", {
    url: "https://gone.example/hook",
    description: "Removed integration",
    state: "disabled",
    state_reason: "gone",
  }),
];

const list = http.get(`${apiBase}/admin/webhooks`, ({ request }) =>
  withViewer(request, () => HttpResponse.json({ items: webhooks })),
);

const meta = { title: "Screens/Admin/Webhooks", ...adminScreen("/admin/webhooks", [list]) } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Five webhooks: two enabled, one disabled by hand, one disabled for failing, and one whose receiver is gone. */
export const Normal: Story = {};

/** No webhook yet (the default handlers). */
export const Empty: Story = adminAnswering();

/** A moderator without manage_webhooks. */
export const MissingPermission: Story = withoutPermission;

/** The webhooks have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/webhooks");

/** The server failed to answer the webhooks. */
export const ServerError: Story = adminFailingOn("get", "/admin/webhooks");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the long URLs wrap. */
export const Narrow: Story = narrow;
