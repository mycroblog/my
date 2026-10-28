// SPDX-License-Identifier: AGPL-3.0-only
// The search (/search; FR-305, FR-307, web-client/screens/search.md) in the states of ADR-0112 §1: the accounts whose
// names match the text, and the handle or URL that the server resolves.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, withViewer } from "../../test/msw/http";
import { abdulrahman, bob, carol, dmitri, haruka, maximiliana, newsBot, pictureHandlers } from "../../test/postStories";
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

/** GET /accounts/search answering `accounts`. */
function nameSearch(accounts: Schemas["Account"][]) {
  return http.get(`${apiBase}/accounts/search`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(accounts)),
  );
}

/** The accounts whose names match "ba": of this server and of others, with long, right-to-left and bot names. */
const matches = [
  { ...bob, note: "<p>I bake bread and write about it.</p>", followers_count: 128, following_count: 64 },
  maximiliana,
  { ...carol, display_name: "Carol the baker" },
  { ...abdulrahman, display_name: "عبد الرحمن · bakery" },
  { ...dmitri, display_name: "Dmitri Bakunin" },
  { ...haruka, display_name: "春香 (bakery club)" },
  { ...newsBot, display_name: "Bakery opening hours" },
];

/** GET /accounts/lookup finding nobody. */
const nobody = http.get(`${apiBase}/accounts/lookup`, () => problem(404, "about:blank", "Not Found"));

const meta = {
  title: "Screens/Search/Accounts",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: { path: "/search?q=ba", signedIn: true, handlers: [...pictureHandlers, nameSearch(matches), nobody] },
  },
  play: async (context) => {
    await settled(context);
    await context.canvas.findByText("Maximiliana Theodora Wolfharte");
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The accounts whose names match the text. */
export const Normal: Story = {};

/** Before anything is typed. */
export const Empty: Story = { parameters: { screen: { path: "/search" } }, play: settled };

/** No account matches the text. */
export const NoMatches: Story = { ...answering(nameSearch([])), play: settled };

/** A handle that no server knows, searched with Enter. */
export const NotFound: Story = {
  parameters: { screen: { path: "/search" } },
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.type(await canvas.findByRole("searchbox"), "@nobody@remote.example{Enter}");
    await expect(await canvas.findByRole("alert")).toBeInTheDocument();
  },
};

/** To someone who is not signed in: only the accounts this server knows, by handle. */
export const SignedOut: Story = { parameters: { screen: { path: "/search", signedIn: false } }, play: settled };

/** The matches have not arrived yet. */
export const Loading: Story = loadingOn("get", "/accounts/search");

/** The server failed to answer the matches. */
export const ServerError: Story = { ...failingOn("get", "/accounts/search"), play: settled };

/** In Japanese. */
export const Japanese: Story = { ...inLocale("ja"), parameters: { screen: { path: "/search?q=%E6%98%A5" } } };

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
