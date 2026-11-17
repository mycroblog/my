// SPDX-License-Identifier: AGPL-3.0-only

// The statuses and the page of the component tests of a post: accounts and a status of the API with the fields a test
// sets, rendered in the reader's locale as the app places it.

import { I18nProvider } from "@my/design-system";
import type { BoundFunctions, queries } from "@testing-library/dom";
import { render } from "@testing-library/react";
import type { ReactNode } from "react";
import type { PostAccount, PostStatus } from "../status";

/** An account of another server. */
export const lena: PostAccount = {
  id: "1",
  username: "lena",
  acct: "lena@toot.example.eu",
  domain: "toot.example.eu",
  display_name: "Lena Novak",
  emojis: [],
};

/** An account of this server, with a custom emoji in its name and an avatar. */
export const ken: PostAccount = {
  id: "2",
  username: "kenji",
  acct: "kenji",
  domain: "my.example",
  display_name: "Kenji :ramen:",
  emojis: [{ shortcode: "ramen", url: "https://my.example/ramen.gif", static_url: "https://my.example/ramen.png" }],
  avatar: { small: { url: "https://my.example/kenji.webp" } },
};

/** A post of Lena's, three minutes old, with the fields a test sets. */
export const status = (over: Partial<PostStatus> = {}): PostStatus => ({
  id: "10",
  account: lena,
  content: "<p>Quiet morning.</p>",
  spoiler_text: "",
  visibility: "public",
  language: "en",
  created_at: new Date(Date.now() - 3 * 60_000).toISOString(),
  media_attachments: [],
  mentions: [],
  emojis: [],
  replies_count: 2,
  boosts_count: 1_234,
  favourites_count: 12_840,
  reactions: [],
  ...over,
});

/** The queries of a part of the page, as `within` gives them. */
export type Part = BoundFunctions<typeof queries>;

/** Renders `node` for a reader of `locale`. */
export const inLocale = (node: ReactNode, locale = "en") => render(<I18nProvider locale={locale}>{node}</I18nProvider>);

/** Whether each of `elements` follows the one before it in the document, the order a screen reader reads them in. */
export const inOrder = (...elements: Element[]) =>
  elements.every((element, i) => {
    const before = elements[i - 1];
    return before === undefined || (before.compareDocumentPosition(element) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
  });

/** Every address the document would have the browser fetch from a server whose address holds `part`. */
export const asked = (part: string) =>
  Array.from(document.querySelectorAll("*"))
    .flatMap((element) => element.getAttributeNames().map((name) => element.getAttribute(name) ?? ""))
    .filter((value) => value.includes(part));
