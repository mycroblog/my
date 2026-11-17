// SPDX-License-Identifier: AGPL-3.0-only

// A thread whose post approves replies and boosts by hand (ADR-0155), on the fake's `approvals`: layla's post admits at
// once those it mentions and everyone else by her approval, and holds yuki's boost that waits, yuki's reply that waits
// and yuki's reply she did not approve. The lines over them say so to yuki alone, and the Boost menu says before a
// boost that it will wait. That the server keeps them from the others is the E2E tests' (web/e2e/specs/approval.spec.ts).

import { expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { loadThread, Thread } from "./thread";

/** A name that holds a person's name, which the app isolates (U+2068…U+2069), whole. */
const named = (before: string, person: string, after: string) => new RegExp(`^${before}⁨?${person}⁩?${after}$`);

const words = {
  en: {
    boostWaits: "Boost waiting for approval",
    pending: "Waiting for approval",
    rejected: "Not approved",
    noReplies: "No replies yet.",
    boost: "Boost",
    boostAsks: named("Your boost goes out once ", "@layla", " approves it"),
  },
  ja: {
    boostWaits: "ブーストの承認待ち",
    pending: "承認待ち",
    rejected: "承認されませんでした",
    noReplies: "まだ返信はありません。",
    boost: "ブースト",
    boostAsks: named("", "@layla", "さんが承認すると、ブーストされます"),
  },
};

/** The thread of layla's post as `viewer` reads it, loaded and drawn in `locale`. */
async function shown(viewer: string, locale: keyof typeof words = "en") {
  const { fake, data } = fakeData({ approvals: true, viewer });
  const id = fake.idOf("layla-asks");
  await loadThread(data, id);
  const user = renderFeature(
    data,
    <PostActions onCompose={() => undefined}>
      <Thread id={id} onBack={() => undefined} />
    </PostActions>,
    locale,
  );
  /** The post of the thread whose text starts with `text`, and the lines over it that say what waits. */
  const post = (text: RegExp) => {
    const found = screen.getAllByRole("article").filter((article) => within(article).queryAllByText(text).length > 0);
    if (found.length !== 1) throw new Error(`test: ${String(found.length)} posts say ${String(text)}`);
    return found[0] as HTMLElement;
  };
  const w = words[locale];
  const lines = (text: RegExp) =>
    [w.boostWaits, w.pending, w.rejected].filter((line) => within(post(text)).queryAllByText(line).length > 0);
  return { user, w, post, lines };
}

for (const locale of ["en", "ja"] as const)
  test(`the reader's boost that waits, their reply that waits and their reply that was not approved each say so over the post (${locale})`, async () => {
    const { w, lines } = await shown("yuki", locale);

    expect(lines(/^Translating a poem/)).toEqual([w.boostWaits]);
    expect(lines(/^どの詩ですか/)).toEqual([w.pending]);
    expect(lines(/^翻訳ができたら/)).toEqual([w.rejected]);
  });

test("another reader sees neither the replies that wait or were not approved nor any line about a boost", async () => {
  const { w, lines } = await shown("sam");

  expect(await screen.findByText(w.noReplies)).toBeDefined();
  expect(screen.getAllByRole("article")).toHaveLength(1);
  expect(lines(/^Translating a poem/)).toEqual([]);
});

for (const locale of ["en", "ja"] as const)
  test(`the Boost menu of a post whose author approves the reader's boosts says, before the boost, that it goes out once the author approves it (${locale})`, async () => {
    const { user, w, post } = await shown("sam", locale);

    await user.click(within(post(/^Translating a poem/)).getByRole("button", { name: w.boost }));

    const menu = within(await screen.findByRole("menu", { name: w.boost }));
    expect(menu.getByText(w.boostAsks)).toBeDefined();
  });
