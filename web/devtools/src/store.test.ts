// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { NoteInput } from "./note";
import { batchId, feedbackDir, layerOf, readBatches, writeBatch } from "./store";

let root = "";
beforeEach(() => {
  root = mkdtempSync(join(tmpdir(), "devtools-"));
  mkdirSync(join(root, "web", "catalog", "src"), { recursive: true });
  writeFileSync(join(root, "web", "catalog", "package.json"), JSON.stringify({ name: "@my/catalog" }));
});
afterEach(() => {
  rmSync(root, { recursive: true, force: true });
});

const note = (source: string | null): NoteInput => ({
  text: "too tight",
  target: { tag: "h1", selector: "main > h1", text: "Type", rect: { x: 0, y: 0, width: 10, height: 10 }, area: null },
  source,
  sources: [],
  state: {
    page: "/",
    route: null,
    viewport: { width: 800, height: 600, devicePixelRatio: 1 },
    theme: { page: null, system: "light" },
    locale: "en",
    direction: "ltr",
    orientation: null,
    account: null,
  },
  reproduction: { scenario: null, step: null },
  accessibility: 'heading "Type" [level=1]',
});

describe("store", () => {
  test("batchId sorts by time", () => {
    expect(batchId(new Date(2026, 10, 12, 9, 5, 3), () => 0.5)).toBe("20261112-090503-8000");
  });

  test("layerOf names the package of the file", () => {
    expect(layerOf(root, "web/catalog/src/Catalog.tsx")).toBe("@my/catalog");
    expect(layerOf(root, "elsewhere/x.tsx")).toBeNull();
  });

  test("writeBatch numbers the notes, adds their layer, and readBatches reads it back", () => {
    const batch = writeBatch(root, { page: "/", notes: [note("web/catalog/src/Catalog.tsx:3"), note(null)] });
    expect(batch.notes.map((n) => [n.id, n.layer])).toEqual([
      [`${batch.id}.1`, "@my/catalog"],
      [`${batch.id}.2`, null],
    ]);
    const file = join(feedbackDir(root), `${batch.id}.json`);
    expect(JSON.parse(readFileSync(file, "utf8"))).toEqual(batch);
    writeFileSync(join(feedbackDir(root), "broken.json"), "{");
    expect(readBatches(root)).toEqual([batch]);
  });
});
