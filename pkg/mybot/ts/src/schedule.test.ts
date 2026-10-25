// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { parseSchedule, ScheduleError } from "./schedule.js";

/** pkg/mybot/testdata/cron.json, the cases that the Go mybot passes too (ADR-0094 §5). */
type CronFixture = {
  next: { name: string; expr: string; tz: string; from: string; next: string[] }[];
  prev: { name: string; expr: string; tz: string; at: string; prev: string }[];
  invalid: { expr: string; why: string }[];
};

const fixture = JSON.parse(readFileSync(new URL("../../testdata/cron.json", import.meta.url), "utf8")) as CronFixture;

/** RFC 3339 in UTC without fractional seconds, as the fixture writes the times. */
function rfc3339(at: number | null): string {
  return at === null ? "none" : new Date(at).toISOString().replace(".000Z", "Z");
}

describe("the shared cron fixture", () => {
  for (const c of fixture.next) {
    test(`next: ${c.name}`, () => {
      const s = parseSchedule(c.expr, c.tz);
      let at: number | null = Date.parse(c.from);
      const got: string[] = [];
      for (let i = 0; i < c.next.length && at !== null; i++) {
        at = s.next(at);
        got.push(rfc3339(at));
      }
      expect(got).toEqual(c.next);
    });
  }

  for (const c of fixture.prev) {
    test(`prev: ${c.name}`, () => {
      expect(rfc3339(parseSchedule(c.expr, c.tz).prev(Date.parse(c.at)))).toBe(c.prev);
    });
  }

  for (const c of fixture.invalid) {
    test(`refuses ${c.why}: ${JSON.stringify(c.expr)}`, () => {
      expect(() => parseSchedule(c.expr)).toThrow(ScheduleError);
    });
  }
});

describe("parseSchedule", () => {
  test("refuses a time zone that is no IANA name", () => {
    for (const tz of ["Local", "Mars/Olympus"]) expect(() => parseSchedule("0 0 * * *", tz)).toThrow(ScheduleError);
  });

  test("names a field of seconds and an alias in its errors", () => {
    expect(() => parseSchedule("0 0 * * * *")).toThrow(/seconds/);
    expect(() => parseSchedule("@daily")).toThrow(/alias/);
  });

  test("is UTC without a time zone", () => {
    expect(parseSchedule("0 9 * * *").timeZone).toBe("UTC");
  });

  // Every time of a schedule is the prev of itself, and the prev of the second before it is the time before.
  test("next and prev agree", () => {
    const s = parseSchedule("*/7 1-3 * * *", "America/New_York");
    let at = s.prev(Date.parse("2026-10-30T00:00:00Z"));
    if (at === null) throw new Error("no prev");
    for (let i = 0; i < 300; i++) {
      const next = s.next(at);
      if (next === null || next <= at) throw new Error(`next(${rfc3339(at)}) = ${rfc3339(next)}`);
      expect(rfc3339(s.prev(next))).toBe(rfc3339(next));
      expect(rfc3339(s.prev(next - 1000))).toBe(rfc3339(at));
      at = next;
    }
  });
});
