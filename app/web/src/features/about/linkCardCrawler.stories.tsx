// SPDX-License-Identifier: AGPL-3.0-only
// The page about the link card crawler (/about/crawler; web-client/screens/about-crawler.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { instanceFixture, type Schemas } from "../../test/fixtures";
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
import { describedInstance } from "./test/storyFixtures";

/** GET /instance answering a server with these settings. */
function serving(settings: Partial<Schemas["Instance"]>) {
  return http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...describedInstance, ...settings }));
}

const meta = {
  title: "Screens/About/LinkCardCrawler",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/about/crawler", handlers: [serving({})] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A server that obeys robots.txt and publishes a contact, seen signed out. */
export const Normal: Story = {};

/** Seen by a signed-in user, with the column about the server on wide screens. */
export const SignedIn: Story = { parameters: { screen: { signedIn: true } } };

/** The operators turned off robots.txt: the page says that a website cannot refuse with it here. */
export const RobotsTxtIgnored: Story = answering(serving({ link_cards_ignore_robots_txt: true }));

/** The operators turned link cards off: the crawler fetches nothing. */
export const LinkCardsOff: Story = answering(serving({ link_cards_enabled: false }));

/** A server that publishes no contact. */
export const Empty: Story = answering(http.get(`${apiBase}/instance`, () => HttpResponse.json(instanceFixture)));

/** The settings of the server have not arrived yet. */
export const Loading: Story = loadingOn("get", "/instance");

/** The settings of the server cannot be read: the page claims nothing, and offers to try again. */
export const ServerError: Story = failingOn("get", "/instance");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB; the samples stay left to right. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
