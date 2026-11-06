// SPDX-License-Identifier: AGPL-3.0-only
// The first page of the supporters screen, the program (/admin/supporters; web-client/screens/admin-supporters.md), in
// the states of ADR-0079 §13. The plans' and the subscriptions' pages are in supporterPages.stories.tsx.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { paymentStatusFixture, supporterProgramFixture } from "./test/handlers";
import { enabled, live, supporterHandlers } from "./test/supporterStories";
import {
  adminAnswering,
  adminFailingOn,
  adminLoadingOn,
  administrator,
  adminScreen,
  withoutPermission,
} from "./test/stories";

const meta = {
  title: "Screens/Admin/Supporters",
  ...adminScreen("/admin/supporters", supporterHandlers(enabled, live)),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** An enabled program in live mode, its form with one save, and the payment provider's state. */
export const Normal: Story = {};

/** Warnings: no commerce disclosure, no webhook delivery, and a reconciliation that the provider refused. */
export const Warnings: Story = adminAnswering(
  ...supporterHandlers(
    supporterProgramFixture({
      enabled: true,
      provider: "stripe",
      live_subscriptions: 47,
      commerce_disclosure: false,
      warnings: ["no_commerce_disclosure"],
    }),
    (() => {
      const { last_webhook_at: _webhook, ...status } = paymentStatusFixture({
        mode: "live",
        last_reconciliation: {
          outcome: "failed",
          error_kind: "permission",
          requests: 3,
          checked_subscriptions: 12,
          corrected_subscriptions: 2,
          occurred_at: "2026-10-22T00:00:00Z",
        },
        warnings: ["no_webhook", "reconciliation_corrected"],
      });
      return status;
    })(),
  ),
);

/** A disabled program on a server without a payment provider: the checkbox stays off and says why. */
export const Disabled: Story = {
  parameters: {
    screen: {
      handlers: [
        administrator,
        http.get(`${apiBase}/admin/supporter_program`, ({ request }) => {
          const { configured_provider: _provider, ...program } = supporterProgramFixture({
            can_enable: false,
            warnings: ["no_provider"],
          });
          return withViewer(request, () => HttpResponse.json(program));
        }),
      ],
    },
  },
};

/** A disabled program (the default handlers). */
export const Empty: Story = adminAnswering();

/** A moderator without manage_supporters. */
export const MissingPermission: Story = withoutPermission;

/** The payment provider's state has not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/payment_status");

/** The server failed to answer the program. */
export const ServerError: Story = adminFailingOn("get", "/admin/supporter_program");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
