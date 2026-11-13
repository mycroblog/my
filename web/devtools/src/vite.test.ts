// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createServer } from "vite";
import { feedbackDir, readBatches } from "./store";
import { devtools } from "./vite";

let root = "";
beforeEach(() => {
  root = realpathSync(mkdtempSync(join(tmpdir(), "devtools-")));
});
afterEach(() => {
  rmSync(root, { recursive: true, force: true });
});

describe("vite", () => {
  test("the dev server records its URL while it listens", async () => {
    const server = await createServer({
      root,
      configFile: false,
      logLevel: "silent",
      plugins: [devtools()],
      server: { host: "127.0.0.1", port: 0, ws: false },
    });
    const file = join(feedbackDir(root), "server.json");
    await server.listen();
    const { url } = JSON.parse(readFileSync(file, "utf8")) as { url: string };
    expect(url).toBe(server.resolvedUrls?.local[0] ?? "");
    expect(url).toMatch(/^http:\/\/127\.0\.0\.1:\d+\/$/);
    expect(readBatches(root)).toEqual([]);
    await server.close();
    expect(existsSync(file)).toBe(false);
  });

  test("the dev server serves the last of two saves in a row, though the page fetched the module between them", async () => {
    const module = join(root, "a.ts");
    writeFileSync(module, "export const v = 0;\n");
    const server = await createServer({
      root,
      configFile: false,
      logLevel: "silent",
      plugins: [devtools()],
      server: { host: "127.0.0.1", port: 0, ws: false },
    });
    await server.listen();
    const base = server.resolvedUrls?.local[0] ?? "";
    // What the server serves of the module once it shows `want`, or after a second.
    const served = async (want: string) => {
      let text = "";
      for (let i = 0; i < 200 && !text.includes(want); i++) {
        if (i > 0) await Bun.sleep(5);
        text = await fetch(`${base}a.ts`).then((r) => r.text());
      }
      return text;
    };
    try {
      expect(await served("v = 0")).toContain("v = 0");
      writeFileSync(module, "export const v = 1;\n");
      // The page fetches the module as soon as the server takes the first save; the second save follows at once.
      expect(await served("v = 1")).toContain("v = 1");
      writeFileSync(module, "export const v = 2;\n");
      expect(await served("v = 2")).toContain("v = 2");
    } finally {
      await server.close();
    }
  });
});
