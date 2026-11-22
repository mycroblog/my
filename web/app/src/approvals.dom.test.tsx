// SPDX-License-Identifier: AGPL-3.0-only

// What waits for the reader's approval (ADR-0155) across the app's screens, on a server whose posts approve by hand
// (the fake's `approvals`): the row over the notifications opens its screen, and counts what is left once the reader
// answered some there and came back. What the row and the screen show is the features' (web/app-features/src/
// notifications/notifications.dom.test.tsx, waiting.dom.test.tsx).

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { within } from "@testing-library/react";
import { main, renderApp } from "./test/app";

test("the row over the notifications opens the screen of what waits; once the reader approved one request and declined another there, Back returns to the notifications, where the row counts the one left", async () => {
  // marco asked to reply to and to boost yuki's post, and mio to quote it, on a server of the posts of `approvals` alone.
  const app = await renderApp("/notifications", { approvals: true, seed: { people: scenario.people } });

  await app.user.click(await main().findByRole("link", { name: "3 waiting for your approval" }));

  const feed = within(await main().findByRole("feed", { name: "Waiting for approval" }));
  expect(app.path()).toBe("/notifications/waiting");
  const request = (asked: RegExp) => within(feed.getByRole("article", { name: asked }));
  await app.user.click(request(/asked to reply to your post$/).getByRole("button", { name: "Approve" }));
  await feed.findByText("Approved");
  await app.user.click(request(/asked to boost your post$/).getByRole("button", { name: "Decline" }));
  await feed.findByText("Declined");
  // The screen's Back, named by where it leads.
  await app.user.click(main().getByRole("button", { name: "Notifications" }));

  expect(await main().findByRole("link", { name: "1 waiting for your approval" })).toBeDefined();
  expect(app.path()).toBe("/notifications");
});
