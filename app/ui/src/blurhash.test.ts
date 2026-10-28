// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from "vitest";
import { decodeBlurhash } from "./blurhash";

describe("decodeBlurhash", () => {
  it("fills every pixel with the average color of a hash without variations", () => {
    // 1 × 1 component, the average color #ff0000.
    const pixels = decodeBlurhash("00TI:j", 2, 2);
    expect(Array.from(pixels ?? [])).toEqual([255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255]);
  });

  it("varies the pixels around the average color", () => {
    const pixels = decodeBlurhash("LEHV6nWB2yk8pyo0adR*.7kCMdnj", 32, 32);
    expect(pixels).toHaveLength(32 * 32 * 4);
    const reds = new Set(Array.from(pixels ?? []).filter((_, i) => i % 4 === 0));
    expect(reds.size).toBeGreaterThan(10);
    expect(Array.from(pixels ?? []).filter((_, i) => i % 4 === 3)).toEqual(Array.from({ length: 32 * 32 }, () => 255));
  });

  it.each([
    "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
    "LGF5]+Yk^6#M@-5c,1J5@[or[Q6.",
    "L6PZfSi_.AyE_3t7t7R**0o#DgR4",
    "LKN]Rv%2Tw=w]~RBVZRi};RPxuwH",
    "KJG8_@Dgx]_4V?xuyE%NRj",
  ])("decodes %s", (hash) => {
    expect(decodeBlurhash(hash, 4, 4)).toHaveLength(4 * 4 * 4);
  });

  it.each([
    ["too short", "00TI"],
    ["a length that does not match the components", "LEHV6nWB2yk8pyo0adR*.7kCMdn"],
    ["a character outside base 83", '00TI"j'],
  ])("rejects %s", (_, hash) => {
    expect(decodeBlurhash(hash, 4, 4)).toBeNull();
  });
});
