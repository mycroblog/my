// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { dataImportsQuery, previewDataImport, startDataImport, type ImportRequest } from "./imports";
import { fakeData } from "./test/data";
import { refusedWith } from "./test/refused";

const mutes: ImportRequest = { kind: "domain_mutes", mode: "merge", text: "domain\nspam.example\nads.example\n" };

describe("previewDataImport", () => {
  test("counts the entries a file holds, and sends nothing to be carried out", async () => {
    const { fake, data, requests } = fakeData();
    const started = fake.script("POST /imports");
    const preview = await previewDataImport(data, mutes);
    expect(preview).toMatchObject({ entries: 2, removals: 0, skipped: [] });
    expect(requests).toEqual(["POST /imports/preview"]);
    expect(started.seen).toHaveLength(0);
  });

  test("names the lines it skips, by line, with the reason", async () => {
    const { data } = fakeData();
    const preview = await previewDataImport(data, {
      ...mutes,
      text: "domain\nspam.example\nnot a domain\nSPAM.example\n\nads.example",
    });
    expect(preview.entries).toBe(2);
    expect(preview.skipped).toEqual([
      { line: 3, value: "not a domain", reason: "invalid" },
      { line: 4, value: "SPAM.example", reason: "duplicate" },
    ]);
  });

  test("overwriting says how many of the reader's present entries it would remove", async () => {
    const { data } = fakeData();
    expect((await previewDataImport(data, { ...mutes, mode: "overwrite" })).removals).toBeGreaterThan(0);
    expect((await previewDataImport(data, { ...mutes, mode: "merge" })).removals).toBe(0);
  });
});

describe("startDataImport", () => {
  test("starts an import that is in progress, and the list of imports holds it first", async () => {
    const { fake, data, requests } = fakeData();
    const sent = fake.script("POST /imports");
    const before = await data.queryClient.query(dataImportsQuery(data));
    expect(before.items.map((i) => i.state)).toEqual(["finished"]);
    const started = await startDataImport(data, mutes);
    expect(sent.seen.map((s) => s.body)).toEqual([mutes]);
    expect(requests.filter((r) => !r.startsWith("GET "))).toEqual(["POST /imports"]);
    expect(started).toMatchObject({ kind: "domain_mutes", mode: "merge", state: "in_progress", total: 2 });
    const after = await data.queryClient.query({ ...dataImportsQuery(data), staleTime: Infinity });
    expect(after.items.map((i) => i.id)).toEqual([started?.id ?? "", before.items[0]?.id ?? ""]);
    expect(after.items.map((i) => i.state)).toEqual(["in_progress", "finished"]);
  });

  test("while one runs, another is refused with the problem that says so, and the list keeps the one", async () => {
    const { data } = fakeData();
    await startDataImport(data, mutes);
    const refused = await refusedWith(startDataImport(data, { ...mutes, kind: "blocks", text: "a@b.example" }));
    expect([refused.status, refused.type]).toEqual([409, "/api/v1/problems/import-running"]);
    const { items } = await data.queryClient.query({ ...dataImportsQuery(data), staleTime: Infinity });
    expect(items.filter((i) => i.state === "in_progress")).toHaveLength(1);
  });
});

describe("dataImportsQuery", () => {
  test("lists the imports newest first, with what each processed and failed", async () => {
    const { data } = fakeData();
    const { items } = await data.queryClient.query(dataImportsQuery(data));
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({ kind: "following", state: "finished", total: 120, processed: 120, failed: 3 });
  });
});
