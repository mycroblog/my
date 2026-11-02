// SPDX-License-Identifier: AGPL-3.0-only
// The signed-in browsers (/settings/sessions) in the states of ADR-0112 §1.
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
import { browserSessions } from "./test/storyFixtures";

/** GET /sessions answering `items`. */
function signedIn(items: Schemas["BrowserSession"][]) {
  return http.get(`${apiBase}/sessions`, ({ request }) => withViewer(request, () => HttpResponse.json({ items })));
}

const meta = {
  title: "Screens/Settings/BrowserSessions",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/sessions", signedIn: true, handlers: [signedIn(browserSessions)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** This browser, another one, one with a User-Agent this does not know, and one whose origin was erased. */
export const Normal: Story = {};

/** Only this browser: nothing to sign out. */
export const OnlyThisBrowser: Story = answering(signedIn(browserSessions.slice(0, 1)));

/** The browsers have not arrived yet. */
export const Loading: Story = loadingOn("get", "/sessions");

/** The server failed to answer the browsers. */
export const ServerError: Story = failingOn("get", "/sessions");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
