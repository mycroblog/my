// SPDX-License-Identifier: MIT
import { readFileSync } from "node:fs";
import { describe, expect, test } from "bun:test";
import { acceptsIdempotencyKey, acceptsIdempotencyKeyAt, idempotencyKeyOperations } from "./idempotency.js";

describe("idempotencyKeyOperations", () => {
  test("match by method and spec path", () => {
    expect(acceptsIdempotencyKey("post", "/statuses")).toBe(true);
    expect(acceptsIdempotencyKey("GET", "/statuses")).toBe(false);
    expect(acceptsIdempotencyKey("POST", "/statuses/{id}/reblog")).toBe(false);
  });

  test("match a request path, a spec parameter matching one segment", () => {
    expect(acceptsIdempotencyKeyAt("POST", "/statuses")).toBe(true);
    expect(acceptsIdempotencyKeyAt("POST", "/statuses/")).toBe(false);
    expect(acceptsIdempotencyKeyAt("POST", "/accounts/a1/follow")).toBe(false);
    expect(acceptsIdempotencyKeyAt("GET", "/statuses")).toBe(false);
  });

  test("lists the operations of schema.gen.ts that take an Idempotency-Key", () => {
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
    const listed: { method: string; path: string }[] = [];
    for (const m of paths.matchAll(/^ {4}"([^"]+)": \{\n([\s\S]*?)^ {4}\};$/gm)) {
      for (const op of (m[2] ?? "").matchAll(
        /^ {8}(get|put|post|delete|patch|head|options): operations\["(\w+)"\];$/gm,
      )) {
        if (withKey.has(op[2] ?? "")) listed.push({ method: (op[1] ?? "").toUpperCase(), path: m[1] ?? "" });
      }
    }
    expect(withKey.size).toBeGreaterThan(0);
    expect(listed).toEqual([...idempotencyKeyOperations]);
  });
});
