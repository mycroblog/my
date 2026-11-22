// SPDX-License-Identifier: AGPL-3.0-only

// The refusal of an approval of an attendee who organizes the event (ADR-0161 §6): the server answers 422
// `/api/v1/problems/event-attendee-organizes` to an approval given or taken back of the organizer or a co-organizer, and
// the app's data names it.

import { describe, expect, test } from "bun:test";
import { eventRefusal, setEventApproval } from "./events";
import { fakeData } from "./test/data";

/** What `write` threw; a write that did not throw fails the test. */
async function thrown(write: () => Promise<unknown>): Promise<unknown> {
  try {
    await write();
  } catch (error) {
    return error;
  }
  throw new Error("test: the write was not refused");
}

describe("eventRefusal", () => {
  for (const [what, approved] of [
    ["an approval", true],
    ["an approval taken back", false],
  ] as const)
    test(`names ${what} of the event's organizer "attendee-organizes"`, async () => {
      const { data, fake } = fakeData();
      // sam organizes the class the reader co-organizes.
      const id = fake.events.id("bread-class");
      const error = await thrown(() => setEventApproval(data, id, fake.accountId("sam"), approved));
      expect(eventRefusal(error)).toBe("attendee-organizes");
    });

  test('names an approval of a co-organizer, the reader themselves, "attendee-organizes"', async () => {
    const { data, fake } = fakeData();
    const id = fake.events.id("bread-class");
    const error = await thrown(() => setEventApproval(data, id, fake.accountId("yuki"), true));
    expect(eventRefusal(error)).toBe("attendee-organizes");
  });
});
