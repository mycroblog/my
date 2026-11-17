// SPDX-License-Identifier: AGPL-3.0-only

// The address of one notification, as a push or an email gives it, once the notification is no longer there (it was
// dismissed, or is another's): the reader lands on their notifications, which say so. Where the address of a
// notification that is there leads is notifications.dom.test.tsx. The reader is Sam.

import { expect, test } from "bun:test";
import { main, renderApp } from "./test/app";

test("the address of a notification that is no longer there opens the notifications, which say so", async () => {
  // No notification of the server's has this ID.
  await renderApp("/notifications/0190a5d0-0000-7000-8000-000000000999", { viewer: "sam" });

  expect(await main().findByText("That notification is no longer there.")).toBeDefined();
  expect(main().getByRole("heading", { level: 1, name: "Notifications" })).toBeDefined();
  expect(await main().findByRole("feed", { name: "Notifications" })).toBeDefined();
});
