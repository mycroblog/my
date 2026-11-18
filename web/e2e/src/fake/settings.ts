// SPDX-License-Identifier: AGPL-3.0-only

// The fake's operations of the reader's account settings (FR-002, FR-004, FR-005, FR-010, FR-012; ADR-0090, ADR-0106):
// how they sign in (the password, passkeys, two-factor authentication, linked accounts), the browsers and apps signed
// in to the account, the apps they register, and the invites they create. The state is the fake's own, in memory, and
// the sample data is the reader's at the start; `SettingsOptions` changes it. The server decides the codes and
// tokens for real; here the authenticator's code is `fakeTotpCode`, and the confirmation a linked provider gives
// `fakeExternalConfirmation`, since a link leaves the page for the provider and no test can make it come back with one.

import type { Handlers, Refusal, Schema } from "./spec";

/** The code of the reader's authenticator app, which the fake accepts as a two-factor code. */
export const fakeTotpCode = "123456";

/** The confirmation of a linked login provider that the fake accepts, for any operation, as many times as it is given. */
export const fakeExternalConfirmation = "fake-external-confirmation";

/** The most passkeys, and OAuth apps, an account holds. */
const maxPasskeys = 20;
const maxOAuthApps = 25;

/** How the reader's account stands at the start. */
export type SettingsOptions = {
  /** Whether the reader signs in with a password; they do by default. Without one a passkey confirms. */
  password?: boolean;
  /** How many passkeys they hold, 3 by default (a laptop, a phone and a security key); at most 20. */
  passkeys?: number;
  /** Two-factor authentication is on, with ten recovery codes left; off by default. */
  twoFactor?: boolean;
  /** One linked account (GitHub) by default, among three login providers; `none` links none, `no-providers` has none. */
  linked?: "some" | "none" | "no-providers";
  /** Browsers signed in: four by default, the first the one that asks; `alone` is the one that asks only. */
  sessions?: "several" | "alone";
  /** Apps authorized: the server's web client and three apps by default. */
  authorizedApps?: "some" | "none";
  /** The reader's own OAuth apps: three by default (a public, a confidential, a disabled one), or none; `disabled` is a server that does not let users register apps. */
  oauthApps?: "some" | "none" | "disabled";
  /**
   * The reader's invites: one in each state by default (and two more usable); `none` has none; `not-allowed` is a
   * reader who may no longer create invites (every invite that would work is then `creator_ineligible`); `at-cap` holds
   * as many usable invites as the instance allows.
   */
  invites?: "all-states" | "none" | "not-allowed" | "at-cap";
};

/** What the handlers need of the fake that holds them. */
export type SettingsContext = {
  options: SettingsOptions;
  /** The username and password of the reader. */
  viewer: { username: string; password: string };
  /** When the fake started, the origin it answers as, and whether the instance is a one-person server. */
  started: number;
  origin: string;
  singleUser: boolean;
  problem: (status: number, title: string, more?: Partial<Schema<"Problem">>) => Refusal;
  notFound: () => Refusal;
  idAt: (ms: number, n: number) => string;
  page: <T extends { id: string }>(
    items: readonly T[],
    query: { get: (name: "cursor" | "limit") => string | null },
  ) => { body: { items: T[]; next_cursor?: string } };
};

const minute = 60_000;
const hour = 60 * minute;
const day = 24 * hour;

type Passkey = Schema<"Passkey">;
type Session = Schema<"BrowserSession">;
type App = Schema<"AuthorizedApp">;
type OAuthApp = Schema<"OAuthApp">;
type Invite = Schema<"OwnInvite">;

/** A stored invite: what the server keeps, from which its state is read. */
type StoredInvite = Omit<Invite, "state"> & { revoked: boolean; ineligible: boolean };

const passkeyNames = ["MacBook Touch ID", "iPhone (iCloud Keychain)", "YubiKey 5C", "Pixel (Google Password Manager)"];

/** A code of the recovery codes: ten, shown once. */
const recoveryCodes = (round: number) =>
  Array.from(
    { length: 10 },
    (_, i) =>
      `${(round * 7919 + i * 104729).toString(36).padStart(5, "q")}-${(i * 31 + round * 17 + 1000).toString(36)}`,
  );

/** The handlers of the operations of the reader's account settings, and nothing else of the fake. */
export function settingsHandlers(c: SettingsContext): Handlers {
  const { options: o, started, origin, problem, notFound, idAt } = c;
  const time = (ago: number) => new Date(started - ago).toISOString();
  let counter = 0;
  const next = () => (counter += 1);

  // ---- The password and the confirmations ---------------------------------------------------------------------------

  let password: string | null = o.password === false ? null : c.viewer.password;

  const passkeys: Passkey[] = Array.from({ length: Math.min(o.passkeys ?? 3, maxPasskeys) }, (_, i) => ({
    id: idAt(started - (40 - i * 9) * day, 70_000 + i),
    name: passkeyNames[i] ?? `Passkey ${String(i + 1)}`,
    created_at: time((40 - i * 9) * day),
  }));

  /** The tokens of passkey confirmations the server made and nobody used (5 minutes), and of registrations. */
  const confirmations = new Set<string>();
  const registrations = new Map<string, string>();

  /** The wrong-confirmation problems, as the operations that confirm answer them. */
  const wrongPassword = () =>
    problem(422, "Wrong password", { type: "/api/v1/problems/wrong-password", field: "password" });
  const invalidPasskey = () => problem(422, "The passkey did not verify", { type: "/api/v1/problems/invalid-passkey" });
  const confirmationExpired = () =>
    problem(422, "The confirmation expired", { type: "/api/v1/problems/passkey-confirmation-expired" });
  const externalExpired = () =>
    problem(422, "The confirmation expired", { type: "/api/v1/problems/external-confirmation-expired" });

  type Confirmation = {
    password?: string | undefined;
    passkey?: Schema<"PasskeyAssertion"> | undefined;
    external_confirmation?: string | undefined;
  };

  /** Checks what confirms an operation: the password, a passkey's assertion, or a linked provider's confirmation. */
  function confirm(given: Confirmation, field: "password" | "current_password" = "password"): Refusal | null {
    if (given.external_confirmation !== undefined)
      return given.external_confirmation === fakeExternalConfirmation ? null : externalExpired();
    if (given.passkey !== undefined) {
      const token = given.passkey.confirmation_token;
      if (!confirmations.has(token)) return confirmationExpired();
      if (typeof given.passkey.credential.id !== "string" || given.passkey.credential.id === "")
        return invalidPasskey();
      confirmations.delete(token);
      return null;
    }
    if (given.password !== undefined && password !== null && given.password === password) return null;
    return given.password === undefined
      ? problem(422, "Confirm with your password or a passkey", { type: "/api/v1/problems/wrong-password", field })
      : { problem: { ...wrongPassword().problem, field } };
  }

  // ---- Two-factor authentication ------------------------------------------------------------------------------------

  let twoFactor = o.twoFactor === true;
  let enrolledAt = time(12 * day);
  let codes = twoFactor ? recoveryCodes(1) : [];
  let rounds = 1;
  const setups = new Set<string>();

  const twoFactorBody = (): Schema<"TwoFactor"> =>
    twoFactor ? { enabled: true, enrolled_at: enrolledAt, recovery_codes_remaining: codes.length } : { enabled: false };

  /** Checks a two-factor code: the app's, or an unused recovery code (which it uses up, once `use`). */
  function checkCode(code: string, use: boolean): Refusal | null {
    if (code === fakeTotpCode) return null;
    const at = codes.indexOf(code);
    if (at < 0) return problem(422, "Wrong code", { type: "/api/v1/problems/invalid-two-factor-code", field: "code" });
    if (use) codes.splice(at, 1);
    return null;
  }

  // ---- Linked accounts ----------------------------------------------------------------------------------------------

  const providers: Schema<"LinkableLoginProvider">[] =
    o.linked === "no-providers"
      ? []
      : [
          { id: idAt(started - 90 * day, 71_000), kind: "github", display_name: "GitHub", allow_link: true },
          { id: idAt(started - 90 * day, 71_001), kind: "oidc", display_name: "Example SSO", allow_link: true },
          {
            id: idAt(started - 60 * day, 71_002),
            kind: "mastodon",
            display_name: "mastodon.example",
            allow_link: false,
          },
        ];
  const linked: Schema<"LinkedIdentity">[] =
    o.linked === "none" || o.linked === "no-providers"
      ? []
      : providers.slice(0, 1).map((p, i) => ({
          id: idAt(started - 30 * day, 72_000 + i),
          provider_id: p.id,
          provider_kind: p.kind,
          provider_name: p.display_name,
          linked_at: time(30 * day),
          available: true,
        }));

  /** The browser's way off to a provider, which comes back to the settings with the outcome in the fragment. */
  const start = (provider: Schema<"LinkableLoginProvider">, intent: string): Schema<"ExternalAuthorizationStart"> => ({
    start_url: `${origin}/auth/external/${provider.id}/${intent}?state=${String(next())}`,
    expires_at: new Date(Date.now() + 10 * minute).toISOString(),
  });

  // ---- Browsers and authorized apps ---------------------------------------------------------------------------------

  const allSessions: Session[] = [
    {
      id: idAt(started - 3 * day, 73_000),
      current: true,
      ip: "203.0.113.24",
      user_agent: "Mozilla/5.0 (X11; Linux x86_64; rv:146.0) Gecko/20100101 Firefox/146.0",
      signed_in_at: time(3 * day),
      last_used_at: time(0),
    },
    {
      id: idAt(started - 9 * day, 73_001),
      current: false,
      ip: "198.51.100.77",
      user_agent:
        "Mozilla/5.0 (iPhone; CPU iPhone OS 19_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.1 Mobile/15E148 Safari/604.1",
      signed_in_at: time(9 * day),
      last_used_at: time(2 * hour),
    },
    {
      id: idAt(started - 21 * day, 73_002),
      current: false,
      ip: "192.0.2.150",
      user_agent:
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36",
      signed_in_at: time(21 * day),
      last_used_at: time(5 * day),
    },
    {
      // A browser whose origin the server no longer keeps (ADR-0090 §11): neither address nor browser is known.
      id: idAt(started - 120 * day, 73_003),
      current: false,
      signed_in_at: time(120 * day),
    },
  ];
  let sessions: Session[] = o.sessions === "alone" ? allSessions.slice(0, 1) : allSessions;

  const allApps: App[] = [
    {
      id: idAt(started - 3 * day, 74_000),
      client_id: "web",
      name: "mycroblog",
      registration: "first_party",
      first_party: true,
      scope: "read write follow push",
      authorized_at: time(120 * day),
      last_used_at: time(0),
    },
    {
      id: idAt(started - 3 * day, 74_001),
      client_id: "tuskr-android",
      name: "Tuskr for Android",
      website: "https://tuskr.example/",
      registration: "dynamic",
      first_party: false,
      scope: "read write:statuses write:favourites write:media push",
      authorized_at: time(40 * day),
      last_used_at: time(3 * hour),
    },
    {
      id: idAt(started - 3 * day, 74_002),
      client_id: "https://reader.example/client.json",
      name: "Feedlet",
      website: "https://reader.example/",
      registration: "metadata_document",
      first_party: false,
      scope: "read:accounts read:statuses",
      authorized_at: time(15 * day),
      last_used_at: time(2 * day),
    },
    {
      id: idAt(started - 3 * day, 74_003),
      client_id: "yuki-bot",
      name: "Morning bot",
      registration: "user",
      registered_by: "hana",
      first_party: false,
      scope: "write:statuses",
      authorized_at: time(6 * day),
    },
  ];
  let apps: App[] = o.authorizedApps === "none" ? [] : allApps;

  // ---- The reader's own OAuth apps ----------------------------------------------------------------------------------

  const ownApps: OAuthApp[] =
    o.oauthApps === "none" || o.oauthApps === "disabled"
      ? []
      : [
          {
            id: idAt(started - 50 * day, 75_000),
            client_id: "cl_7f3a91c0e2b84d5a",
            name: "Morning bot",
            website: "https://bot.example/",
            redirect_uris: ["urn:ietf:wg:oauth:2.0:oob"],
            scope: "read write:statuses",
            client_type: "confidential",
            disabled: false,
            created_at: time(50 * day),
          },
          {
            id: idAt(started - 20 * day, 75_001),
            client_id: "cl_2d90b6a41c7e3f58",
            name: "Home dashboard",
            redirect_uris: ["https://dash.example/callback", "http://localhost:5173/callback"],
            scope: "read",
            client_type: "public",
            disabled: false,
            created_at: time(20 * day),
          },
          {
            id: idAt(started - 200 * day, 75_002),
            client_id: "cl_a41e8c07d95b2f63",
            name: "Old importer",
            redirect_uris: ["https://importer.example/done"],
            scope: "read write",
            client_type: "confidential",
            disabled: true,
            created_at: time(200 * day),
          },
        ];

  const secretOf = () => `secret-${String(next()).padStart(4, "0")}-${(started % 99_991).toString(36)}`;

  /** The fault of a setting of an app, as the server names it, or none. */
  function appFault(s: {
    name?: string | undefined;
    redirect_uris?: string[] | undefined;
    scope?: string | undefined;
  }) {
    if (s.name !== undefined && (s.name.trim() === "" || s.name.length > 100))
      return problem(422, "The name is not 1 to 100 characters", { field: "name" });
    if (s.redirect_uris !== undefined) {
      if (s.redirect_uris.length === 0) return problem(422, "An app needs a redirect URI", { field: "redirect_uris" });
      if (!s.redirect_uris.every((u) => URL.canParse(u)))
        return problem(422, "A redirect URI is not a URI", { field: "redirect_uris" });
    }
    if (s.scope !== undefined && s.scope.trim() === "") return problem(422, "An app needs a scope", { field: "scope" });
    return null;
  }

  // ---- The reader's invites -----------------------------------------------------------------------------------------

  const limits: Schema<"MemberInviteLimits"> = {
    max_active: 5,
    max_uses: 10,
    max_expires_in: 30 * 24 * 3600,
    default_max_uses: 1,
    default_expires_in: 7 * 24 * 3600,
  };
  const canCreate = o.invites !== "not-allowed";

  const code = (n: number) =>
    `${(n * 2_654_435_761).toString(36).padStart(8, "k")}${(n * 40_503 + 99_991).toString(36).padStart(8, "m")}`.slice(
      0,
      16,
    );
  const invite = (
    n: number,
    ago: number,
    life: number,
    uses: number,
    maxUses: number,
    more: Partial<StoredInvite> = {},
  ): StoredInvite => ({
    id: idAt(started - ago, 76_000 + n),
    code: code(n),
    url: `${origin}/auth/sign_up?invite=${code(n)}`,
    max_uses: maxUses,
    uses,
    expires_at: new Date(started - ago + life).toISOString(),
    created_at: time(ago),
    revoked: false,
    ineligible: false,
    ...more,
  });

  // Newest first: two that work, then one in each other state.
  const invites: StoredInvite[] =
    o.invites === "none"
      ? []
      : [
          invite(1, 2 * hour, 7 * day, 0, 1),
          invite(2, 1 * day, 6 * day, 1, 5),
          invite(3, 3 * day, 7 * day, 0, 3, { revoked: true }),
          invite(4, 10 * day, 7 * day, 1, 1),
          invite(5, 12 * day, 30 * day, 4, 4),
          invite(6, 14 * day, 30 * day, 0, 2, { ineligible: true }),
        ];
  if (o.invites === "at-cap")
    while (invites.filter((i) => stateOf(i) === "usable").length < limits.max_active)
      invites.unshift(invite(100 + invites.length, (invites.length + 1) * hour, 7 * day, 0, 1));

  function stateOf(i: StoredInvite): Schema<"AdminInviteState"> {
    if (i.revoked) return "revoked";
    if (Date.parse(i.expires_at) <= Date.now()) return "expired";
    if (i.uses >= i.max_uses) return "exhausted";
    if (i.ineligible || !canCreate) return "creator_ineligible";
    return "usable";
  }
  const inviteBody = ({ revoked: _revoked, ineligible: _ineligible, ...rest }: StoredInvite): Invite => ({
    ...rest,
    state: stateOf({ revoked: _revoked, ineligible: _ineligible, ...rest }),
  });
  const singleUserRefusal = () => problem(404, "Not Found", { type: "/api/v1/problems/single-user-mode" });

  // ---- The operations -----------------------------------------------------------------------------------------------

  return {
    // Password
    "GET /accounts/password": () => ({ status: 200, body: { has_password: password !== null } }),
    "POST /accounts/password": ({ body }) => {
      const refused = confirm({ ...body, password: body.current_password }, "current_password");
      if (refused !== null) return refused;
      if (body.new_password.length < 8 || body.new_password.length > 128)
        return problem(422, "The password is not 8 to 128 characters", { field: "new_password" });
      password = body.new_password;
      // Every other session and grant ends; the app that asks stays.
      sessions = sessions.filter((s) => s.current);
      apps = apps.filter((a) => a.first_party);
      return { status: 204 };
    },
    "POST /accounts/password/remove": ({ body }) => {
      if (password === null) return { status: 204 };
      if (passkeys.length === 0 && linked.length === 0)
        return problem(409, "This is your last way to sign in", { type: "/api/v1/problems/last-sign-in-method" });
      const refused = confirm(body);
      if (refused !== null) return refused;
      password = null;
      sessions = sessions.filter((s) => s.current);
      apps = apps.filter((a) => a.first_party);
      return { status: 204 };
    },

    // Passkeys
    "GET /passkeys": () => ({ status: 200, body: { items: passkeys } }),
    "POST /passkeys/registration": ({ body }) => {
      if (passkeys.length >= maxPasskeys)
        return problem(409, "You have as many passkeys as you may", { type: "/api/v1/problems/too-many-passkeys" });
      const refused = confirm(body);
      if (refused !== null) return refused;
      const token = `registration-${String(next())}`;
      registrations.set(token, body.name.trim() === "" ? "Passkey" : body.name.trim());
      return {
        status: 200,
        body: {
          options: {
            rp: { id: new URL(origin).hostname, name: "mycroblog" },
            user: { id: btoa(c.viewer.username), name: c.viewer.username, displayName: c.viewer.username },
            challenge: btoa(token),
            pubKeyCredParams: [
              { type: "public-key", alg: -7 },
              { type: "public-key", alg: -257 },
            ],
            authenticatorSelection: { residentKey: "required", userVerification: "required" },
            attestation: "none",
            timeout: 300_000,
          },
          registration_token: token,
          expires_at: new Date(Date.now() + 5 * minute).toISOString(),
        },
      };
    },
    "POST /passkeys": ({ body }) => {
      const name = registrations.get(body.registration_token);
      if (name === undefined)
        return problem(422, "The registration expired", { type: "/api/v1/problems/passkey-registration-expired" });
      if (typeof body.credential.id !== "string" || body.credential.id === "") return invalidPasskey();
      if (passkeys.length >= maxPasskeys)
        return problem(409, "You have as many passkeys as you may", { type: "/api/v1/problems/too-many-passkeys" });
      registrations.delete(body.registration_token);
      const made: Passkey = { id: idAt(Date.now(), 70_500 + next()), name, created_at: new Date().toISOString() };
      passkeys.push(made);
      return { status: 201, body: made };
    },
    "POST /passkeys/confirmation": () => {
      const token = `confirmation-${String(next())}`;
      confirmations.add(token);
      return {
        status: 200,
        body: {
          options: {
            rpId: new URL(origin).hostname,
            challenge: btoa(token),
            allowCredentials: passkeys.map((p) => ({ type: "public-key", id: btoa(p.id) })),
            userVerification: "required",
            timeout: 300_000,
          },
          confirmation_token: token,
          expires_at: new Date(Date.now() + 5 * minute).toISOString(),
        },
      };
    },
    "DELETE /passkeys/{id}": ({ params }) => {
      const at = passkeys.findIndex((p) => p.id === params.id);
      if (at < 0) return notFound();
      if (password === null && linked.length === 0 && passkeys.length === 1)
        return problem(409, "This is your last way to sign in", { type: "/api/v1/problems/last-sign-in-method" });
      passkeys.splice(at, 1);
      return { status: 204 };
    },

    // Two-factor authentication
    "GET /two_factor": () => ({ status: 200, body: twoFactorBody() }),
    "POST /two_factor/totp/setup": () => {
      if (twoFactor)
        return problem(409, "Two-factor authentication is on", { type: "/api/v1/problems/two-factor-enabled" });
      const token = `setup-${String(next())}`;
      setups.add(token);
      const secret = "JBSWY3DPEHPK3PXP";
      return {
        status: 200,
        body: {
          secret,
          uri: `otpauth://totp/${new URL(origin).hostname}:${c.viewer.username}?secret=${secret}&issuer=${new URL(origin).hostname}`,
          qr_code: { size: 5, path: "M0 0h3v1H0zM4 0h1v3H4zM0 2h1v3H0zM2 3h3v1H2z" },
          setup_token: token,
          expires_at: new Date(Date.now() + 15 * minute).toISOString(),
        },
      };
    },
    "POST /two_factor/totp": ({ body }) => {
      if (!setups.has(body.setup_token))
        return problem(422, "The setup expired", { type: "/api/v1/problems/totp-setup-expired" });
      if (body.code !== fakeTotpCode)
        return problem(422, "Wrong code", { type: "/api/v1/problems/invalid-two-factor-code", field: "code" });
      setups.delete(body.setup_token);
      twoFactor = true;
      enrolledAt = new Date().toISOString();
      rounds += 1;
      codes = recoveryCodes(rounds);
      return { status: 200, body: { recovery_codes: [...codes] } };
    },
    "POST /two_factor/totp/disable": ({ body }) => {
      if (!twoFactor)
        return problem(409, "Two-factor authentication is off", { type: "/api/v1/problems/two-factor-not-enabled" });
      const refused = checkCode(body.code, true);
      if (refused !== null) return refused;
      twoFactor = false;
      codes = [];
      return { status: 200, body: twoFactorBody() };
    },
    "POST /two_factor/recovery_codes": ({ body }) => {
      if (!twoFactor)
        return problem(409, "Two-factor authentication is off", { type: "/api/v1/problems/two-factor-not-enabled" });
      const refused = checkCode(body.code, false);
      if (refused !== null) return refused;
      rounds += 1;
      codes = recoveryCodes(rounds);
      return { status: 200, body: { recovery_codes: [...codes] } };
    },

    // Linked accounts
    "GET /linked_identities": () => ({ status: 200, body: { items: linked, providers } }),
    "POST /linked_identities": ({ body }) => {
      const provider = providers.find((p) => p.id === body.provider_id);
      if (provider === undefined || !provider.allow_link)
        return problem(422, "The login provider is not available", {
          type: "/api/v1/problems/login-provider-unavailable",
        });
      if (linked.some((l) => l.provider_id === provider.id))
        return problem(409, "You link an account at this provider already", {
          type: "/api/v1/problems/login-provider-linked",
        });
      const refused = confirm(body);
      if (refused !== null) return refused;
      return { status: 200, body: start(provider, "link") };
    },
    "POST /linked_identities/confirmation": ({ body }) => {
      const provider = providers.find((p) => p.id === body.provider_id);
      if (provider === undefined || !linked.some((l) => l.provider_id === provider.id))
        return problem(422, "The login provider is not available", {
          type: "/api/v1/problems/login-provider-unavailable",
        });
      return { status: 200, body: start(provider, body.operation) };
    },
    "DELETE /linked_identities/{id}": ({ params }) => {
      const at = linked.findIndex((l) => l.id === params.id);
      if (at < 0) return notFound();
      if (password === null && passkeys.length === 0 && linked.length === 1)
        return problem(409, "This is your last way to sign in", { type: "/api/v1/problems/last-sign-in-method" });
      linked.splice(at, 1);
      return { status: 204 };
    },

    // Browsers
    "GET /sessions": () => ({ status: 200, body: { items: sessions } }),
    "DELETE /sessions/{id}": ({ params }) => {
      const found = sessions.find((s) => s.id === params.id);
      if (found === undefined) return notFound();
      if (found.current)
        return problem(409, "This is the browser that asks", { type: "/api/v1/problems/current-session" });
      sessions = sessions.filter((s) => s !== found);
      return { status: 204 };
    },
    "POST /sessions/revoke_others": () => {
      sessions = sessions.filter((s) => s.current);
      return { status: 204 };
    },

    // Authorized apps
    "GET /authorized_apps": () => ({ status: 200, body: { items: apps } }),
    "DELETE /authorized_apps/{id}": ({ params }) => {
      const found = apps.find((a) => a.id === params.id);
      if (found === undefined) return notFound();
      apps = apps.filter((a) => a !== found);
      // The server's own web client ends every browser, this one too.
      if (found.first_party) sessions = [];
      return { status: 204 };
    },

    // The reader's OAuth apps
    "GET /oauth_apps": () => ({ status: 200, body: { items: ownApps } }),
    "POST /oauth_apps": ({ body }) => {
      if (o.oauthApps === "disabled")
        return problem(403, "Users may not register apps here", {
          type: "/api/v1/problems/oauth-app-registration-disabled",
        });
      const fault = appFault(body);
      if (fault !== null) return fault;
      if (ownApps.length >= maxOAuthApps)
        return problem(409, "You have as many apps as you may", { type: "/api/v1/problems/too-many-oauth-apps" });
      const n = next();
      const app: OAuthApp = {
        id: idAt(Date.now(), 75_500 + n),
        client_id: `cl_${n.toString(16).padStart(16, "0")}`,
        name: body.name.trim(),
        ...(body.website === undefined || body.website === "" ? {} : { website: body.website }),
        redirect_uris: body.redirect_uris,
        scope: body.scope,
        client_type: body.client_type,
        disabled: false,
        created_at: new Date().toISOString(),
      };
      ownApps.push(app);
      return {
        status: 201,
        body: { app, ...(app.client_type === "confidential" ? { client_secret: secretOf() } : {}) },
      };
    },
    "GET /oauth_apps/{id}": ({ params }) => {
      const found = ownApps.find((a) => a.id === params.id);
      return found === undefined ? notFound() : { status: 200, body: found };
    },
    "PATCH /oauth_apps/{id}": ({ params, body }) => {
      const at = ownApps.findIndex((a) => a.id === params.id);
      const found = ownApps[at];
      if (found === undefined) return notFound();
      const fault = appFault(body);
      if (fault !== null) return fault;
      const changed: OAuthApp = {
        ...found,
        ...(body.name === undefined ? {} : { name: body.name.trim() }),
        ...(body.website === undefined ? {} : { website: body.website }),
        ...(body.redirect_uris === undefined ? {} : { redirect_uris: body.redirect_uris }),
        ...(body.scope === undefined ? {} : { scope: body.scope }),
      };
      ownApps[at] = changed;
      return { status: 200, body: changed };
    },
    "DELETE /oauth_apps/{id}": ({ params }) => {
      const at = ownApps.findIndex((a) => a.id === params.id);
      if (at < 0) return notFound();
      ownApps.splice(at, 1);
      return { status: 204 };
    },
    "POST /oauth_apps/{id}/rotate_secret": ({ params }) => {
      const found = ownApps.find((a) => a.id === params.id);
      if (found === undefined) return notFound();
      if (found.client_type === "public")
        return problem(409, "A public app has no secret", { type: "/api/v1/problems/public-oauth-app" });
      return { status: 200, body: { client_secret: secretOf() } };
    },
    "POST /oauth_apps/{id}/tokens": ({ params, body }) => {
      const found = ownApps.find((a) => a.id === params.id);
      if (found === undefined) return notFound();
      if (found.disabled)
        return problem(409, "An administrator disabled this app", { type: "/api/v1/problems/oauth-app-disabled" });
      const wanted = (body?.scope ?? found.scope).split(" ").filter((s) => s !== "");
      const allowed = found.scope.split(" ");
      // A scope is within the app's when it is one of them or under one (`write` holds `write:statuses`).
      if (!wanted.every((s) => allowed.some((a) => a === s || s.startsWith(`${a}:`))))
        return problem(422, "The scope goes beyond the app's", { field: "scope" });
      const n = next();
      return {
        status: 201,
        body: {
          access_token: `at-${String(n).padStart(4, "0")}-${(started % 99_991).toString(36)}`,
          token_type: "Bearer",
          expires_in: 7200,
          refresh_token: `rt-${String(n).padStart(4, "0")}-${(started % 99_989).toString(36)}`,
          scope: wanted.join(" "),
        },
      };
    },

    // The reader's invites
    "GET /invites": ({ query }) => {
      if (c.singleUser) return singleUserRefusal();
      const { body } = c.page(invites.map(inviteBody), query);
      return {
        status: 200,
        body: {
          ...body,
          can_create: canCreate,
          usable: invites.filter((i) => stateOf(i) === "usable").length,
          limits,
        },
      };
    },
    "POST /invites": ({ body }) => {
      if (c.singleUser) return singleUserRefusal();
      if (!canCreate)
        return problem(403, "You may not create invites", { type: "/api/v1/problems/invites-not-allowed" });
      const maxUses = body?.max_uses ?? limits.default_max_uses;
      const expiresIn = body?.expires_in ?? limits.default_expires_in;
      if (!Number.isInteger(maxUses) || maxUses < 1 || maxUses > limits.max_uses)
        return problem(422, "Too many uses", { field: "max_uses" });
      if (!Number.isInteger(expiresIn) || expiresIn < 1 || expiresIn > limits.max_expires_in)
        return problem(422, "Too long a lifetime", { field: "expires_in" });
      if (invites.filter((i) => stateOf(i) === "usable").length >= limits.max_active)
        return problem(409, "You hold as many invites as you may", { type: "/api/v1/problems/too-many-invites" });
      const n = 200 + next();
      const made = invite(n, 0, expiresIn * 1000, 0, maxUses, {
        id: idAt(Date.now(), 76_500 + n),
        created_at: new Date().toISOString(),
      });
      made.expires_at = new Date(Date.now() + expiresIn * 1000).toISOString();
      invites.unshift(made);
      return { status: 201, body: inviteBody(made) };
    },
    "POST /invites/{id}/revoke": ({ params }) => {
      if (c.singleUser) return singleUserRefusal();
      const found = invites.find((i) => i.id === params.id);
      if (found === undefined) return notFound();
      found.revoked = true;
      return { status: 200, body: inviteBody(found) };
    },
  };
}
