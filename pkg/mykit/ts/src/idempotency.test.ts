// SPDX-License-Identifier: MIT
import { readFileSync } from "node:fs";
import { describe, expect, test } from "bun:test";
import {
  acceptsIdempotencyKey,
  acceptsIdempotencyKeyAt,
  addsIdempotencyKey,
  idempotencyKeyOperations,
  isWrite,
  writesSentWithoutIdempotencyKey,
  writesWithoutIdempotencyKey,
} from "./idempotency.js";

describe("writesWithoutIdempotencyKey", () => {
  test("every write but those takes a key, by method and spec path", () => {
    expect(acceptsIdempotencyKey("post", "/statuses")).toBe(true);
    expect(acceptsIdempotencyKey("DELETE", "/statuses/{id}")).toBe(true);
    expect(acceptsIdempotencyKey("GET", "/statuses")).toBe(false);
    expect(acceptsIdempotencyKey("POST", "/oauth_apps")).toBe(false);
    expect(acceptsIdempotencyKey("POST", "/oauth_apps/{id}/tokens")).toBe(false);
  });

  test("match a request path, a spec parameter matching one segment", () => {
    expect(acceptsIdempotencyKeyAt("POST", "/statuses")).toBe(true);
    expect(acceptsIdempotencyKeyAt("POST", "/accounts/a1/follow")).toBe(true);
    expect(acceptsIdempotencyKeyAt("POST", "/oauth_apps/a1/tokens")).toBe(false);
    expect(acceptsIdempotencyKeyAt("GET", "/statuses")).toBe(false);
  });

  test("are the writes of schema.gen.ts that take no Idempotency-Key", () => {
    const schema = readFileSync(new URL("schema.gen.ts", import.meta.url), "utf8");
    // The operations whose header parameters have Idempotency-Key, by operationId.
    const operations = schema.slice(schema.indexOf("\nexport interface operations {"));
    const withKey = new Set<string>();
    for (const m of operations.matchAll(/^ {4}(\w+): \{\n([\s\S]*?)^ {4}\};$/gm)) {
      const header = /^ {12}header\?: \{\n([\s\S]*?)^ {12}\};$/m.exec(m[2] ?? "");
      if (header?.[1]?.includes('"Idempotency-Key"') === true) withKey.add(m[1] ?? "");
    }
    // The method and path of each operation, from the paths interface.
    const paths = schema.slice(schema.indexOf("\nexport interface paths {"), schema.indexOf("\nexport type webhooks"));
    const without: { method: string; path: string }[] = [];
    const listed: { method: string; path: string }[] = [];
    for (const m of paths.matchAll(/^ {4}"([^"]+)": \{\n([\s\S]*?)^ {4}\};$/gm)) {
      for (const op of (m[2] ?? "").matchAll(
        /^ {8}(get|put|post|delete|patch|head|options): operations\["(\w+)"\];$/gm,
      )) {
        const method = (op[1] ?? "").toUpperCase();
        const path = m[1] ?? "";
        const takes = withKey.has(op[2] ?? "");
        expect({ method, path, takes: acceptsIdempotencyKey(method, path) }).toEqual({ method, path, takes });
        if (isWrite(method) && !takes) without.push({ method, path });
        if (takes) listed.push({ method, path });
      }
    }
    expect(withKey.size).toBeGreaterThan(0);
    const byPath = (a: { method: string; path: string }, b: { method: string; path: string }) =>
      `${a.path} ${a.method}`.localeCompare(`${b.path} ${b.method}`);
    expect(without.sort(byPath)).toEqual([...writesWithoutIdempotencyKey].sort(byPath));
    // The deprecated list is every operation that takes the key, in the order of the spec.
    // oxlint-disable-next-line typescript/no-deprecated -- the test keeps the deprecated list true until it goes.
    expect([...idempotencyKeyOperations]).toEqual(listed);
  });
});

describe("writesSentWithoutIdempotencyKey", () => {
  test("take a key in the spec, and get none from the client", () => {
    for (const op of writesSentWithoutIdempotencyKey) {
      expect({ ...op, takes: acceptsIdempotencyKey(op.method, op.path) }).toEqual({ ...op, takes: true });
      expect({ ...op, adds: addsIdempotencyKey(op.method, op.path) }).toEqual({ ...op, adds: false });
      // oxlint-disable-next-line typescript/no-deprecated -- the list of the spec's keyed operations.
      expect(idempotencyKeyOperations).toContainEqual(op);
    }
    expect(addsIdempotencyKey("POST", "/statuses")).toBe(true);
    expect(addsIdempotencyKey("POST", "/oauth_apps")).toBe(false);
  });
});
