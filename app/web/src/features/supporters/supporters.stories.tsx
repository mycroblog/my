// SPDX-License-Identifier: AGPL-3.0-only
// The public supporter list (/about/supporters; web-client/screens/supporters.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http } from "msw";
import { apiBase, problem } from "../../test/msw/http";
import { answering, failingOn, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { listing, members, programOn } from "./test/storyFixtures";

const listed = [programOn("public"), listing(members)];

const meta = {
  title: "Screens/About/Supporters",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/about/supporters", handlers: listed } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Eight members on three plans, with long and right-to-left names. */
export const Normal: Story = {};

/** A published list that nobody is on yet. */
export const Empty: Story = answering(programOn("public"), listing([]));

/** A server that does not publish the list. */
export const NotPublished: Story = answering(
  http.get(`${apiBase}/instance/supporters`, () => problem(404, "about:blank", "Not Found")),
);

/** A list for signed-in users only, seen signed out. */
export const SignInRequired: Story = answering(
  programOn("signed_in"),
  http.get(`${apiBase}/instance/supporters`, () => problem(401, "/api/v1/problems/unauthenticated", "Unauthenticated")),
);

/** The list has not arrived yet. */
export const Loading: Story = loadingOn("get", "/instance/supporters");

/** The server failed to answer the list. */
export const ServerError: Story = failingOn("get", "/instance/supporters");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
