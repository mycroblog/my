// SPDX-License-Identifier: AGPL-3.0-only
// Inventories a small build as `web:build` would: the payment entry's chunks, with the page and on demand, each with the
// integrity of its file, and payjp.js as @my/app-features loads it.

import { afterAll, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { payjpScript } from "@my/app-features";
import { paymentScripts, payjpScript as inventoried } from "./payment-scripts";

const root = mkdtempSync(join(tmpdir(), "payment-scripts-test-"));
afterAll(() => {
  rmSync(root, { recursive: true, force: true });
});

test("the inventory lists the entry's chunks with their integrity and payjp.js", () => {
  mkdirSync(join(root, ".vite"));
  mkdirSync(join(root, "assets"));
  writeFileSync(
    join(root, ".vite/manifest.json"),
    JSON.stringify({
      "pay.html": { file: "assets/pay-A.js", isEntry: true, imports: ["_shared"], dynamicImports: ["lazy.tsx"] },
      _shared: { file: "assets/shared-B.js" },
      "lazy.tsx": { file: "assets/lazy-C.js", imports: ["_shared"] },
      "index.html": { file: "assets/index-D.js", isEntry: true, imports: ["_shared"] },
    }),
  );
  writeFileSync(join(root, "assets/pay-A.js"), "");
  writeFileSync(join(root, "assets/shared-B.js"), "hello\n");
  writeFileSync(join(root, "assets/lazy-C.js"), "");
  writeFileSync(join(root, "assets/index-D.js"), "");

  const empty = "sha384-OLBgp1GsljhM2TJ+sbHjaiH9txEUvgdDTAzHv2P24donTt6/529l+9Ua0vFImLlb";
  expect(paymentScripts(root)).toEqual({
    entry: "pay.html",
    scripts: [
      { url: "/assets/pay-A.js", loads: "entry", integrity: empty },
      {
        url: "/assets/shared-B.js",
        loads: "page",
        integrity: "sha384-HQ8oTv4+3qS5yjvVFPoTSxfq42HMx6Hu/v+AG5vWYE4B8h9r8knvAwWZ8MIY8rqM",
      },
      { url: "/assets/lazy-C.js", loads: "on demand", integrity: empty },
      { url: "https://js.pay.jp/v2/pay.js", loads: "third party", provider: "PAY.JP" },
    ],
  });
});

test("the inventory names payjp.js where the card form loads it from", () => {
  expect(inventoried).toBe(payjpScript);
});
