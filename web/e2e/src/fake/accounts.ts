// SPDX-License-Identifier: AGPL-3.0-only

// The fake's moderation of accounts (FR-703, FR-707, FR-708, FR-713, FR-714, FR-719, FR-721, FR-011, FR-931,
// ADR-0058, ADR-0147): what a holder of `manage_accounts` reads and does under /admin/accounts, /admin/account_actions,
// /admin/appeals, /admin/registrations and /admin/signup_blocks, a holder of `manage_login_recovery` under an account's
// password reset link and two-factor authentication, a holder of `manage_invites` under /admin/invites, and a holder of
// `manage_reports` under /admin/hook_decisions, /admin/hook_appeals, and the ending of a voice chat room and the
// cancelling of an event that a report points at (reports.ts holds them). The seed files no action, so these are the
// fake's own, made of the seed's people, a person the seed lacks dropping what needs them:
//
// - every person is an account as the moderators see it (hana the administrator, marco a moderator), sam's email
//   address suppressed after a hard bounce, kai with two-factor authentication; and accounts of the fake's own: a
//   banned seller of followers (`cheapfollowers`) and three applicants whose registrations wait, of whom one signed up
//   with an invite, one has not confirmed their email address, and one has a long username;
// - actions (`action:<key>`): `quickdeals:warn` (spam, expired), `quickdeals:limit` (spam, from the report `review`,
//   in effect for 28 more days), `kai` (`restrict_posting`, a week from a day ago, from the report `room`), `ban`
//   (`cheapfollowers`, with a sign-up block of its email address), `mio` (`suspend`, lifted when its appeal was
//   overturned), `layla` (`warn`, whose appeal was upheld), `sora` (`force_sensitive` of the remote
//   sora@photos.example, until lifted);
// - appeals (`appeal:<key>`): `kai` and `ban` waiting, `mio` overturned, `layla` upheld;
// - registrations (`registration:<n>`, longest waiting first): 0 `x9shop` (unconfirmed email, a disposable domain,
//   the IP address of the ban), 1 `breadandmountainsandlongwalks` (with an invite of hana's), 2 `newleaf`;
// - the decision hooks' decisions (`decision:<key>`): `drop` (an `inbound.status` drop in force, restorable), `observe`
//   (an `inbound.status` drop in `observe`), `signup` (a `signup.check` require_approval of x9shop), `media` (a
//   `media.check` mark_sensitive), `reject` (kai's post refused by `local.status`, appealed), `upheld` (mio's, whose
//   appeal was upheld), `timeout` (an `inbound.follow` that timed out), `overridden` (an `inbound.actor` drop a
//   moderator cancelled), `allow` (listed only by its outcome); and their appeals (`hook-appeal:<key>`): `kai`
//   waiting, `mio` upheld;
// - sign-up blocks (`block:<key>`): `domain` (spam.example, refused), `range` (192.0.2.0/24, approval, expiring),
//   `ban` (the banned account's email address), `lifted`, `expired`;
// - invites (`invite:<key>`): `staff` (hana's, usable, 3 of 10 used), `member` (sam's, used up by layla), `revoked`,
//   `ineligible` (quickdeals', whose account is limited), `cli` (made with the command line), `deleted` (a member's
//   who deleted their account), `expired`.

import { day, idAt, minute, notFound, page, problem } from "./common";
import type { FakeReports } from "./reports";
import type { Handlers, Refusal, Schema } from "./spec";

type Account = Schema<"Account">;
type Kind = Schema<"AdminAccountActionKind">;
type Action = Schema<"AdminAccountAction">;
type Appeal = Schema<"AdminAppeal">;
type Restriction = Schema<"AdminAccountRestriction">;
type Decision = Schema<"AdminHookDecision">;
type HookAppeal = Schema<"AdminHookAppeal">;
type Block = Schema<"AdminSignupBlock">;
type Invite = Schema<"AdminInvite">;
type Permission = Schema<"Permission">;

/**
 * What the moderation holds when the fake starts: the fake's own (`seeded`, by default), or nothing at all; `crowded`,
 * the fake's own and kai's 28 more sign-ins, a day apart, and 24 accounts of the fake's own (`ring01` to `ring24`)
 * that signed up and signed in from kai's address, so that the account search and kai's lists run past a page.
 */
export type FakeModerationOptions = "seeded" | "none" | "crowded";

/** A person of the fake, as the moderation reads them. */
type ModeratedPerson = {
  readonly username: string;
  readonly email: string;
  readonly staff?: "admin" | "moderator" | undefined;
  readonly twoFactor?: boolean | undefined;
};

/** What the moderation's handlers take from the fake. */
export type ModerationContext = {
  readonly started: number;
  readonly options: FakeModerationOptions | undefined;
  /** The reader's username, and their permissions. */
  readonly viewer: string;
  readonly permissions: () => readonly Permission[];
  readonly people: readonly ModeratedPerson[];
  readonly accountOf: (username: string) => Account;
  readonly accountId: (username: string) => string;
  /** The status of the ID, as anyone reads it, if the fake has it. */
  readonly statusById: (id: string) => Schema<"Status"> | undefined;
  /** The moderators' reports (reports.ts). */
  readonly reports: FakeReports;
  /** A one-person server: no registrations, sign-up blocks or invites. */
  readonly singleUser: boolean;
  /** Registrations are closed, so no invite is created. */
  readonly registrationClosed: boolean;
  /** The domain the fake answers as. */
  readonly host: string;
};

/** What a test does with the fake's moderation. */
export type FakeModeration = {
  /**
   * The ID of the fake's item of `key` (`action:kai`, `appeal:ban`, `registration:0`, …), or of the account of a
   * username, the fake's own included (`account:cheapfollowers`, `account:x9shop`); "none" for an unknown key.
   */
  readonly id: (key: string) => string;
  /** The registrations that wait for a moderator. */
  readonly pendingRegistrations: () => number;
  /** The appeals of actions that wait for a moderator. */
  readonly pendingAppeals: () => number;
};

/** A registration as the server holds it. */
type Registration = {
  id: string;
  submitted: number;
  state: Schema<"RegistrationState">;
  changed: number;
  reason?: string;
  note?: string;
  invite?: string;
};

/** An account as the fake's moderation holds it. */
type Held = {
  username: string;
  account: Account;
  local: boolean;
  staff?: "admin" | "moderator" | undefined;
  email?: string;
  emailVerified: boolean;
  twoFactor: boolean;
  suppression?: Schema<"EmailSuppression">;
  registration?: Registration;
  registrationIp?: string;
  signIns: { ip: string; ua: string; at: number }[];
};

const allRestrictions: readonly Restriction[] = [
  "login",
  "post",
  "interact",
  "visibility_ceiling_private",
  "public_discovery",
  "unsolicited_reach",
  "media_sensitive",
  "profile_view",
  "federation",
];

/** What each kind of action takes away. */
const takes: Readonly<Record<Kind, readonly Restriction[]>> = {
  warn: [],
  mark_statuses_sensitive: [],
  delete_statuses: [],
  force_sensitive: ["media_sensitive"],
  limit: ["visibility_ceiling_private", "public_discovery", "unsolicited_reach"],
  restrict_posting: ["post"],
  restrict_interaction: ["interact"],
  disable_login: ["login"],
  suspend: allRestrictions,
  ban: allRestrictions,
};
const postKinds: readonly Kind[] = ["mark_statuses_sensitive", "delete_statuses"];
const remoteKinds: readonly Kind[] = ["limit", "suspend", "force_sensitive"];
const tenYears = 315_360_000;

const browsers = [
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_6) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15",
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:132.0) Gecko/20100101 Firefox/132.0",
  "Mozilla/5.0 (Linux; Android 15; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Mobile Safari/537.36",
];

/** The hex SHA-256 of `text`. */
async function sha256Hex(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

/** An email address as an `email_hash` block compares it: in lower case, without its local part from its first `+`. */
const canonicalEmail = (email: string) => {
  const [local = "", domain = ""] = email.toLowerCase().split("@");
  return `${local.split("+")[0] ?? ""}@${domain}`;
};

/** Whether `text` is longer than `max` characters or holds U+0000, which the server refuses. */
const refusedText = (text: string | undefined, max: number) =>
  text !== undefined && (text.includes("\u0000") || Array.from(text).length > max);

/** The server's refusal of a member of the request, by its name. */
const invalid = (field: string): Refusal =>
  problem(422, "This could not be done.", { detail: "This value is not valid. Check what you entered.", field });

/** An IPv4 address's four parts, or undefined for none. */
function ipv4(text: string): number[] | undefined {
  const parts = text.split(".");
  if (parts.length !== 4 || parts.some((p) => !/^\d{1,3}$/.test(p) || Number(p) > 255)) return undefined;
  return parts.map(Number);
}

/** A CIDR prefix or a single address in canonical form (masked), or undefined for none. */
function canonicalRange(value: string): string | undefined {
  const [address = "", bits, ...more] = value.trim().split("/");
  if (more.length > 0) return undefined;
  const v4 = ipv4(address);
  if (v4 !== undefined) {
    const n = bits === undefined ? 32 : Number(bits);
    if (!Number.isInteger(n) || n < 0 || n > 32 || (bits !== undefined && !/^\d+$/.test(bits))) return undefined;
    const whole = v4.reduce((a, b) => a * 256 + b, 0);
    const masked = Math.floor(whole / 2 ** (32 - n)) * 2 ** (32 - n);
    const out = [24, 16, 8, 0].map((s) => Math.floor(masked / 2 ** s) % 256).join(".");
    return `${out}/${String(n)}`;
  }
  if (/^[0-9a-f:]+$/i.test(address) && address.includes(":")) {
    const n = bits === undefined ? 128 : Number(bits);
    if (!Number.isInteger(n) || n < 0 || n > 128) return undefined;
    return `${address.toLowerCase()}/${String(n)}`;
  }
  return undefined;
}

/** A domain in IDNA A-labels, or undefined for a value that is no domain. */
function canonicalDomain(value: string): string | undefined {
  const bare = value.trim().replace(/^@/, "").toLowerCase();
  if (bare === "" || bare.includes("@") || bare.includes("/") || !bare.includes(".")) return undefined;
  try {
    const host = new URL(`http://${bare}`).hostname;
    return /^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(host) ? host : undefined;
  } catch {
    return undefined;
  }
}

/** A value of a sign-up block in canonical form, or undefined for one not valid for its kind. */
async function canonicalValue(kind: Schema<"AdminSignupBlockKind">, value: string): Promise<string | undefined> {
  if (kind === "email_domain") return canonicalDomain(value);
  if (kind === "ip_range") return canonicalRange(value);
  const text = value.trim();
  if (/^[0-9a-f]{64}$/i.test(text)) return text.toLowerCase();
  const [local, domain, ...more] = text.split("@");
  if (local === undefined || local === "" || domain === undefined || more.length > 0) return undefined;
  if (canonicalDomain(domain) === undefined) return undefined;
  return sha256Hex(canonicalEmail(text));
}

/** The handlers of the moderation of accounts, and what a test does with them. */
export function moderationHandlers(c: ModerationContext): { handlers: Handlers; moderation: FakeModeration } {
  const iso = (ms: number) => new Date(ms).toISOString();
  const ago = (minutes: number) => c.started - minutes * minute;
  const seeded = c.options !== "none";
  const known = (username: string) => c.people.some((p) => p.username === username);
  const keyed = new Map<string, string>();
  const key = (k: string, id: string) => {
    keyed.set(k, id);
    return id;
  };
  let n = 0;
  const next = (ms: number, base: number) => idAt(ms, base + (n += 1));

  // ---- The accounts -------------------------------------------------------------------------------------------------

  const held = new Map<string, Held>();
  const template = c.people[0] === undefined ? undefined : c.accountOf(c.people[0].username);
  /** An account of the fake's own, which no person of the seed is. */
  const ownAccount = (username: string, name: string, note: string, createdAgo: number): Account | undefined =>
    template === undefined
      ? undefined
      : {
          ...template,
          id: idAt(ago(createdAgo), 46_100 + held.size),
          username,
          acct: username,
          display_name: name,
          note: `<p>${note}</p>`,
          fields: [],
          emojis: [],
          badges: [],
          uri: `https://${c.host}/users/${username}`,
          url: `https://${c.host}/@${username}`,
          locked: false,
          bot: false,
          created_at: iso(ago(createdAgo)),
        };

  const ipOf = (i: number) => `203.0.113.${String(10 + i * 7)}`;
  c.people.forEach((p, i) => {
    const account = c.accountOf(p.username);
    const local = !p.username.includes("@");
    const created = Date.parse(account.created_at);
    const shared = p.username === "yuki" || p.username === "ren" ? "203.0.113.24" : ipOf(i);
    held.set(account.id, {
      username: p.username,
      account,
      local,
      staff: p.staff,
      ...(local && p.email !== "" ? { email: p.email } : {}),
      emailVerified: local,
      twoFactor: p.twoFactor === true,
      ...(local
        ? {
            registration: {
              id: idAt(created, 46_050 + i),
              submitted: created,
              state: "approved" as const,
              changed: created,
            },
            registrationIp: shared,
          }
        : {}),
      signIns: local
        ? [
            {
              ip: p.username === "kai" ? "2001:db8:85a3::8a2e:370:7334" : shared,
              ua: browsers[i % 3] ?? "",
              at: ago(90 + i),
            },
            { ip: shared, ua: browsers[(i + 1) % 3] ?? "", at: ago(2 * 24 * 60 + i * 13) },
          ]
        : [],
    });
  });
  const heldOf = (username: string) => [...held.values()].find((h) => h.username === username);
  const sam = heldOf("sam");
  if (seeded && sam?.email !== undefined)
    sam.suppression = { kind: "hard_bounce", state_changed_at: iso(ago(3 * 24 * 60)) };

  /** Adds an account of the fake's own. */
  const addOwn = (
    username: string,
    name: string,
    note: string,
    createdAgo: number,
    more: Partial<Held> & { email: string },
  ): Held | undefined => {
    const account = ownAccount(username, name, note, createdAgo);
    if (account === undefined) return undefined;
    const h: Held = {
      username,
      account,
      local: true,
      emailVerified: true,
      twoFactor: false,
      signIns: [],
      ...more,
    };
    held.set(account.id, h);
    return h;
  };

  // ---- The invites --------------------------------------------------------------------------------------------------

  const invites: Invite[] = [];
  const inviteUrl = (code: string) => `https://${c.host}/auth/sign_up?invite=${code}`;
  const addInvite = (
    k: string,
    code: string,
    createdAgo: number,
    more: Partial<Invite> & Pick<Invite, "creator_kind" | "state" | "uses">,
  ) => {
    const at = ago(createdAgo);
    invites.push({
      id: key(`invite:${k}`, idAt(at, 46_800 + invites.length)),
      code,
      url: inviteUrl(code),
      created_by_deleted: false,
      created_at: iso(at),
      ...more,
    });
  };
  if (seeded) {
    if (known("hana"))
      addInvite("staff", "BREADSWAP7K2M9QX", 6 * 24 * 60, {
        creator_kind: "staff",
        created_by: c.accountOf("hana"),
        max_uses: 10,
        uses: 3,
        state: "usable",
        expires_at: iso(c.started + 8 * day),
      });
    if (known("sam"))
      addInvite("member", "SAMSKITCHEN4LAYL", 30 * 24 * 60, {
        creator_kind: "member",
        created_by: c.accountOf("sam"),
        max_uses: 1,
        uses: 1,
        state: "exhausted",
      });
    if (known("marco"))
      addInvite("revoked", "MOUNTAINWEATHER2", 12 * 24 * 60, {
        creator_kind: "staff",
        created_by: c.accountOf("marco"),
        uses: 0,
        state: "revoked",
      });
    if (known("quickdeals"))
      addInvite("ineligible", "QUICKDEALSFRIEND", 9 * 24 * 60, {
        creator_kind: "member",
        created_by: c.accountOf("quickdeals"),
        max_uses: 5,
        uses: 2,
        state: "creator_ineligible",
      });
    addInvite("cli", "OPENINGDAYGUEST1", 60 * 24 * 60, { creator_kind: "staff", uses: 14, state: "usable" });
    addInvite("deleted", "FORMERMEMBERCODE", 45 * 24 * 60, {
      creator_kind: "member",
      created_by_deleted: true,
      max_uses: 3,
      uses: 1,
      state: "usable",
    });
    if (known("hana"))
      addInvite("expired", "SPRINGPICNIC2026", 90 * 24 * 60, {
        creator_kind: "staff",
        created_by: c.accountOf("hana"),
        uses: 6,
        state: "expired",
        expires_at: iso(ago(60 * 24 * 60)),
      });
  }
  const inviteNow = (i: Invite): Invite =>
    i.state === "usable" && i.expires_at !== undefined && Date.parse(i.expires_at) <= Date.now()
      ? { ...i, state: "expired" }
      : i;
  const layla = heldOf("layla");
  const memberInvite = keyed.get("invite:member");
  if (layla?.registration !== undefined && memberInvite !== undefined) layla.registration.invite = memberInvite;

  // ---- The accounts of the fake's own -------------------------------------------------------------------------------

  const registrations: Registration[] = [];
  const banned = seeded
    ? addOwn("cheapfollowers", "Cheap Followers 24/7", "10k followers in a day. DM for prices.", 5 * 24 * 60, {
        email: "sales@cheapfollowers.example",
        registrationIp: "198.51.100.7",
        signIns: [{ ip: "198.51.100.7", ua: browsers[1] ?? "", at: ago(3 * 24 * 60 + 30) }],
      })
    : undefined;
  if (banned !== undefined)
    banned.registration = {
      id: idAt(ago(5 * 24 * 60), 46_090),
      submitted: ago(5 * 24 * 60),
      state: "approved",
      changed: ago(5 * 24 * 60),
    };
  const kai = heldOf("kai");
  const kaiIp = kai?.signIns[0]?.ip;
  if (c.options === "crowded" && kai !== undefined && kaiIp !== undefined) {
    const otherIp = kai.signIns[1]?.ip ?? kaiIp;
    for (let i = 0; i < 28; i += 1)
      kai.signIns.push({ ip: i % 2 === 0 ? kaiIp : otherIp, ua: browsers[i % 3] ?? "", at: ago((3 + i) * 24 * 60) });
    for (let i = 0; i < 24; i += 1) {
      const n = String(i + 1).padStart(2, "0");
      const created = (4 + i) * 24 * 60;
      const ring = addOwn(`ring${n}`, `Ring ${n}`, "", created, {
        email: `ring${n}@throwaway.example`,
        registrationIp: kaiIp,
        signIns: [{ ip: kaiIp, ua: browsers[(i + 2) % 3] ?? "", at: ago(created - 60) }],
      });
      if (ring !== undefined)
        ring.registration = {
          id: idAt(ago(created), 46_600 + i),
          submitted: ago(created),
          state: "approved",
          changed: ago(created),
        };
    }
  }
  /** An applicant whose registration waits. */
  const applicant = (
    index: number,
    username: string,
    name: string,
    minutesAgo: number,
    more: Partial<Held> & { email: string },
    reason?: string,
    invite?: string,
  ) => {
    const h = addOwn(username, name, "", minutesAgo, more);
    if (h === undefined) return;
    const registration: Registration = {
      id: key(`registration:${String(index)}`, idAt(ago(minutesAgo), 46_400 + index)),
      submitted: ago(minutesAgo),
      state: "pending",
      changed: ago(minutesAgo),
      ...(reason === undefined ? {} : { reason }),
      ...(invite === undefined ? {} : { invite }),
    };
    h.registration = registration;
    registrations.push(registration);
  };
  if (seeded) {
    applicant(0, "x9shop", "x9shop", 26 * 60, {
      email: "x9@throwaway.example",
      emailVerified: false,
      registrationIp: "198.51.100.7",
    });
    applicant(
      1,
      "breadandmountainsandlongwalks",
      "Bread, mountains and long walks on Sunday mornings",
      9 * 60,
      { email: "walks.and.bread@mail.example", registrationIp: "203.0.113.99" },
      "Hana invited me after the bread class. I bake on weekends, walk the hills behind the station, and would like to share both with people nearby. 週末にパンを焼き、駅の裏の丘を歩いています。",
      keyed.get("invite:staff"),
    );
    applicant(
      2,
      "newleaf",
      "New Leaf Garden",
      2 * 60,
      { email: "newleaf@mail.example", registrationIp: "203.0.113.120" },
      "I run a community garden in town and want to post our harvest days.",
    );
  }

  // ---- The actions --------------------------------------------------------------------------------------------------

  const actions: Action[] = [];
  /** An action of the seed, issued `issuedAgo` minutes ago. */
  const addAction = (
    k: string,
    username: string,
    kind: Kind,
    issuedAgo: number,
    more: Partial<Action> = {},
  ): Action | undefined => {
    const h = heldOf(username);
    if (h === undefined) return undefined;
    const at = ago(issuedAgo);
    const action: Action = {
      id: key(`action:${k}`, idAt(at, 46_200 + actions.length)),
      account_id: h.account.id,
      kind,
      reason_category: "other",
      issuer: "moderator",
      ...(known("marco") ? { issued_by_account_id: c.accountId("marco") } : {}),
      notify_target: h.local,
      state: "active",
      issued_at: iso(at),
      ...more,
    };
    actions.push(action);
    return action;
  };
  if (seeded) {
    addAction("quickdeals:warn", "quickdeals", "warn", 40 * 24 * 60, {
      reason_category: "spam",
      public_reason: "Advertising in replies is not allowed here.",
      expires_at: iso(ago(33 * 24 * 60)),
      state: "expired",
    });
    const review = c.reports.id("review");
    addAction("quickdeals:limit", "quickdeals", "limit", 2 * 24 * 60 - 40, {
      reason_category: "spam",
      public_reason: "Your posts link to the same spam campaign. They stay off the public timelines for a month.",
      private_note: "Same campaign as the account's earlier posts.",
      ...(review === "none" ? {} : { report_id: review }),
      expires_at: iso(c.started + 28 * day),
    });
    const room = c.reports.id("room");
    addAction("kai", "kai", "restrict_posting", 24 * 60, {
      reason_category: "violation",
      public_reason: "Shouting at guests in a voice chat room, after a warning from its participants.",
      private_note: "Second complaint about the late night radio room.",
      ...(room === "none" ? {} : { report_id: room }),
      expires_at: iso(c.started + 6 * day),
    });
    if (banned !== undefined)
      addAction("ban", "cheapfollowers", "ban", 3 * 24 * 60, {
        reason_category: "spam",
        public_reason: "Selling followers and sending the same offer to strangers.",
        private_note: "Every post is an advertisement. The email address is blocked too.",
      });
    addAction("mio", "mio", "suspend", 20 * 24 * 60, {
      reason_category: "other",
      public_reason: "Your event page links to a shop instead of a swap.",
      state: "lifted",
      lifted_at: iso(ago(18 * 24 * 60)),
    });
    addAction("layla", "layla", "warn", 10 * 24 * 60, {
      reason_category: "violation",
      public_reason: "Please credit the photographer when you share a photo.",
    });
    addAction("sora", "sora@photos.example", "force_sensitive", 6 * 24 * 60, {
      reason_category: "violation",
      private_note: "Posts film photos of nudes without marking them sensitive.",
      notify_target: false,
    });
  }
  const actionNow = (a: Action): Action =>
    a.state === "active" && a.expires_at !== undefined && Date.parse(a.expires_at) <= Date.now()
      ? { ...a, state: "expired" }
      : a;
  const actionsOf = (accountId: string) =>
    actions
      .filter((a) => a.account_id === accountId)
      .map(actionNow)
      .toSorted((a, b) => b.issued_at.localeCompare(a.issued_at));
  const inForce = (accountId: string) => actionsOf(accountId).filter((a) => a.state === "active");

  // ---- The appeals --------------------------------------------------------------------------------------------------

  const appeals: {
    id: string;
    actionId: string;
    text: string;
    submitted: number;
    state: Appeal["state"];
    decided?: number;
    reason?: string;
    by?: string;
  }[] = [];
  const addAppeal = (
    k: string,
    submittedAgo: number,
    text: string,
    decision?: { state: "upheld" | "overturned"; ago: number; reason: string },
  ) => {
    const action = keyed.get(`action:${k}`);
    if (action === undefined) return;
    appeals.push({
      id: key(`appeal:${k}`, idAt(ago(submittedAgo), 46_300 + appeals.length)),
      actionId: action,
      text,
      submitted: ago(submittedAgo),
      state: decision?.state ?? "pending",
      ...(decision === undefined
        ? {}
        : {
            decided: ago(decision.ago),
            reason: decision.reason,
            ...(known("marco") ? { by: c.accountId("marco") } : {}),
          }),
    });
  };
  if (seeded) {
    addAppeal(
      "kai",
      20 * 60,
      "I was not shouting at a guest. The guest kept talking over the person who had the floor, and I raised my voice once to stop them, then removed them as the room's rules say. Everyone else in the room can tell you the same. I host the radio every week and a week without posting means I cannot even tell my listeners that it is cancelled.",
    );
    addAppeal(
      "ban",
      2 * 24 * 60,
      "I bought this handle from someone else last week and did not know what they had posted. Please give me a chance to start over.",
    );
    addAppeal(
      "mio",
      19 * 24 * 60,
      "The link is to the list of books for the swap. The shop was my mistake and I removed it.",
      {
        state: "overturned",
        ago: 18 * 24 * 60,
        reason: "Sorry, our mistake: the list is of books for the swap.",
      },
    );
    addAppeal("layla", 9 * 24 * 60, "The photo is mine, I took it at the market.", {
      state: "upheld",
      ago: 8 * 24 * 60,
      reason: "The photographer sent us the original file, taken before yours was posted.",
    });
  }
  const actionById = (id: string) => actions.find((a) => a.id === id);
  const appealOf = (a: (typeof appeals)[number]): Appeal | undefined => {
    const action = actionById(a.actionId);
    if (action === undefined) return undefined;
    return {
      id: a.id,
      account_id: action.account_id,
      text: a.text,
      state: a.state,
      submitted_at: iso(a.submitted),
      action: actionNow(action),
      ...(a.decided === undefined ? {} : { decided_at: iso(a.decided) }),
      ...(a.reason === undefined ? {} : { decision_reason: a.reason }),
      ...(a.by === undefined ? {} : { decided_by_account_id: a.by }),
    };
  };

  // ---- The sign-up blocks -------------------------------------------------------------------------------------------

  const blocks: Block[] = [];
  const addBlock = (k: string, createdAgo: number, more: Omit<Block, "id" | "created_at">) => {
    const at = ago(createdAgo);
    blocks.push({ id: key(`block:${k}`, idAt(at, 46_700 + blocks.length)), created_at: iso(at), ...more });
  };
  if (seeded) {
    addBlock("expired", 120 * 24 * 60, {
      kind: "ip_range",
      value: "2001:db8::/32",
      severity: "reject",
      state: "expired",
      expires_at: iso(ago(30 * 24 * 60)),
    });
    addBlock("lifted", 60 * 24 * 60, {
      kind: "email_domain",
      value: "old-spam.example",
      severity: "reject",
      comment: "Sold to a new owner, who runs it properly.",
      state: "lifted",
      lifted_at: iso(ago(10 * 24 * 60)),
    });
    addBlock("domain", 30 * 24 * 60, {
      kind: "email_domain",
      value: "spam.example",
      severity: "reject",
      comment: "Disposable addresses",
      state: "active",
    });
    addBlock("range", 5 * 24 * 60, {
      kind: "ip_range",
      value: "192.0.2.0/24",
      severity: "require_approval",
      comment: "A hosting provider that the follower sellers sign up from.",
      state: "active",
      expires_at: iso(c.started + 25 * day),
    });
    const ban = keyed.get("action:ban");
    if (ban !== undefined)
      addBlock("ban", 3 * 24 * 60, {
        kind: "email_hash",
        value: "07562f54802847f31990b380b2988ba3ced41cac34f40af43580410a23c76568",
        severity: "reject",
        moderation_action_id: ban,
        state: "active",
      });
  }
  const blockNow = (b: Block): Block =>
    b.state === "active" && b.expires_at !== undefined && Date.parse(b.expires_at) <= Date.now()
      ? { ...b, state: "expired" }
      : b;
  const newestBlocks = () => blocks.map(blockNow).toSorted((a, b) => b.created_at.localeCompare(a.created_at));

  // ---- The decision hooks -------------------------------------------------------------------------------------------

  const decisions: Decision[] = [];
  const addDecision = (k: string, decidedAgo: number, more: Omit<Decision, "id" | "decided_at">, account?: string) => {
    if (account !== undefined && heldOf(account) === undefined) return;
    const at = ago(decidedAgo);
    decisions.push({
      id: key(`decision:${k}`, idAt(at, 46_500 + decisions.length)),
      decided_at: iso(at),
      ...(account === undefined ? {} : { account_id: heldOf(account)?.account.id ?? "" }),
      ...more,
    });
  };
  if (seeded) {
    addDecision(
      "allow",
      15,
      {
        hook: "inbound.status",
        subject_key: "https://photos.example/users/sora/statuses/88213",
        mode: "enforce",
        outcome: "allow",
        latency_ms: 84,
        overridable: false,
      },
      "sora@photos.example",
    );
    addDecision(
      "drop",
      45,
      {
        hook: "inbound.status",
        subject_key: "https://far.example/users/kiri/statuses/112349",
        mode: "enforce",
        outcome: "drop",
        reason: "spam",
        note: "Matched 3 links of a known campaign.",
        latency_ms: 212,
        restorable_until: iso(c.started + 7 * day - 45 * minute),
        overridable: true,
      },
      "kiri@far.example",
    );
    addDecision(
      "observe",
      3 * 60,
      {
        hook: "inbound.status",
        subject_key: "https://bread.example/users/kobo/statuses/5521",
        mode: "observe",
        outcome: "drop",
        reason: "other",
        latency_ms: 160,
        overridable: false,
      },
      "kobo@bread.example",
    );
    addDecision(
      "signup",
      26 * 60,
      {
        hook: "signup.check",
        subject_key: "x9shop",
        mode: "enforce",
        outcome: "require_approval",
        reason: "spam",
        note: "Disposable email domain, an address seen with a ban.",
        latency_ms: 340,
        overridable: true,
      },
      "x9shop",
    );
    addDecision(
      "media",
      5 * 60,
      {
        hook: "media.check",
        subject_key: "media/7a1c42",
        mode: "enforce",
        outcome: "mark_sensitive",
        reason: "sexual",
        latency_ms: 1180,
        overridable: true,
      },
      "ren",
    );
    addDecision(
      "reject",
      22 * 60,
      {
        hook: "local.status",
        subject_key: "local.status/kai/1",
        mode: "enforce",
        outcome: "reject",
        reason: "harassment",
        note: "Insults aimed at a person by name.",
        latency_ms: 450,
        overridable: false,
      },
      "kai",
    );
    addDecision(
      "upheld",
      6 * 24 * 60,
      {
        hook: "local.status",
        subject_key: "local.status/mio/1",
        mode: "enforce",
        outcome: "reject",
        reason: "spam",
        latency_ms: 390,
        overridable: false,
      },
      "mio",
    );
    addDecision(
      "timeout",
      8 * 60,
      {
        hook: "inbound.follow",
        subject_key: "https://bread.example/users/kobo#follows/77",
        mode: "enforce",
        outcome: "timeout",
        latency_ms: 2000,
        overridable: false,
      },
      "kobo@bread.example",
    );
    addDecision(
      "overridden",
      2 * 24 * 60,
      {
        hook: "inbound.actor",
        subject_key: "https://far.example/users/kiri",
        mode: "enforce",
        outcome: "drop",
        reason: "spam",
        latency_ms: 260,
        overridden_at: iso(ago(2 * 24 * 60 - 30)),
        overridable: false,
      },
      "kiri@far.example",
    );
  }
  const decisionById = (id: string) => decisions.find((d) => d.id === id);
  const newestDecisions = () => decisions.toSorted((a, b) => b.decided_at.localeCompare(a.decided_at));

  const hookAppeals: {
    id: string;
    decision: string;
    account: string;
    text: string;
    comment?: string;
    state: HookAppeal["state"];
    submitted: number;
    decided?: number;
  }[] = [];
  const addHookAppeal = (
    k: string,
    decision: string,
    submittedAgo: number,
    text: string,
    more: { comment?: string; decidedAgo?: number },
  ) => {
    const d = decisionById(keyed.get(`decision:${decision}`) ?? "");
    if (d?.account_id === undefined) return;
    hookAppeals.push({
      id: key(`hook-appeal:${k}`, idAt(ago(submittedAgo), 46_600 + hookAppeals.length)),
      decision: d.id,
      account: d.account_id,
      text,
      ...(more.comment === undefined ? {} : { comment: more.comment }),
      state: more.decidedAgo === undefined ? "pending" : "upheld",
      submitted: ago(submittedAgo),
      ...(more.decidedAgo === undefined ? {} : { decided: ago(more.decidedAgo) }),
    });
  };
  if (seeded) {
    addHookAppeal(
      "kai",
      "reject",
      21 * 60,
      "Whoever keeps talking over guests in my room will be removed. That is the rule, and it applies to you too, @ren.",
      {
        comment:
          "This is a rule of my room, not an insult. I named the person because they asked me why they were removed.",
      },
    );
    addHookAppeal("mio", "upheld", 6 * 24 * 60 - 60, "Books for sale, cheap, see the link in my profile.", {
      decidedAgo: 5 * 24 * 60,
    });
  }
  const hookAppealOf = (a: (typeof hookAppeals)[number]): HookAppeal | undefined => {
    const decision = decisionById(a.decision);
    if (decision === undefined) return undefined;
    return {
      id: a.id,
      decision,
      account_id: a.account,
      status_text: a.text,
      ...(a.comment === undefined ? {} : { comment: a.comment }),
      state: a.state,
      submitted_at: iso(a.submitted),
      ...(a.decided === undefined ? {} : { decided_at: iso(a.decided) }),
    };
  };

  // ---- What the accounts show -----------------------------------------------------------------------------------------

  /** The state of an account, the first that applies. */
  const stateOf = (h: Held): Schema<"AdminAccountState"> => {
    const kinds = new Set(inForce(h.account.id).map((a) => a.kind));
    if (kinds.has("suspend") || kinds.has("ban")) return "suspended";
    if (kinds.has("disable_login")) return "disabled";
    if (h.registration?.state === "rejected") return "rejected";
    if (h.registration?.state === "pending") return "pending";
    if (kinds.has("limit")) return "limited";
    return "active";
  };
  const restrictionsOf = (h: Held): Restriction[] => {
    const taken = new Set(inForce(h.account.id).flatMap((a) => takes[a.kind]));
    return allRestrictions.filter((r) => taken.has(r));
  };
  const adminAccount = (h: Held): Schema<"AdminAccount"> => ({
    id: h.account.id,
    account: h.account,
    local: h.local,
    state: stateOf(h),
    restrictions: restrictionsOf(h),
    ...(h.local ? { registration_state: h.registration?.state ?? "approved" } : {}),
  });
  const inviteOf = (id: string | undefined): Schema<"AdminAccountInvite"> | undefined => {
    const invite = invites.find((i) => i.id === id);
    if (invite === undefined) return undefined;
    return {
      id: invite.id,
      creator_kind: invite.creator_kind,
      ...(invite.created_by === undefined ? {} : { created_by: invite.created_by }),
      created_by_deleted: invite.created_by_deleted,
    };
  };
  const detailOf = (h: Held): Schema<"AdminAccountDetail"> => {
    const r = h.registration;
    const invite = inviteOf(r?.invite);
    return {
      ...adminAccount(h),
      ...(h.local && r !== undefined
        ? {
            registration: {
              id: r.id,
              submitted_at: iso(r.submitted),
              ...(r.reason === undefined ? {} : { reason: r.reason }),
              ...(r.note === undefined ? {} : { decision_note: r.note }),
              ...(r.invite === undefined ? {} : { invite_id: r.invite }),
              ...(invite === undefined ? {} : { invite }),
              state_changed_at: iso(r.changed),
            },
          }
        : {}),
      ...(h.email === undefined ? {} : { email_verified: h.emailVerified }),
      ...(h.suppression === undefined ? {} : { email_suppression: h.suppression }),
      ...(h.local ? { two_factor_enabled: h.twoFactor } : {}),
      roles: h.staff === undefined ? [] : [h.staff],
      actions: actionsOf(h.account.id).slice(0, 20),
      ...(may("manage_reports") ? { reports: c.reports.ofAccount(h.account.id) } : {}),
    };
  };

  // ---- The rules of every operation -------------------------------------------------------------------------------------

  const may = (permission: Permission) => c.permissions().includes(permission);
  const forbidden = () =>
    problem(403, "Forbidden", {
      type: "/api/v1/problems/missing-permission",
      detail: "Your roles do not have the permission this needs.",
    });
  const outranked = () =>
    problem(403, "The account's roles do not rank below yours", { type: "/api/v1/problems/target-outranks-actor" });
  const singleUserMode = () =>
    problem(404, "Not available on a one-person server", { type: "/api/v1/problems/single-user-mode" });
  /** Whether the reader may act on the account: its roles rank below theirs (no staff, never their own). */
  const outranks = (h: Held) => h.staff !== undefined || h.username === c.viewer;
  /** The account of `id`, or the refusal of a request about it. */
  const accountFor = (id: string, permission: Permission = "manage_accounts"): Held | Refusal => {
    if (!may(permission)) return forbidden();
    return held.get(id) ?? notFound();
  };
  const refusal = (x: unknown): x is Refusal => typeof x === "object" && x !== null && "problem" in x;
  const pendingAppeals = () =>
    appeals.filter((a) => a.state === "pending" && actionById(a.actionId) !== undefined).length;
  const pendingRegistrations = () => registrations.filter((r) => r.state === "pending").length;
  const newestAccounts = () =>
    [...held.values()].toSorted((a, b) => b.account.created_at.localeCompare(a.account.created_at));
  const ipsOf = (h: Held) => [
    ...(h.registrationIp === undefined ? [] : [h.registrationIp]),
    ...h.signIns.map((s) => s.ip),
  ];
  const domainOfAcct = (acct: string) => (acct.includes("@") ? acct.slice(acct.indexOf("@") + 1) : undefined);

  /** Ends an action, as a lift or an overturned appeal does; a ban's account is no longer deleted. */
  const lift = (a: Action, reason: string | undefined) => {
    if (actionNow(a).state !== "active") return;
    a.state = "lifted";
    a.lifted_at = iso(Date.now());
    if (reason !== undefined && reason !== "") a.lift_reason = reason;
  };

  // ---- The handlers -----------------------------------------------------------------------------------------------------

  const handlers: Handlers = {
    "GET /admin/accounts": ({ query }) => {
      if (!may("manage_accounts")) return forbidden();
      const get = (name: "username" | "domain" | "origin" | "state" | "email" | "email_domain" | "ip") =>
        query.get(name) ?? undefined;
      const [username, domain, origin, state, email, emailDomain, ip] = [
        get("username"),
        get("domain"),
        get("origin"),
        get("state"),
        get("email"),
        get("email_domain"),
        get("ip"),
      ];
      const bad = (name: string) => problem(400, "Bad Request", { detail: `${name} is not valid.`, field: name });
      for (const [name, value] of Object.entries({ username, domain, email, email_domain: emailDomain, ip }))
        if (value !== undefined && (value === "" || value.length > 320)) return bad(name);
      if ([email, emailDomain, ip].filter((v) => v !== undefined).length > 1)
        return problem(400, "Bad Request", { detail: "Give at most one of email, email_domain and ip." });
      if (origin !== undefined && origin !== "local" && origin !== "remote") return bad("origin");
      const states = ["active", "pending", "rejected", "limited", "disabled", "deactivated", "suspended", "deleted"];
      if (state !== undefined && !states.includes(state)) return bad("state");
      if (emailDomain?.replace(/^@/, "").includes("@") === true) return bad("email_domain");
      if (ip !== undefined && ipv4(ip) === undefined && !/^[0-9a-f:]+$/i.test(ip)) return bad("ip");
      const wanted = emailDomain?.replace(/^@/, "").toLowerCase();
      const kept = newestAccounts().filter((h) => {
        const name = h.account.username.toLowerCase();
        return (
          (username === undefined ||
            (h.local || !name.includes("@") ? name.startsWith(username.toLowerCase()) : false)) &&
          (domain === undefined || domainOfAcct(h.account.acct) === domain.toLowerCase()) &&
          (origin === undefined || h.local === (origin === "local")) &&
          (state === undefined || stateOf(h) === state) &&
          (email === undefined || h.email?.toLowerCase() === email.toLowerCase()) &&
          (wanted === undefined || h.email?.toLowerCase().endsWith(`@${wanted}`) === true) &&
          (ip === undefined || (h.local && ipsOf(h).includes(ip)))
        );
      });
      const { body } = page(kept.map(adminAccount), query);
      return { status: 200, body };
    },
    "GET /admin/accounts/{id}": ({ params }) => {
      const h = accountFor(params.id);
      return refusal(h) ? h : { status: 200, body: detailOf(h) };
    },
    "GET /admin/accounts/{id}/actions": ({ params, query }) => {
      const h = accountFor(params.id);
      if (refusal(h)) return h;
      const active = query.get("active") === "true";
      const listed = actionsOf(h.account.id).filter((a) => !active || a.state === "active");
      return { status: 200, body: page(listed, query).body };
    },
    "POST /admin/accounts/{id}/actions": async ({ params, body }) => {
      const h = accountFor(params.id);
      if (refusal(h)) return h;
      const kinds = Object.keys(takes);
      if (!kinds.includes(body.kind) || (!h.local && !remoteKinds.includes(body.kind))) return invalid("kind");
      if (outranks(h)) return outranked();
      const onPosts = postKinds.includes(body.kind);
      const ids = body.status_ids;
      if (onPosts) {
        if (ids === undefined || ids.length === 0 || ids.length > 100 || new Set(ids).size !== ids.length)
          return invalid("status_ids");
        if (ids.some((id) => c.statusById(id)?.account.id !== h.account.id)) return invalid("status_ids");
      } else if (ids !== undefined) return invalid("status_ids");
      if (body.expires_in !== undefined && (onPosts || body.kind === "ban")) return invalid("expires_in");
      if (
        body.expires_in !== undefined &&
        (!Number.isInteger(body.expires_in) || body.expires_in < 1 || body.expires_in > tenYears)
      )
        return invalid("expires_in");
      if (refusedText(body.public_reason, 5000)) return invalid("public_reason");
      if (refusedText(body.private_note, 5000)) return invalid("private_note");
      if (body.block_email === true && (body.kind !== "ban" || !h.local || h.email === undefined))
        return invalid("block_email");
      if (body.report_id !== undefined && !c.reports.actioned(body.report_id, h.account.id))
        return invalid("report_id");
      const now = Date.now();
      const action: Action = {
        id: next(now, 46_250),
        account_id: h.account.id,
        kind: body.kind,
        reason_category: body.reason_category,
        ...(body.public_reason === undefined || body.public_reason === "" ? {} : { public_reason: body.public_reason }),
        ...(body.private_note === undefined || body.private_note === "" ? {} : { private_note: body.private_note }),
        ...(body.report_id === undefined ? {} : { report_id: body.report_id }),
        issuer: "moderator",
        issued_by_account_id: c.accountId(c.viewer),
        notify_target: h.local && body.notify_target,
        state: "active",
        issued_at: iso(now),
        ...(body.expires_in === undefined ? {} : { expires_at: iso(now + body.expires_in * 1000) }),
        ...(ids === undefined ? {} : { status_ids: [...ids].sort() }),
      };
      actions.push(action);
      if (body.block_email === true && h.email !== undefined)
        blocks.push({
          id: next(now, 46_750),
          kind: "email_hash",
          value: await sha256Hex(canonicalEmail(h.email)),
          severity: "reject",
          moderation_action_id: action.id,
          state: "active",
          created_at: iso(now),
        });
      return { status: 201, body: structuredClone(action) };
    },
    "GET /admin/account_actions/{id}": ({ params }) => {
      if (!may("manage_accounts")) return forbidden();
      const a = actionById(params.id);
      return a === undefined ? notFound() : { status: 200, body: actionNow(a) };
    },
    "POST /admin/account_actions/{id}/lift": ({ params, body }) => {
      if (!may("manage_accounts")) return forbidden();
      const a = actionById(params.id);
      if (a === undefined) return notFound();
      if (a.kind === "ban") return invalid("id");
      if (refusedText(body?.reason, 5000)) return invalid("reason");
      const h = held.get(a.account_id);
      if (h !== undefined && outranks(h)) return outranked();
      lift(a, body?.reason);
      return { status: 200, body: actionNow(a) };
    },
    "POST /admin/accounts/{id}/reveal": ({ params }) => {
      const h = accountFor(params.id);
      if (refusal(h)) return h;
      if (!h.local) return invalid("id");
      if (outranks(h)) return outranked();
      const ips = [
        ...new Set([...h.signIns.map((s) => s.ip), ...(h.registrationIp === undefined ? [] : [h.registrationIp])]),
      ];
      return {
        status: 200,
        body: {
          ...(h.email === undefined ? {} : { email: h.email, email_verified: h.emailVerified }),
          ...(h.registrationIp === undefined || h.registration === undefined
            ? {}
            : { registration_ip: h.registrationIp, registered_at: iso(h.registration.submitted) }),
          sign_ins: h.signIns
            .toSorted((a, b) => b.at - a.at)
            .slice(0, 20)
            .map((s) => ({ ip: s.ip, user_agent: s.ua, signed_in_at: iso(s.at) })),
          same_ip_accounts: ips.flatMap((ip) => {
            const others = newestAccounts().filter((o) => o !== h && o.local && ipsOf(o).includes(ip));
            return others.length === 0 ? [] : [{ ip, accounts: others.slice(0, 20).map(adminAccount) }];
          }),
        },
      };
    },
    "GET /admin/accounts/{id}/sign_ins": ({ params, query }) => {
      const h = accountFor(params.id);
      if (refusal(h)) return h;
      if (!h.local) return invalid("id");
      if (outranks(h)) return outranked();
      const signIns = h.signIns
        .toSorted((a, b) => b.at - a.at)
        .map((s, i) => ({
          key: `${String(s.at)} ${String(i)}`,
          item: { ip: s.ip, user_agent: s.ua, signed_in_at: iso(s.at) },
        }));
      const { body } = page(signIns, query, (s) => s.key);
      return { status: 200, body: { ...body, items: body.items.map((s) => s.item) } };
    },
    "GET /admin/accounts/{id}/same_ip_accounts": ({ params, query }) => {
      const h = accountFor(params.id);
      if (refusal(h)) return h;
      const ip = query.get("ip") ?? "";
      if (ipv4(ip) === undefined && !/^[0-9a-f:]+$/i.test(ip))
        return problem(400, "Bad Request", { detail: "ip is not valid.", field: "ip" });
      if (!h.local) return invalid("id");
      if (outranks(h)) return outranked();
      const others = newestAccounts().filter((o) => o !== h && o.local && ipsOf(o).includes(ip));
      const { body } = page(others.map(adminAccount), query);
      return { status: 200, body };
    },
    "POST /admin/accounts/{id}/email_suppression/lift": ({ params }) => {
      const h = accountFor(params.id);
      if (refusal(h)) return h;
      if (!h.local) return invalid("id");
      if (outranks(h)) return outranked();
      if (h.suppression === undefined)
        return problem(409, "The email address is not suppressed", { type: "/api/v1/problems/email-not-suppressed" });
      delete h.suppression;
      return { status: 204 };
    },
    "POST /admin/accounts/{id}/password_reset_link": ({ params }) => {
      const h = accountFor(params.id, "manage_login_recovery");
      if (refusal(h)) return h;
      if (!h.local) return invalid("id");
      if (outranks(h)) return outranked();
      const token = "q7Rk2vXo9LmBf3TzW8yNc1HdPa6GsJe4UiKt5OrYw0E";
      return {
        status: 201,
        body: {
          url: `https://${c.host}/auth/password/reset?token=${token}`,
          expires_at: iso(Date.now() + 60 * minute),
          login_providers_only: false,
        },
      };
    },
    "POST /admin/accounts/{id}/two_factor/disable": ({ params }) => {
      const h = accountFor(params.id, "manage_login_recovery");
      if (refusal(h)) return h;
      if (!h.local) return invalid("id");
      if (outranks(h)) return outranked();
      if (!h.twoFactor)
        return problem(409, "Two-factor authentication is not on", { type: "/api/v1/problems/two-factor-not-enabled" });
      h.twoFactor = false;
      return { status: 204 };
    },
    "POST /admin/accounts/{id}/refetch_statuses": ({ params }) => {
      const h = accountFor(params.id, "manage_domains");
      if (refusal(h)) return h;
      if (h.local) return invalid("id");
      return { status: 202 };
    },

    "GET /admin/appeals": ({ query }) => {
      if (!may("manage_accounts")) return forbidden();
      const state = query.get("state");
      const listed = appeals
        .filter((a) => state === null || a.state === state)
        .flatMap((a) => {
          const shown = appealOf(a);
          return shown === undefined ? [] : [shown];
        })
        .toSorted((a, b) => b.submitted_at.localeCompare(a.submitted_at));
      return { status: 200, body: page(listed, query).body };
    },
    "GET /admin/appeals/{id}": ({ params }) => {
      if (!may("manage_accounts")) return forbidden();
      const found = appeals.find((a) => a.id === params.id);
      const shown = found === undefined ? undefined : appealOf(found);
      return shown === undefined ? notFound() : { status: 200, body: shown };
    },

    "GET /admin/hook_decisions": ({ query }) => {
      if (!may("manage_reports")) return forbidden();
      const [hook, mode, outcome] = [query.get("hook"), query.get("mode"), query.get("outcome")];
      const listed = newestDecisions().filter(
        (d) =>
          (hook === null || d.hook === hook) &&
          (mode === null || d.mode === mode) &&
          (outcome === null ? d.outcome !== "allow" : d.outcome === outcome),
      );
      return { status: 200, body: page(listed, query).body };
    },
    "GET /admin/hook_decisions/summary": () => {
      if (!may("manage_reports")) return forbidden();
      const since = Date.now() - 7 * day;
      const hooks: Schema<"HookName">[] = [
        "inbound.status",
        "inbound.follow",
        "signup.check",
        "media.check",
        "local.status",
        "inbound.actor",
        "local.profile",
      ];
      const recent = decisions.filter((d) => Date.parse(d.decided_at) >= since);
      return {
        status: 200,
        body: {
          since: iso(since),
          hooks: hooks.map((hook) => {
            const own = recent.filter((d) => d.hook === hook);
            const pairs = [...new Set(own.map((d) => `${d.mode} ${d.outcome}`))];
            // Every hook has a plugin but `local.profile`; each enforces, `inbound.status` after observing a while.
            const configured = hook !== "local.profile";
            return {
              hook,
              configured,
              mode: configured ? "enforce" : "off",
              ...(configured ? { on_failure: hook === "signup.check" ? "require_approval" : "pass" } : {}),
              outcomes: pairs.map((pair) => {
                const of = own.filter((d) => `${d.mode} ${d.outcome}` === pair);
                const latencies = of.map((d) => d.latency_ms);
                const [first] = of;
                return {
                  mode: first?.mode ?? "enforce",
                  outcome: first?.outcome ?? "allow",
                  count: of.length,
                  average_latency_ms: Math.round(latencies.reduce((a, b) => a + b, 0) / of.length),
                  max_latency_ms: latencies.reduce((a, b) => Math.max(a, b), 0),
                };
              }),
            };
          }),
        },
      };
    },
    "GET /admin/hook_decisions/{id}": ({ params }) => {
      if (!may("manage_reports")) return forbidden();
      const d = decisionById(params.id);
      return d === undefined ? notFound() : { status: 200, body: d };
    },
    "POST /admin/hook_decisions/{id}/override": ({ params }) => {
      if (!may("manage_reports")) return forbidden();
      const d = decisionById(params.id);
      if (d === undefined) return notFound();
      if (d.overridden_at !== undefined) return { status: 200, body: d };
      if (!d.overridable)
        return problem(409, "This decision cannot be cancelled", {
          type: "/api/v1/problems/hook-decision-not-overridable",
        });
      d.overridden_at = iso(Date.now());
      d.overridable = false;
      return { status: 200, body: d };
    },
    "GET /admin/hook_appeals": ({ query }) => {
      if (!may("manage_reports")) return forbidden();
      const state = query.get("state");
      const listed = hookAppeals
        .filter((a) => state === null || a.state === state)
        .flatMap((a) => {
          const shown = hookAppealOf(a);
          return shown === undefined ? [] : [shown];
        })
        .toSorted((a, b) => b.submitted_at.localeCompare(a.submitted_at));
      return { status: 200, body: page(listed, query).body };
    },
    "GET /admin/hook_appeals/{id}": ({ params }) => {
      if (!may("manage_reports")) return forbidden();
      const found = hookAppeals.find((a) => a.id === params.id);
      const shown = found === undefined ? undefined : hookAppealOf(found);
      return shown === undefined ? notFound() : { status: 200, body: shown };
    },

    "GET /admin/registrations": ({ query }) => {
      if (c.singleUser) return singleUserMode();
      if (!may("manage_accounts")) return forbidden();
      const listed = registrations
        .filter((r) => r.state === "pending")
        .toSorted((a, b) => a.submitted - b.submitted)
        .flatMap((r) => {
          const shown = registrationOf(r, true);
          return shown === undefined ? [] : [shown];
        });
      return { status: 200, body: page(listed, query).body };
    },

    "GET /admin/signup_blocks": ({ query }) => {
      if (c.singleUser) return singleUserMode();
      if (!may("manage_accounts")) return forbidden();
      const kind = query.get("kind");
      const active = query.get("active") === "true";
      const listed = newestBlocks().filter(
        (b) => (kind === null || b.kind === kind) && (!active || b.state === "active"),
      );
      return { status: 200, body: page(listed, query).body };
    },
    "POST /admin/signup_blocks": async ({ body }) => {
      if (c.singleUser) return singleUserMode();
      if (!may("manage_accounts")) return forbidden();
      if (refusedText(body.comment, 1000)) return invalid("comment");
      if (body.expires_in !== undefined && (body.expires_in < 1 || body.expires_in > tenYears))
        return invalid("expires_in");
      if (body.value.length > 320) return invalid("value");
      const value = await canonicalValue(body.kind, body.value);
      if (value === undefined) return invalid("value");
      const now = Date.now();
      const block: Block = {
        id: next(now, 46_750),
        kind: body.kind,
        value,
        severity: body.severity,
        ...(body.comment === undefined || body.comment === "" ? {} : { comment: body.comment }),
        state: "active",
        created_at: iso(now),
        ...(body.expires_in === undefined ? {} : { expires_at: iso(now + body.expires_in * 1000) }),
      };
      blocks.push(block);
      return { status: 201, body: block };
    },
    "POST /admin/signup_blocks/import": async ({ body }) => {
      if (c.singleUser) return singleUserMode();
      if (!may("manage_accounts")) return forbidden();
      if (refusedText(body.comment, 1000)) return invalid("comment");
      if (body.expires_in !== undefined && (body.expires_in < 1 || body.expires_in > tenYears))
        return invalid("expires_in");
      const lines = body.text.split(/\r?\n/).map((text, i) => ({ line: i + 1, text: text.trim() }));
      const values = lines.filter((l) => l.text !== "" && !l.text.startsWith("#"));
      if (values.length > 10_000) return invalid("text");
      const strength = (s: Schema<"AdminSignupBlockSeverity">) => (s === "reject" ? 2 : 1);
      const seen = new Set<string>();
      const skipped: Schema<"SignupBlockImportSkipped">[] = [];
      const now = Date.now();
      let added = 0;
      for (const { line, text } of values) {
        const value = (text.startsWith('"') ? (/^"([^"]*)"/.exec(text)?.[1] ?? "") : (text.split(",")[0] ?? "")).trim();
        const canonical = await canonicalValue(body.kind, value);
        if (canonical === undefined) skipped.push({ line, value, reason: "invalid" });
        else if (seen.has(canonical)) skipped.push({ line, value, reason: "duplicate" });
        else if (
          newestBlocks().some(
            (b) =>
              b.state === "active" &&
              b.kind === body.kind &&
              b.value === canonical &&
              strength(b.severity) >= strength(body.severity),
          )
        ) {
          seen.add(canonical);
          skipped.push({ line, value, reason: "already_blocked" });
        } else {
          seen.add(canonical);
          added += 1;
          blocks.push({
            id: next(now, 46_750),
            kind: body.kind,
            value: canonical,
            severity: body.severity,
            ...(body.comment === undefined || body.comment === "" ? {} : { comment: body.comment }),
            state: "active",
            created_at: iso(now),
            ...(body.expires_in === undefined ? {} : { expires_at: iso(now + body.expires_in * 1000) }),
          });
        }
      }
      return { status: 200, body: { added, skipped } };
    },
    "GET /admin/signup_blocks/{id}": ({ params }) => {
      if (c.singleUser) return singleUserMode();
      if (!may("manage_accounts")) return forbidden();
      const b = blocks.find((x) => x.id === params.id);
      return b === undefined ? notFound() : { status: 200, body: blockNow(b) };
    },
    "POST /admin/signup_blocks/{id}/lift": ({ params }) => {
      if (c.singleUser) return singleUserMode();
      if (!may("manage_accounts")) return forbidden();
      const b = blocks.find((x) => x.id === params.id);
      if (b === undefined) return notFound();
      if (blockNow(b).state === "active") {
        b.state = "lifted";
        b.lifted_at = iso(Date.now());
      }
      return { status: 200, body: blockNow(b) };
    },

    "GET /admin/invites": ({ query }) => {
      if (c.singleUser) return singleUserMode();
      if (!may("manage_invites")) return forbidden();
      const listed = invites.map(inviteNow).toSorted((a, b) => b.created_at.localeCompare(a.created_at));
      return { status: 200, body: page(listed, query).body };
    },
    "POST /admin/invites": ({ body }) => {
      if (c.singleUser) return singleUserMode();
      if (!may("manage_invites")) return forbidden();
      if (c.registrationClosed)
        return problem(409, "Registrations are closed", { type: "/api/v1/problems/registration-closed" });
      const max = body?.max_uses;
      const expires = body?.expires_in;
      if (max !== undefined && (!Number.isInteger(max) || max < 1 || max > 1_000_000)) return invalid("max_uses");
      if (expires !== undefined && (!Number.isInteger(expires) || expires < 1 || expires > tenYears))
        return invalid("expires_in");
      const now = Date.now();
      const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";
      const code = Array.from({ length: 16 }, (_, i) => alphabet[(invites.length * 7 + i * 13) % alphabet.length]).join(
        "",
      );
      const invite: Invite = {
        id: next(now, 46_850),
        code,
        url: inviteUrl(code),
        ...(max === undefined ? {} : { max_uses: max }),
        uses: 0,
        state: "usable",
        creator_kind: "staff",
        created_by: c.accountOf(c.viewer),
        created_by_deleted: false,
        ...(expires === undefined ? {} : { expires_at: iso(now + expires * 1000) }),
        created_at: iso(now),
      };
      invites.push(invite);
      return { status: 201, body: invite };
    },
    "POST /admin/invites/{id}/revoke": ({ params }) => {
      if (c.singleUser) return singleUserMode();
      if (!may("manage_invites")) return forbidden();
      const i = invites.find((x) => x.id === params.id);
      if (i === undefined) return notFound();
      i.state = "revoked";
      return { status: 200, body: inviteNow(i) };
    },

    "POST /admin/voice_rooms/{id}/end": ({ params, body }) => {
      if (!may("manage_reports")) return forbidden();
      if (refusedText(body?.reason, 500)) return invalid("reason");
      return c.reports.endRoom(params.id, body?.reason) ? { status: 204 } : notFound();
    },
    "POST /admin/events/{id}/cancel": ({ params, body }) => {
      if (!may("manage_reports")) return forbidden();
      if (refusedText(body?.reason, 500)) return invalid("reason");
      const result = c.reports.cancelEvent(params.id, body?.reason);
      if (result === "missing") return notFound();
      if (result === "closed")
        return problem(409, "The event is cancelled already", { type: "/api/v1/problems/event-closed" });
      return { status: 204 };
    },
  };

  /** A registration as the moderators review it; its email address only while it waits (`withEmail`). */
  function registrationOf(r: Registration, withEmail: boolean): Schema<"AdminRegistration"> | undefined {
    const h = [...held.values()].find((x) => x.registration === r);
    const shows = withEmail && r.state === "pending" && h?.email !== undefined;
    return {
      id: r.id,
      ...(h === undefined ? {} : { account_id: h.account.id, username: h.username }),
      state: r.state,
      state_changed_at: iso(r.changed),
      ...(shows ? { email: h.email ?? "", email_verified: h.emailVerified } : {}),
      ...(r.note === undefined ? {} : { decision_note: r.note }),
      ...(r.reason === undefined ? {} : { reason: r.reason }),
      ...(r.invite === undefined ? {} : { invite_id: r.invite }),
      submitted_at: iso(r.submitted),
    };
  }

  /** Decides a registration of `id` as `state`, once. */
  const decide = (id: string, state: "approved" | "rejected", note: string | undefined) => {
    if (c.singleUser) return singleUserMode();
    if (!may("manage_accounts")) return forbidden();
    if (refusedText(note, 1000)) return invalid("note");
    const r = registrations.find((x) => x.id === id);
    if (r === undefined) return notFound();
    if (r.state === "pending") {
      r.state = state;
      r.changed = Date.now();
      if (note !== undefined && note !== "") r.note = note;
    }
    const shown = registrationOf(r, false);
    return shown === undefined ? notFound() : { status: 200 as const, body: shown };
  };
  handlers["POST /admin/registrations/{id}/approve"] = ({ params, body }) => decide(params.id, "approved", body?.note);
  handlers["POST /admin/registrations/{id}/reject"] = ({ params, body }) => decide(params.id, "rejected", body?.note);

  /** Decides an appeal of an action. */
  const decideAppeal = (id: string, state: "upheld" | "overturned", reason: string | undefined) => {
    if (!may("manage_accounts")) return forbidden();
    if (refusedText(reason, 2000)) return invalid("public_reason");
    const a = appeals.find((x) => x.id === id);
    const action = a === undefined ? undefined : actionById(a.actionId);
    if (a === undefined || action === undefined) return notFound();
    const h = held.get(action.account_id);
    if (h !== undefined && outranks(h)) return outranked();
    if (a.state !== "pending" && a.state !== state)
      return problem(409, "The appeal was decided the other way", { type: "/api/v1/problems/appeal-decided" });
    if (a.state === "pending") {
      a.state = state;
      a.decided = Date.now();
      a.by = c.accountId(c.viewer);
      if (reason !== undefined && reason !== "") a.reason = reason;
      // Overturning lifts the action, a ban's too, which cancels the account's deletion.
      if (state === "overturned") lift(action, undefined);
    }
    const shown = appealOf(a);
    return shown === undefined ? notFound() : { status: 200 as const, body: shown };
  };
  handlers["POST /admin/appeals/{id}/uphold"] = ({ params, body }) =>
    decideAppeal(params.id, "upheld", body?.public_reason);
  handlers["POST /admin/appeals/{id}/overturn"] = ({ params, body }) =>
    decideAppeal(params.id, "overturned", body?.public_reason);

  /** Decides an appeal of a refused post. */
  const decideHookAppeal = (id: string, state: "upheld" | "overturned") => {
    if (!may("manage_reports")) return forbidden();
    const a = hookAppeals.find((x) => x.id === id);
    if (a === undefined) return notFound();
    if (a.state !== "pending" && a.state !== state)
      return problem(409, "The appeal was decided the other way", { type: "/api/v1/problems/appeal-decided" });
    if (a.state === "pending") {
      a.state = state;
      a.decided = Date.now();
      const d = decisionById(a.decision);
      if (state === "overturned" && d !== undefined) d.overridden_at = iso(Date.now());
    }
    const shown = hookAppealOf(a);
    return shown === undefined ? notFound() : { status: 200 as const, body: shown };
  };
  handlers["POST /admin/hook_appeals/{id}/uphold"] = ({ params }) => decideHookAppeal(params.id, "upheld");
  handlers["POST /admin/hook_appeals/{id}/overturn"] = ({ params }) => decideHookAppeal(params.id, "overturned");

  return {
    handlers,
    moderation: {
      id: (k) =>
        keyed.get(k) ??
        (k.startsWith("account:")
          ? [...held].find(([, h]) => h.username === k.slice("account:".length))?.[0]
          : undefined) ??
        "none",
      pendingRegistrations: () => (c.singleUser ? 0 : pendingRegistrations()),
      pendingAppeals,
    },
  };
}
