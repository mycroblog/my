// SPDX-License-Identifier: AGPL-3.0-only

// What waits for the reader's approval (ADR-0155) as a feature: a request per reply, boost and quote of the reader's
// post that asks, each with Decline and Approve; and the answer the reader gave, which stands in the place of the
// request's buttons and takes the focus they leave, while the other requests keep theirs. That the answer reaches the
// thread and the other readers is the E2E tests' (web/e2e/specs/approval.spec.ts).

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { loadWaitingRequests, WaitingRequests } from "./waiting";

/** A name that holds a person's name, which the app isolates (U+2068…U+2069), whole. */
const named = (before: string, person: string, after: string) => new RegExp(`^${before}⁨?${person}⁩?${after}$`);

const words = {
  en: {
    waiting: "Waiting for approval",
    reply: named("", "Marco", " asked to reply to your post"),
    boost: named("", "Marco", " asked to boost your post"),
    quote: named("", "Mio 澪", " asked to quote your post"),
    approve: "Approve",
    decline: "Decline",
    approved: "Approved",
    declined: "Declined",
  },
  ja: {
    waiting: "承認待ち",
    reply: named("", "Marco", "さんが返信の承認を求めています"),
    boost: named("", "Marco", "さんがブーストの承認を求めています"),
    quote: named("", "Mio 澪", "さんが引用の承認を求めています"),
    approve: "承認",
    decline: "断る",
    approved: "承認しました",
    declined: "断りました",
  },
};

/**
 * What waits for yuki's approval on a server whose posts approve by hand (the fake's `approvals`): marco's reply to
 * and boost of yuki's post, and mio's quote of it; drawn in `locale`.
 */
async function shown(locale: keyof typeof words = "en") {
  const made = fakeData({ approvals: true });
  await loadWaitingRequests(made.data);
  const user = renderFeature(made.data, <WaitingRequests onBack={() => undefined} />, locale);
  const w = words[locale];
  const feed = within(screen.getByRole("feed", { name: w.waiting }));
  /** The buttons of the request named `asked`, by their words. */
  const buttons = (asked: RegExp) =>
    within(feed.getByRole("article", { name: asked }))
      .queryAllByRole("button")
      .map((button) => button.textContent);
  return { ...made, user, w, feed, buttons };
}

for (const locale of ["en", "ja"] as const)
  test(`a request per reply, boost and quote that asks, each with Decline before Approve (${locale})`, async () => {
    const { w, feed, buttons } = await shown(locale);

    expect(feed.getAllByRole("article")).toHaveLength(3);
    for (const asked of [w.reply, w.boost, w.quote]) expect(buttons(asked)).toEqual([w.decline, w.approve]);
  });

for (const locale of ["en", "ja"] as const)
  for (const answer of ["approve", "decline"] as const)
    test(`${words.en[answer]} puts the answer in the place of the request's buttons, with the focus on it, and the other requests keep theirs (${locale})`, async () => {
      const { user, w, feed, buttons } = await shown(locale);
      const said = answer === "approve" ? w.approved : w.declined;
      const request = feed.getByRole("article", { name: w.reply });

      await user.click(within(request).getByRole("button", { name: w[answer] }));

      await waitFor(() => {
        if (document.activeElement?.textContent !== said)
          throw new Error(`the focus is on "${document.activeElement?.textContent ?? ""}", not on the answer`);
      });
      expect(request.contains(document.activeElement)).toBe(true);
      expect(buttons(w.reply)).toEqual([]);
      expect(buttons(w.boost)).toEqual([w.decline, w.approve]);
      expect(buttons(w.quote)).toEqual([w.decline, w.approve]);
    });
