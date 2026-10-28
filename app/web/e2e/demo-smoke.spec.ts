// SPDX-License-Identifier: AGPL-3.0-only
// The demo server (`node e2e/server.ts demo`, ADR-0117): a person of the demo scenario signs in and reads a home
// timeline with the scenario's posts; a second run of the seed adds nothing; and a password reset arrives in Mailpit
// with a link that sets the password. It runs in the demo-chromium project of playwright.config.ts, which exists
// only with E2E_DEMO=1, as the demo server needs the test build (`GO_TAGS=e2e_fakes scripts/task.sh go:bin`) and
// Mailpit (`scripts/task.sh docker:mailpit`).
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { personas } from "../demo/scenario.ts";
import { expect, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";
import { mailpit, myBin, serverEnv } from "./server-cli.ts";
import { demoServerURL } from "./server-origin.ts";

test.use({ baseURL: demoServerURL });
test.describe.configure({ mode: "serial" });

/** The client message `key` (the project is in English). */
function message(key: string): string {
  const text = readCatalog("en")[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key}`);
  return text;
}

function persona(username: string) {
  const p = personas.find((x) => x.username === username);
  if (p === undefined) throw new Error(`no persona ${username}`);
  return p;
}

/** Runs demo/seed.ts against the demo server with `args` and returns its output. */
function runSeed(...args: string[]): string {
  return execFileSync(
    "bun",
    [
      "--conditions=@mycroblog/source",
      join(import.meta.dirname, "../demo/seed.ts"),
      "--server",
      demoServerURL,
      ...args,
    ],
    { env: { ...serverEnv("demo"), DEMO_MY_BIN: myBin }, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] },
  );
}

/** Signs in to the app through the server's sign-in screen and waits for the home page. */
async function signIn(page: Page, email: string, password: string): Promise<void> {
  await page.goto("/login");
  await page.getByRole("main").getByRole("button").click();
  await page.locator('input[type="email"][autocomplete="username"]').fill(email);
  await page.locator('input[type="password"][autocomplete="current-password"]').fill(password);
  await page.locator('main button[type="submit"]').first().click();
  await expect(page).toHaveURL(`${demoServerURL}/home`);
  await expect(page.getByRole("main").getByRole("heading", { level: 1, name: message("home.title") })).toBeVisible();
}

/** A message in Mailpit, as its REST API lists it. */
type MailpitMessage = { ID: string; Created: string };

const mailpitAPI = `http://127.0.0.1:${String(mailpit.uiPort)}/api/v1`;

/**
 * Asks for a password reset of `email` on the sign-in pages, opens the link of the email that arrives in Mailpit, and
 * sets `password`.
 */
async function resetPassword(page: Page, email: string, password: string): Promise<void> {
  const since = Date.now();
  await page.goto("/auth/forgot_password");
  await page.locator('input[type="email"][autocomplete="username"]').fill(email);
  await page.locator('main button[type="submit"]').first().click();
  await expect(page.getByRole("status")).toBeVisible();

  let link = "";
  await expect
    .poll(
      async () => {
        const search = await page.request.get(`${mailpitAPI}/search`, { params: { query: `to:${email}` } });
        const { messages } = (await search.json()) as { messages: MailpitMessage[] };
        const latest = messages.find((m) => Date.parse(m.Created) >= since - 1000);
        if (latest === undefined) return "";
        const read = await page.request.get(`${mailpitAPI}/message/${latest.ID}`);
        const { Text } = (await read.json()) as { Text: string };
        link = /https?:\/\/\S+\/auth\/reset_password\?\S+/.exec(Text)?.[0] ?? "";
        return link;
      },
      { timeout: 30_000 },
    )
    .toContain(`${demoServerURL}/auth/reset_password`);

  await page.goto(link);
  await page.locator('input[type="password"][autocomplete="new-password"]').fill(password);
  await page.locator('main button[type="submit"]').first().click();
  await expect(page.locator('input[type="password"][autocomplete="new-password"]')).toBeHidden();
}

test("a person of the scenario signs in and reads the home timeline", async ({ page }) => {
  const yuki = persona("yuki");
  await signIn(page, yuki.email, yuki.password);
  const main = page.getByRole("main");
  await expect(main.getByRole("article").first()).toBeVisible();
  await expect(main.getByText("Third attempt at a sourdough loaf").first()).toBeVisible();
});

test("a second run of the seed adds nothing", () => {
  test.setTimeout(180_000);
  const before: unknown = JSON.parse(runSeed("--census"));
  runSeed();
  const after: unknown = JSON.parse(runSeed("--census"));
  expect(after).toEqual(before);
});

test("a password reset arrives in Mailpit and its link sets the password", async ({ browser }) => {
  test.setTimeout(120_000);
  const marco = persona("marco");
  const changed = `${marco.password}-changed`;
  const reset = await browser.newContext({ baseURL: demoServerURL });
  await resetPassword(await reset.newPage(), marco.email, changed);
  const signedIn = await browser.newContext({ baseURL: demoServerURL });
  await signIn(await signedIn.newPage(), marco.email, changed);
  // The scenario's password again, for the other tests and the seed.
  const restore = await browser.newContext({ baseURL: demoServerURL });
  await resetPassword(await restore.newPage(), marco.email, marco.password);
  await Promise.all([reset.close(), signedIn.close(), restore.close()]);
});
