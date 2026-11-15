// SPDX-License-Identifier: AGPL-3.0-only

// A visitor without a session opens the server's address and lands on its landing page: the server's name, the way to
// an account, and what they may read without one, which the instance decides (the demo server shows its local
// timeline to everyone and its federated one to signed-in people alone).

import { instance } from "../src/scenario";
import { Frame, Landing, SignIn } from "./screens";
import { expect, test } from "./test";

test("a visitor at the server's address sees its landing page, with the way to an account", async ({ page }) => {
  const landing = new Landing(page);
  await landing.open();
  await expect(landing.heading(instance["name"] ?? "")).toBeVisible();
  await expect(landing.createAccount).toBeVisible();
  await expect(landing.place("Read the local timeline")).toBeVisible();
  await expect(landing.place("Read the federated timeline")).toHaveCount(0);
  await landing.signIn.click();
  await expect(new SignIn(page).heading).toBeVisible();
});

test("a visitor goes on from the landing page to the local timeline", async ({ page }) => {
  const landing = new Landing(page);
  await landing.open();
  await landing.place("Read the local timeline").click();
  await expect(new Frame(page).heading("Local")).toBeVisible();
});
