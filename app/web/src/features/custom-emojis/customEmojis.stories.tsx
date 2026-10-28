// SPDX-License-Identifier: AGPL-3.0-only
// The user's custom emoji page (/settings/custom-emojis): their emoji with what they used of their limit, its pending
// part apart, their requests, and the form that requests a new one, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, withViewer } from "../../test/msw/http";
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
import { customEmojiRequestFixture, ownCustomEmojiFixture } from "./test/handlers";

/** GET /custom_emojis/mine answering `items` with `used` of `limit`, `pending` of them pending requests. */
function mine(items: Schemas["OwnCustomEmoji"][], used: number, pending: number, limit?: number) {
  return http.get(`${apiBase}/custom_emojis/mine`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        items,
        used,
        pending,
        ...(limit === undefined ? {} : { limit }),
      } satisfies Schemas["OwnCustomEmojiPage"]),
    ),
  );
}

/** GET /custom_emoji_requests answering `items`. */
function requests(items: Schemas["CustomEmojiRequest"][]) {
  return http.get(`${apiBase}/custom_emoji_requests`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items } satisfies Schemas["CustomEmojiRequestPage"])),
  );
}

const creator = operatorHandler(["create_custom_emojis"]);

/** Two emoji and one pending request, of a limit of five. */
const loaded = [
  creator,
  mine(
    [
      ownCustomEmojiFixture("0199f0e0-0000-7000-8000-00000000e001", "blobcat"),
      ownCustomEmojiFixture("0199f0e0-0000-7000-8000-00000000e002", "blobfox"),
    ],
    3,
    1,
    5,
  ),
  requests([customEmojiRequestFixture("0199f0e0-0000-7000-8000-00000000e101", "blobdog")]),
];

const meta = {
  title: "Screens/Settings/CustomEmojis",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/custom-emojis", signedIn: true, handlers: loaded } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two emoji and a pending request: the meter draws the pending one apart and says so. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByRole("meter", { name: "Custom emoji used" })).toHaveAttribute(
      "aria-valuetext",
      "3 of 5, 1 of them pending",
    );
  },
};

/** The limit reached, two of it by pending requests. */
export const LimitReached: Story = answering(
  creator,
  mine(
    [
      ownCustomEmojiFixture("0199f0e0-0000-7000-8000-00000000e001", "blobcat"),
      ownCustomEmojiFixture("0199f0e0-0000-7000-8000-00000000e002", "blobfox"),
      ownCustomEmojiFixture("0199f0e0-0000-7000-8000-00000000e003", "blobowl"),
    ],
    5,
    2,
    5,
  ),
  requests([
    customEmojiRequestFixture("0199f0e0-0000-7000-8000-00000000e101", "blobdog"),
    customEmojiRequestFixture("0199f0e0-0000-7000-8000-00000000e102", "blobbun"),
  ]),
);

/** No emoji, no requests, and no limit. */
export const Empty: Story = answering(creator, mine([], 0, 0), requests([]));

/** The emoji have not arrived yet. */
const loading = loadingOn("get", "/custom_emojis/mine");
export const Loading: Story = {
  ...loading,
  ...answering(...loading.parameters.screen.handlers, ...loaded),
};

/** The server failed to answer the emoji. */
export const ServerError: Story = answering(
  ...failingOn("get", "/custom_emojis/mine").parameters.screen.handlers,
  ...loaded,
);

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
