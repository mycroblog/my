// SPDX-License-Identifier: AGPL-3.0-only
// The follow lists of a profile (/@{acct}/followers and /@{acct}/following; web-client/screens/profile-follows.md) in the states
// of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase } from "../../test/msw/http";
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
import { lookingUp, manyAccounts, relatedAs, richBob } from "./test/storyFixtures";

/** Both lists of every account hold `accounts`, with a next page when `more`. */
function listing(accounts: Schemas["Account"][], more = false) {
  const page: Schemas["AccountPage"] = more ? { accounts, next_cursor: "next" } : { accounts };
  return [
    http.get(`${apiBase}/accounts/:id/followers`, () => HttpResponse.json(page)),
    http.get(`${apiBase}/accounts/:id/following`, () => HttpResponse.json(page)),
  ];
}

/** Bob, followed by twelve accounts, some of which the viewer follows. */
const bobsFollowers = [lookingUp(richBob), ...listing(manyAccounts, true)];

const meta = {
  title: "Screens/Profile/FollowLists",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/@bob/followers", signedIn: true, handlers: bobsFollowers } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Bob's followers: long names, right-to-left names, long domains, and a next page. */
export const Normal: Story = {};

/** The accounts Bob follows, each of which the viewer follows too. */
export const Following: Story = {
  parameters: {
    screen: { path: "/@bob/following", handlers: [relatedAs({ following: true }), ...bobsFollowers] },
  },
};

/** Seen signed out: the rows offer no follow buttons. */
export const SignedOut: Story = { parameters: { screen: { signedIn: false } } };

/** An account that follows nobody. */
export const Empty: Story = {
  parameters: { screen: { path: "/@bob/following", handlers: [lookingUp(richBob), ...listing([])] } },
};

/** The first page has not arrived yet: the heading and a skeleton of rows. */
export const Loading: Story = loadingOn("get", "/accounts/:id/followers");

/** The server failed to answer the list. */
export const ServerError: Story = failingOn("get", "/accounts/:id/followers");

/** In Japanese. */
export const Japanese: Story = { ...inLocale("ja"), ...answering(...bobsFollowers) };

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = { ...inLocale("ar-XB"), ...answering(...bobsFollowers) };

/** At 320 CSS px. */
export const Narrow: Story = narrow;
