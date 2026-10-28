// SPDX-License-Identifier: AGPL-3.0-only
import { existsSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { personas } from "../demo/scenario.ts";
import { setupOf, setupScreens } from "./screens.ts";
import { parseTasks, screensDir } from "./tasks.ts";

const root = resolve(import.meta.dirname, "../../..");
const usernames = new Set(personas.map((p) => p.username));

describe("the screens' setups", () => {
  for (const screen of setupScreens) {
    it(`${screen} names an L1 file, its tasks, and the people of the demo scenario`, () => {
      const file = join(root, screensDir, `${screen}.md`);
      expect(existsSync(file)).toBe(true);
      const tasks = new Set(parseTasks(screen, readFileSync(file, "utf8")).map((task) => task.task));
      const setup = setupOf(screen);
      for (const named of [
        ...Object.keys(setup.starts ?? {}),
        ...Object.keys(setup.mail ?? {}),
        ...Object.keys(setup.unavailable ?? {}),
      ]) {
        expect(tasks, `a task of ${screen}.md`).toContain(named);
      }
      for (const username of [setup.persona, ...Object.values(setup.mail ?? {})]) {
        if (username !== null) {
          expect(usernames, "a person of demo/scenario.ts").toContain(username);
        }
      }
    });
  }
});
