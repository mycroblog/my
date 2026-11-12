// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync } from "node:fs";
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
});
