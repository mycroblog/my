// SPDX-License-Identifier: AGPL-3.0-only
// The pages the Go server renders without JavaScript (ADR-0030): sign-in, sign-up and OAuth consent,
// against a real `my serve` (e2e/server.ts) in every width × locale project. Fields are found by their
// autocomplete tokens, which are part of what these pages promise (password managers), and stay the same
// in every locale.
import { createHash, createHmac, randomBytes } from "node:crypto";
import type AxeBuilder from "@axe-core/playwright";
import type { Browser, BrowserContext, Page } from "@playwright/test";
import { axeBuilder, expect, expectNoAxeViolations, expectNoHorizontalScroll, localeDir, test } from "./fixtures";
import { ownsServer, runMy } from "./server-cli.ts";
import { admin, serverURL, thirdPartyClient } from "./server-origin.ts";

test.use({ baseURL: serverURL });

const emailField = (page: Page) => page.locator('input[type="email"][autocomplete="username"]');
const passwordField = (page: Page, autocomplete: "current-password" | "new-password") =>
  page.locator(`input[type="password"][autocomplete="${autocomplete}"]`);
const submitButton = (page: Page) => page.locator('main button[type="submit"]').first();

/**
 * Asserts what every server-rendered page shares: the project's locale and its direction (the server
 * resolves Accept-Language, and `my serve -pseudo-locales` makes en-XA and ar-XB selectable), and a main
 * landmark with one h1.
 */
async function expectPageBasics(page: Page): Promise<void> {
  const locale = test.info().project.use.locale ?? "en";
  await expect(page.locator("html")).toHaveAttribute("lang", locale);
  await expect(page.locator("html")).toHaveAttribute("dir", localeDir(locale));
  await expect(page.locator("main h1")).toHaveCount(1);
  await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
}

/** Asserts that the error message element `id` describes the field and marks it invalid. */
async function expectDescribedError(page: Page, field: ReturnType<Page["locator"]>, id: string): Promise<void> {
  await expect(page.locator(`[id="${id}"]`)).toBeVisible();
  await expect(field).toHaveAttribute("aria-invalid", "true");
  await expect(field).toHaveAttribute("aria-describedby", new RegExp(`(?:^| )${id}(?: |$)`));
}

/**
 * Checks the current page in both themes. Each theme gets a fresh load, so that axe does not sample
 * colors in the middle of a transition.
 */
async function expectNoAxeViolationsInBothThemes(page: Page, makeAxeBuilder: () => AxeBuilder): Promise<void> {
  for (const colorScheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme });
    await page.reload();
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
  }
}

/**
 * Opens the page that a JavaScript-less browser is on in a browser with JavaScript and the same cookies,
 * for what needs a script in the page: axe. Contexts made in a test inherit the project's options (device,
 * locale), so only JavaScript and the cookies are set here.
 */
async function withJavaScript(
  browser: Browser,
  context: BrowserContext,
  page: Page,
  check: (page: Page, makeAxeBuilder: () => AxeBuilder) => Promise<void>,
): Promise<void> {
  const enabled = await browser.newContext({ javaScriptEnabled: true, storageState: await context.storageState() });
  try {
    const other = await enabled.newPage();
    await other.goto(page.url());
    await check(other, () => axeBuilder(other));
  } finally {
    await enabled.close();
  }
}

async function signIn(page: Page): Promise<void> {
  await emailField(page).fill(admin.email);
  await passwordField(page, "current-password").fill(admin.password);
  await submitButton(page).click();
}

test.describe("sign-in page", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/auth/sign_in");
    await expectPageBasics(page);
  });

  test("has no axe violations in the light and the dark theme", async ({ page, makeAxeBuilder }) => {
    await expectNoAxeViolationsInBothThemes(page, makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });

  test("labels its fields for password managers", async ({ page }) => {
    await expect(emailField(page)).toHaveAccessibleName(/\S/);
    await expect(emailField(page)).toHaveAttribute("required", "");
    await expect(passwordField(page, "current-password")).toHaveAccessibleName(/\S/);
    await expect(page.getByRole("main")).toMatchAriaSnapshot(`
      - main:
        - heading [level=1]
        - textbox
        - button
    `);
  });

  test("shows a focus ring on keyboard focus", async ({ page }) => {
    await page.keyboard.press("Tab");
    await expect(emailField(page)).toBeFocused();
    const outline = await emailField(page).evaluate((el) => getComputedStyle(el).outlineStyle);
    expect(outline).toBe("solid");
  });

  test("has 44 px targets on touch screens", async ({ page }) => {
    const coarse = await page.evaluate(() => matchMedia("(pointer: coarse)").matches);
    test.skip(!coarse, "only coarse pointers get the larger targets (web-client/README.md D4)");
    for (const target of [emailField(page), passwordField(page, "current-password"), submitButton(page)]) {
      const box = await target.boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    }
  });

  test("associates the error with the fields when the form expired", async ({ page, context, makeAxeBuilder }) => {
    // Without the anti-forgery cookie the server rejects the form before checking the password, so the
    // test uses up no sign-in rate limit.
    await context.clearCookies({ name: "__Host-my_csrf" });
    await signIn(page);
    await expect(page.getByRole("alert")).toBeVisible();
    await expectDescribedError(page, emailField(page), "sign-in-error");
    await expectDescribedError(page, passwordField(page, "current-password"), "sign-in-error");
    await expect(emailField(page)).toHaveValue(admin.email);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("without JavaScript", () => {
  test.use({ javaScriptEnabled: false });

  test("signs in and shows the signed-in page", async ({ browser, context, page }) => {
    await page.goto("/auth/sign_in");
    await signIn(page);
    await expect(page).toHaveURL(`${serverURL}/`);

    await page.goto("/auth/sign_in");
    await expectPageBasics(page);
    await expect(page.locator('form[action="/auth/sign_out"] button[type="submit"]')).toBeVisible();
    await withJavaScript(browser, context, page, async (signedIn, makeAxeBuilder) => {
      await expect(signedIn.locator('form[action="/auth/sign_out"]')).toBeVisible();
      await expectNoAxeViolationsInBothThemes(signedIn, makeAxeBuilder);
      await expectNoHorizontalScroll(signedIn);
    });
  });

  test("creates an account and shows that it waits for approval", async ({ browser, context, page }, testInfo) => {
    // Every sign-up counts against a per-address limit (5 in 30 minutes), so one project creates the account.
    test.skip(testInfo.project.name !== "en-320", "the sign-up rate limit allows only a few accounts per run");
    const username = `e2e_${randomBytes(4).toString("hex")}`;
    await page.goto("/auth/sign_up");
    await page.locator('input[autocomplete="nickname"]').fill(username);
    await emailField(page).fill(`${username}@example.com`);
    await passwordField(page, "new-password").fill("e2e-password-new-user");
    await page.locator("textarea").fill("E2E");
    // The server publishes its documents, so the form asks to agree to them.
    await page.locator('input[name="agreement"]').check();
    await submitButton(page).click();

    await expect(page).toHaveURL(/\/auth\/sign_in$/);
    await expectPageBasics(page);
    await expect(page.locator('form[action="/auth/sign_out"]')).toBeVisible();
    await withJavaScript(browser, context, page, async (status, makeAxeBuilder) => {
      await expect(status.locator('form[action="/auth/sign_out"]')).toBeVisible();
      await expectNoAxeViolationsInBothThemes(status, makeAxeBuilder);
      await expectNoHorizontalScroll(status);
    });
  });
});

test.describe("status page of a suspended account", () => {
  test.use({ javaScriptEnabled: false });
  test.skip(
    !ownsServer,
    "the test suspends a user with bin/my, which needs the database of the server e2e/server.ts starts",
  );

  test("appeals the suspension without JavaScript", async ({ browser, context, page }) => {
    // A user of its own per project, suspended by the CLI as a moderator would (docs/operations/moderation.md).
    const username = `e2e_appeal_${randomBytes(4).toString("hex")}`;
    const user = { email: `${username}@example.com`, password: `e2e-password-${username}` };
    runMy(["admin", "create", "-username", username, "-email", user.email], `${user.password}\n`);
    const issued = runMy([
      "admin",
      "account",
      "action",
      "issue",
      "-kind",
      "suspend",
      "-category",
      "spam",
      `@${username}`,
    ]);
    const actionID = /\(action (\S+),/.exec(issued)?.[1] ?? "";
    expect(actionID).not.toBe("");

    await page.goto("/auth/sign_in");
    await emailField(page).fill(user.email);
    await passwordField(page, "current-password").fill(user.password);
    await submitButton(page).click();
    await expect(page).toHaveURL(/\/auth\/sign_in$/);
    // The status page is worded in the user's saved locale, not the browser's.
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    const appeal = page.locator(`[id="appeal-${actionID}"]`);
    await expect(appeal).toHaveAccessibleName(/\S/);
    await expect(appeal).toHaveAccessibleDescription(/\S/);
    await withJavaScript(browser, context, page, async (status, makeAxeBuilder) => {
      await expect(status.locator('form[action="/auth/appeal"]')).toBeVisible();
      await expectNoAxeViolationsInBothThemes(status, makeAxeBuilder);
      await expectNoHorizontalScroll(status);
    });

    await appeal.fill("The links were not spam.");
    await page.locator('form[action="/auth/appeal"] button[type="submit"]').click();
    await expect(page.getByRole("status")).toBeVisible();
    await expect(page.locator('form[action="/auth/appeal"]')).toHaveCount(0);
    await expect(page.getByRole("main")).toContainText("The links were not spam.");
    expect(runMy(["admin", "appeal", "list", "-state", "pending"])).toContain(actionID);
    await expectNoHorizontalScroll(page);
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
  const message = Buffer.alloc(8);
  message.writeBigUInt64BE(BigInt(step));
  const mac = createHmac("sha1", secret).update(message).digest();
  const offset = (mac.at(-1) ?? 0) & 0x0f;
  return String((mac.readUInt32BE(offset) & 0x7fffffff) % 1_000_000).padStart(6, "0");
}

const currentStep = (): number => Math.floor(Date.now() / 30_000);

/**
 * A code that the server accepts and has not seen: that of the current step, or of the next one while the
 * current one is the step whose code enabled TOTP (codes of the adjacent steps are accepted, each once).
 */
function unusedCode(user: { secret: Buffer; step: number }): string {
  const step = currentStep();
  return totp(user.secret, step > user.step ? step : user.step + 1);
}

const codeField = (page: Page) => page.locator('input[autocomplete="one-time-code"]');

test.describe("two-factor sign-in", () => {
  test.use({ javaScriptEnabled: false });
  test.skip(
    !ownsServer,
    "the test creates a user with bin/my, which needs the database of the server e2e/server.ts starts",
  );

  /** A user of its own per project with TOTP enabled through the client API, and the step of the code that enabled it. */
  async function twoFactorUser(
    page: Page,
  ): Promise<{ email: string; password: string; secret: Buffer; step: number; recoveryCodes: string[] }> {
    const username = `e2e_totp_${randomBytes(4).toString("hex")}`;
    const user = { email: `${username}@example.com`, password: `e2e-password-${username}` };
    runMy(["admin", "create", "-username", username, "-email", user.email], `${user.password}\n`);

    // An access token of the web client by the authorization code flow with PKCE. JavaScript is off, so
    // the client's callback page does not redeem the code first.
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
    await passwordField(page, "current-password").fill(user.password);
    await submitButton(page).click();
    await expect(page).toHaveURL(/\/login\/callback\?/);
    const code = new URL(page.url()).searchParams.get("code") ?? "";
    const token = await page.request.post("/oauth/token", {
      form: {
        grant_type: "authorization_code",
        code,
        redirect_uri: redirectURI,
        client_id: "my-web",
        code_verifier: verifier,
      },
    });
    expect(token.ok()).toBe(true);
    const { access_token: accessToken } = (await token.json()) as { access_token: string };
    const headers = { Authorization: `Bearer ${accessToken}` };

    const setup = await page.request.post("/api/v1/two_factor/totp/setup", { headers });
    expect(setup.ok()).toBe(true);
    const { secret, setup_token } = (await setup.json()) as { secret: string; setup_token: string };
    const key = base32Decode(secret);
    const step = currentStep();
    const enabled = await page.request.post("/api/v1/two_factor/totp", {
      headers,
      data: { setup_token, code: totp(key, step) },
    });
    expect(enabled.ok()).toBe(true);
    const { recovery_codes } = (await enabled.json()) as { recovery_codes: string[] };
    await page.context().clearCookies();
    return { ...user, secret: key, step, recoveryCodes: recovery_codes };
  }

  async function signInWithPassword(page: Page, user: { email: string; password: string }): Promise<void> {
    await page.goto("/auth/sign_in");
    await emailField(page).fill(user.email);
    await passwordField(page, "current-password").fill(user.password);
    await submitButton(page).click();
    await expect(page).toHaveURL(/\/auth\/sign_in\/two_factor$/);
  }

  test("asks for the code after the password and signs in with it", async ({ browser, context, page }) => {
    const user = await twoFactorUser(page);
    await signInWithPassword(page, user);
    await expectPageBasics(page);
    await expect(codeField(page)).toHaveAccessibleName(/\S/);
    await expect(codeField(page)).toHaveAccessibleDescription(/\S/);
    await expect(page.getByRole("main")).toMatchAriaSnapshot(`
      - main:
        - heading [level=1]
        - paragraph
        - textbox
        - button
        - paragraph:
          - link
    `);
    await withJavaScript(browser, context, page, async (form, makeAxeBuilder) => {
      await expect(codeField(form)).toBeVisible();
      await expectNoAxeViolationsInBothThemes(form, makeAxeBuilder);
      await expectNoHorizontalScroll(form);
    });

    // An expired form is refused before the code counts against a rate limit.
    await context.clearCookies({ name: "__Host-my_csrf" });
    await codeField(page).fill(unusedCode(user));
    await submitButton(page).click();
    await expect(page.getByRole("alert")).toBeVisible();
    await expectDescribedError(page, codeField(page), "two-factor-error");
    await withJavaScript(browser, context, page, async (form, makeAxeBuilder) => {
      await expect(codeField(form)).toBeVisible();
      await expectNoAxeViolations(makeAxeBuilder);
    });

    await codeField(page).fill(unusedCode(user));
    await submitButton(page).click();
    await expect(page).toHaveURL(`${serverURL}/`);
    await page.goto("/auth/sign_in");
    await expect(page.locator('form[action="/auth/sign_out"]')).toBeVisible();
  });

  test("signs in once with each recovery code", async ({ page }, testInfo) => {
    // A reused code counts against the sign-in rate limit of the address, so one project checks it.
    test.skip(testInfo.project.name !== "en-320", "a refused code counts against the sign-in rate limit");
    const user = await twoFactorUser(page);
    const [recoveryCode = ""] = user.recoveryCodes;
    await signInWithPassword(page, user);
    await codeField(page).fill(recoveryCode.toUpperCase());
    await submitButton(page).click();
    await expect(page).toHaveURL(`${serverURL}/`);

    await page.context().clearCookies();
    await signInWithPassword(page, user);
    await codeField(page).fill(recoveryCode);
    await submitButton(page).click();
    await expect(page.getByRole("alert")).toBeVisible();
    await expectDescribedError(page, codeField(page), "two-factor-error");
    await expect(page).toHaveURL(/\/auth\/sign_in\/two_factor$/);
  });
});

test.describe("password reset", () => {
  test("links from the sign-in page to the page that asks for a link", async ({ page, makeAxeBuilder }) => {
    await page.goto("/auth/sign_in");
    await page.locator('main a[href="/auth/forgot_password"]').click();
    await expect(page).toHaveURL(/\/auth\/forgot_password$/);
    await expectPageBasics(page);
    await expectNoAxeViolationsInBothThemes(page, makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test.describe("with a link from the CLI", () => {
    test.use({ javaScriptEnabled: false });
    test.skip(
      !ownsServer,
      "the test creates a user with bin/my, which needs the database of the server e2e/server.ts starts",
    );

    test("sets a new password once", async ({ browser, context, page }) => {
      const username = `e2e_reset_${randomBytes(4).toString("hex")}`;
      const email = `${username}@example.com`;
      runMy(["admin", "create", "-username", username, "-email", email], `e2e-password-${username}\n`);
      const link = new URL(runMy(["admin", "account", "password-reset-link", `@${username}`]).trim());
      await page.goto(link.pathname + link.search);
      await expectPageBasics(page);
      await expect(passwordField(page, "new-password")).toHaveAccessibleName(/\S/);
      await expect(passwordField(page, "new-password")).toHaveAccessibleDescription(/\S/);
      await withJavaScript(browser, context, page, async (form, makeAxeBuilder) => {
        await expectNoAxeViolationsInBothThemes(form, makeAxeBuilder);
        await expectNoHorizontalScroll(form);
      });

      // Opening the link does not use it; setting the password does, once.
      await passwordField(page, "new-password").fill(`e2e-new-password-${username}`);
      await submitButton(page).click();
      await expectPageBasics(page);
      await expect(page.getByRole("main").getByRole("status")).toBeVisible();
      await page.goto(link.pathname + link.search);
      await expect(passwordField(page, "new-password")).toHaveCount(0);

      await page.goto("/auth/sign_in");
      await emailField(page).fill(email);
      await passwordField(page, "current-password").fill(`e2e-new-password-${username}`);
      await submitButton(page).click();
      await expect(page).not.toHaveURL(/\/auth\/sign_in$/);
    });
  });
});

test.describe("sign-up page", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/auth/sign_up");
    await expectPageBasics(page);
  });

  test("has no axe violations in the light and the dark theme", async ({ page, makeAxeBuilder }) => {
    await expectNoAxeViolationsInBothThemes(page, makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });

  test("describes every field by its hint", async ({ page }) => {
    for (const field of [
      page.locator('input[autocomplete="nickname"]'),
      emailField(page),
      passwordField(page, "new-password"),
    ]) {
      await expect(field).toHaveAccessibleName(/\S/);
      await expect(field).toHaveAccessibleDescription(/\S/);
    }
  });

  test("associates each field error with its field", async ({ page, makeAxeBuilder }) => {
    // The server checks the fields before the sign-up counts against its rate limit.
    const username = page.locator('input[autocomplete="nickname"]');
    await username.fill("not a username!");
    await emailField(page).fill("someone@example.com");
    await passwordField(page, "new-password").fill("e2e-password-new-user");
    // The required agreement would stop the browser from sending the form at all.
    await page.locator('input[name="agreement"]').check();
    await submitButton(page).click();

    await expect(page.getByRole("alert")).toBeVisible();
    await expectDescribedError(page, username, "sign-up-username-error");
    await expect(username).toHaveAttribute("aria-describedby", /sign-up-username-hint/);
    await expect(emailField(page)).not.toHaveAttribute("aria-invalid", "true");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("OAuth consent page", () => {
  /** An authorization request of the third-party client, with a PKCE challenge (RFC 7636). */
  function authorizeURL(state: string): string {
    const challenge = createHash("sha256").update(randomBytes(32).toString("base64url")).digest("base64url");
    const query = new URLSearchParams({
      response_type: "code",
      client_id: thirdPartyClient.clientID,
      redirect_uri: thirdPartyClient.redirectURI,
      scope: "read",
      state,
      code_challenge: challenge,
      code_challenge_method: "S256",
    });
    return `/oauth/authorize?${query.toString()}`;
  }

  test.beforeEach(async ({ page }) => {
    await page.goto(authorizeURL("e2e-state"));
    // Without a session the endpoint asks for a sign-in and comes back.
    await expect(page).toHaveURL(/\/auth\/sign_in\?return_to=/);
    await signIn(page);
    await expect(page).toHaveURL(/\/oauth\/authorize\?/);
    await expectPageBasics(page);
    await expect(page.getByRole("main")).toContainText(thirdPartyClient.name);
  });

  test("has no axe violations in the light and the dark theme", async ({ page, makeAxeBuilder }) => {
    await expectNoAxeViolationsInBothThemes(page, makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });

  test("lists the scopes under the heading of their weight and offers to approve or deny", async ({ page }) => {
    await expect(page.getByRole("main")).toMatchAriaSnapshot(`
      - main:
        - heading [level=1]
        - region:
          - heading [level=2]
          - list:
            - listitem
        - button
        - button
    `);
  });

  test("says the app is unverified, where it comes from, and warns of impersonation", async ({ page }) => {
    // The seeded client is a dynamic registration whose redirect URI is not https, so the page shows the
    // unverified notice, the origin, the impersonation alert, and the redirect host.
    const main = page.getByRole("main");
    const host = new URL(thirdPartyClient.redirectURI).host;
    await expect(main.locator("p.caution-note")).toHaveCount(1);
    await expect(main.locator("p.alert")).toHaveCount(1);
    await expect(main.locator("p.alert")).toContainText(host);
    await expect(main.locator("p.hint")).toContainText(host);
  });

  test.describe("without JavaScript", () => {
    test.use({ javaScriptEnabled: false });

    test("shows the unverified notice, and approving redirects to the client with a code", async ({ page }) => {
      await expect(page.getByRole("main").locator("p.caution-note")).toHaveCount(1);
      await page.locator('button[name="decision"][value="approve"]').click();
      await expect(page).toHaveURL((url) => `${url.origin}${url.pathname}` === thirdPartyClient.redirectURI);
      const query = new URL(page.url()).searchParams;
      expect(query.get("code")).toMatch(/\S/);
      expect(query.get("state")).toBe("e2e-state");
      // The server runs with web_origin http://{host}, so it is its own issuer over plain HTTP (Config.ServerOrigin).
      expect(query.get("iss")).toBe(serverURL);
    });
  });
});
