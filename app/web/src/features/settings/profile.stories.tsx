// SPDX-License-Identifier: AGPL-3.0-only
// The profile screen (/settings/profile; web-client/screens/settings-profile.md) in the states of ADR-0165 §1
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
  viewerWith,
} from "../../test/stories";

/** A hashtag the account features, on `count` of its posts, the last one at `lastAt`. */
function featuredTag(name: string, count = 0, lastAt?: string): Schemas["FeaturedHashtag"] {
  return {
    name,
    url: `https://my.example/tags/${encodeURIComponent(name)}`,
    statuses_count: count,
    ...(lastAt !== undefined && { last_status_at: lastAt }),
  };
}

/** A filled-in profile: a name, a note, and two fields. */
const filled = viewerWith({
  display_name: "Sam Rivera",
  note: "<p>Writes about bread, bikes, and the occasional bug.</p>",
  source: {
    note: "Writes about bread, bikes, and the occasional bug.",
    fields: [
      { name: "Bakery", value: "https://bread.example" },
      { name: "Pronouns", value: "they/them" },
    ],
  },
});

const meta = {
  title: "Screens/Settings/Profile",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/profile", signedIn: true, handlers: [filled] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A filled-in profile; changing the name and saving shows the result beside the save button. */
export const Normal: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    const name = canvas.getByRole("textbox", { name: "Display name" });
    await userEvent.clear(name);
    await userEvent.type(name, "Sam R.");
    await userEvent.click(canvas.getByRole("button", { name: "Save profile" }));
    await expect(await canvas.findByText("Your profile has been saved.")).toBeInTheDocument();
  },
};

/** A profile that features three hashtags, in the order of their rows. */
export const FeaturedHashtags: Story = answering(
  filled,
  http.get(`${apiBase}/featured_tags`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        items: [featuredTag("sourdough", 42), featuredTag("bikes", 3), featuredTag("rye")],
      }),
    ),
  ),
);

/** A new account's profile: no name, note, or fields yet. */
export const Empty: Story = answering(viewerWith({}));

/** The profile has not arrived yet. */
export const Loading: Story = loadingOn("get", "/accounts/verify_credentials");

/** The server failed to answer the profile. */
export const ServerError: Story = failingOn("get", "/accounts/verify_credentials");

/** In Japanese, with a long name and a note of several lines. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...answering(
    viewerWith({
      display_name: "雪のち晴れの日に山を歩く写真好きのゆき",
      source: {
        note: "写真と散歩。週末は山にいます。\n冬は雪の写真、夏は高山植物を撮っています。機材の話も歓迎です。\nフォローはお気軽にどうぞ。",
        fields: [{ name: "ウェブサイト", value: "https://yuki.example/photos/mountains/winter" }],
      },
    }),
  ),
};

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
