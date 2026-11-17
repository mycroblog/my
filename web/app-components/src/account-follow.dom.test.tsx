// SPDX-License-Identifier: AGPL-3.0-only

// The follow at the end of an account's row, from the reader's relationship with it: Follow, Follow back where the
// account follows the reader, Request to follow where it approves its followers, and in place of a button Following or
// Requested once the reader follows it or asked to; in the reader's language. A press asks the caller once.

import { expect, test } from "bun:test";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AccountFollow, type FollowState } from "./account";
import { accountOf } from "./post.samples";
import { inLocale } from "./test/post";

const open = accountOf({ name: "Alice Chen", handle: "@alice@example.social" });
const locked = { ...accountOf({ name: "Ravi", handle: "@ravi@fedi.example.net" }), locked: true };

/** The follow of `account` for a reader whose relationship with it is `relationship`, alone in the document. */
function follow(
  account: typeof locked | typeof open,
  relationship: FollowState | undefined,
  locale = "en",
  onFollow: () => Promise<unknown> = () => Promise.resolve(),
) {
  cleanup();
  const { container } = inLocale(
    <AccountFollow account={account} relationship={relationship} onFollow={onFollow} />,
    locale,
  );
  return { buttons: screen.queryAllByRole("button").map((button) => button.textContent), said: container.textContent };
}

for (const [state, account, relationship, en, ja] of [
  ["one the reader does not follow", open, {}, "Follow", "フォロー"],
  ["one that follows the reader", open, { followed_by: true }, "Follow back", "フォローバック"],
  ["a locked one", locked, {}, "Request to follow", "フォローをリクエスト"],
] as const)
  test(`${state} has a button, ${en}`, () => {
    expect(follow(account, relationship).buttons).toEqual([en]);
    expect(follow(account, relationship, "ja").buttons).toEqual([ja]);
  });

for (const [state, account, relationship, en] of [
  ["one the reader follows", open, { following: true }, "Following"],
  ["one that follows the reader back", open, { following: true, followed_by: true }, "Following"],
  ["one the reader asked to follow", locked, { requested: true }, "Requested"],
] as const)
  test(`${state} shows ${en} in place of a button`, () => {
    expect(follow(account, relationship)).toEqual({ buttons: [], said: en });
  });

test("nothing shows until the relationship is known, nor for an account the reader blocks", () => {
  expect(follow(open, undefined)).toEqual({ buttons: [], said: "" });
  expect(follow(open, { blocking: true })).toEqual({ buttons: [], said: "" });
});

test("a press asks to follow once: while that runs another press does nothing, and the button never says busy", async () => {
  const user = userEvent.setup();
  let asked = 0;
  let answer: () => void = () => undefined;
  follow(open, {}, "en", () => {
    asked += 1;
    return new Promise<void>((resolve) => {
      answer = resolve;
    });
  });
  const button = screen.getByRole("button", { name: "Follow" });

  await user.click(button);
  await user.click(button);
  await user.keyboard("{Enter}");

  expect(asked).toBe(1);
  expect(button.getAttribute("aria-busy") ?? "false").toBe("false");
  answer();
});
