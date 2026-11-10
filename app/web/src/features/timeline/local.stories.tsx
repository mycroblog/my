// SPDX-License-Identifier: AGPL-3.0-only
// The local timeline (/local; FR-202): the public posts of this server's accounts, in the states of ADR-0079 §13
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
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

/**
 * Posts that the viewer's keyword filters fold (ADR-0157 §2): the first matched one filter, the second two; the button
 * shows the first in its place.
 */
export const Filtered: Story = {
  ...answering(
    timeline(
      variedTimeline.slice(0, 3).map((status, index) =>
        index > 1
          ? status
          : {
              ...status,
              filtered: [
                { filter: { id: "f1", title: "Season finale" }, keyword_matches: ["finale"] },
                ...(index === 1 ? [{ filter: { id: "f2", title: "Election night" }, keyword_matches: ["vote"] }] : []),
              ],
            },
      ),
    ),
  ),
  play: async (context) => {
    await settled(context);
    const { canvas, userEvent } = context;
    const [show] = canvas.getAllByRole("button", { name: "Show" });
    if (show !== undefined) await userEvent.click(show);
    await expect(canvas.getAllByRole("button", { name: "Show" })).toHaveLength(1);
  },
};

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
