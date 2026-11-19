// SPDX-License-Identifier: AGPL-3.0-only

// The server's administration on the real server (#3262, FR-010, FR-705, FR-725): an administrator changes a setting
// on a page of Server settings and finds it saved after a reload; sets an icon of the server's own, which the app
// draws the renditions of in the browser, and puts the built-in one back; writes two rules, moves the second
// first among the rules and retires both; and sets up a GitHub login provider, reads the callback URL to register at GitHub,
// and deletes it. What each screen shows in every state, and what Save asks before what it ends, are the stories' and
// the route tests' on the fake (web/app, admin.stories.tsx). A setting changes the whole server for every test that
// runs beside this one, a repeat of this file on another worker among them: the one changed is the security contact,
// which no other spec reads, read back as one this spec wrote (the repeat may have saved its own since); the icon is
// read from the answers of the page that saved it, never after a reload, and put back at once; and the rules are told
// by their order, not their places, since the repeat writes rules of its own.

import { randomUUID } from "node:crypto";
import { Administration } from "./screens";
import { expect, test } from "./test";

test("an administrator sets the security contact and finds it saved", async ({ page, signIn, fresh }) => {
  await signIn(await fresh.person({ admin: true }));
  const admin = new Administration(page);
  const contact = `mailto:security-${randomUUID().slice(0, 8)}@example.com`;
  await admin.open();
  await admin.go("Administration screens", "Server settings");
  await admin.go("Server settings", "Search and crawlers");

  await admin.textbox("Security contact").fill(contact);
  await admin.saveSettings();
  await expect(admin.saved).toBeVisible();

  await page.reload();
  await expect(admin.textbox("Security contact")).toHaveValue(/^mailto:security-[0-9a-f]{8}@example\.com$/);
});

test("an administrator sets an icon of the server's own, sees it saved, and puts the built-in one back", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person({ admin: true }));
  const admin = new Administration(page);
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" fill="#2f6f5e"/><circle cx="32" cy="32" r="16" fill="#ffffff"/></svg>`;
  await admin.open();
  await admin.go("Administration screens", "Server settings");
  await admin.go("Server settings", "Icon and logo");

  await admin.chooseBrand("Icon", { name: "icon.svg", mimeType: "image/svg+xml", buffer: Buffer.from(svg) });
  await admin.saveBrand("icon", "PUT");
  await expect(admin.saved).toBeVisible();
  await expect(admin.builtIn("Icon")).toBeVisible();

  await admin.builtIn("Icon").click();
  await admin.saveBrand("icon", "DELETE");
  await expect(admin.builtIn("Icon")).toHaveCount(0);
});

test("an administrator writes two rules, moves the second first among the rules, and retires them", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person({ admin: true }));
  const admin = new Administration(page);
  const word = randomUUID().slice(0, 8);
  const before = `Read the about page first ${word}.`;
  const text = `Say hello to newcomers ${word}.`;
  await admin.open();
  await admin.go("Administration screens", "Rules");

  // Two rules, the second added after the first, so that it has a place to move to.
  await admin.addRule(before);
  await expect(admin.rule(before)).toBeVisible();
  await admin.addRule(text);
  await expect(admin.rule(text)).toBeVisible();

  await admin.rule(text).click();
  await admin.moveRule("1st");
  await expect(admin.saved).toBeVisible();
  await page.goto("/admin/rules");
  await expect(admin.inEffect).toHaveText(admin.inOrder(text, before));

  await admin.rule(text).click();
  await admin.retireRule();
  await expect(admin.retired(text)).toBeVisible();
  await expect(admin.rule(text)).toHaveCount(0);
  await admin.rule(before).click();
  await admin.retireRule();
  await expect(admin.retired(before)).toBeVisible();
});

test("an administrator sets up a GitHub login provider, reads its callback URL, and deletes it", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person({ admin: true }));
  const admin = new Administration(page);
  const name = `GitHub ${randomUUID().slice(0, 8)}`;
  await admin.open();
  await admin.go("Administration screens", "Login providers");

  await admin.addGitHubProvider(name, `client-${randomUUID().slice(0, 8)}`, randomUUID());
  await expect(admin.callbackUrl).toHaveText(/^https?:\/\/.+\/auth\/external\/.+\/callback$/);

  await admin.deleteProvider(name);
  await expect(admin.provider(name)).toHaveCount(0);
});
