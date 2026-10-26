// SPDX-License-Identifier: AGPL-3.0-only
// The pure parts of the room form (web-client/README.md T4): reading the hashtags typed in its field, and which
// visibilities have a start post (ADR-0100 §9, 回答 13).
import { describe, expect, test } from "vitest";
import { characterCount, isValidRoomName, offersStartPost, parseHashtags } from "./useVoiceRoomForm";

describe("parseHashtags", () => {
  test("splits on spaces and commas, drops the # (also full-width), and keeps each tag once", () => {
    expect(parseHashtags(" #music  talk,#music ＃猫、jazz ")).toEqual(["music", "talk", "猫", "jazz"]);
  });

  test("keeps each hashtag once by its normalized name, in its first spelling", () => {
    expect(parseHashtags("#Music #music #ＭＵＳＩＣ")).toEqual(["Music"]);
  });

  test("reads an empty field as no hashtags", () => {
    expect(parseHashtags("  # ")).toEqual([]);
  });
});

describe("offersStartPost", () => {
  test("offers the post only for the visibilities that a post can match", () => {
    expect(offersStartPost("local")).toBe(true);
    expect(offersStartPost("followers")).toBe(true);
    expect(offersStartPost("mutuals")).toBe(false);
    expect(offersStartPost("invited")).toBe(false);
  });
});

describe("isValidRoomName", () => {
  test("counts what a reader sees as one character, as the server does", () => {
    const family = "👨‍👩‍👧‍👦";
    expect(characterCount(family)).toBe(7);
    expect(isValidRoomName(family.repeat(100))).toBe(true);
    expect(isValidRoomName(family.repeat(101))).toBe(false);
    expect(isValidRoomName("a".repeat(100))).toBe(true);
    expect(isValidRoomName("a".repeat(101))).toBe(false);
  });

  test("refuses an empty name and control characters", () => {
    expect(isValidRoomName("")).toBe(false);
    expect(isValidRoomName("Late\nshow")).toBe(false);
    expect(isValidRoomName("Late\tshow")).toBe(false);
  });
});
