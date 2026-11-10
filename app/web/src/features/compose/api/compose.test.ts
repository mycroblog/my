// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import { type QuotePolicy, quotePolicyChange, quotePolicyOf } from "./compose";

describe("quotePolicyOf", () => {
  test.each([
    ["everyone", "public"],
    ["followers", "followers"],
    ["nobody", "nobody"],
    ["mentioned", "nobody"],
  ] as const)("reads a quote approved at once for %s as %s", (automatic, policy) => {
    expect(quotePolicyOf({ automatic })).toBe(policy);
  });
});

describe("quotePolicyChange", () => {
  test.each([
    ["public", "everyone"],
    ["followers", "followers"],
    ["nobody", "nobody"],
  ] as const)("sets the quote rule alone for %s", (policy, automatic) => {
    expect(quotePolicyChange(policy)).toEqual({ quote: { automatic } });
  });

  test.each<QuotePolicy>(["public", "followers", "nobody"])("reads %s back", (policy) => {
    const { quote } = quotePolicyChange(policy);
    expect(quote === undefined ? undefined : quotePolicyOf(quote)).toBe(policy);
  });
});
