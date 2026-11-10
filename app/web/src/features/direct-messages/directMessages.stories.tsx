// SPDX-License-Identifier: AGPL-3.0-only
// The direct messages (/direct-messages; web-client/screens/direct-messages.md, ADR-0154 §6) in the states of ADR-0079
// §13: unread and read conversations, one with a group, one behind a content warning, one of pictures only, and one
// with a name in a right-to-left script.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { type Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { abdulrahman, bob, carol, haruka, maximiliana, picture, pictureHandlers, post } from "../../test/postStories";
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

/** Conversation `n`, whose newest post is `last`, with `overrides`. */
function conversation(
  n: number,
  last: Schemas["Status"],
  overrides: Partial<Schemas["Conversation"]> = {},
): Schemas["Conversation"] {
  return {
    id: `0192f0c4-7a8e-7000-8000-00000000d${String(n).padStart(3, "0")}`,
    accounts: [last.account],
    last_status: last,
    unread: false,
    ...overrides,
  };
}

/** A direct post `n` of `account`, `minutes` before 2026-10-28T12:00:00Z, with `overrides`. */
function direct(
  n: number,
  account: Schemas["Account"],
  minutes: number,
  overrides: Partial<Schemas["Status"]> = {},
): Schemas["Status"] {
  return post(n, account, {
    visibility: "direct",
    created_at: new Date(Date.UTC(2026, 9, 28, 12, 0) - minutes * 60_000).toISOString(),
    ...overrides,
  });
}

const someConversations: Schemas["Conversation"][] = [
  conversation(
    1,
    direct(201, bob, 4, {
      content:
        '<p><span class="h-card"><a href="https://my.example/@alice" class="u-url mention">@<span>alice</span></a></span> Are you coming to the bakery club on Saturday? We are trying rye this time, and I could use a hand with the starter.</p>',
    }),
    { unread: true },
  ),
  conversation(
    2,
    direct(202, haruka, 50, { content: "<p>柿をたくさんもらったので、明日少し持っていきますね。</p>", language: "ja" }),
    { accounts: [haruka, carol, maximiliana, abdulrahman], unread: true },
  ),
  conversation(3, direct(203, carol, 180, { spoiler_text: "Spoilers for the book club", content: "<p>…</p>" })),
  conversation(4, direct(204, abdulrahman, 2 * 24 * 60, { content: "", media_attachments: [picture(1, "A loaf")] })),
  conversation(5, direct(205, abdulrahman, 9 * 24 * 60, { content: "<p>شكرًا على الترجمة!</p>", language: "ar" })),
];

/** GET /conversations answering `items`. */
function conversations(items: Schemas["Conversation"][]) {
  return http.get(`${apiBase}/conversations`, ({ request }) => withViewer(request, () => HttpResponse.json({ items })));
}

const meta = {
  title: "Screens/Direct messages",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/direct-messages",
      signedIn: true,
      handlers: [...pictureHandlers, conversations(someConversations)],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two unread conversations, one with a group, and read ones behind a warning, of a picture, and in Arabic. */
export const Normal: Story = {};

/** No conversation yet. */
export const Empty: Story = answering(conversations([]));

/** The conversations have not arrived yet. */
export const Loading: Story = loadingOn("get", "/conversations");

/** The server failed to answer. */
export const ServerError: Story = failingOn("get", "/conversations");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
