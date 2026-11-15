// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from "bun:test";
import { loadUnicodeEmoji, loadUnicodeEmojiNames, unicodeEmojiLocale } from ".";

describe("unicodeEmojiLocale", () => {
  it.each([
    ["en", "en"],
    ["ja", "ja"],
    ["ja-JP", "ja"],
    ["en-XA", "en"],
    ["ar-XB", "en"],
    ["fr", "en"],
    ["zh-Hant-TW", "en"],
  ])("shows the names of %s in %s", (locale, want) => {
    expect<string>(unicodeEmojiLocale(locale)).toBe(want);
  });
});

describe("loadUnicodeEmoji", () => {
  it("loads the nine groups of emoji-test.txt with their skin tone variants", async () => {
    const data = await loadUnicodeEmoji();
    expect(data.groups.map((g) => g.id)).toEqual([
      "smileys-emotion",
      "people-body",
      "animals-nature",
      "food-drink",
      "travel-places",
      "activities",
      "objects",
      "symbols",
      "flags",
    ]);
    const waving = data.groups[1]?.emoji.find(([emoji]) => emoji === "👋");
    expect(waving?.[2]?.map(([emoji]) => emoji)).toEqual(["👋🏻", "👋🏼", "👋🏽", "👋🏾", "👋🏿"]);
  });
});

describe("loadUnicodeEmojiNames", () => {
  it("names every emoji in Japanese, the fully-qualified sequence included", async () => {
    const [data, names] = await Promise.all([loadUnicodeEmoji(), loadUnicodeEmojiNames("ja-JP")]);
    for (const group of data.groups) {
      for (const [emoji, , skins] of group.emoji) {
        expect(names.emoji[emoji]?.[0], emoji).toBeTruthy();
        for (const [variant] of skins ?? []) expect(names.skins[variant], variant).toBeTruthy();
      }
    }
    // CLDR keys ☺️ without VS16; the data keys the fully-qualified sequence.
    expect(names.emoji["☺️"]?.[0]).toBe("スマイリー");
    expect(names.tones["🏻"]).toBe("薄い肌色");
  });

  it("falls back to the English names for a pseudo-locale", async () => {
    const names = await loadUnicodeEmojiNames("ar-XB");
    expect(names.emoji["😀"]?.[0]).toBe("grinning face");
  });
});
