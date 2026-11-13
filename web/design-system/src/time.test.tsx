// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { RelativeTime, relativeTime } from "./time";

const now = new Date(2026, 10, 13, 15, 0);
const ago = (ms: number) => new Date(now.getTime() - ms);
const s = 1000;
const m = 60 * s;
const h = 60 * m;
const d = 24 * h;
const read = (locale: string, at: Date) => relativeTime(at, now, locale).text;

test("the time counts, in the reader's locale, and never says a word such as yesterday", () => {
  expect(read("en", ago(0))).toBe("1s ago");
  expect(read("en", ago(45 * s))).toBe("45s ago");
  expect(read("ja", ago(45 * s))).toBe("45秒前");
  expect(read("en", ago(3 * m))).toBe("3m ago");
  expect(read("ja", ago(3 * m))).toBe("3分前");
  expect(read("ja", ago(2 * h))).toBe("2時間前");
  expect(read("en", ago(d + m))).toBe("1d ago");
  expect(read("ja", ago(d + m))).toBe("1日前");
  expect(read("en", ago(6 * d))).toBe("6d ago");
});

test("after a week it reads the date, with the year only when it is not the current one", () => {
  expect(read("en", ago(21 * d))).toBe("Oct 23");
  expect(read("ja", ago(21 * d))).toBe("10月23日");
  expect(read("en", new Date(2025, 11, 20))).toBe("Dec 20, 2025");
  expect(read("ja", new Date(2025, 11, 20))).toBe("2025年12月20日");
  expect(relativeTime(ago(21 * d), now, "en").changes).toBe(false);
});

test("the time carries the full date in its datetime attribute and its tooltip", () => {
  const html = renderToStaticMarkup(<RelativeTime date="2026-11-12T09:41:00Z" locale="en" />);
  expect(html).toMatch(/^<time dateTime="2026-11-12T09:41:00.000Z" title="November 12, 2026 at [^"]+" lang="en">/);
});
