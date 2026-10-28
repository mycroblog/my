// SPDX-License-Identifier: AGPL-3.0-only
import { readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it, vi } from "vitest";
import { applicant, personas } from "../demo/scenario.ts";
import { applySeed, parseSetup } from "./screens.ts";
import { parseTasks, screensDir } from "./tasks.ts";

const root = resolve(import.meta.dirname, "../../..");
const usernames = new Set([...personas, applicant].map((p) => p.username));

describe("parseSetup", () => {
  it("reads the keys of the front matter as JSON", () => {
    const markdown = [
      "---",
      'start: "/settings"',
      "persona: null",
      'paths: ["app/web/src/features/x/**"]',
      'seed: [{"my": ["admin", "role", "grant", "yuki", "translator"]}, {"api": "yuki", "method": "PUT", "path": "/p", "body": {"a": false}}]',
      'personas: {"task": null}',
      'unavailable: {"task": "why"}',
      "---",
      "",
      "## X",
    ].join("\n");
    expect(parseSetup("x", markdown)).toEqual({
      start: "/settings",
      persona: null,
      paths: ["app/web/src/features/x/**"],
      seed: [
        { my: ["admin", "role", "grant", "yuki", "translator"] },
        { api: "yuki", method: "PUT", path: "/p", body: { a: false } },
      ],
      personas: { task: null },
      unavailable: { task: "why" },
    });
  });

  it("rejects a file without front matter, a missing key, an unknown key, and a value of the wrong form", () => {
    expect(() => parseSetup("x", "## X\n")).toThrow(/no front matter/);
    expect(() => parseSetup("x", '---\nstart: "/"\npaths: []\n---\n')).toThrow(/"persona"/);
    expect(() => parseSetup("x", '---\nstart: "/"\npersona: null\npaths: []\nother: 1\n---\n')).toThrow(/unknown/);
    expect(() => parseSetup("x", "---\nstart: /\npersona: null\npaths: []\n---\n")).toThrow(/not JSON/);
    expect(() => parseSetup("x", '---\nstart: "/"\npersona: null\npaths: "a"\n---\n')).toThrow(/patterns/);
    expect(() => parseSetup("x", '---\nstart: "/"\npersona: null\npaths: []\nseed: [{"my": "x"}]\n---\n')).toThrow(
      /seed/,
    );
  });
});

describe("applySeed", () => {
  it("runs the steps in order", async () => {
    const calls: string[] = [];
    const runMy = vi.fn((args: readonly string[]) => {
      calls.push(`my ${args.join(" ")}`);
      return "";
    });
    const api = vi.fn((username: string, method: string, path: string) => {
      calls.push(`${username} ${method} ${path}`);
      return Promise.resolve();
    });
    await applySeed([{ my: ["a", "b"] }, { api: "yuki", method: "PUT", path: "/p", body: {} }], { runMy, api });
    expect(calls).toEqual(["my a b", "yuki PUT /p"]);
  });
});

describe("the screens' setups", () => {
  for (const name of readdirSync(join(root, screensDir)).filter((n) => n.endsWith(".md") && n !== "README.md")) {
    const screen = name.slice(0, -3);
    it(`${screen} names its tasks and the people of the demo scenario`, () => {
      const markdown = readFileSync(join(root, screensDir, name), "utf8");
      const tasks = new Set(parseTasks(screen, markdown).map((task) => task.task));
      const setup = parseSetup(screen, markdown);
      for (const named of [
        ...Object.keys(setup.starts ?? {}),
        ...Object.keys(setup.personas ?? {}),
        ...Object.keys(setup.mail ?? {}),
        ...Object.keys(setup.push ?? {}),
        ...Object.keys(setup.notes ?? {}),
        ...Object.keys(setup.unavailable ?? {}),
      ]) {
        expect(tasks, `a task of ${name}`).toContain(named);
      }
      if (Object.keys(setup.push ?? {}).length > 0) {
        expect(setup.persona, `${name}: a task that turns Web Push on needs a persona`).not.toBeNull();
      }
      const seedUsers = (setup.seed ?? []).flatMap((step) => ("api" in step ? [step.api] : []));
      for (const username of [
        setup.persona,
        ...Object.values(setup.personas ?? {}),
        ...Object.values(setup.mail ?? {}),
        ...Object.values(setup.push ?? {}),
        ...seedUsers,
      ]) {
        if (username !== null) {
          expect(usernames, "a person of demo/scenario.ts").toContain(username);
        }
      }
    });
  }
});
