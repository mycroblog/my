// SPDX-License-Identifier: AGPL-3.0-only

// The aliases of a custom emoji as an administrator writes them in one field: split at spaces and commas, the colons
// around each taken off, each once.

import { describe, expect, test } from "bun:test";
import { aliasesOf } from "./admin-custom-emojis";

describe("aliasesOf", () => {
  for (const [text, aliases] of [
    ["", []],
    ["cat", ["cat"]],
    [":cat: :dog:", ["cat", "dog"]],
    ["::cat::,dog、 cat", ["cat", "dog"]],
    ["a:b", ["a:b"]],
    [":::", []],
  ] as const)
    test(`"${text}" names ${JSON.stringify(aliases)}`, () => {
      expect(aliasesOf(text)).toEqual([...aliases]);
    });

  test("a long run of colons takes time linear in its length", () => {
    const start = performance.now();
    expect(aliasesOf(`a${":".repeat(50_000)}a`)).toEqual([`a${":".repeat(50_000)}a`]);
    expect(performance.now() - start).toBeLessThan(100);
  });
});
