// SPDX-License-Identifier: AGPL-3.0-only

// The follow at the end of an account's row, from the reader's relationship with it: Follow, Follow back where the
// account follows the reader, Request to follow where it approves its followers, and in place of a button Following or
// Requested once the reader follows it or asked to; in the reader's language.

import { expect, test } from "bun:test";
import { I18nProvider } from "@my/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { AccountFollow, type FollowState } from "./account";
import { accountOf } from "./post.samples";

const open = accountOf({ name: "Alice Chen", handle: "@alice@example.social" });
const locked = { ...accountOf({ name: "Ravi", handle: "@ravi@fedi.example.net" }), locked: true };

const shown = (account: typeof locked | typeof open, relationship: FollowState, locale = "en") =>
  renderToStaticMarkup(
    <I18nProvider locale={locale}>
      <AccountFollow account={account} relationship={relationship} onFollow={() => Promise.resolve()} />
    </I18nProvider>,
  );

/** The names of the buttons in `html`. */
const buttons = (html: string) => [...html.matchAll(/<button[^>]*>(.*?)<\/button>/gsu)].map((m) => text(m[1] ?? ""));

/** `html` as text. */
const text = (html: string) => html.replace(/<[^>]+>/gu, "").trim();

for (const [state, account, relationship, en, ja] of [
  ["one the reader does not follow", open, {}, "Follow", "フォロー"],
  ["one that follows the reader", open, { followed_by: true }, "Follow back", "フォローバック"],
  ["a locked one", locked, {}, "Request to follow", "フォローをリクエスト"],
] as const)
  test(`${state} has a button, ${en}`, () => {
    expect(buttons(shown(account, relationship))).toEqual([en]);
    expect(buttons(shown(account, relationship, "ja"))).toEqual([ja]);
  });

for (const [state, account, relationship, en] of [
  ["one the reader follows", open, { following: true }, "Following"],
  ["one that follows the reader back", open, { following: true, followed_by: true }, "Following"],
  ["one the reader asked to follow", locked, { requested: true }, "Requested"],
] as const)
  test(`${state} shows ${en} in place of a button`, () => {
    const html = shown(account, relationship);
    expect(buttons(html)).toEqual([]);
    expect(text(html)).toBe(en);
  });
