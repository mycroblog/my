// SPDX-License-Identifier: AGPL-3.0-only

// The line under the post a thread is about: when it was posted, as a date and a time on the reader's clock, which is
// the device's unless the reader chose a zone. The device's zone is the process's here, so the tests set it and put
// it back.

import { afterAll, beforeAll, expect, test } from "bun:test";
import { TimeZoneProvider } from "@my/design-system";
import { cleanup, screen } from "@testing-library/react";
import { PostDetails } from "./post-details";
import { inLocale, status } from "./test/post";

const device = process.env.TZ;

beforeAll(() => {
  process.env.TZ = "Asia/Tokyo";
});

afterAll(() => {
  if (device === undefined) delete process.env.TZ;
  else process.env.TZ = device;
});

// 02:00 UTC on Jan 1 is 21:00 on Dec 31 in New York, and 11:00 on Jan 1 in Tokyo.
const posted = "2025-01-01T02:00:00Z";

/** The time in the details of a post of `posted`, for a reader of `locale` in `timeZone`, alone in the document. */
function detailsIn(timeZone: string | undefined, locale = "en") {
  cleanup();
  inLocale(
    <TimeZoneProvider timeZone={timeZone}>
      <PostDetails status={status({ created_at: posted })} onHistory={() => undefined} />
    </TimeZoneProvider>,
    locale,
  );
  return screen.getByRole<HTMLTimeElement>("time");
}

test("the details say when the post was posted on the clock of the reader's zone, in the reader's locale", () => {
  const time = detailsIn("America/New_York");

  expect(time.textContent).toMatch(/^Dec 31, 2024, 9:00\sPM$/);
  // The moment itself is the post's, whatever the zone.
  expect(time.dateTime).toBe(posted);
  expect(detailsIn("America/New_York", "ja").textContent).toBe("2024/12/31 21:00");
});

test("with no zone chosen, the details read on the device's clock", () => {
  expect(detailsIn(undefined).textContent).toMatch(/^Jan 1, 2025, 11:00\sAM$/);
});
