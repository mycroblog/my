// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji (/admin/custom-emojis; web-client/screens/admin-custom-emojis.md) in the states of ADR-0112 §1:
// this server's emoji, those of other servers, and the requests for new ones.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { adminCustomEmojiFixture, adminCustomEmojiRequestFixture } from "./test/handlers";
import {
  adminAnswering,
  adminFailingOn,
  adminLoadingOn,
  administrator,
  adminScreen,
  withoutPermission,
} from "./test/stories";

const colors = ["#e8590c", "#2b8a3e", "#1971c2", "#9c36b5", "#e03131", "#f08c00"];

/** An image of an emoji that the story draws itself: a disc of the `index`th color with the shortcode's initial. */
function emojiImage(shortcode: string, index: number): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><circle cx="16" cy="16" r="15" fill="${colors[index % colors.length] ?? "#000"}"/><text x="16" y="21" font-size="14" text-anchor="middle" fill="#fff" font-family="sans-serif">${shortcode.charAt(0)}</text></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

const localShortcodes = [
  "blobcat",
  "blobcat_happy",
  "blobfox",
  "blobfox_coffee",
  "party_parrot",
  "thinking_face_but_much_longer_than_usual",
  "ok_hand",
  "neko",
  "sushi",
  "ramen",
  "rocket",
  "sparkles",
];

/** This server's emoji: categories, aliases, one hidden from the picker and one disabled. */
const localEmoji: Schemas["AdminCustomEmoji"][] = localShortcodes.map((shortcode, index) =>
  adminCustomEmojiFixture(`0199f0e0-0000-7000-8000-00000000e1${String(index).padStart(2, "0")}`, shortcode, {
    url: emojiImage(shortcode, index),
    static_url: emojiImage(shortcode, index),
    ...(index < 4 && { category: "Blobs" }),
    ...(index >= 7 && index < 10 && { category: "Food and animals" }),
    aliases: index % 3 === 0 ? [shortcode.slice(0, 4), "cute"] : [],
    visible_in_picker: index !== 5,
    enabled: index !== 6,
  }),
);

/** The emoji of other servers, seen in their posts. */
const remoteEmoji: Schemas["AdminCustomEmoji"][] = ["partyparrot", "blobcat", "meow_wave", "ablobcatrainbow"].map(
  (shortcode, index) =>
    adminCustomEmojiFixture(`0199f0e0-0000-7000-8000-00000000e2${String(index).padStart(2, "0")}`, shortcode, {
      local: false,
      domain: index % 2 === 0 ? "remote.example" : "a-very-long-instance-name.social.example",
      uri: `https://remote.example/emojis/${String(index)}`,
      url: emojiImage(shortcode, index + 2),
      static_url: emojiImage(shortcode, index + 2),
      enabled: index !== 3,
    }),
);

/** The pending requests of the members who may ask for emoji, a new one and a new image of an emoji. */
const pendingRequests: Schemas["AdminCustomEmojiRequest"][] = [
  adminCustomEmojiRequestFixture("0199f0e0-0000-7000-8000-00000000f001", "blobcat_love", {
    url: emojiImage("blobcat_love", 4),
    static_url: emojiImage("blobcat_love", 4),
  }),
  adminCustomEmojiRequestFixture("0199f0e0-0000-7000-8000-00000000f002", "blobfox", {
    kind: "image",
    emoji_id: "0199f0e0-0000-7000-8000-00000000e102",
    url: emojiImage("blobfox", 1),
    static_url: emojiImage("blobfox", 1),
  }),
  adminCustomEmojiRequestFixture("0199f0e0-0000-7000-8000-00000000f003", "our_server_mascot_waving_hello", {
    url: emojiImage("our_server_mascot_waving_hello", 3),
    static_url: emojiImage("our_server_mascot_waving_hello", 3),
  }),
];

/** The emoji of this server or of others, by the query's `local`. */
function emojiListing(local: Schemas["AdminCustomEmoji"][], remote: Schemas["AdminCustomEmoji"][]) {
  return http.get(`${apiBase}/admin/custom_emojis`, ({ request }) => {
    const items = new URL(request.url).searchParams.get("local") === "false" ? remote : local;
    return withViewer(request, () => HttpResponse.json({ items }));
  });
}

/** The pending requests; those of the other states are none. */
function requestListing(pending: Schemas["AdminCustomEmojiRequest"][]) {
  return http.get(`${apiBase}/admin/custom_emoji_requests`, ({ request }) => {
    const state = new URL(request.url).searchParams.get("state") ?? "pending";
    return withViewer(request, () => HttpResponse.json({ items: state === "pending" ? pending : [] }));
  });
}

const meta = {
  title: "Screens/Admin/CustomEmojis",
  ...adminScreen("/admin/custom-emojis", [emojiListing(localEmoji, remoteEmoji), requestListing(pendingRequests)]),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** This server's emoji, in categories, with aliases, a hidden one and a disabled one; three requests pending. */
export const Normal: Story = {};

/** The emoji of other servers. */
export const OtherServers: Story = { parameters: { screen: { path: "/admin/custom-emojis?origin=remote" } } };

/** The pending requests for new emoji and new images. */
export const Requests: Story = { parameters: { screen: { path: "/admin/custom-emojis?tab=requests" } } };

/** A server without custom emoji or requests. */
export const Empty: Story = adminAnswering();

/** No request is pending. */
export const NoRequests: Story = {
  parameters: {
    screen: {
      path: "/admin/custom-emojis?tab=requests",
      handlers: [administrator, emojiListing(localEmoji, remoteEmoji)],
    },
  },
};

/** A moderator without manage_custom_emojis. */
export const MissingPermission: Story = withoutPermission;

/** The emoji have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/custom_emojis");

/** The server failed to answer the emoji. */
export const ServerError: Story = adminFailingOn("get", "/admin/custom_emojis");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the long shortcodes wrap. */
export const Narrow: Story = narrow;
