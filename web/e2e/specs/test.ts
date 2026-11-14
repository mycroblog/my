// SPDX-License-Identifier: AGPL-3.0-only

// The test of the specs: Playwright's, with the demo server as its base URL and a `signIn(person)` fixture, which signs
// in through the server's sign-in screen with the person's password and waits for the home timeline; and what a spec
// needs of the scenario beyond it: `person(username)` the person to sign in as, `secrets()` what the seed made that the
// server decided (a TOTP secret, a passkey's key, an app's client ID), and `totp(secret)` an authenticator app's code.

import { readFileSync } from "node:fs";
import { expect, test as base } from "@playwright/test";
import { personOf, type Person } from "../src/scenario";
import type { SeedState } from "../src/seed/seed";
import { totp } from "../src/seed/totp";

export { expect, personOf as person, totp };

/** The demo server's origin, which global-setup.ts set for the workers. */
function demoURL(): string {
  const url = process.env["E2E_DEMO_URL"];
  if (url === undefined) throw new Error("e2e: no demo server; run the specs with scripts/task.sh web:e2e");
  return url;
}

/** What the seed made that the server decided: the TOTP secrets, the passkeys' keys, the apps' client IDs. */
export function secrets(): SeedState {
  return JSON.parse(readFileSync(process.env["E2E_DEMO_STATE"] ?? "", "utf8")) as SeedState;
}

type Fixtures = {
  /** Signs in as `p` with the password and waits until the home timeline shows. */
  signIn: (p: Person) => Promise<void>;
};

export const test = base.extend<Fixtures>({
  signIn: async ({ page }, use) => {
    await use(async (p) => {
      await page.goto("/login");
      await page.getByRole("textbox", { name: "Email address" }).fill(p.email);
      await page.getByLabel("Password", { exact: true }).fill(p.password);
      await page.getByRole("button", { name: "Sign in", exact: true }).click();
      await expect(page.getByRole("feed", { name: "Home timeline" })).toBeVisible();
    });
  },
});

test.use({ baseURL: demoURL() });
