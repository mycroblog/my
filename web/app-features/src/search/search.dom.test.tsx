// SPDX-License-Identifier: AGPL-3.0-only

// The search as a feature (FR-305–FR-307, FR-309): the people it suggests before anything is written; what it finds
// by kind, people and hashtags and, on a server with a search engine, posts; an account of another server looked up by
// its handle; and the one line, or the one failure, that stands in the results' place. Following from a row is in
// people/people.dom.test.tsx, and that a hashtag found opens its timeline is the E2E tests' (web/e2e/specs/search.spec.ts).

import { expect, test } from "bun:test";
import type { FakeOptions, Route } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { Search } from "./search";

/** A server with a search engine, which finds posts too. */
const engine: FakeOptions = { search: { posts: true } };

/** What the search asks the server for when something is written: the people, the hashtags, and the posts. */
const reads = ["GET /accounts/search", "GET /tags/search", "GET /statuses/search"] as const satisfies readonly Route[];

/** The search of `q` on a server of `options`, drawn for a reader of `locale`, which `script` scripts first. */
function searching(
  q: string,
  options: FakeOptions = {},
  { locale = "en", script }: { locale?: string; script?: (fake: ReturnType<typeof fakeData>["fake"]) => void } = {},
) {
  const made = fakeData(options);
  script?.(made.fake);
  renderFeature(made.data, <Search q={q} onQuery={() => undefined} />, locale);
  return made;
}

/** The rows of the list named `name`, each by the address of its account and its buttons, once the list shows. */
const rows = async (name: string) =>
  within(await screen.findByRole("list", { name }))
    .getAllByRole("listitem")
    .map((row) => ({
      to: within(row).getByRole("link").getAttribute("href"),
      buttons: within(row).queryAllByRole("button").length,
    }));

/** The headings over what was found, in their order: one per kind. */
const kinds = () => screen.queryAllByRole("heading", { level: 2 }).map((heading) => heading.textContent);

/** What describes the field. */
const hint = () =>
  (screen.getByRole("searchbox").getAttribute("aria-describedby") ?? "")
    .split(" ")
    .map((id) => document.getElementById(id)?.textContent)
    .join(" ");

/** The one line that stands in the results' place, once it says what `says` matches. */
const line = (says: RegExp) =>
  waitFor(() => {
    const [status, ...more] = screen.getAllByRole("status").filter((s) => s.textContent !== "");
    if (status === undefined || more.length > 0 || !says.test(status.textContent))
      throw new Error(`the screen says ${JSON.stringify(screen.getAllByRole("status").map((s) => s.textContent))}`);
    return status.textContent;
  });

/** Nothing was found, of any kind, and nothing failed. */
function expectNoResults() {
  expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  expect(screen.queryAllByRole("article")).toHaveLength(0);
  expect(screen.queryAllByRole("alert")).toHaveLength(0);
  expect(kinds()).toEqual([]);
}

test("before anything is written, the search suggests people under its field, each with its follow", async () => {
  // The server suggests mio and marco to yuki.
  searching("");

  const suggested = await rows("Suggested for you");
  expect(suggested.map((row) => row.to)).toEqual(["/@mio", "/@marco"]);
  expect(suggested.map((row) => row.buttons)).toEqual([1, 1]);
  expect(screen.queryAllByRole("link", { name: "Find more people" })).toHaveLength(0);
});

test("of more than five people suggested, the search shows five, then the way to the rest", async () => {
  // quickdeals follows no one: the server suggests everyone else to them.
  searching("", { viewer: "quickdeals" });

  expect(await rows("Suggested for you")).toHaveLength(5);
  expect(screen.getByRole("link", { name: "Find more people" }).getAttribute("href")).toBe("/people");
});

test("on a server that suggests no one, the search before anything is written is its field alone", async () => {
  let answered = false;
  const { data, requests } = searching(
    "",
    {},
    {
      script: (fake) => {
        const suggestions = fake.script("GET /accounts/suggestions", { change: (page) => ({ ...page, items: [] }) });
        void suggestions.served().then(() => (answered = true));
      },
    },
  );

  // Nothing shows while the suggestions are on their way either: the test waits for the answer itself.
  await waitFor(() => {
    if (!answered || data.queryClient.isFetching() > 0) throw new Error("the server's answers are still on their way");
  });
  expect(screen.getByRole("searchbox", { name: "Search" })).not.toBeNull();
  expect(screen.queryAllByRole("heading")).toHaveLength(0);
  expect(screen.queryAllByRole("link")).toHaveLength(0);
  expect(requests.filter((request) => request.startsWith("GET /accounts/relationships"))).toEqual([]);
});

test("on a server without a search engine, the search finds people and hashtags and offers nothing of posts", async () => {
  const { requests } = searching("sa");

  expect((await rows("People")).map((row) => row.to)).toEqual(["/@sam"]);
  expect(kinds()).not.toContain("Posts");
  expect(screen.queryAllByRole("article")).toHaveLength(0);
  expect(hint()).toBe("People by name or handle, and hashtags");
  expect(requests.filter((request) => request.startsWith("GET /statuses/search"))).toEqual([]);
});

for (const [locale, w] of [
  [
    "en",
    {
      kinds: ["People", "Hashtags", "Posts"],
      count: "2 posts",
      hint: "People by name or handle, hashtags, and posts by their words",
    },
  ],
  [
    "ja",
    {
      kinds: ["人", "ハッシュタグ", "投稿"],
      count: "2 件の投稿",
      hint: "名前やハンドルで人を、ハッシュタグを、言葉で投稿を探せます",
    },
  ],
] as const)
  test(`on a server with a search engine, the search finds people, hashtags and posts at once, in that order, a hashtag with how many posts carry it (${locale})`, async () => {
    // "bak" finds Bakery Kobo, #baking and sam's two posts that carry it.
    searching("bak", engine, { locale });
    const [people, hashtags, posts] = w.kinds;

    expect((await rows(people)).map((row) => row.to)).toEqual(["/@kobo@bread.example"]);
    const tag = within(await screen.findByRole("list", { name: hashtags })).getByRole("link");
    expect(tag.getAttribute("href")).toBe("/tags/baking");
    expect(tag.textContent).toMatch(new RegExp(`^#baking\\s*${w.count}$`));
    const found = await screen.findByRole("feed", { name: posts });
    expect(within(found).getAllByRole("article", { name: "Sam Rivera" })).toHaveLength(2);
    expect(kinds()).toEqual([...w.kinds]);
    expect(hint()).toBe(w.hint);
  });

test("an account of another server named by its handle is looked up there, and stands first", async () => {
  // No search finds kiri before a lookup fetched their account from far.example.
  const { requests } = searching("@kiri@far.example");

  expect((await rows("People")).map((row) => row.to)).toEqual(["/@kiri@far.example"]);
  expect(requests).toContain("GET /accounts/lookup?acct=kiri%40far.example&resolve=true");
});

/** Whether `element` is inside what says it is busy. */
const busy = (element: Element | null): boolean =>
  element !== null && (element.getAttribute("aria-busy") === "true" || busy(element.parentElement));

test("while a handle is looked up on its server, one line names the handle and the field is busy", async () => {
  let lookup: { release: () => void } | undefined;
  searching(
    "@kiri@far.example",
    {},
    {
      script: (fake) => {
        lookup = fake.script("GET /accounts/lookup", { hold: "request" });
      },
    },
  );

  expect(await line(/^Looking up/)).toMatch(/^Looking up ⁨?@kiri@far\.example⁩? on its server…$/);
  expect(busy(screen.getByRole("searchbox"))).toBe(true);
  expect(screen.queryAllByRole("listitem")).toHaveLength(0);

  lookup?.release();
  expect(await rows("People")).toHaveLength(1);
  expect(busy(screen.getByRole("searchbox"))).toBe(false);
});

test("a handle no server has says so in one line, and is no failure", async () => {
  searching("@nobody@far.example");

  expect(await line(/^No account/)).toMatch(/^No account was found at ⁨?@nobody@far\.example⁩?\. Check the handle\.$/);
  expectNoResults();
});

test("words that find nothing say so in one line: no kind, no failure", async () => {
  searching("zzqx", engine);

  expect(await line(/^Nothing/)).toMatch(/^Nothing was found for “⁨?zzqx⁩?”\. Try other words\.$/);
  expectNoResults();
});

test("while it searches, one line says so, not one per kind", async () => {
  const held: { release: () => void }[] = [];
  searching("sa", engine, {
    script: (fake) => {
      for (const read of reads) held.push(fake.script(read, { hold: "request" }));
    },
  });

  expect(await line(/^Searching/)).toBe("Searching…");
  expectNoResults();

  for (const read of held) read.release();
  expect(await rows("People")).toHaveLength(1);
});

test("a search that failed says so once, with one Try again, not once per kind", async () => {
  searching("sa", engine, {
    script: (fake) => {
      for (const read of reads) fake.script(read, { fail: 500 });
    },
  });

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toMatch(/^The search could not be done\.\s?Try again$/);
  expect(screen.getAllByRole("alert")).toHaveLength(1);
  expect(screen.getAllByRole("button", { name: "Try again" })).toHaveLength(1);
  expect(screen.queryAllByRole("listitem")).toHaveLength(0);
});
