// SPDX-License-Identifier: AGPL-3.0-only
// The page about the server (/about; web-client/screens/about.md) in the states of ADR-0112 §1.
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
import { publicDomainActionsFixture } from "./test/handlers";
import { describedInstance } from "./test/storyFixtures";

/** The moderated domains: a few of each kind, one with a long name and comment. */
const manyDomainActions: Schemas["InstanceDomainAction"][] = [
  ...publicDomainActionsFixture,
  {
    domain: "the-longest-spam-server-name-in-the-fediverse.social.example.com",
    digest: "3b8e0c9d1a6d5e2f4a1c0d9e8f7a615233445566778899001122aabbccddeeff",
    kind: "suspend",
    comment: "Thousands of automated accounts posting links to the same shop, every few seconds, for weeks.",
  },
  {
    domain: "noisy.example",
    digest: "4c9f1d0e2b7e6f3a5b2d1e0f9a8b726344556677889900112233aabbccddeeff",
    kind: "limit",
    comment: "Mostly fine, often loud.",
  },
];

const described = [
  http.get(`${apiBase}/instance`, () => HttpResponse.json(describedInstance)),
  http.get(`${apiBase}/instance/domain_actions`, () => HttpResponse.json({ items: manyDomainActions })),
];

const meta = {
  title: "Screens/About/About",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/about", handlers: described } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A server that describes itself fully, seen signed out. */
export const Normal: Story = {};

/** Seen by a signed-in user, with the column about the server on wide screens. */
export const SignedIn: Story = { parameters: { screen: { signedIn: true } } };

/** A server that says nothing about itself and moderates no domain. */
export const Empty: Story = answering(
  http.get(`${apiBase}/instance/domain_actions`, () => HttpResponse.json({ items: [] })),
);

/** The moderated domains have not arrived yet. */
export const Loading: Story = loadingOn("get", "/instance/domain_actions");

/** The server failed to answer the moderated domains, which say so in their section. */
export const ServerError: Story = failingOn("get", "/instance/domain_actions");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
