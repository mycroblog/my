// SPDX-License-Identifier: AGPL-3.0-only

// What each auth screen shows of the page the server gave it, and what its form submits to the server: the server's
// mark and documents, each field's name, autofill and error, the refusal that takes the focus when the page opens,
// the consent's wait. The entry moves the focus and counts the wait down; the screens are drawn as the server's page.

import { expect, onTestFinished, test } from "bun:test";
import { I18nProvider } from "@my/design-system";
import { cleanup, render, screen, within } from "@testing-library/react";
import { consentWaitSeconds } from "./consent-wait";
import type { AuthPage, Consent } from "./data";
import { AuthScreen } from "./screen";

const base = { server: { name: "my.example" }, csrfToken: "token" } as const;

/** The screen of `page`, alone in the document: its main part, and the form each of its buttons submits. */
function shown(page: AuthPage, { locale = "en", preview = false } = {}) {
  cleanup();
  render(
    <I18nProvider locale={locale}>
      <AuthScreen page={page} wordmark={{ light: "/light.svg", dark: "/dark.svg" }} preview={preview} />
    </I18nProvider>,
  );
  const main = within(screen.getByRole("main"));
  return {
    main,
    title: () => main.getByRole("heading", { level: 1 }).textContent,
    /** The form the button named `name` submits: where it posts, and the fields it sends. */
    form: (name: string) => {
      const { form } = main.getByRole<HTMLButtonElement>("button", { name });
      if (form === null) throw new Error(`test: ${name} submits no form`);
      return {
        form,
        action: form.getAttribute("action"),
        method: form.method,
        sends: Object.fromEntries(
          Array.from(new FormData(form), ([field, value]) => [field, typeof value === "string" ? value : value.name]),
        ),
      };
    },
  };
}

/** What describes `field`, part by part. */
const described = (field: Element) =>
  (field.getAttribute("aria-describedby") ?? "").split(" ").map((id) => document.getElementById(id)?.textContent ?? "");

const follows = (before: Element, after: Element) =>
  (before.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;

/** The pictures the document asks for, decoration all: the server's name says them. */
const pictures = () => screen.getAllByRole("presentation", { hidden: true }).map((image) => image.getAttribute("src"));

test("a screen is the server's page: its mark, its introduction, the title, and the footer of its documents", () => {
  const { title } = shown({
    ...base,
    screen: "sign_in",
    server: { name: "Hanami Town", logo: "/brand/1/logo.svg", description: "A town square.", banner: "/media/b.png" },
    documents: [{ path: "/about/terms", title: "Terms of service" }],
    statusPageUrl: "https://status.example",
  });

  const about = within(screen.getByRole("complementary", { name: "Hanami Town" }));
  expect(about.getByText("A town square.")).not.toBeNull();
  expect(pictures()).toContain("/media/b.png");
  expect(pictures()).toContain("/brand/1/logo.svg");
  expect(pictures()).not.toContain("/light.svg");
  expect(title()).toBe("Sign in");
  const documents = within(screen.getByRole("navigation", { name: "Terms and policies" }));
  expect(documents.getByRole("link", { name: "Terms of service" }).getAttribute("href")).toBe("/about/terms");
  expect(documents.getByRole("link", { name: "Service status" }).getAttribute("href")).toBe("https://status.example");
});

test("a server without a logo of its own shows the app's wordmark", () => {
  shown({ ...base, screen: "sign_in" });

  expect(pictures()).toContain("/light.svg");
});

test("the sign-in form posts the anti-forgery token, the return target and what was typed, to the server", () => {
  const { main, form } = shown({
    ...base,
    screen: "sign_in",
    returnTo: "/oauth/authorize?x=1",
    email: "a@example.com",
  });
  const email = main.getByLabelText<HTMLInputElement>("Email address");
  const password = main.getByLabelText<HTMLInputElement>("Password");

  expect([email.value, email.autocomplete, email.required]).toEqual(["a@example.com", "username", true]);
  expect([password.type, password.autocomplete, password.required]).toEqual(["password", "current-password", true]);
  const posted = form("Sign in");
  expect([posted.method, posted.action]).toEqual(["post", "/auth/sign_in"]);
  expect(posted.sends).toEqual({
    csrf_token: "token",
    return_to: "/oauth/authorize?x=1",
    email: "a@example.com",
    password: "",
  });
});

test("a refusal is an alert right before the form's button, which a script can focus and the entry focuses on load", () => {
  const { main } = shown({ ...base, screen: "sign_in", error: "The email address or password is incorrect." });
  const refusal = main.getByRole("alert");

  expect(refusal.textContent).toBe("The email address or password is incorrect.");
  expect(follows(main.getByLabelText("Password"), refusal)).toBe(true);
  expect(follows(refusal, main.getByRole("button", { name: "Sign in" }))).toBe(true);
  expect(refusal.tabIndex).toBe(-1);
  expect(refusal.hasAttribute("data-focus-on-load")).toBe(true);
});

test("a preview of many screens leaves the focus alone", () => {
  const { main } = shown({ ...base, screen: "sign_in", error: "No." }, { preview: true });

  expect(main.getByRole("alert").hasAttribute("data-focus-on-load")).toBe(false);
});

test("with passkeys, the email field offers them in its autofill; the providers are links by their names", () => {
  const { main } = shown({
    ...base,
    screen: "sign_in",
    passkey: { options: { challenge: "c" }, challenge: "sealed" },
    external: [{ label: "Sign in with GitHub", url: "/auth/external/1/start", kind: "github" }],
  });

  expect(main.getByLabelText<HTMLInputElement>("Email address").autocomplete).toBe("username webauthn");
  const others = within(main.getByRole("region", { name: "Or sign in with" }));
  expect(others.getByRole("link", { name: "Sign in with GitHub" }).getAttribute("href")).toBe("/auth/external/1/start");
});

test("the human check asks for the service's script and places its widget inside the form, with the notice it asks for", () => {
  // The simulation loads no script, and says so as an error: here a script it does not load counts as loaded.
  const { settings } = (
    window as unknown as { happyDOM: { settings: { handleDisabledFileLoadingAsSuccess: boolean } } }
  ).happyDOM;
  settings.handleDisabledFileLoadingAsSuccess = true;
  onTestFinished(() => {
    settings.handleDisabledFileLoadingAsSuccess = false;
  });
  const { form } = shown({
    ...base,
    screen: "sign_in",
    humanCheck: {
      scriptSrc: "https://challenges.example/api.js",
      containerClassName: "cf-turnstile",
      siteKey: "key",
      action: "sign_in",
      language: "ja",
      theme: "auto",
      notice: "This page uses a check.",
      links: [{ url: "https://example.com/privacy", title: "Privacy" }],
    },
  });
  const signIn = form("Sign in").form;

  expect(Array.from(document.scripts, (script) => script.src)).toContain("https://challenges.example/api.js");
  // What the service's script reads to draw its widget: its own class, and its settings as data attributes.
  const [widget] = signIn.getElementsByClassName("cf-turnstile");
  expect(widget instanceof HTMLElement ? Object.fromEntries(Object.entries(widget.dataset)) : undefined).toEqual({
    sitekey: "key",
    theme: "auto",
    action: "sign_in",
    language: "ja",
  });
  expect(within(signIn).getByText(/This page uses a check\./)).not.toBeNull();
  const privacy = within(signIn).getByRole<HTMLAnchorElement>("link", { name: /^Privacy/ });
  expect([privacy.getAttribute("href"), privacy.target]).toEqual(["https://example.com/privacy", "_blank"]);
  expect(privacy.rel.split(" ")).toContain("noopener");
});

test("while only the login providers sign in, the screen offers them alone", () => {
  const { main } = shown({
    ...base,
    screen: "sign_in",
    loginProvidersOnly: true,
    external: [{ label: "Example ID", url: "/auth/external/2/start", kind: "oidc" }],
  });

  expect(main.getByText(/On this server, you sign in only with another service\./)).not.toBeNull();
  expect(main.getByRole("link", { name: "Example ID" }).getAttribute("href")).toBe("/auth/external/2/start");
  expect(main.queryAllByRole("textbox")).toHaveLength(0);
  expect(main.queryAllByRole("button")).toHaveLength(0);
});

test("the two-factor code field is the one-time code's, its refusal sits right before its button, and a link leads back", () => {
  const { main } = shown({ ...base, screen: "two_factor", returnTo: "/x", error: "The code is incorrect." });
  const [code] = main.getAllByRole<HTMLInputElement>("textbox");
  const [submit] = main.getAllByRole("button");
  if (code === undefined || submit === undefined) throw new Error("test: the screen has no code field or no button");

  expect(code.autocomplete).toBe("one-time-code");
  const refusal = main.getByRole("alert");
  expect(refusal.textContent).toBe("The code is incorrect.");
  expect(follows(code, refusal) && follows(refusal, submit)).toBe(true);
  expect(main.getAllByRole("link").map((link) => link.getAttribute("href"))).toContain("/auth/sign_in?return_to=%2Fx");
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

test("a field's error sits under it and is read with its hint; the summary sits right before the button", () => {
  const { main } = shown({
    ...base,
    screen: "sign_up",
    error: "Fix the marked fields.",
    documents: [{ path: "/about/terms", title: "Terms" }],
    signUp: { ...signUpForm, username: "alice", errors: { username: "Taken.", agreement: "Agree." } },
  });
  const username = main.getByLabelText<HTMLInputElement>("Username");

  expect([username.value, username.maxLength, username.getAttribute("aria-invalid")]).toEqual(["alice", 30, "true"]);
  expect(described(username)).toEqual(["Up to 30.", "Taken."]);
  expect(follows(username, main.getByText("Taken."))).toBe(true);
  const email = main.getByLabelText("Email address");
  expect(email.getAttribute("aria-invalid")).toBeNull();
  const agreement = main.getByRole("checkbox", { name: /I have read and agree/ });
  expect(agreement.getAttribute("aria-invalid")).toBe("true");
  expect(described(agreement).at(-1)).toBe("Agree.");
  const summary = main.getByRole("alert");
  expect(summary.textContent).toBe("Fix the marked fields.");
  expect(follows(summary, main.getByRole("button", { name: "Create an account" }))).toBe(true);
});

test("the sign-up's fields say their limits, and its documents are in the agreement, not repeated in the footer", () => {
  const { main, form } = shown({
    ...base,
    screen: "sign_up",
    documents: [{ path: "/about/terms", title: "Terms" }],
    signUp: signUpForm,
  });
  const password = main.getByLabelText<HTMLInputElement>("Password");

  expect([password.autocomplete, password.minLength, password.maxLength]).toEqual(["new-password", 8, 128]);
  expect(described(password)).toEqual(["8 to 128."]);
  expect(main.getByLabelText<HTMLTextAreaElement>(/Why do you want to join/).maxLength).toBe(500);
  const terms = screen.getAllByRole<HTMLAnchorElement>("link", { name: /^Terms/ });
  expect(terms.map((link) => [link.getAttribute("href"), link.target])).toEqual([["/about/terms", "_blank"]]);
  expect(form("Create an account").action).toBe("/auth/sign_up");
  expect(Object.keys(form("Create an account").sends).sort()).toEqual([
    "csrf_token",
    "email",
    "invite_code",
    "password",
    "reason",
    "username",
  ]);
});

test("the sign-up of an account at a login provider has no password and carries its ticket", () => {
  const { title, form } = shown({
    ...base,
    screen: "sign_up",
    signUp: { ...signUpForm, external: { ticket: "sealed", provider: "GitHub" } },
  });

  expect(title()).toBe("Create an account with GitHub");
  const posted = form("Create an account");
  expect(posted.action).toBe("/auth/external/sign_up");
  expect(posted.sends["ticket"]).toBe("sealed");
  expect(Object.keys(posted.sends)).not.toContain("password");
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

test("the consent says who asks for what and as whom it is granted", () => {
  const { main, title } = shown({ ...base, screen: "authorize", consent });

  expect(title()).toBe("Allow Tusky to use your account");
  for (const words of ["Not verified.", "Comes from tusky.example", "Another app may claim it."])
    expect(main.getByText(words)).not.toBeNull();
  const reads = within(main.getByRole("region", { name: "Permissions that only read" }));
  expect(reads.getAllByRole("listitem").map((item) => item.textContent)).toEqual(["Read posts"]);
  expect(main.getByText("Allowing as @kumi@my.example")).not.toBeNull();
});

test("the consent waits before Deny, at the start, and Allow, at the end, and posts what the app asked with", () => {
  const { main, form } = shown({ ...base, screen: "authorize", consent });
  const deny = main.getByRole<HTMLButtonElement>("button", { name: "Deny" });
  const allow = main.getByRole<HTMLButtonElement>("button", { name: "Allow" });

  expect(main.getByText(new RegExp(`you can choose in ${String(consentWaitSeconds)} s\\.$`))).not.toBeNull();
  expect(main.getAllByRole("button")).toEqual([deny, allow]);
  expect([deny.disabled, allow.disabled]).toEqual([true, true]);
  expect([deny.name, deny.value, allow.name, allow.value]).toEqual(["decision", "deny", "decision", "approve"]);
  const posted = form("Allow");
  expect([posted.method, posted.action]).toEqual(["post", "/oauth/authorize"]);
  expect(posted.sends).toEqual({
    csrf_token: "token",
    response_type: "code",
    client_id: "https://tusky.example/client.json",
    redirect_uri: "tusky://callback",
    scope: "read",
    state: "s",
    code_challenge: "challenge",
    code_challenge_method: "S256",
  });
});

test("a deactivated account's deletion sits after its reactivation, apart, and asks for the password", () => {
  const { main, title, form } = shown({
    ...base,
    screen: "status",
    status: {
      deactivated: true,
      deletionDate: "2026-12-14",
      deletionAt: "2026-12-14T00:00:00Z",
      deleteAsksForPassword: true,
    },
  });

  expect(title()).toBe("Your account is deactivated");
  expect(main.getByRole<HTMLTimeElement>("time").dateTime).toBe("2026-12-14T00:00:00Z");
  const reactivate = form("Reactivate your account");
  expect(reactivate.action).toBe("/auth/reactivate");
  const deletion = main.getByRole("region", { name: "Delete it now" });
  expect(follows(reactivate.form, deletion)).toBe(true);
  expect(form("Delete account").action).toBe("/auth/deactivated/delete");
  const password = within(deletion).getByLabelText<HTMLInputElement>("Current password");
  expect([password.autocomplete, password.required]).toEqual(["current-password", true]);
  expect(described(password)[0]).toMatch(/^This cannot be undone\./);
});

test("a moderation action offers its appeal with the server's hint and the refused text", () => {
  const { main } = shown({
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
  const [appeal] = main.getAllByRole<HTMLTextAreaElement>("textbox");
  if (appeal === undefined) throw new Error("test: the screen has no field of an appeal");

  expect([appeal.value, appeal.maxLength, appeal.getAttribute("aria-invalid")]).toEqual([
    "It was not spam.",
    2000,
    "true",
  ]);
  expect(described(appeal)).toEqual(["Once, within 14 days.", "Too long."]);
  expect(new FormData(appeal.form ?? undefined).get("action_id")).toBe("a1");
  for (const words of ["Suspended", "Reason: Spam", "Ends in 6 days"]) expect(main.getByText(words)).not.toBeNull();
});

test("the screens are worded in the page's locale", () => {
  expect(shown({ ...base, screen: "sign_in" }, { locale: "ja" }).title()).toBe("ログイン");
  expect(shown({ ...base, screen: "error", error: "x" }, { locale: "ja" }).title()).toBe("問題が発生しました");
});

test("every screen has its title, the page's one heading of the first level", () => {
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
    shown(page);
    expect([
      page.screen,
      screen.getAllByRole("heading", { level: 1 }).map((title) => title.textContent !== ""),
    ]).toEqual([page.screen, [true]]);
  }
});

test("the unsubscribe posts to its token's address, and a reader signed in continues where they came from", () => {
  const unsubscribe = shown({ ...base, screen: "mail_unsubscribe", token: "t&x", unsubscribe: { prompt: "Stop?" } });
  const [button] = unsubscribe.main.getAllByRole<HTMLButtonElement>("button");
  expect(button?.form?.getAttribute("action")).toBe("/mail/unsubscribe?token=t%26x");

  const signedIn = shown({ ...base, screen: "signed_in", returnTo: "/login" });
  expect(signedIn.main.getByRole("link", { name: /Continue/ }).getAttribute("href")).toBe("/login");
});
