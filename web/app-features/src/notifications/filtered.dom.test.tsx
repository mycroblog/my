// SPDX-License-Identifier: AGPL-3.0-only

// A notification line about a post that matches the reader's keyword filter (ADR-0157 §6): the line says which filter
// instead of quoting the post. The fake cannot produce one, since every line-shaped notification is about the reader's
// own post, which never matches, so the page is made by hand.

import { expect, test } from "bun:test";
import { NavigationProvider } from "@my/design-system";
import { fakeHost } from "@my/e2e/fake";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { loadNotifications, Notifications } from "./notifications";

/** sam's notifications, whose first line is a closed poll about a post, matching `titles` when it has any. */
async function pollClosed(titles: string[], locale = "en") {
  const made = fakeData({ viewer: "sam" });
  made.fake.script("GET /notifications", {
    change: (page) => {
      const about = page.items.find((item) => "status" in item);
      if (about === undefined || !("status" in about))
        throw new Error("test: no notification of the fake is about a post");
      const status = {
        ...about.status,
        content: "<p>Bread tomorrow, bring butter.</p>",
        filtered: titles.map((title, i) => ({ filter: { id: `f${String(i)}`, title }, keyword_matches: ["bread"] })),
      };
      return { ...page, items: [{ ...about, type: "poll_closed" as const, status }] };
    },
  });
  await loadNotifications(made.data);
  cleanup();
  const user = renderFeature(
    made.data,
    <NavigationProvider navigate={() => undefined}>
      <PostActions onCompose={() => undefined}>
        <Notifications instance={fakeHost} />
      </PostActions>
    </NavigationProvider>,
    locale,
  );
  await waitFor(() => {
    if (screen.queryAllByRole("article").length === 0) throw new Error("test: no notification yet");
  });
  return { ...made, user, row: screen.getAllByRole("article")[0] as HTMLElement };
}

test("a closed poll's line says which filter its post matches in place of the post's excerpt, and still links to the post", async () => {
  const { row } = await pollClosed(["Baking"]);

  expect(within(row).getByText(/^Matches filter/).textContent).toMatch(/^Matches filter “\s*Baking\s*”$/);
  expect(within(row).queryAllByText(/Bread tomorrow/)).toHaveLength(0);
  expect(within(row).queryAllByRole("button", { name: /^(Show|Hide)$/ })).toHaveLength(0);
  expect(
    within(row)
      .getAllByRole("link")
      .some((link) => (link.getAttribute("href") ?? "").includes("/status/")),
  ).toBe(true);
});

test("it reads in the reader's language, every filter named", async () => {
  const { row } = await pollClosed(["A", "B"], "ja");

  expect(within(row).getByText(/^フィルタ/).textContent).toMatch(/^フィルタ「\s*A\s*」、「\s*B\s*」に一致$/);
});

test("a post that matches no filter keeps its excerpt on the line", async () => {
  const { row } = await pollClosed([]);

  expect(within(row).getByText(/Bread tomorrow/)).not.toBeNull();
  expect(within(row).queryAllByText(/^Matches filter/)).toHaveLength(0);
});
