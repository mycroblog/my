// SPDX-License-Identifier: AGPL-3.0-only
// The landing page (/ for people who are not signed in; web-client/screens/landing.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import { instanceFixture, type Schemas, statusPage } from "../../test/fixtures";
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

/** GET /instance answering a server with a summary and rules, which takes members by approval. */
function instanceWith(changes: Partial<Schemas["Instance"]>) {
  return http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, ...changes }));
}

const described = instanceWith({
  short_description: "A small server for people who bake, ride, and write about it.",
  rules: [
    { id: "0192f0c4-7a8e-7000-8000-0000000000r1", text: "Be kind.", hint: "Disagree with ideas, not with people." },
    { id: "0192f0c4-7a8e-7000-8000-0000000000r2", text: "No spam or advertising." },
  ],
});

const meta = {
  title: "Screens/Landing",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/", handlers: [described] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A server with a summary, rules, counts, and its operator; the progress line moves to the section it names. */
export const Normal: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    const parts = await canvas.findByRole("navigation", { name: "Parts of this page" });
    await userEvent.click(await canvas.findByRole("link", { name: "What do we agree to?" }));
    const promise = canvas.getByRole("region", { name: "What do we agree to?" });
    await expect(promise.querySelector("h2")).toHaveFocus();
    await expect(parts).toBeInTheDocument();
  },
};

/** A server that just started: no summary, no rules, one member, and no public posts yet. */
export const Empty: Story = answering(
  instanceWith({}),
  http.get(`${apiBase}/instance/overview`, () =>
    HttpResponse.json({
      counts: {
        members: 1,
        daily_posts: [0, 0, 0, 0, 0, 0, 0],
        started_at: "2026-10-27T00:00:00Z",
        counted_at: "2026-10-28T00:00:00Z",
      },
    } satisfies Schemas["InstanceOverview"]),
  ),
  http.get(`${apiBase}/timelines/local`, () => HttpResponse.json(statusPage([]))),
);

/** The server's counts and operators have not arrived yet. */
export const Loading: Story = loadingOn("get", "/instance/overview");

/** The server failed to answer its counts and operators. */
export const ServerError: Story = failingOn("get", "/instance/overview");

/** In Japanese, with a summary of the length a Japanese server writes. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...answering(
    instanceWith({
      name: "パンと自転車と、ときどきバグの話をする小さなサーバー",
      short_description:
        "パンを焼く人、自転車に乗る人、そのことを書く人のための小さなサーバーです。週末の写真や、うまくいかなかった日の話も歓迎します。",
    }),
  ),
};

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
