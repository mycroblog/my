// SPDX-License-Identifier: AGPL-3.0-only

// The app offline (ADR-0028 §3): once its Service Worker controls the page, a navigation without the network opens the
// precached shell, which starts the app and says that the reader is offline; back online, a navigation goes to the
// network again, which serves the build of now, never a page the worker kept.

import { controlledBy } from "../src/service-worker";
import { Frame } from "./screens";
import { expect, person, test } from "./test";

test("offline, a navigation opens the app from its shell and says so; online, it reaches the network again", async ({
  page,
  context,
  signIn,
}) => {
  await signIn(person("yuki"));
  await controlledBy(page);

  await context.setOffline(true);
  await page.goto("/local");
  const frame = new Frame(page);
  await expect(frame.navigation).toBeVisible();
  await expect(
    page.getByText("You are offline. What you see may be out of date until you reconnect.", { exact: true }),
  ).toBeVisible();

  await context.setOffline(false);
  let fetched = 0;
  // The worker's own fetch of the page reaches the network, where the context routes it (Chromium).
  await context.route(
    (url) => url.pathname === "/notifications",
    async (route) => {
      fetched += 1;
      await route.continue();
    },
  );
  await page.goto("/notifications");
  await expect(frame.heading("Notifications")).toBeVisible();
  expect(fetched).toBe(1);
});
