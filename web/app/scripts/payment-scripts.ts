// SPDX-License-Identifier: AGPL-3.0-only
// Writes .vite/payment-scripts.json, the inventory of every script that the payment entry (pay.html, ADR-0074 §11)
// loads (PCI DSS 6.4.3): from the build manifest, the entry's chunk and the chunks it imports, each with the Subresource
// Integrity of its file, which the server's shell puts on the entry's <script> and each <link rel="modulepreload">,
// and payjp.js, the one script of another site, which PAY.JP serves at one address and updates itself, so it has no
// fixed hash and the page's Content-Security-Policy names its origin instead.
// The page may run no script that is not checked by its hash, so the entry's graph imports nothing on demand: a chunk
// that a dynamic import() loads, and the modulepreload that Vite's helper adds for it, carry no integrity, and the
// browsers the client supports take no import map with integrity (Chrome 127, Firefox 138, Safari 18; the build
// targets Chrome 123, Firefox 120, Safari 17.5). The inventory therefore refuses a graph with a dynamic import, which
// fails `scripts/task.sh web:build`, and so does the Go server (internal/interface/web, PaymentScriptsPath), which also
// takes the integrity from it and refuses a build whose inventory does not list the entry's chunks with their files'
// hashes; each release attaches it (release.yml). The entry shares chunks with the app, so the list changes with the
// app; it costs nothing, being generated.
// Usage: bun web/app/scripts/payment-scripts.ts <build directory>
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

/** Where the inventory goes in the build: hidden, like Vite's manifest, so the server never serves it. */
export const inventoryPath = ".vite/payment-scripts.json";

/** The manifest key of the payment entry. */
export const payEntry = "pay.html";

/** payjp.js, as @my/app-features/pay loads it (pay/payjp.ts, payjpScript). */
export const payjpScript = "https://js.pay.jp/v2/pay.js";

/** A chunk of Vite's manifest (https://vite.dev/guide/backend-integration). */
type Chunk = { file: string; isEntry?: boolean; imports?: string[]; dynamicImports?: string[] };

/** A script of the inventory: its URL, how the page loads it, and the integrity of its file, for this site's. */
export type InventoryScript = {
  url: string;
  /** `entry`: the entry module; `page`: imported with the page; `third party`: of another site. */
  loads: "entry" | "page" | "third party";
  integrity?: string;
  /** Who serves a script of another site. */
  provider?: string;
};

export type Inventory = { entry: string; scripts: InventoryScript[] };

/**
 * The inventory of the payment entry of the build in `dir`, from its manifest and its files. It throws on a chunk of
 * the entry's graph that imports another on demand.
 */
export function paymentScripts(dir: string): Inventory {
  const manifest = JSON.parse(readFileSync(join(dir, ".vite/manifest.json"), "utf8")) as Record<string, Chunk>;
  const entry = manifest[payEntry];
  if (entry?.isEntry !== true) throw new Error(`.vite/manifest.json has no entry ${payEntry}`);
  const loads = new Map<string, InventoryScript["loads"]>();
  const visit = (key: string, how: InventoryScript["loads"]) => {
    const chunk = manifest[key];
    if (chunk === undefined) throw new Error(`.vite/manifest.json imports the missing chunk ${key}`);
    if (loads.has(chunk.file)) return;
    const lazy = chunk.dynamicImports ?? [];
    if (lazy.length > 0) {
      throw new Error(
        `${chunk.file}, which ${payEntry} loads, imports ${lazy.join(", ")} on demand, which no integrity checks: ` +
          "import nothing lazily from the payment pages' modules, or keep such a module out of their graph",
      );
    }
    loads.set(chunk.file, how);
    for (const k of chunk.imports ?? []) visit(k, "page");
  };
  visit(payEntry, "entry");
  const own = [...loads].map(([file, how]): InventoryScript => ({
    url: `/${file}`,
    loads: how,
    integrity: `sha384-${createHash("sha384")
      .update(readFileSync(join(dir, file)))
      .digest("base64")}`,
  }));
  const order = { entry: 0, page: 1, "third party": 2 };
  own.sort((a, b) => order[a.loads] - order[b.loads] || (a.url < b.url ? -1 : a.url > b.url ? 1 : 0));
  return { entry: payEntry, scripts: [...own, { url: payjpScript, loads: "third party", provider: "PAY.JP" }] };
}

/** The inventory as the build holds it: JSON, two spaces, a newline at the end. */
export const inventoryText = (inventory: Inventory) => `${JSON.stringify(inventory, null, 2)}\n`;

if (import.meta.main) {
  const dir = process.argv[2];
  if (dir === undefined) throw new Error("usage: payment-scripts.ts <build directory>");
  writeFileSync(join(dir, inventoryPath), inventoryText(paymentScripts(dir)));
}
