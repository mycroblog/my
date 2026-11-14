// SPDX-License-Identifier: AGPL-3.0-only

// The data of an auth screen (ADR-0090 §16): the JSON data block <script type="application/json" id="auth"> that the
// server's shell of the auth entry carries (internal/interface/authpages, page.go). The server decides which screen
// shows and words what it decides (a form's outcome, what it reads of the server); the screens word the rest.

/** The screens of the auth entry, by the server's name for each. */
export type ScreenName =
  | "sign_in"
  | "two_factor"
  | "signed_in"
  | "error"
  | "sign_up"
  | "status"
  | "verify_email"
  | "verified"
  | "account_deleted"
  | "authorize"
  | "forgot_password"
  | "reset_password"
  | "password_reset"
  | "mail_unsubscribe"
  | "mail_unsubscribed";

type Server = {
  name: string;
  logo?: string;
  logoDark?: string;
  description?: string;
  banner?: string;
};

type DocumentLink = { path: string; title: string };

export type HumanCheck = {
  scriptSrc: string;
  containerClass: string;
  siteKey: string;
  action?: string;
  language?: string;
  theme: string;
  notice: string;
  links?: { url: string; title: string }[];
};

export type Passkey = { options: unknown; challenge: string; error?: string };

export type ExternalButton = { label: string; url: string; kind: string };

export type SignUpForm = {
  closed?: boolean;
  loginProvidersOnly?: boolean;
  askInvite?: boolean;
  needInvite?: boolean;
  askReason?: boolean;
  username?: string;
  email?: string;
  inviteCode?: string;
  reason?: string;
  needAgreement?: boolean;
  agreed?: boolean;
  usernameHint: string;
  passwordHint: string;
  reasonHint: string;
  inviteHint: string;
  /** A field's error by its name: username, email, password, invite_code, reason, agreement. */
  errors?: Record<string, string>;
  usernameMax: number;
  passwordMin: number;
  passwordMax: number;
  reasonMax: number;
  external?: { ticket: string; provider: string; emailVerifiedHint?: string };
};

export type StatusAction = {
  id: string;
  kind: string;
  category: string;
  reason?: string;
  ends: string;
  endsAt?: string;
  appeal?: { state: string; stateLabel: string; text: string; answer?: string };
  canAppeal?: boolean;
  appealHint?: string;
  appealMax?: number;
  noAppeal?: string;
  appealText?: string;
  appealError?: string;
};

export type Status = {
  pending?: boolean;
  rejected?: boolean;
  verifyEmail?: string;
  restricted?: boolean;
  actions?: StatusAction[];
  deactivated?: boolean;
  deletionDate?: string;
  deletionAt?: string;
  deleteAsksForPassword?: boolean;
};

export type Consent = {
  clientName: string;
  redirectHost: string;
  unverified: string;
  origin: string;
  website?: string;
  impersonation?: string;
  scopeGroups: { risk: string; title: string; scopes: { text: string; cautions?: string[] }[] }[];
  account?: { handle: string; displayName?: string };
  responseType: string;
  clientId: string;
  redirectUri: string;
  scope: string;
  state?: string;
  codeChallenge: string;
  codeChallengeMethod: string;
};

/** The data of an auth screen. */
export type AuthPage = {
  screen: ScreenName;
  server: Server;
  documents?: DocumentLink[];
  statusPageUrl?: string;
  csrfToken?: string;
  returnTo?: string;
  email?: string;
  /** What the server refused of the form, worded by it. */
  error?: string;
  /** What the server did, worded by it. */
  notice?: string;
  canSignUp?: boolean;
  canResetPassword?: boolean;
  loginProvidersOnly?: boolean;
  signUp?: SignUpForm;
  status?: Status;
  token?: string;
  forgotPassword?: { unavailable?: boolean; loginProvidersOnly?: boolean; sent?: boolean };
  newPassword?: { hint: string; error?: string; min: number; max: number };
  unsubscribe?: { prompt?: string; done?: string };
  consent?: Consent;
  humanCheck?: HumanCheck;
  passkey?: Passkey;
  external?: ExternalButton[];
};

const screens = new Set<string>([
  "sign_in",
  "two_factor",
  "signed_in",
  "error",
  "sign_up",
  "status",
  "verify_email",
  "verified",
  "account_deleted",
  "authorize",
  "forgot_password",
  "reset_password",
  "password_reset",
  "mail_unsubscribe",
  "mail_unsubscribed",
]);

/**
 * The screen of the data block's text. It throws when the text is no data of a screen; the server writes it, so a
 * mismatch is a server and an entry of different versions.
 */
export function parseAuthPage(block: string | null): AuthPage {
  if (block === null) throw new Error("auth: the page has no data block");
  const value: unknown = JSON.parse(block);
  if (typeof value !== "object" || value === null) throw new Error("auth: the data block is not an object");
  const { screen, server } = value as { screen?: unknown; server?: unknown };
  if (typeof screen !== "string" || !screens.has(screen)) throw new Error(`auth: no screen ${String(screen)}`);
  if (typeof server !== "object" || server === null || typeof (server as { name?: unknown }).name !== "string")
    throw new Error("auth: the data block names no server");
  return value as AuthPage;
}
