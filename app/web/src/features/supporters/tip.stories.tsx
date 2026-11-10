// SPDX-License-Identifier: AGPL-3.0-only
// The user's page of the tips (/settings/supporter/tip, ADR-0162 §17, §19) in its states: the amounts with the final
// confirmation, a user who must verify their address first, the wait for the provider's confirmation and the thanks
// after the checkout, a checkout that was left, and a server that takes no tips.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { http, HttpResponse } from "msw";
import { apiBase, problem, withViewer } from "../../test/msw/http";
import { answering, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { ownTipFixture, tipId } from "./test/handlers";
import { offeringTips, tipsOn } from "./test/storyFixtures";

/** GET /supporter/tips/{id} answering the tip in `state`. */
function returnedTip(state: "open" | "paid" | "expired") {
  return http.get(`${apiBase}/supporter/tips/:id`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json(ownTipFixture({ state, ...(state === "paid" && { paid_at: "2026-10-22T00:01:00Z" }) })),
    ),
  );
}

const tips = [tipsOn, offeringTips];

const meta = {
  title: "Screens/Settings/Tip",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/supporter/tip", signedIn: true, handlers: tips } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Three amounts to choose from. */
export const Normal: Story = {};

/** The final confirmation before the provider's checkout. */
export const Confirmation: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(canvas.getByRole("button", { name: "Leave a tip" }));
    await within(document.body).findByRole("alertdialog");
  },
};

/** A user who must verify their email address first, who reads how instead of the form. */
export const Ineligible: Story = answering(
  http.get(`${apiBase}/supporter/subscription`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ ineligible_reason: "email_unverified" })),
  ),
  ...tips,
);

/** Back from the checkout, while the provider has not confirmed the payment. */
export const Confirming: Story = {
  parameters: {
    screen: { path: `/settings/supporter/tip?tip=${tipId}&checkout=success`, handlers: [returnedTip("open"), ...tips] },
  },
  // The page waits with a spinner, which `settled` would wait out.
  play: async ({ canvas }) => {
    await canvas.findByText(/Confirming your payment with the payment provider/u, undefined, { timeout: 10_000 });
  },
};

/** Back from the checkout of a paid tip: only the thanks. */
export const Thanks: Story = {
  parameters: {
    screen: { path: `/settings/supporter/tip?tip=${tipId}&checkout=success`, handlers: [returnedTip("paid"), ...tips] },
  },
};

/** Back from a checkout that the user left: nothing was charged, and the amounts are offered again. */
export const Canceled: Story = {
  parameters: {
    screen: {
      path: `/settings/supporter/tip?tip=${tipId}&checkout=canceled`,
      handlers: [returnedTip("expired"), ...tips],
    },
  },
};

/** A server that takes no tips. */
export const Empty: Story = answering(
  http.get(`${apiBase}/instance/tip_amounts`, () => problem(404, "about:blank", "Not Found")),
);

/** The amounts have not arrived yet. */
export const Loading: Story = (() => {
  const loading = loadingOn("get", "/instance/tip_amounts");
  return { ...loading, ...answering(...loading.parameters.screen.handlers, tipsOn) };
})();

/** The server failed to answer the amounts: the page says so in its body, with a retry. */
export const ServerError: Story = answering(
  http.get(`${apiBase}/instance/tip_amounts`, () => problem(500, "about:blank", "Internal Server Error")),
  tipsOn,
);

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
