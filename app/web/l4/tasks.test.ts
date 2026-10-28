// SPDX-License-Identifier: AGPL-3.0-only
import { readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { capTasks, globToRegExp, parseTasks, screenChanged, screensDir, type Task } from "./tasks.ts";

const root = resolve(import.meta.dirname, "../../..");

describe("parseTasks", () => {
  it("reads the rows of the task table with their shortest paths", () => {
    const markdown = [
      "## バッジ（`/settings/badges`）",
      "",
      "### 1. 使う人と頻度",
      "",
      "| not | tasks |",
      "| --- | --- |",
      "| x | 9 |",
      "",
      "### 2. 作業",
      "",
      "| 作業 | 最短の手順の数 |",
      "| --- | --- |",
      "| バッジを出す | 2 |",
      "| バッジを外す | 3 |",
      "",
      "手順は 2 つ。",
      "",
      "### 3. 情報の優先順位とまとまり",
      "| a | 1 |",
    ].join("\n");
    expect(parseTasks("badges", markdown)).toEqual([
      { screen: "badges", screenTitle: "バッジ", task: "バッジを出す", shortest: 2 },
      { screen: "badges", screenTitle: "バッジ", task: "バッジを外す", shortest: 3 },
    ]);
  });

  it("fails on a row without a count", () => {
    expect(() => parseTasks("x", "## X\n### 2. 作業\n| a | b |\n| - | - |\n| task | many |\n")).toThrow(/task/);
  });

  it("reads every L1 file of the repository", () => {
    for (const name of readdirSync(join(root, screensDir)).filter((n) => n.endsWith(".md") && n !== "README.md")) {
      const tasks = parseTasks(name.slice(0, -3), readFileSync(join(root, screensDir, name), "utf8"));
      expect(tasks.length, name).toBeGreaterThan(0);
    }
  });
});

describe("screenChanged", () => {
  const paths = ["app/web/src/features/settings/**", "app/web/src/features/other/*.ts"];

  it("is true for the L1 file and the paths of its front matter", () => {
    expect(screenChanged("badges", ["docs/architecture/web-client/screens/badges.md"], [])).toBe(true);
    expect(screenChanged("badges", ["app/web/src/features/settings/components/Badges.tsx"], paths)).toBe(true);
    expect(screenChanged("badges", ["app/web/src/features/other/a.ts"], paths)).toBe(true);
  });

  it("is false for other paths", () => {
    expect(screenChanged("badges", ["app/web/src/features/third/a.ts", "README.md"], paths)).toBe(false);
    expect(screenChanged("badges", ["app/web/src/features/other/deep/a.ts"], paths)).toBe(false);
    expect(screenChanged("badges", ["docs/architecture/web-client/screens/settings.md"], paths)).toBe(false);
  });
});

describe("globToRegExp", () => {
  it("matches one segment with * and any with **", () => {
    expect(globToRegExp("a/*.ts").test("a/b.ts")).toBe(true);
    expect(globToRegExp("a/*.ts").test("a/b/c.ts")).toBe(false);
    expect(globToRegExp("a/**").test("a/b/c.ts")).toBe(true);
    expect(globToRegExp("a/**/c.ts").test("a/c.ts")).toBe(true);
    expect(globToRegExp("schema/*/14-voice.sql").test("schema/sqlite/14-voice.sql")).toBe(true);
  });
});

describe("capTasks", () => {
  const task = (screen: string, n: number): Task => ({
    screen,
    screenTitle: screen,
    task: `${screen}${String(n)}`,
    shortest: 1,
  });

  it("takes the screens in turn up to the cap", () => {
    const tasks = [task("a", 1), task("a", 2), task("a", 3), task("b", 1)];
    expect(capTasks(tasks, 3).map((t) => t.task)).toEqual(["a1", "b1", "a2"]);
    expect(capTasks(tasks, 10)).toHaveLength(4);
  });
});
