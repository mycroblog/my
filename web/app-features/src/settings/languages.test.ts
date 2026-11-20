// SPDX-License-Identifier: AGPL-3.0-only

// The languages Display and posting offers for a post and for the timelines (#3320): each name once, on a platform
// whose CLDR takes Twi for Akan as on one that names Twi apart.

import { afterEach, describe, expect, test } from "bun:test";
import { languageOptions } from "./languages";

const platform = Intl.DisplayNames;

/** Puts `names` in place of the platform's language names. */
const nameWith = (names: typeof Intl.DisplayNames) => {
  Object.defineProperty(Intl, "DisplayNames", { value: names, configurable: true, writable: true });
};

afterEach(() => {
  nameWith(platform);
});

/** The names of `options` that appear more than once. */
function repeated(options: readonly { label: string }[]): string[] {
  const labels = options.map((o) => o.label);
  return labels.filter((label, i) => labels.indexOf(label) !== i);
}

describe("languageOptions", () => {
  test("names each language once", () => {
    expect(repeated(languageOptions("en"))).toEqual([]);
  });

  test("names each language once where the platform names Twi as Akan", () => {
    // CLDR 46 and later make tw an alias of ak, as Chromium's ICU does.
    class AkanForTwi extends platform {
      override of(code: string): string | undefined {
        return super.of(code === "tw" ? "ak" : code);
      }
    }
    nameWith(AkanForTwi);
    const options = languageOptions("de");
    expect(repeated(options)).toEqual([]);
    expect(options.filter((o) => o.label === "Akan").map((o) => o.value)).toEqual(["ak"]);
  });
});
