// SPDX-License-Identifier: AGPL-3.0-only
// Deactivating one's own account (ADR-0145) against a real `my serve` (e2e/server.ts), in every width × locale project:
// the status page that a deactivated account signs in to, with axe, in both themes and without JavaScript, and the
// confirmations through the app and the real client API that the MSW and page.route tests only play: a code of the
// authenticator app, and a passkey on Chromium's virtual authenticator.
//
// The CLI cannot deactivate an account, on purpose: only its owner does (ADR-0145 §1). So the spec does what an owner
// does, through the client API with the web client's token, and adds nothing to the server. Every test creates a user
// of its own (a random name), so a retry and the other projects never meet it; the server's first administrator stays,
// so the user is never the last one, which the deactivation refuses.
import { createHash, createHmac, randomBytes } from "node:crypto";
import type AxeBuilder from "@axe-core/playwright";
import type { Browser, Page } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, localeDir, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";
import { ownsServer, runMy } from "./server-cli.ts";
import { serverURL } from "./server-origin.ts";

test.use({ baseURL: serverURL });
test.skip(
  !ownsServer,
  "the tests create users with bin/my, which needs the database of the server e2e/server.ts starts",
);

type User = { email: string; password: string };

const projectLocale = (): string => test.info().project.use.locale ?? "en";

/** The message `key` of the catalog `namespace` in the project's locale, with its `{name}` arguments replaced. */
function message(namespace: "client" | "server", key: string, values: Record<string, string> = {}): string {
  const text = readCatalog(projectLocale(), namespace)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${namespace}:${key} in ${projectLocale()}`);
  return text.replaceAll(/\{(\w+)\}/g, (_, name: string) => values[name] ?? `{${name}}`);
}

const emailField = (page: Page) => page.locator('input[type="email"][autocomplete="username"]');
const passwordField = (page: Page) => page.locator('input[type="password"][autocomplete="current-password"]');
const submitButton = (page: Page) => page.locator('main button[type="submit"]').first();
const reactivateForm = (page: Page) => page.locator('form[action="/auth/reactivate"]');
const deleteForm = (page: Page) => page.locator('form[action="/auth/deactivated/delete"]');

function newUser(prefix: string): User {
  const username = `e2e_${prefix}_${randomBytes(4).toString("hex")}`;
  const user = { email: `${username}@example.com`, password: `e2e-password-${username}` };
  runMy(["admin", "create", "-username", username, "-email", user.email], `${user.password}\n`);
  return user;
}

/**
 * An access token of the web client by the authorization code flow with PKCE, in a browser of its own without
 * JavaScript, so that the client's callback page does not redeem the code first. The user's display language becomes
 * the project's locale, which the server-rendered pages and the app show.
 */
async function bearerOf(browser: Browser, user: User): Promise<{ Authorization: string }> {
  const context = await browser.newContext({ baseURL: serverURL, javaScriptEnabled: false });
  try {
    const page = await context.newPage();
    const verifier = randomBytes(32).toString("base64url");
    const redirectURI = `${serverURL}/login/callback`;
    const query = new URLSearchParams({
      response_type: "code",
      client_id: "my-web",
      redirect_uri: redirectURI,
      state: "e2e-state",
      code_challenge: createHash("sha256").update(verifier).digest("base64url"),
      code_challenge_method: "S256",
    });
    await page.goto(`/oauth/authorize?${query.toString()}`);
    await emailField(page).fill(user.email);
    await passwordField(page).fill(user.password);
    await submitButton(page).click();
    await expect(page).toHaveURL(/\/login\/callback\?/);
    const token = await page.request.post("/oauth/token", {
      form: {
        grant_type: "authorization_code",
        code: new URL(page.url()).searchParams.get("code") ?? "",
        redirect_uri: redirectURI,
        client_id: "my-web",
        code_verifier: verifier,
      },
    });
    expect(token.ok()).toBe(true);
    const { access_token: accessToken } = (await token.json()) as { access_token: string };
    const headers = { Authorization: `Bearer ${accessToken}` };
    const preferences = await page.request.patch("/api/v1/preferences", {
      headers,
      data: { locale: projectLocale() },
    });
    expect(preferences.ok(), `PATCH /api/v1/preferences: ${String(preferences.status())}`).toBe(true);
    return headers;
  } finally {
    await context.close();
  }
}

/** A new user who deactivated the account through the client API, as the settings screen does; returns the date. */
async function deactivatedUser(browser: Browser, page: Page, prefix: string): Promise<User & { due: string }> {
  const user = newUser(prefix);
  const headers = await bearerOf(browser, user);
  const response = await page.request.post("/api/v1/accounts/deactivation", {
    headers,
    data: { password: user.password },
  });
  expect(response.ok(), `POST /api/v1/accounts/deactivation: ${String(response.status())}`).toBe(true);
  const { deletion_due_at: due } = (await response.json()) as { deletion_due_at: string };
  const old = await page.request.get("/api/v1/accounts/verify_credentials", { headers });
  expect(old.status()).toBe(401);
  return { ...user, due };
}

/** Signs in with the password on the server's sign-in page. */
async function signIn(page: Page, user: User): Promise<void> {
  await page.goto("/auth/sign_in");
  await emailField(page).fill(user.email);
  await passwordField(page).fill(user.password);
  await submitButton(page).click();
}

/** The status page of a deactivated account: in the user's language, with the date, and with both ways out. */
async function expectStatusPage(page: Page, due: string | undefined): Promise<void> {
  await expect(page.locator("html")).toHaveAttribute("lang", projectLocale());
  await expect(page.locator("html")).toHaveAttribute("dir", localeDir(projectLocale()));
  await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toHaveText(
    message("server", "auth.status.deactivated_title"),
  );
  const date = page.getByRole("main").locator("time");
  await expect(date).toBeVisible();
  if (due !== undefined) {
    // The page writes whole seconds; the API, milliseconds.
    const shown = new Date((await date.getAttribute("datetime")) ?? "").getTime();
    expect(shown).toBe(Math.floor(new Date(due).getTime() / 1000) * 1000);
  }
  await expect(reactivateForm(page).getByRole("button")).toHaveText(
    message("server", "auth.status.deactivated_reactivate"),
  );
  await expect(deleteForm(page).getByRole("button")).toHaveText(message("server", "auth.status.deactivated_delete"));
}

/** Checks the current page in both themes, each after a fresh load so that axe samples no transition. */
async function expectNoAxeViolationsInBothThemes(page: Page, makeAxeBuilder: () => AxeBuilder): Promise<void> {
  for (const colorScheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme });
    await page.reload();
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
  }
}

test.describe("the status page of a deactivated account", () => {
  test("states the date, passes axe, and brings the account back", async ({ browser, page, makeAxeBuilder }) => {
    const user = await deactivatedUser(browser, page, "status");
    await signIn(page, user);
    await expectStatusPage(page, user.due);
    await expectNoAxeViolationsInBothThemes(page, makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await reactivateForm(page).getByRole("button").click();
    await expect(page).toHaveURL(/\/auth\/sign_in$/);
    await expect(reactivateForm(page)).toHaveCount(0);
    await expect(page.locator('form[action="/auth/sign_out"]')).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);

    // The app works again, with a new sign-in: the deactivation ended the old tokens.
    const headers = await bearerOf(browser, user);
    const me = await page.request.get("/api/v1/accounts/verify_credentials", { headers });
    expect(me.ok(), `GET /api/v1/accounts/verify_credentials: ${String(me.status())}`).toBe(true);
  });

  test("asks for the password to delete the account, and ends on a page of its own", async ({
    browser,
    page,
    makeAxeBuilder,
  }) => {
    const user = await deactivatedUser(browser, page, "delete");
    await signIn(page, user);
    await expectStatusPage(page, user.due);

    const password = deleteForm(page).locator('input[type="password"]');
    await expect(password).toHaveAccessibleName(message("server", "auth.status.deactivated_password_label"));
    await expect(password).toHaveAccessibleDescription(message("server", "auth.status.deactivated_delete_hint"));
    await password.fill("not the password");
    await deleteForm(page).getByRole("button").click();
    await expect(page.getByRole("alert")).toHaveText(message("server", "auth.status.deactivated_wrong_password"));
    await expectStatusPage(page, user.due);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await deleteForm(page).locator('input[type="password"]').fill(user.password);
    await deleteForm(page).getByRole("button").click();
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toHaveText(
      message("server", "auth.status.deleted_title"),
    );
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The deletion signed the browser out.
    await page.goto("/auth/sign_in");
    await expect(emailField(page)).toBeVisible();
  });

  test.describe("without JavaScript", () => {
    test.use({ javaScriptEnabled: false });

    test("reactivates", async ({ browser, page }) => {
      const user = await deactivatedUser(browser, page, "nojs");
      await signIn(page, user);
      await expectStatusPage(page, user.due);
      await reactivateForm(page).getByRole("button").click();
      await expect(reactivateForm(page)).toHaveCount(0);
      await expect(page.locator('form[action="/auth/sign_out"]')).toBeVisible();
    });
  });
});

/** Decodes unpadded base32 (RFC 4648), the form of TOTP secrets. */
function base32Decode(text: string): Buffer {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = 0;
  let value = 0;
  const out: number[] = [];
  for (const char of text) {
    value = (value << 5) | alphabet.indexOf(char);
    bits += 5;
    if (bits >= 8) {
      out.push((value >>> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }
  return Buffer.from(out);
}

/** The TOTP code (RFC 6238: HMAC-SHA-1, six digits) of `secret` at the 30-second `step`. */
function totp(secret: Buffer, step: number): string {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(step));
  const mac = createHmac("sha1", secret).update(counter).digest();
  const offset = (mac.at(-1) ?? 0) & 0x0f;
  return String((mac.readUInt32BE(offset) & 0x7fffffff) % 1_000_000).padStart(6, "0");
}

const currentStep = (): number => Math.floor(Date.now() / 30_000);

const codeField = (page: Page) => page.locator('input[autocomplete="one-time-code"]');

/** The code of the current step, or of the next one while the current one enabled TOTP (each is accepted once). */
function unusedCode(secret: Buffer, enabledAt: number): string {
  const step = currentStep();
  return totp(secret, step > enabledAt ? step : enabledAt + 1);
}

test("a code of the authenticator app confirms it in the app, and the status page asks for the second factor", async ({
  browser,
  page,
  makeAxeBuilder,
}) => {
  const user = newUser("code");
  const headers = await bearerOf(browser, user);
  const setup = await page.request.post("/api/v1/two_factor/totp/setup", { headers });
  expect(setup.ok()).toBe(true);
  const { secret: encoded, setup_token } = (await setup.json()) as { secret: string; setup_token: string };
  const secret = base32Decode(encoded);
  const enabledAt = currentStep();
  const enabled = await page.request.post("/api/v1/two_factor/totp", {
    headers,
    data: { setup_token, code: totp(secret, enabledAt) },
  });
  expect(enabled.ok()).toBe(true);
  const { recovery_codes: recovery } = (await enabled.json()) as { recovery_codes: string[] };

  // Into the app with a recovery code, which leaves the codes of the app to the confirmation.
  await page.goto("/login");
  await emailField(page).fill(user.email);
  await passwordField(page).fill(user.password);
  await submitButton(page).click();
  await expect(page).toHaveURL(/\/auth\/sign_in\/two_factor(\?|$)/u);
  await codeField(page).fill(recovery[0] ?? "");
  await submitButton(page).click();
  await expect(page).toHaveURL(`${serverURL}/home`);

  await page.goto("/settings/account");
  const main = page.getByRole("main");
  await main.getByRole("button", { name: message("client", "settings.account.deactivate.open"), exact: true }).click();
  const dialog = page.getByRole("alertdialog");
  await dialog
    .getByRole("button", { name: message("client", "settings.account.deactivate.use_code"), exact: true })
    .click();
  await expectNoAxeViolations(makeAxeBuilder);

  const confirm = dialog.getByRole("button", {
    name: message("client", "settings.account.deactivate.confirm"),
    exact: true,
  });
  await codeField(page).fill("000000");
  await confirm.click();
  await expect(dialog.getByText(message("client", "settings.account.deactivate.wrong_code"))).toBeVisible();

  await codeField(page).fill(unusedCode(secret, enabledAt));
  await confirm.click();
  await expect(page).toHaveURL(/\/deactivated\?/u);
  await expect(page.getByRole("heading", { name: message("client", "deactivated.title") })).toBeVisible();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);

  // The sign-in goes through the second factor to the status page.
  await page.context().clearCookies();
  await signIn(page, user);
  await expect(page).toHaveURL(/\/auth\/sign_in\/two_factor(\?|$)/u);
  await codeField(page).fill(recovery[1] ?? "");
  await submitButton(page).click();
  await page.goto("/auth/sign_in");
  await expectStatusPage(page, undefined);
  await reactivateForm(page).getByRole("button").click();
  await expect(reactivateForm(page)).toHaveCount(0);
});

test("a passkey confirms it in the app, and signs in to the status page that asks for no password", async ({
  browser,
  page,
  makeAxeBuilder,
}) => {
  const user = newUser("passkey");
  // The display language first, so that the app and the status page show the project's.
  await bearerOf(browser, user);
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("WebAuthn.enable");
  await cdp.send("WebAuthn.addVirtualAuthenticator", {
    options: {
      protocol: "ctap2",
      transport: "internal",
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  });

  await page.goto("/login");
  await emailField(page).fill(user.email);
  await passwordField(page).fill(user.password);
  await submitButton(page).click();
  await expect(page).toHaveURL(`${serverURL}/home`);

  await page.goto("/settings/passkeys");
  const main = page.getByRole("main");
  await main.getByRole("button", { name: message("client", "settings.passkeys.add.title"), exact: true }).click();
  const form = main.getByRole("region", { name: message("client", "settings.passkeys.add.title") });
  await form.getByRole("textbox", { name: message("client", "settings.passkeys.name") }).fill("Only key");
  await form.locator('input[type="password"][autocomplete="current-password"]').fill(user.password);
  await form.getByRole("button", { name: message("client", "settings.passkeys.add.submit"), exact: true }).click();
  await expect(main.getByText(message("client", "settings.passkeys.added", { name: "Only key" }))).toBeVisible();

  // The password goes, so that the passkey is what confirms.
  await page.goto("/settings/password");
  const removal = main.getByRole("region", { name: message("client", "settings.password.remove.title") });
  await removal.getByRole("button", { name: message("client", "settings.password.remove.open"), exact: true }).click();
  const removing = page.getByRole("alertdialog", { name: message("client", "settings.password.remove.confirm.title") });
  await removing
    .getByRole("button", { name: message("client", "settings.password.remove.confirm"), exact: true })
    .click();
  await expect(main.getByText(message("client", "settings.password.removed"))).toBeVisible();

  await page.goto("/settings/account");
  await main.getByRole("button", { name: message("client", "settings.account.deactivate.open"), exact: true }).click();
  const dialog = page.getByRole("alertdialog");
  await expect(
    dialog.getByText(message("client", "settings.account.deactivate.confirm.passkey"), { exact: false }),
  ).toBeVisible();
  await expect(dialog.locator('input[type="password"]')).toHaveCount(0);
  await expectNoAxeViolations(makeAxeBuilder);
  await dialog
    .getByRole("button", { name: message("client", "settings.account.deactivate.confirm"), exact: true })
    .click();
  await expect(page).toHaveURL(/\/deactivated\?/u);
  await expect(page.getByRole("heading", { name: message("client", "deactivated.title") })).toBeVisible();

  // The passkey signs in to the status page, whose deletion the fresh sign-in confirms.
  await page.context().clearCookies();
  await page.goto("/auth/sign_in");
  await page
    .getByRole("main")
    .getByRole("button", { name: message("server", "auth.sign_in.passkey"), exact: true })
    .click();
  await expect(reactivateForm(page)).toBeVisible();
  await expectStatusPage(page, undefined);
  await expect(deleteForm(page).locator('input[type="password"]')).toHaveCount(0);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
  await deleteForm(page).getByRole("button").click();
  await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toHaveText(
    message("server", "auth.status.deleted_title"),
  );
});
