// SPDX-License-Identifier: AGPL-3.0-only
import { readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  type Baseline,
  baselineOf,
  budgetsOf,
  checkOmissions,
  checkStory,
  outOfOrder,
  parseScreen,
  readBaseline,
  readScreens,
  type Screen,
  screensOf,
  type StoryRecord,
  summary,
} from "./budget.ts";
import { screensDir } from "./tasks.ts";

const root = resolve(import.meta.dirname, "../../..");

function l1(heading: string, front: string[] = [], body: string[] = []): string {
  return ["---", 'start: "/"', "persona: null", "paths: []", ...front, "---", "", `## ${heading}`, ...body].join("\n");
}

const rules = parseScreen(
  "admin-rules",
  l1(
    "ルール（`/admin/rules`）",
    ['budget: {"controls": 5, "helpers": 1, "words": 40}'],
    [
      "### 3. 情報の優先順位とまとまり",
      "",
      "1. 有効なルール: 表。",
      "2. <ルールの文>: 1 つずつ。",
      "3. 廃止: 表。",
      "### 4. x",
    ],
  ),
);
const rule = parseScreen("admin-rule", l1("ルールのページ（`/admin/rules/{id}`・`/admin/rules/$id/edit`）"));
const ruleNew = parseScreen("admin-rule-new", l1("新しいルール（`/admin/rules/new`）", [], ["### 7. 載せないもの"]));
const menu = parseScreen("admin-menu", l1("メニュー（`/admin/rules` の横）", ["budget: null"]));
const profile = parseScreen("profile", l1("プロフィール（`/@{acct}`）"));
const costs = parseScreen(
  "about-costs",
  l1("費用（`/about/costs`）", [], ["### 3. 情報の優先順位とまとまり", "1. 費用: 図。", "2. 収入: 図。"]),
);
const screens = [rules, rule, ruleNew, menu, profile, costs];

const none: Baseline = { regionControls: {}, regionOrder: [], noOmissions: [] };

function story(id: string, path: string, measure: Partial<StoryRecord["measure"]> = {}, locale = "en"): StoryRecord {
  return { id, path, locale, viewport: false, measure: { controls: 0, helpers: 0, words: 0, regions: [], ...measure } };
}

const names = (found: Screen[]) => found.map((screen) => screen.name);

describe("parseScreen", () => {
  it("reads the routes of the first heading, the budget, the list of regions and the omissions", () => {
    expect(rules).toEqual({
      name: "admin-rules",
      routes: ["/admin/rules"],
      budget: { controls: 5, helpers: 1, words: 40 },
      regions: ["有効なルール: 表。", "<ルールの文>: 1 つずつ。", "廃止: 表。"],
      omissions: false,
    });
    expect(rule.routes).toEqual(["/admin/rules/{id}", "/admin/rules/$id/edit"]);
    expect(ruleNew.omissions).toBe(true);
    expect(menu.budget).toBeNull();
  });
});

describe("screensOf", () => {
  it("finds the screen whose route matches most literally, without the query", () => {
    expect(names(screensOf("/admin/rules?filter=retired", screens))).toEqual(["admin-rules"]);
    expect(names(screensOf("/admin/rules/new", screens))).toEqual(["admin-rule-new"]);
    expect(names(screensOf("/admin/rules/0199", screens))).toEqual(["admin-rule"]);
    expect(names(screensOf("/admin/rules/0199/edit", screens))).toEqual(["admin-rule"]);
    expect(names(screensOf("/@alice@remote.example", screens))).toEqual(["profile"]);
  });

  it("finds none for a route no L1 has, and every L1 that shares a route", () => {
    expect(screensOf("/admin/rules/0199/history", screens)).toEqual([]);
    expect(screensOf("/admin", screens)).toEqual([]);
    expect(names(screensOf("/admin/rules", [...screens, { ...rules, name: "other" }]))).toEqual([
      "admin-rules",
      "other",
    ]);
  });
});

describe("outOfOrder", () => {
  it("accepts the h2 that begin items in the list's order, and headings from data at a `<` item", () => {
    expect(outOfOrder(["有効なルール", "廃止"], rules.regions)).toBeUndefined();
    expect(outOfOrder(["有効なルール", "Be kind", "No spam", "廃止"], rules.regions)).toBeUndefined();
    expect(outOfOrder([], [])).toBeUndefined();
  });

  it("names the first h2 that is out of order or in no item", () => {
    expect(outOfOrder(["廃止", "有効なルール"], ["廃止: 表", "有効なルール: 表"])).toBeUndefined();
    expect(outOfOrder(["廃止", "有効なルール"], ["有効なルール: 表", "廃止: 表"])).toBe("有効なルール");
    expect(outOfOrder(["履歴"], ["有効なルール: 表"])).toBe("履歴");
  });
});

describe("checkStory", () => {
  it("passes a story within its screen's budget, and one that sets its own viewport", () => {
    expect(
      checkStory(story("s--normal", "/admin/rules", { controls: 5, helpers: 1, words: 40 }), screens, none),
    ).toEqual([]);
    const narrow = { ...story("s--narrow", "/admin/rules", { controls: 99 }), viewport: true };
    expect(checkStory(narrow, screens, none)).toEqual([]);
  });

  it("fails a story over the budget with the budget, the count and what to do", () => {
    const [failure, ...rest] = checkStory(
      story("s--normal", "/admin/rules", { controls: 6, words: 41 }),
      screens,
      none,
    );
    expect(failure).toBe(
      "controls: 6 on this screen, over its budget of 5. Remove or merge something on this screen, or raise the " +
        `budget in ${screensDir}/admin-rules.md (the owner reviews that diff).`,
    );
    expect(rest).toHaveLength(1);
    expect(rest[0]).toMatch(/^words: 41 .* budget of 40\./);
  });

  it("fails a region with more than 7 controls, on a screen with an L1 or without", () => {
    const regions = [
      { title: "Edit", controls: 8 },
      { title: "Delete", controls: 7 },
    ];
    for (const path of ["/admin/rules/new", "/no-l1"]) {
      const failures = checkStory(story("s--normal", path, { regions }), screens, none);
      expect(failures).toHaveLength(1);
      expect(failures[0]).toMatch(/the region "Edit" has 8 controls, over the 7 of a region: split the region/);
    }
  });

  it("lets the baseline's regions be as they are listed, and fails when they grow or shrink", () => {
    const baseline = { ...none, regionControls: { "s--normal": { Edit: 9 } } };
    const check = (controls: number) =>
      checkStory(story("s--normal", "/no-l1", { regions: [{ title: "Edit", controls }] }), screens, baseline);
    expect(check(9)).toEqual([]);
    expect(check(10)[0]).toMatch(/grew past .* the baseline only shrinks/);
    expect(check(8)[0]).toMatch(/smaller than .* lists: set it to \{"Edit":8\} under regionControls/);
    expect(check(7)[0]).toMatch(/smaller than .* lists: remove the entry under regionControls/);
  });

  it("checks the order of the h2 on the stories in Japanese, with the baseline", () => {
    const ordered = story("s--ja", "/about/costs", { regions: [{ title: "収入", controls: 1 }] }, "ja");
    const other = story("s--ja", "/about/costs", { regions: [{ title: "履歴", controls: 1 }] }, "ja");
    const listed = { ...none, regionOrder: ["s--ja"] };
    expect(checkStory(ordered, screens, none)).toEqual([]);
    expect(checkStory({ ...other, locale: "en" }, screens, none)).toEqual([]);
    expect(checkStory(other, screens, none)[0]).toMatch(/the h2 "履歴" is not where the list .*about-costs\.md/);
    expect(checkStory(other, screens, listed)).toEqual([]);
    expect(checkStory(ordered, screens, listed)[0]).toMatch(/remove it from regionOrder/);
  });

  it("fails a story whose route several L1 files have", () => {
    const failures = checkStory(story("s--normal", "/admin/rules"), [...screens, { ...rules, name: "other" }], none);
    expect(failures[0]).toMatch(/is the route of .*admin-rules\.md and .*other\.md/);
  });
});

describe("the budgets and the baseline of a run", () => {
  const stories = [
    story("a--normal", "/admin/rules", { controls: 3, helpers: 2, words: 10, regions: [{ title: "T", controls: 9 }] }),
    story("a--empty", "/admin/rules", { controls: 4, helpers: 0, words: 30 }),
    story("a--ja", "/about/costs", { controls: 50, regions: [{ title: "履歴", controls: 9 }] }, "ja"),
    { ...story("a--narrow", "/admin/rules", { controls: 60 }), viewport: true },
    story("b--normal", "/no-l1", { controls: 70 }),
  ];

  it("takes the largest counts of each screen's stories in English at the default viewport", () => {
    expect([...budgetsOf(stories, screens)]).toEqual([["admin-rules", { controls: 4, helpers: 2, words: 30 }]]);
  });

  it("lists what fails now, so that every story passes with it", () => {
    const baseline = baselineOf(stories, screens);
    expect(baseline).toEqual({
      regionControls: { "a--normal": { T: 9 } },
      regionOrder: ["a--ja"],
      noOmissions: ["admin-rules", "admin-rule", "admin-menu", "profile", "about-costs"],
    });
    const budgets = budgetsOf(stories, screens);
    const budgeted = screens.map((screen) => ({ ...screen, budget: screen.budget && budgets.get(screen.name) }));
    expect(stories.flatMap((s) => checkStory(s, budgeted, baseline))).toEqual([]);
  });

  it("tells the screens no story measured, the routes without an L1 and the entries of stories that are gone", () => {
    const baseline = { ...none, regionControls: { "gone--normal": { T: 9 } }, regionOrder: ["a--ja", "gone--ja"] };
    expect(summary(stories, screens, baseline)).toEqual({
      unmeasured: ["admin-rule", "admin-rule-new", "profile", "about-costs"],
      unmapped: ["/no-l1"],
      stale: ["gone--normal", "gone--ja"],
    });
  });
});

describe("checkOmissions", () => {
  it("wants the section of every screen the baseline does not list, and the list to shrink", () => {
    const listed = { ...none, noOmissions: ["admin-rules", "admin-rule-new"] };
    expect(checkOmissions(ruleNew, none)).toBeUndefined();
    expect(checkOmissions(rules, listed)).toBeUndefined();
    expect(checkOmissions(rules, none)).toMatch(/admin-rules\.md has no section "### 7\. 載せないもの"/);
    expect(checkOmissions(ruleNew, listed)).toMatch(/remove "admin-rule-new" from noOmissions/);
  });
});

describe("the L1 files and the baseline", () => {
  const baseline = readBaseline(root);
  const all = readScreens(root);

  it("every screen says what it leaves out, or is listed", () => {
    expect(all.map((screen) => checkOmissions(screen, baseline)).filter((failure) => failure !== undefined)).toEqual(
      [],
    );
  });

  it("the baseline lists only screens that exist", () => {
    const files = new Set(readdirSync(join(root, screensDir)).map((file) => file.replace(/\.md$/, "")));
    expect(baseline.noOmissions.filter((name) => !files.has(name))).toEqual([]);
  });

  it("no two screens with stories of their own share a route", () => {
    const shared = all.flatMap((screen) =>
      screen.routes.filter((route) => !route.includes("*") && screensOf(route, all).length > 1),
    );
    expect(shared).toEqual([]);
  });
});
