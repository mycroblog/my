// SPDX-License-Identifier: AGPL-3.0-only
// The followed hashtags (/settings/followed-tags; web-client/screens/settings-followed-tags.md) in the states of
// ADR-0079 §13
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
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
import { followedTagFixture, hashtagFixture } from "./test/handlers";

/** The server's followed hashtags of the viewer, the latest first, which following and unfollowing change. */
function followedTags(initial: string[]) {
  let names = initial;
  const list = () =>
    HttpResponse.json({
      items: names.map((name) => followedTagFixture(name)),
    } satisfies Schemas["FollowedHashtagCollection"]);
  return [
    http.get(`${apiBase}/followed_tags`, ({ request }) => withViewer(request, list)),
    http.post(`${apiBase}/tags/:hashtag/follow`, ({ request, params }) =>
      withViewer(request, () => {
        const name = String(params["hashtag"]).replace(/^#/u, "").toLowerCase();
        names = [name, ...names.filter((other) => other !== name)];
        return HttpResponse.json(hashtagFixture(name, true));
      }),
    ),
    http.delete(`${apiBase}/tags/:hashtag/follow`, ({ request, params }) =>
      withViewer(request, () => {
        const name = String(params["hashtag"]).replace(/^#/u, "").toLowerCase();
        names = names.filter((other) => other !== name);
        return HttpResponse.json(hashtagFixture(name, false));
      }),
    ),
  ];
}

const meta = {
  title: "Screens/Settings/FollowedTags",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/settings/followed-tags",
      signedIn: true,
      handlers: followedTags(["sourdough", "週末のパン作り記録", "rye"]),
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Three followed hashtags, the latest first, with the count of the limit beside the heading. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByRole("link", { name: "#sourdough" })).toBeInTheDocument();
  },
};

/** The form that follows a hashtag, open in place of the button with focus in its field. */
export const Adding: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(canvas.getByRole("button", { name: "Follow a hashtag" }));
    const field = canvas.getByRole("textbox", { name: /Hashtag/ });
    await expect(field).toHaveFocus();
    await userEvent.type(field, "#bread");
  },
};

/** As many followed hashtags as allowed: the button gives way to the reason. */
export const Full: Story = answering(...followedTags(Array.from({ length: 100 }, (_, i) => `tag${String(i + 1)}`)));

/** No followed hashtag: the empty state leads to following one. */
export const Empty: Story = {
  ...answering(...followedTags([])),
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByRole("button", { name: "Follow a hashtag" })).toBeInTheDocument();
  },
};

/** The followed hashtags have not arrived yet. */
export const Loading: Story = loadingOn("get", "/followed_tags");

/** The server failed to answer the followed hashtags. */
export const ServerError: Story = failingOn("get", "/followed_tags");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
