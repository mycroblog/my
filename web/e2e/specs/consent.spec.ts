// SPDX-License-Identifier: AGPL-3.0-only

// A signed-in person meets the consent screen of an app another person registered, reads what it asks for, and allows
// or denies it; the server sends them back to the app with a code or with the denial.

import { Consent } from "./screens";
import { apps } from "../src/scenario";
import { expect, person, secrets, test } from "./test";

const tidepool = apps.find((a) => a.key === "tidepool");
if (tidepool === undefined) throw new Error("the scenario has no app tidepool");

test("a person allows an app and the server sends them back with a code", async ({ page, signIn }) => {
  await signIn(person("yuki"));
  const consent = new Consent(page, tidepool);
  await consent.open(secrets().apps["tidepool"]?.clientId ?? "", "allow-state");
  await expect(consent.heading).toBeVisible();
  await expect(consent.paragraph("An application registered by @sam")).toBeVisible();
  await expect(consent.permissions).toContainText(["Read your account", "Post, edit, delete"]);
  const params = await consent.allow();
  expect(params.get("code")).toBeTruthy();
  expect(params.get("state")).toBe("allow-state");
});

test("a person denies an app and the server sends them back with the denial", async ({ page, signIn }) => {
  await signIn(person("hana"));
  const consent = new Consent(page, tidepool);
  await consent.open(secrets().apps["tidepool"]?.clientId ?? "", "deny-state");
  await expect(consent.heading).toBeVisible();
  const params = await consent.deny();
  expect(params.get("error")).toBe("access_denied");
  expect(params.get("code")).toBeNull();
  expect(params.get("state")).toBe("deny-state");
});
