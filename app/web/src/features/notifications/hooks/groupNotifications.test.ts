// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import { accountFixture, statusFixture } from "../../../test/fixtures";
import type { Notification, NotificationType } from "../api/notifications";
import { groupNotifications } from "./groupNotifications";

const postA = statusFixture("0192f0c4-7a8e-7000-8000-0000000000b1");
const postB = statusFixture("0192f0c4-7a8e-7000-8000-0000000000b2");

/** Notification `n` (a higher `n` is older) of `type`, about `status`, or about no post the user may see when null. */
function notification(n: number, type: NotificationType, status: Notification["status"] | null = postA): Notification {
  return {
    id: `0192f0c4-7a8e-7000-8000-0000000000${String(99 - n)}`,
    type,
    created_at: "2026-10-16T09:00:00Z",
    account: accountFixture(10 + n, `User${String(n)}`),
    ...(type === "follow" || status === null ? {} : { status }),
  };
}

/** The IDs of each row's notifications. */
function rows(notifications: Notification[]): string[][] {
  return groupNotifications(notifications).map((group) => group.notifications.map((item) => item.id));
}

describe("groupNotifications", () => {
  test("puts the favourites of one post in the row of the newest one, wherever the others are", () => {
    const [a, b, c, d] = [
      notification(1, "favourite"),
      notification(2, "follow"),
      notification(3, "favourite"),
      notification(4, "favourite", postB),
    ];

    expect(rows([a, b, c, d])).toEqual([[a.id, c.id], [b.id], [d.id]]);
  });

  test("keeps the favourites and the boosts of a post apart, and never groups other types", () => {
    const items = [
      notification(1, "favourite"),
      notification(2, "boost"),
      notification(3, "boost"),
      notification(4, "mention"),
      notification(5, "mention"),
    ];

    expect(rows(items)).toEqual([[items[0]?.id], [items[1]?.id, items[2]?.id], [items[3]?.id], [items[4]?.id]]);
  });

  test("keys a row by its type and post, so it keeps its key as it grows", () => {
    const [a, b] = [notification(1, "favourite"), notification(2, "favourite")];

    expect(groupNotifications([b]).map((group) => group.key)).toEqual(
      groupNotifications([a, b]).map((group) => group.key),
    );
  });

  test("does not group notifications whose post the user may not see, and counts a repeated one once", () => {
    const hidden = notification(1, "favourite", null);
    const other = notification(2, "favourite", null);
    const fav = notification(3, "favourite");

    expect(rows([hidden, other, fav, fav])).toEqual([[hidden.id], [other.id], [fav.id]]);
  });
});
