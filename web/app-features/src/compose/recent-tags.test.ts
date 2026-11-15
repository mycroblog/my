// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { recentTags, rememberTags } from "./recent-tags";

const key = "my:compose:recent-tags";

/** A device's storage that keeps what it is given; Bun has none of its own. */
function deviceStorage(kept = new Map<string, string>()): Storage {
  return {
    get length() {
      return kept.size;
    },
    clear: () => {
      kept.clear();
    },
    getItem: (k) => kept.get(k) ?? null,
    key: (i) => Array.from(kept.keys())[i] ?? null,
    removeItem: (k) => {
      kept.delete(k);
    },
    setItem: (k, v) => {
      kept.set(k, v);
    },
  };
}

/** A device that keeps nothing: every use of its storage throws, as a blocked one does. */
function blockedStorage(): Storage {
  const refuse = () => {
    throw new DOMException("The storage is blocked", "SecurityError");
  };
  return { length: 0, clear: refuse, getItem: refuse, key: refuse, removeItem: refuse, setItem: refuse };
}

afterEach(() => {
  Reflect.deleteProperty(globalThis, "localStorage");
});

describe("recentTags and rememberTags", () => {
  beforeEach(() => {
    globalThis.localStorage = deviceStorage();
  });

  test("a device that posted nothing knows no hashtag", () => {
    expect(recentTags()).toEqual([]);
  });

  test("keeps the hashtags of a post without their #, the latest post's first", () => {
    rememberTags("Bread #baking #sourdough");
    expect(recentTags()).toEqual(["baking", "sourdough"]);
    rememberTags("A walk #写真 and more #baking");
    expect(recentTags()).toEqual(["写真", "baking", "sourdough"]);
  });

  test("keeps a hashtag once whatever its case, as it was posted last", () => {
    rememberTags("#Go");
    rememberTags("#go #rust");
    expect(recentTags()).toEqual(["go", "rust"]);
  });

  test("keeps the 16 latest", () => {
    for (let i = 1; i <= 20; i += 1) rememberTags(`post #tag${String(i)}`);
    expect(recentTags()).toEqual(Array.from({ length: 16 }, (_, i) => `tag${String(20 - i)}`));
  });

  test("a post without a hashtag changes nothing; a mention, a URL or a shortcode is no hashtag", () => {
    rememberTags("#baking");
    rememberTags("@sam see https://example.com/#anchor :wave:");
    expect(recentTags()).toEqual(["baking"]);
  });

  test("reads what an earlier load of the app kept, and nothing from a value it cannot read", () => {
    localStorage.setItem(key, JSON.stringify(["baking", "写真"]));
    expect(recentTags()).toEqual(["baking", "写真"]);
    localStorage.setItem(key, "{not json");
    expect(recentTags()).toEqual([]);
    localStorage.setItem(key, JSON.stringify({ baking: 1 }));
    expect(recentTags()).toEqual([]);
  });
});

test("a device that keeps nothing knows no hashtag, and posting one does not fail", () => {
  globalThis.localStorage = blockedStorage();
  expect(() => {
    rememberTags("#baking");
  }).not.toThrow();
  expect(recentTags()).toEqual([]);
});
