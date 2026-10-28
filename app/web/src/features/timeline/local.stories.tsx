// SPDX-License-Identifier: AGPL-3.0-only
// The local timeline (/local; FR-202): the public posts of this server's accounts, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { type Schemas, statusPage } from "../../test/fixtures";
import { apiBase, problem } from "../../test/msw/http";
import { pictureHandlers, variedTimeline } from "../../test/postStories";
import {
  answering,
  answeringWithErrorPage,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";

/** GET /timelines/local answering `items`. */
function timeline(items: Schemas["Status"][]) {
  return http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage(items)));
}

const meta = {
  title: "Screens/Timeline/Local",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/local", signedIn: true, handlers: [...pictureHandlers, timeline(variedTimeline)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Posts of every kind, to a signed-in viewer. */
export const Normal: Story = {};

/** To someone who is not signed in. */
export const SignedOut: Story = { parameters: { screen: { signedIn: false } } };

/** No post yet. */
export const Empty: Story = answering(timeline([]));

/** The instance keeps the timeline for signed-in users, and the visitor is not signed in. */
export const SignInRequired: Story = {
  parameters: {
    screen: {
      signedIn: false,
      errorPage: "request",
      handlers: [
        http.get(`${apiBase}/timelines/local`, () =>
          problem(401, "/api/v1/problems/unauthenticated", "Unauthenticated"),
        ),
      ],
    },
  },
};

/** The instance turned the timeline off. */
export const TurnedOff: Story = answeringWithErrorPage(
  http.get(`${apiBase}/timelines/local`, () => problem(404, "about:blank", "Not Found")),
);

/** The first page has not arrived yet. */
export const Loading: Story = loadingOn("get", "/timelines/local");

/** The server failed to answer the first page. */
export const ServerError: Story = failingOn("get", "/timelines/local");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
