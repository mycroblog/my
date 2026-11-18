// SPDX-License-Identifier: AGPL-3.0-only

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { TimeZoneProvider } from "@my/design-system";
import { cleanup, render, screen } from "@testing-library/react";
import { I18nProvider } from "react-aria-components";
import { RelativeTime } from "./time";

test("a time carries its moment for machines and the full date for a reader who asks, in its language", () => {
  render(<RelativeTime date="2025-12-20T09:41:00Z" locale="en" />);

  const time = screen.getByRole("time");
  expect(time.getAttribute("datetime")).toBe("2025-12-20T09:41:00.000Z");
  // The full date, at the reader's own hour.
  expect(time.title).toMatch(/^December 20, 2025 at \d{1,2}:\d{2}/);
  expect(time.textContent).toBe("Dec 20, 2025");
  expect(time.lang).toBe("en");
});

test("a time takes a Date as it takes a string", () => {
  render(<RelativeTime date={new Date("2025-12-20T09:41:00Z")} locale="en" />);

  expect(screen.getByRole("time").getAttribute("datetime")).toBe("2025-12-20T09:41:00.000Z");
});

test("a time reads in the I18nProvider's locale, and in its own over it", () => {
  render(
    <I18nProvider locale="ja">
      <p>
        <RelativeTime date="2025-12-20T09:41:00Z" />
      </p>
      <p>
        <RelativeTime date="2025-12-20T09:41:00Z" locale="en" />
      </p>
    </I18nProvider>,
  );

  const [provided, own] = screen.getAllByRole("time");
  expect([provided?.textContent, provided?.lang]).toEqual(["2025年12月20日", "ja"]);
  expect(provided?.title).toMatch(/^2025年12月20日/);
  expect([own?.textContent, own?.lang]).toEqual(["Dec 20, 2025", "en"]);
});

describe("in the reader's time zone", () => {
  const device = process.env.TZ;

  beforeAll(() => {
    process.env.TZ = "Asia/Tokyo";
  });

  afterAll(() => {
    if (device === undefined) delete process.env.TZ;
    else process.env.TZ = device;
  });

  /** The time of `date` under a provider of `timeZone`, alone in the document. */
  function timeIn(timeZone: string | undefined, date: string) {
    cleanup();
    render(
      <TimeZoneProvider timeZone={timeZone}>
        <RelativeTime date={date} locale="en" />
      </TimeZoneProvider>,
    );
    return screen.getByRole("time");
  }

  test("a time reads its date and its full date in the zone its provider gives", () => {
    // 02:00 UTC on Jan 1, 2025 is 21:00 on Dec 31 in New York, and 11:00 on Jan 1 in Tokyo, the device's zone.
    const newYork = timeIn("America/New_York", "2025-01-01T02:00:00Z");

    expect(newYork.textContent).toBe("Dec 31, 2024");
    expect(newYork.title).toMatch(/^December 31, 2024 at 9:00\sPM/);
    // The moment is the same for machines.
    expect(newYork.getAttribute("datetime")).toBe("2025-01-01T02:00:00.000Z");
  });

  test("with no zone given, or one the platform does not know, a time reads in the device's", () => {
    for (const timeZone of [undefined, "Mars/Olympus_Mons"]) {
      const time = timeIn(timeZone, "2025-01-01T02:00:00Z");

      expect(time.textContent).toBe("Jan 1, 2025");
      expect(time.title).toMatch(/^January 1, 2025 at 11:00\sAM/);
    }
  });
});
