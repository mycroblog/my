// SPDX-License-Identifier: AGPL-3.0-only
// The pure parts of the room form (web-client/README.md T4): reading the hashtags typed in its field, and which
// visibilities have a start post (ADR-0100 §9, 回答 13).
import { describe, expect, test } from "vitest";
import { offersStartPost, parseHashtags } from "./useVoiceRoomForm";

describe("parseHashtags", () => {
  test("splits on spaces and commas, drops the # (also full-width), and keeps each tag once", () => {
    expect(parseHashtags(" #music  talk,#music ＃猫、jazz ")).toEqual(["music", "talk", "猫", "jazz"]);
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
