// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { formatjs } from "./format";

const file = `${JSON.stringify(
  {
    "disclosure.show": { defaultMessage: "Show", description: "The button that shows." },
    "post.replies": { defaultMessage: "{count, plural, one {# reply} other {# replies}}" },
  },
  null,
  2,
)}\n`;
const ctx = (locale: string) => ({ locale, sourceLocale: "en", filename: `${locale}/disclosure.json` });

test("the key is the message id; the source locale's message is its translation as well", async () => {
  expect(await formatjs.parse(file, ctx("en"))).toEqual({
    "disclosure.show": { message: "Show", translation: "Show", comments: ["The button that shows."] },
    "post.replies": {
      message: "{count, plural, one {# reply} other {# replies}}",
      translation: "{count, plural, one {# reply} other {# replies}}",
      comments: [],
    },
  });
  expect((await formatjs.parse(file, ctx("ja")))["disclosure.show"]).toEqual({
    translation: "Show",
    comments: ["The button that shows."],
  });
});

test("it writes back the file it read", async () => {
  for (const locale of ["en", "ja"]) {
    const catalog = await formatjs.parse(file, ctx(locale));
    expect(await formatjs.serialize(catalog, { ...ctx(locale), existing: file })).toBe(file);
  }
});

test("it refuses an entry outside the shared format", () => {
  expect(() => formatjs.parse('{"a.b": {"message": "x"}}', ctx("en"))).toThrow(
    'en/disclosure.json a.b: not {"defaultMessage", "description"}',
  );
});
