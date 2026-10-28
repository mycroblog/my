// SPDX-License-Identifier: AGPL-3.0-only
// The app shell of people who are not signed in (web-client/09-app-shell.md §9, "signed-out navigation"): the top bar
// with Local, Federated, Search, About, Sign in, and Sign up while the server takes new accounts, in the states of
// ADR-0112 §1. The page under it is the search page, which needs no data before a query.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import { instanceFixture } from "../../test/fixtures";
import { apiBase } from "../../test/msw/http";
import { answering, inLocale, narrow, renderScreen, screenLoader, settled } from "../../test/stories";

/** GET /instance answering a server that takes no new accounts. */
const registrationsClosed = http.get(`${apiBase}/instance`, () =>
  HttpResponse.json({ ...instanceFixture, registrations: { mode: "closed" } }),
);

const meta = {
  title: "Screens/Session/Shell",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/search" } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Nobody is signed in, on a server that takes members by approval: the top bar offers to sign up. */
export const SignedOut: Story = {
  play: async (context) => {
    await settled(context);
    await expect(await context.canvas.findByRole("link", { name: "Sign up" })).toBeInTheDocument();
  },
};

/** A server that takes no new accounts: signing in is the top bar's call to action, and nothing offers to sign up. */
export const RegistrationsClosed: Story = {
  ...answering(registrationsClosed),
  play: async (context) => {
    await settled(context);
    await expect(await context.canvas.findByRole("link", { name: "Sign in" })).toBeInTheDocument();
    await expect(context.canvas.queryByRole("link", { name: "Sign up" })).not.toBeInTheDocument();
  },
};

/** At 320 CSS px, with the bar's menu open: the items that do not fit the bar. */
export const NarrowMenuOpen: Story = {
  ...narrow,
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(await canvas.findByRole("button", { name: "Menu" }));
    await expect(await canvas.findByRole("link", { name: "Sign in" })).toBeVisible();
  },
};

/** An address that no page has, signed out: the shell stays around the not-found page. */
export const UnknownAddress: Story = {
  parameters: { screen: { path: "/no-such-page", errorPage: "request" } },
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByText("There is no page at this address.")).toBeInTheDocument();
  },
};

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the top bar runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
