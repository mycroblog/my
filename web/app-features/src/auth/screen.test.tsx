// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { I18nProvider } from "@my/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { consentWaitSeconds, holdWhileWaiting, secondsLeft } from "./consent-wait";
import { parseAuthPage, type AuthPage, type Consent } from "./data";
import { AuthScreen } from "./screen";

const render = (page: AuthPage, { locale = "en", preview = false } = {}) =>
  renderToStaticMarkup(
    <I18nProvider locale={locale}>
      <AuthScreen page={page} wordmark={{ light: "/light.svg", dark: "/dark.svg" }} preview={preview} />
    </I18nProvider>,
  );

const base = { server: { name: "my.example" }, csrfToken: "token" } as const;

/** The opening tag of the element with `id`. */
const tagOf = (html: string, id: string) => new RegExp(`<[a-z]+[^>]* id="${id}"[^>]*>`).exec(html)?.[0] ?? "";

test("a screen is the server's page: its mark, its introduction, the title, and the footer of its documents", () => {
  const html = render({
    ...base,
    screen: "sign_in",
    server: { name: "Hanami Town", logo: "/brand/1/logo.svg", description: "A town square.", banner: "/media/b.png" },
    documents: [{ path: "/about/terms", title: "Terms of service" }],
    statusPageUrl: "https://status.example",
  });
  expect(html).toContain("<aside class=");
  expect(html).toContain('aria-label="Hanami Town"');
  expect(html).toContain('src="/media/b.png"');
  expect(html).toContain('src="/brand/1/logo.svg"');
  expect(html).toContain("A town square.");
  expect(html).not.toContain("/light.svg");
  expect(html).toMatch(/<h1[^>]*>Sign in<\/h1>/);
  expect(html).toContain('<a class="text-muted-text" href="/about/terms">Terms of service</a>');
  expect(html).toContain('href="https://status.example"');
  expect(render({ ...base, screen: "sign_in" })).toContain('src="/light.svg"');
});

test("the sign-in form posts the anti-forgery token and the return target, and its refusal sits right before its button", () => {
  const html = render({
    ...base,
    screen: "sign_in",
    returnTo: "/oauth/authorize?x=1",
    email: "a@example.com",
    error: "No.",
  });
  expect(html).toContain('action="/auth/sign_in"');
  expect(html).toContain('<input type="hidden" name="csrf_token" value="token"/>');
  expect(html).toContain('<input type="hidden" name="return_to" value="/oauth/authorize?x=1"/>');
  expect(tagOf(html, "sign-in-email")).toContain('value="a@example.com"');
  expect(tagOf(html, "sign-in-email")).toContain('autoComplete="username"');
  // The refusal is announced, takes the focus when the page opens, and the button is the next stop.
  expect(html).toMatch(
    /role="alert" tabindex="-1" data-focus-on-load=""><svg[^]*?No\.<\/span><\/p><button[^>]*type="submit"/,
  );
  expect(html.indexOf('id="sign-in-password"')).toBeLessThan(html.indexOf('role="alert"'));
});

test("a preview of many screens leaves the focus alone", () => {
  expect(render({ ...base, screen: "sign_in", error: "No." }, { preview: true })).not.toContain("data-focus-on-load");
});

test("with passkeys, the email field offers them in its autofill; the providers are links with their marks", () => {
  const html = render({
    ...base,
    screen: "sign_in",
    passkey: { options: { challenge: "c" }, challenge: "sealed" },
    external: [{ label: "Sign in with GitHub", url: "/auth/external/1/start", kind: "github" }],
  });
  expect(tagOf(html, "sign-in-email")).toContain('autoComplete="username webauthn"');
  expect(html).toMatch(/<a [^>]*href="\/auth\/external\/1\/start"[^>]*>[^]*?<svg[^]*?Sign in with GitHub/);
});

test("the human check places the service's widget inside the form with the notice the service asks for", () => {
  const html = render({
    ...base,
    screen: "sign_in",
    humanCheck: {
      scriptSrc: "https://challenges.example/api.js",
      containerClass: "cf-turnstile",
      siteKey: "key",
      action: "sign_in",
      language: "ja",
      theme: "auto",
      notice: "This page uses a check.",
      links: [{ url: "https://example.com/privacy", title: "Privacy" }],
    },
  });
  const form = /<form[^]*?<\/form>/.exec(html)?.[0] ?? "";
  expect(form).toMatch(
    /<div class="cf-turnstile [^"]*" data-sitekey="key" data-theme="auto" data-action="sign_in" data-language="ja">/,
  );
  expect(form).toContain("This page uses a check.");
  expect(form).toMatch(/href="https:\/\/example.com\/privacy" target="_blank" rel="noopener noreferrer">Privacy/);
});

test("while only the login providers sign in, the screen offers them alone", () => {
  const html = render({
    ...base,
    screen: "sign_in",
    loginProvidersOnly: true,
    external: [{ label: "Example ID", url: "/auth/external/2/start", kind: "oidc" }],
  });
  expect(html).not.toContain("<form");
  expect(html).toContain("This server signs in only through the login providers.");
  expect(html).toContain("Example ID");
});

test("the two-factor code field is the one-time code's", () => {
  const html = render({ ...base, screen: "two_factor", returnTo: "/x", error: "Wrong." });
  expect(tagOf(html, "two-factor-code")).toContain('autoComplete="one-time-code"');
  expect(html).toContain('href="/auth/sign_in?return_to=%2Fx"');
  expect(html).toMatch(/role="alert"[^]*?<\/p><button[^>]*type="submit"/);
});

const signUpForm = {
  askInvite: true,
  askReason: true,
  usernameHint: "Up to 30.",
  passwordHint: "8 to 128.",
  reasonHint: "Why.",
  inviteHint: "Optional.",
  usernameMax: 30,
  passwordMin: 8,
  passwordMax: 128,
  reasonMax: 500,
};

test("a field's error sits under it and is read with it; the summary sits right before the button", () => {
  const html = render({
    ...base,
    screen: "sign_up",
    error: "Fix the marked fields.",
    documents: [{ path: "/about/terms", title: "Terms" }],
    signUp: { ...signUpForm, username: "alice", errors: { username: "Taken.", agreement: "Agree." } },
  });
  const username = tagOf(html, "sign-up-username");
  expect(username).toContain('aria-invalid="true"');
  expect(username).toContain('aria-describedby="sign-up-username-hint sign-up-username-error"');
  expect(username).toContain('value="alice"');
  expect(html.indexOf('id="sign-up-username"')).toBeLessThan(html.indexOf('id="sign-up-username-error"'));
  expect(tagOf(html, "sign-up-email")).not.toContain('aria-invalid="true"');
  expect(tagOf(html, "sign-up-agreement-check")).toContain('aria-invalid="true"');
  expect(html).toMatch(/Fix the marked fields\.<\/span><\/p><button[^>]*type="submit"/);
  // The documents are in the agreement, not repeated in the footer.
  expect(html.match(/href="\/about\/terms"/g)).toHaveLength(1);
});

test("the sign-up of an account at a login provider has no password and carries its ticket", () => {
  const html = render({
    ...base,
    screen: "sign_up",
    signUp: { ...signUpForm, external: { ticket: "sealed", provider: "GitHub" } },
  });
  expect(html).toContain('action="/auth/external/sign_up"');
  expect(html).toContain('<input type="hidden" name="ticket" value="sealed"/>');
  expect(html).not.toContain('name="password"');
  expect(html).toMatch(/<h1[^>]*>Create an account with GitHub<\/h1>/);
});

const consent: Consent = {
  clientName: "Tusky",
  redirectHost: "tusky.example",
  unverified: "Not verified.",
  origin: "Comes from tusky.example",
  impersonation: "Another app may claim it.",
  scopeGroups: [{ risk: "read", title: "Permissions that only read", scopes: [{ text: "Read posts" }] }],
  account: { handle: "@kumi@my.example", displayName: "Kumi" },
  responseType: "code",
  clientId: "https://tusky.example/client.json",
  redirectUri: "tusky://callback",
  scope: "read",
  state: "s",
  codeChallenge: "challenge",
  codeChallengeMethod: "S256",
};

test("the consent names the granting account and waits before Deny, at the start, and Allow, at the end", () => {
  const html = render({ ...base, screen: "authorize", consent });
  expect(html).toMatch(/<h1[^>]*>Authorize Tusky<\/h1>/);
  expect(html).toContain("Allowing as @kumi@my.example");
  expect(html).toContain("Another app may claim it.");
  expect(html).toContain(`you can choose in ${consentWaitSeconds} s.`);
  const deny = html.indexOf('value="deny"');
  const allow = html.indexOf('value="approve"');
  expect(deny).toBeGreaterThan(-1);
  expect(deny).toBeLessThan(allow);
  const buttons = html.match(/<button[^>]*>/g) ?? [];
  expect(buttons).toHaveLength(2);
  for (const button of buttons) expect(button).toContain("disabled");
  for (const [name, value] of [
    ["csrf_token", "token"],
    ["client_id", "https://tusky.example/client.json"],
    ["redirect_uri", "tusky://callback"],
    ["state", "s"],
    ["code_challenge", "challenge"],
    ["code_challenge_method", "S256"],
  ])
    expect(html).toContain(`<input type="hidden" name="${name}" value="${value}"/>`);
});

test("the consent's wait counts whole seconds down to 0, and no submission gets through while it lasts", () => {
  expect([0, 1, 999, 1000, 4001, 5000, 9000].map((ms) => secondsLeft(ms))).toEqual([5, 5, 5, 4, 1, 0, 0]);
  let prevented = 0;
  const event = { preventDefault: () => void prevented++ };
  holdWhileWaiting(3, event);
  holdWhileWaiting(1, event);
  holdWhileWaiting(0, event);
  expect(prevented).toBe(2);
});

test("a deactivated account's deletion sits after its reactivation, apart", () => {
  const html = render({
    ...base,
    screen: "status",
    status: {
      deactivated: true,
      deletionDate: "2026-12-14",
      deletionAt: "2026-12-14T00:00:00Z",
      deleteAsksForPassword: true,
    },
  });
  expect(html).toMatch(/<h1[^>]*>Your account is deactivated<\/h1>/);
  expect(html.indexOf('action="/auth/reactivate"')).toBeLessThan(html.indexOf('action="/auth/deactivated/delete"'));
  expect(html).toContain('<time dateTime="2026-12-14T00:00:00Z">');
  expect(tagOf(html, "delete-password")).toContain('autoComplete="current-password"');
});

test("a moderation action offers its appeal with the server's hint and the refused text", () => {
  const html = render({
    ...base,
    screen: "status",
    status: {
      restricted: true,
      actions: [
        {
          id: "a1",
          kind: "Suspended",
          category: "Reason: Spam",
          ends: "Ends in 6 days",
          canAppeal: true,
          appealHint: "Once, within 14 days.",
          appealMax: 2000,
          appealText: "It was not spam.",
          appealError: "Too long.",
        },
      ],
    },
  });
  expect(html).toContain('<input type="hidden" name="action_id" value="a1"/>');
  const field = tagOf(html, "appeal-a1");
  expect(field).toContain('aria-invalid="true"');
  expect(field).toContain('maxLength="2000"');
  expect(html).toContain("It was not spam.</textarea>");
});

test("the screens are worded in the page's locale", () => {
  expect(render({ ...base, screen: "sign_in" }, { locale: "ja" })).toMatch(/<h1[^>]*>ログイン<\/h1>/);
  expect(render({ ...base, screen: "error", error: "x" }, { locale: "ja" })).toMatch(
    /<h1[^>]*>問題が発生しました<\/h1>/,
  );
});

test("every screen renders", () => {
  for (const page of [
    { ...base, screen: "signed_in", returnTo: "/login" },
    { ...base, screen: "verify_email", token: "t" },
    { ...base, screen: "verified", returnTo: "/login?return_to=%2Fwelcome" },
    { ...base, screen: "account_deleted" },
    { ...base, screen: "forgot_password", forgotPassword: { sent: true }, notice: "Sent." },
    { ...base, screen: "forgot_password", forgotPassword: { unavailable: true } },
    { ...base, screen: "reset_password", token: "t", newPassword: { hint: "8 to 128.", min: 8, max: 128 } },
    { ...base, screen: "password_reset" },
    { ...base, screen: "mail_unsubscribe", token: "t&x", unsubscribe: { prompt: "Stop?" } },
    { ...base, screen: "mail_unsubscribed", unsubscribe: { done: "Stopped." } },
    { ...base, screen: "status", status: { pending: true, verifyEmail: "Confirm it." } },
  ] satisfies AuthPage[]) {
    const html = render(page);
    expect(html, page.screen).toMatch(/<h1[^>]*>[^<]+<\/h1>/);
  }
  expect(render({ ...base, screen: "mail_unsubscribe", token: "t&x", unsubscribe: { prompt: "Stop?" } })).toContain(
    'action="/mail/unsubscribe?token=t%26x"',
  );
  expect(render({ ...base, screen: "signed_in", returnTo: "/login" })).toMatch(/href="\/login"[^>]*>[^]*?Continue/);
});

test("the data block names its screen and its server, or the entry fails", () => {
  expect(parseAuthPage('{"screen":"sign_in","server":{"name":"x"}}').screen).toBe("sign_in");
  for (const bad of [null, "[]", '{"screen":"nope","server":{"name":"x"}}', '{"screen":"sign_in"}'])
    expect(() => parseAuthPage(bad)).toThrow();
});
