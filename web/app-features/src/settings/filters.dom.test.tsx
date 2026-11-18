// SPDX-License-Identifier: AGPL-3.0-only

// Keyword filters, the settings' entry (#3202, FR-209, ADR-0157 §6, §9): the reader's filters as a list with its count
// and its one add action, each row the way to that filter's editor, saying what the filter does, to how many words,
// where, and when it ends or ended, an expired one marked as such; with none, what a filter is and the way to add
// one; at the most filters a reader can have, that, and no way to add. Before the filters are read the screen says that
// it loads them, and when they cannot be read, that. The editor is filters-edit.dom.test.tsx; a row and its facts are
// the design system's (settings-rows.dom.test.tsx); the entry's place in the settings and the list after a save or a
// delete are the app's (web/app/src/settings-filters.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { instanceQuery, keywordFiltersQuery, viewerQuery } from "@my/app-data";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { fakeData, renderFeature, type FakeData } from "../test/feature";
import { answersOutsideAct } from "../test/filter-form";
import { inOrder, plain, read } from "../test/relationships";
import { SettingsEntryScreen } from "./screen";

/** Draws Keyword filters on the server `on`, in the main landmark, once the shell read the instance and the account. */
async function drawn({ data }: FakeData, locale = "en"): Promise<{ user: UserEvent; told: string[] }> {
  answersOutsideAct();
  await data.queryClient.query(instanceQuery(data));
  await data.queryClient.query(viewerQuery(data));
  const told: string[] = [];
  const user = renderFeature(
    data,
    <main>
      <SettingsEntryScreen entry="filters" onBack={() => told.push("back")} languages={["en", "ja"]} />
    </main>,
    locale,
  );
  return { user, told };
}

/** Keyword filters on a server of `options`, once the filters are read. */
async function entry(options: FakeOptions = {}, locale = "en") {
  const on = fakeData(options);
  const shown = await drawn(on, locale);
  await screen.findAllByRole("region");
  await read(on);
  return { ...on, ...shown };
}

type SeedFilter = NonNullable<NonNullable<FakeOptions["settings"]>["filters"]>[number];

/** A filter that folds, with `count` words, where `contexts` says. */
const filterOf = (title: string, count: number, more: Partial<SeedFilter> = {}): SeedFilter => ({
  title,
  action: "warn",
  contexts: ["home"],
  keywords: Array.from({ length: count }, (_, i) => ({ keyword: `${title} ${String(i + 1)}`, whole_word: false })),
  ...more,
});

/** The one section of the screen, whose heading says how many filters there are after its name. */
const section = () => screen.getByRole("region", { name: /^Filters(\s*[\d,]+)?$/ });
/** The rows of the list: each link's words, and where it leads. */
const rows = () =>
  within(within(section()).getByRole("list", { name: "Filters" }))
    .getAllByRole("listitem")
    .map((row) => {
      const link = within(row).getByRole("link");
      return [plain(link.textContent), link.getAttribute("href")] as const;
    });
/** A day as a row writes it. */
const day = (at: string | undefined) =>
  new Intl.DateTimeFormat("en", { dateStyle: "medium" }).format(new Date(at ?? ""));
const caption = "Each one folds or hides the posts that contain its words, where it applies.";

describe("the list", () => {
  test("is a section with its count, the line of what a filter does, and Add a filter, which leads to a new filter", async () => {
    const { requests } = await entry();

    expect(requests).toContain("GET /filters");
    expect(plain(within(section()).getAllByRole("heading")[0]?.textContent)).toBe("Filters 3");
    expect(within(section()).getAllByText(caption)).toHaveLength(1);
    const add = within(section()).getByRole("link", { name: "Add a filter" });
    expect(add.getAttribute("href")).toBe("/settings/filters/new");
    // The add action comes before the rows, with the heading.
    const list = within(section()).getByRole("list", { name: "Filters" });
    expect((add.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0).toBe(true);
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });

  test("each row leads to its filter's editor, and says what it does to how many words, then where and until when; an expired one says Expired by its name", async () => {
    const { fake, data } = await entry();
    const { items } = await data.queryClient.query({ ...keywordFiltersQuery(data), staleTime: Infinity });
    const [, election, marathon] = items;

    const [first, second, third, ...others] = rows();
    expect(others).toEqual([]);
    // Oldest first, as the server lists them.
    expect(first?.[0]).toMatch(
      inOrder(
        "Spoilers",
        "Folds matching posts",
        "·",
        "2 words",
        "In home and lists, public timelines, and threads",
        "·",
        "Never ends",
      ),
    );
    expect(second?.[0]).toMatch(
      inOrder(
        "Election season",
        "Hides matching posts",
        "·",
        "3 words",
        "In home and lists, notifications, and public timelines",
        "·",
        `Ends ${day(election?.expires_at)}`,
      ),
    );
    expect(third?.[0]).toMatch(
      inOrder(
        "Marathon week",
        "Expired",
        "Folds matching posts",
        "·",
        "1 word",
        "In home and lists",
        "·",
        `Ended ${day(marathon?.expires_at)}`,
      ),
    );
    expect([first?.[1], second?.[1], third?.[1]]).toEqual([0, 1, 2].map((i) => fake.filterPath(i)));
    // Only the expired one is marked.
    expect(within(section()).getAllByText("Expired")).toHaveLength(1);
  });

  test("a row names each place a filter applies in, in the editor's order, and counts its words as a number is written", async () => {
    await entry({
      settings: {
        filters: [
          filterOf("One", 1, { contexts: ["account"] }),
          filterOf("Two", 2, { contexts: ["account", "thread"] }),
          filterOf("All", 100, { contexts: ["account", "thread", "public", "notifications", "home"] }),
        ],
      },
    });

    expect(rows().map(([words]) => words.replace(/^.*words?\s*/, "").replace(/\s*·\s*Never ends$/, ""))).toEqual([
      "In profiles",
      "In threads and profiles",
      "In home and lists, notifications, public timelines, threads, and profiles",
    ]);
    expect(rows().map(([words]) => /·\s*([\d,]+ words?)/.exec(words)?.[1])).toEqual(["1 word", "2 words", "100 words"]);
  });

  test("with no filter, the section says what a filter does and leads to adding one, with no count, no list and no caption", async () => {
    await entry({ settings: { filters: [] } });

    expect(plain(within(section()).getAllByRole("heading")[0]?.textContent)).toBe("Filters");
    expect(
      within(section()).getAllByText(
        "You have no keyword filters. A filter folds or hides the posts that contain its words, where you choose.",
      ),
    ).toHaveLength(1);
    expect(within(section()).queryAllByText(caption)).toHaveLength(0);
    expect(within(section()).queryAllByRole("list")).toHaveLength(0);
    // One way to add, after the sentence.
    const [add, ...others] = within(section()).getAllByRole("link");
    expect(others).toEqual([]);
    expect([add?.textContent, add?.getAttribute("href")]).toEqual(["Add a filter", "/settings/filters/new"]);
  });

  // ADR-0157 §9: a reader has at most 100 filters.
  for (const [count, full] of [
    [99, false],
    [100, true],
  ] as const)
    test(`with ${String(count)} filters, adding is ${full ? "not offered, and the section says that this is the most" : "still offered"}`, async () => {
      await entry({ settings: { filters: Array.from({ length: count }, (_, i) => filterOf(`F${String(i)}`, 1)) } });
      const limit = "You have 100 filters, the most you can have. Delete one to add another.";

      expect(plain(within(section()).getAllByRole("heading")[0]?.textContent)).toBe(`Filters ${String(count)}`);
      expect(rows()).toHaveLength(count);
      expect(within(section()).queryAllByRole("link", { name: "Add a filter" })).toHaveLength(full ? 0 : 1);
      expect(within(section()).queryAllByText(limit)).toHaveLength(full ? 1 : 0);
    });

  test("in Japanese the section, its add action and a row's facts read in it", async () => {
    await entry({}, "ja");

    const part = within(screen.getByRole("region", { name: /^フィルタ\s*3$/ }));
    expect(part.getAllByRole("link", { name: "フィルタを追加" })).toHaveLength(1);
    const [first] = within(part.getByRole("list")).getAllByRole("listitem");
    expect(plain(first?.textContent)).toMatch(/^Spoilers.*2.*ホームとリスト.*$/);
    expect(part.getAllByText("期限切れ")).toHaveLength(1);
  });

  test("the way back, named Settings, is the owner's to follow", async () => {
    const { user, told } = await entry();

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(told).toEqual(["back"]);
  });
});

describe("before the filters are read", () => {
  const said = () =>
    screen
      .queryAllByRole("status")
      .map((status) => plain(status.textContent))
      .filter((words) => words !== "");

  test("the screen says that it loads the settings, with no section, until the filters come", async () => {
    const on = fakeData();
    const reading = on.fake.script("GET /filters", { hold: "request" });
    await drawn(on);
    await reading.arrived();

    expect(said()).toEqual(["Loading your settings…"]);
    expect(screen.queryAllByRole("region")).toHaveLength(0);
    expect(screen.queryAllByRole("link")).toHaveLength(0);

    reading.release();
    await screen.findAllByRole("region");
    expect(said()).toEqual([]);
    expect(rows()).toHaveLength(3);
  });

  test("when the filters cannot be read the screen says that the settings could not be loaded, and Try again, inside the alert, reads them", async () => {
    const on = fakeData();
    const reading = on.fake.script("GET /filters", { times: 1, fail: 500 });
    const { user } = await drawn(on);

    const failed = await screen.findByRole("alert");
    expect(plain(failed.textContent)).toMatch(inOrder("Your settings could not be loaded.", "Try again"));
    expect(screen.queryAllByRole("region")).toHaveLength(0);

    await user.click(within(failed).getByRole("button", { name: "Try again" }));

    await screen.findAllByRole("region");
    expect(reading.seen).toHaveLength(2);
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
    expect(rows()).toHaveLength(3);
  });
});
