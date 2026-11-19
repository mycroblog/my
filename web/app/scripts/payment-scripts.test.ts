// SPDX-License-Identifier: AGPL-3.0-only
// Inventories a small build as `web:build` would: the payment entry's chunks, each with the integrity of its file, and
// payjp.js as @my/app-features/pay loads it; and refuses a graph that imports a chunk on demand.

import { afterAll, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { payjpScript } from "@my/app-features/pay";
import { inventoryPath, inventoryText, paymentScripts, payjpScript as inventoried } from "./payment-scripts";

const root = mkdtempSync(join(tmpdir(), "payment-scripts-test-"));
afterAll(() => {
  rmSync(root, { recursive: true, force: true });
});

/** A build in a directory of its own with `manifest` and an empty file of each chunk but `assets/shared-B.js`. */
function build(name: string, manifest: Record<string, object>) {
  const dir = join(root, name);
  mkdirSync(join(dir, ".vite"), { recursive: true });
  mkdirSync(join(dir, "assets"));
  writeFileSync(join(dir, ".vite/manifest.json"), JSON.stringify(manifest));
  for (const chunk of Object.values(manifest)) writeFileSync(join(dir, (chunk as { file: string }).file), "");
  writeFileSync(join(dir, "assets/shared-B.js"), "hello\n");
  return dir;
}

test("the inventory lists the entry's chunks with their integrity and payjp.js", () => {
  const dir = build("sound", {
    "pay.html": { file: "assets/pay-A.js", isEntry: true, imports: ["_shared"] },
    _shared: { file: "assets/shared-B.js" },
    "index.html": { file: "assets/index-D.js", isEntry: true, imports: ["_shared"], dynamicImports: ["lazy.tsx"] },
    "lazy.tsx": { file: "assets/lazy-C.js", imports: ["_shared"] },
  });
  expect(paymentScripts(dir)).toEqual({
    entry: "pay.html",
    scripts: [
      {
        url: "/assets/pay-A.js",
        loads: "entry",
        integrity: "sha384-OLBgp1GsljhM2TJ+sbHjaiH9txEUvgdDTAzHv2P24donTt6/529l+9Ua0vFImLlb",
      },
      {
        url: "/assets/shared-B.js",
        loads: "page",
        integrity: "sha384-HQ8oTv4+3qS5yjvVFPoTSxfq42HMx6Hu/v+AG5vWYE4B8h9r8knvAwWZ8MIY8rqM",
      },
      { url: "https://js.pay.jp/v2/pay.js", loads: "third party", provider: "PAY.JP" },
    ],
  });
});

test("a chunk of the entry's graph that imports another on demand fails the inventory", () => {
  const dir = build("lazy", {
    "pay.html": { file: "assets/pay-A.js", isEntry: true, imports: ["_shared"] },
    _shared: { file: "assets/shared-B.js", dynamicImports: ["lazy.tsx"] },
    "lazy.tsx": { file: "assets/lazy-C.js" },
  });
  expect(() => paymentScripts(dir)).toThrow("assets/shared-B.js, which pay.html loads, imports lazy.tsx on demand");
});

test("every chunk the entry reaches through other chunks is listed once", () => {
  // pay.html → _a → _shared and pay.html → _b → _shared, and _b imports the entry back.
  const dir = build("deep", {
    "pay.html": { file: "assets/pay-A.js", isEntry: true, imports: ["_a", "_b"] },
    _a: { file: "assets/a-E.js", imports: ["_shared"] },
    _b: { file: "assets/b-F.js", imports: ["_shared", "pay.html"] },
    _shared: { file: "assets/shared-B.js" },
  });
  const { scripts } = paymentScripts(dir);
  expect(scripts.map((script) => `${script.url} ${script.loads}`).toSorted()).toEqual([
    "/assets/a-E.js page",
    "/assets/b-F.js page",
    "/assets/pay-A.js entry",
    "/assets/shared-B.js page",
    "https://js.pay.jp/v2/pay.js third party",
  ]);
  for (const script of scripts)
    expect([script.url, script.loads === "third party" ? "" : script.integrity?.slice(0, 7)]).toEqual([
      script.url,
      script.loads === "third party" ? "" : "sha384-",
    ]);
});

test("a lazy import fails the inventory wherever it stands in the entry's graph", () => {
  const lazy = { file: "assets/lazy-C.js" };
  for (const [name, manifest, chunk] of [
    [
      "entry-lazy",
      { "pay.html": { file: "assets/pay-A.js", isEntry: true, dynamicImports: ["lazy.tsx"] }, "lazy.tsx": lazy },
      "assets/pay-A.js",
    ],
    [
      "deep-lazy",
      {
        "pay.html": { file: "assets/pay-A.js", isEntry: true, imports: ["_a"] },
        _a: { file: "assets/a-E.js", imports: ["_shared"] },
        _shared: { file: "assets/shared-B.js", dynamicImports: ["lazy.tsx"] },
        "lazy.tsx": lazy,
      },
      "assets/shared-B.js",
    ],
  ] as const)
    expect(() => paymentScripts(build(name, manifest))).toThrow(
      `${chunk}, which pay.html loads, imports lazy.tsx on demand`,
    );
});

test("a build without the payment entry has no inventory", () => {
  const dir = build("no-entry", { "index.html": { file: "assets/index-D.js", isEntry: true } });
  expect(() => paymentScripts(dir)).toThrow("has no entry pay.html");
});

test("the inventory's file reads back as the inventory", () => {
  const inventory = paymentScripts(
    build("text", {
      "pay.html": { file: "assets/pay-A.js", isEntry: true, imports: ["_shared"] },
      _shared: { file: "assets/shared-B.js" },
    }),
  );
  expect(inventoryPath).toBe(".vite/payment-scripts.json");
  expect(JSON.parse(inventoryText(inventory))).toEqual(inventory);
});

test("the inventory names payjp.js where the card form loads it from", () => {
  expect(inventoried).toBe(payjpScript);
});
