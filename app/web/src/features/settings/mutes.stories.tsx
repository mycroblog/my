// SPDX-License-Identifier: AGPL-3.0-only
// The muted accounts (/settings/mutes; web-client/screens/mutes.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { answering, failingOn, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { listedAccounts } from "./test/storyFixtures";

/** GET /mutes answering `mutes`, with a next page when `more`. */
function muting(mutes: Schemas["MutedAccount"][], more = false) {
  return http.get(`${apiBase}/mutes`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(more ? { mutes, next_cursor: "next" } : { mutes })),
  );
}

/** Ten muted accounts: some with their notifications, some until a date. */
const tenMutes: Schemas["MutedAccount"][] = listedAccounts.map((account, i) => ({
  account,
  notifications: i % 2 === 0,
  ...(i % 3 === 0 ? { expires_at: `2026-11-${String(i + 1).padStart(2, "0")}T00:00:00Z` } : {}),
}));

const meta = {
  title: "Screens/Settings/Mutes",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/mutes", signedIn: true, handlers: [muting(tenMutes, true)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Ten muted accounts, some with their notifications and some until a date, and a next page. */
export const Normal: Story = {};

/** Nobody is muted. */
export const Empty: Story = answering(muting([]));

/** The list has not arrived yet. */
export const Loading: Story = loadingOn("get", "/mutes");

/** The server failed to answer the list. */
export const ServerError: Story = failingOn("get", "/mutes");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
