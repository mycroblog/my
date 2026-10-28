// SPDX-License-Identifier: AGPL-3.0-only
// The blocked accounts (/settings/blocks) in the states of ADR-0112 §1.
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
} from "../../test/stories";
import { listedAccounts } from "./test/storyFixtures";

/** GET /blocks answering `accounts`, with a next page when `more`. */
function blocking(accounts: Schemas["Account"][], more = false) {
  return http.get(`${apiBase}/blocks`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(more ? { accounts, next_cursor: "next" } : { accounts })),
  );
}

const meta = {
  title: "Screens/Settings/Blocks",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/blocks", signedIn: true, handlers: [blocking(listedAccounts, true)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Ten blocked accounts with long, right-to-left and plain names, and a next page. */
export const Normal: Story = {};

/** Nobody is blocked. */
export const Empty: Story = answering(blocking([]));

/** The list has not arrived yet. */
export const Loading: Story = loadingOn("get", "/blocks");

/** The server failed to answer the list. */
export const ServerError: Story = failingOn("get", "/blocks");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
