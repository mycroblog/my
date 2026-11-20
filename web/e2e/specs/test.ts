// SPDX-License-Identifier: AGPL-3.0-only

// The test of the specs: Playwright's, with the demo server as its base URL, and its fixtures:
//
// - `signIn(person)` signs in through the server's sign-in screen with the person's password and waits for the home
//   screen's heading (a fresh person's home may show no feed).
// - `fresh` makes what a test changes, new on every run of it (src/fresh.ts), so that the specs pass in any order and
//   with `--repeat-each`: a test that follows, favourites, accepts or posts acts on its own subjects, never on the
//   scenario's people and posts, which every test reads.
//   - `fresh.person({ locked?, totp?, follows?, followedBy?, followsVisibility? })` a new person with a password, a
//     display name unique to them ("Fresh 1a2b3c4d") and a username `fresh_…`, locked or not, following and followed
//     by the people given (each follow accepted), who shows their lists of follows to `followsVisibility`. With
//     `totp: true` it enables two-factor authentication last and returns `totp: { secret, step }`: sign them in on the
//     screen with `totpCode(p.totp.secret, { avoidStep: p.totp.step })`.
//   - `fresh.post(author, { text, visibility?, image?, images?, files?, sensitive?, spoiler?, poll?, language?,
//     replyTo? })` a new post of `author` (a person of the scenario or a fresh one) whose text is `text` and a word
//     unique to it, in `language` or the author's, with images, a video or a sound (src/av.ts), replying to the post of
//     the ID `replyTo`; it returns `{ id, text, media }`, and
//     `text`, the whole text, finds the post on the screen.
//   - `fresh.reaction(by, post, emoji)` the emoji reaction of `by` to a fresh post: a Unicode emoji, or a custom one's
//     shortcode without colons.
//   - `fresh.deletion(author, post)` the deletion of a fresh post by its author.
//   - `fresh.favourite(by, post)` the favourite of `by` on a fresh post.
//   - `fresh.notificationOf(p, type)` the ID of `p`'s newest notification of `type` (mention, favourite, follow).
//   - `fresh.notificationPolicy(p, policy)` what `p`'s notification policy does with the sources that meet each
//     condition given (`not_following`, `not_follower`, `new_account`): accept, filter (hold back) or drop.
//   - `fresh.source(author, id)` the text and content warning of the author's post `id` as the server keeps them.
//   - `fresh.draft(author, { text, inSeconds? })` a new draft of `author` whose text is `text` and a word unique to it,
//     scheduled `inSeconds` from now when given; it returns `{ id, text }`.
//   - `fresh.event(organizer, { title, inDays?, candidates? })` a new event for every local user, two hours long,
//     `inDays` from now or a date poll of the days given; it returns `{ id, title }`.
//   - `fresh.evenings(inDays)` the evenings (18:00 of the test's zone) that many days from now, for a form of dates.
//   - `fresh.followRequest(from, to)` a follow request of `from` to the locked `to`, waiting for `to` to decide it.
//   - `fresh.move(from, to)` the move of the fresh `from`'s account to the fresh `to`'s, which names `from` as an alias.
//   - `fresh.moderate(staff, target, { kind, publicReason, expiresIn? })` a moderation action of `staff`, a fresh person
//     made with `admin: true`, on the fresh `target`'s account, which is told of it.
//   - `fresh.audited(staff)` what the audit log holds of the operations of `staff` (a fresh person made with
//     `admin: true`), the newest first, each as its kind and its target's (`account_action_issued account`).
//   - `fresh.newcomer({ approval? })` a username, an email address and a password nobody has signed up with, for a
//     person who signs up on the registration screen: the server is open to everyone, and with `approval` the address
//     is of the domain whose sign-ups wait for a moderator (the scenario's `approvalDomain`);
//     `fresh.approve(newcomer)` the moderator's approval of their registration, and `fresh.emailConfirmed(newcomer)`
//     whether the server holds their email address as confirmed.
//   - `fresh.invite()` an invite of the administrator for one person: `{ code, url }`, the registration screen's
//     address with the code.
//   - `fresh.providerIdentity()` a subject, an email address, a name and a username nobody has signed in with, for a
//     person who signs in through the login provider (screens/login-provider.ts).
//
// And what a spec needs of the scenario: `person(username)` the person to sign in as, `secrets()` what the seed made
// that the server decided (a TOTP secret and the step its enrollment used, a passkey's key, an app's client ID),
// `totpCode(secret, { avoidStep })` a code of an authenticator app that the server has not taken: the current step's,
// or the next one's when the current step is `avoidStep`, and `mailedLink(address, path)` the link to the server's
// page at `path` in the mail the server wrote to `address` (src/mail.ts).

import { randomBytes } from "node:crypto";
import { readFileSync } from "node:fs";
import { expect, test as base } from "@playwright/test";
import { Fresh } from "../src/fresh";
import { mailedLinkTo } from "../src/mail";
import { personOf, type Person } from "../src/scenario";
import type { SeedState } from "../src/seed/seed";
import { totpCode } from "../src/seed/totp";
import { runMyOn } from "../src/server/demo";

export { expect, personOf as person, totpCode };
export type { FreshPerson, FreshPost, Newcomer } from "../src/fresh";

/** The demo server's origin, which global-setup.ts set for the workers. */
function demoURL(): string {
  const url = process.env["E2E_DEMO_URL"];
  if (url === undefined) throw new Error("e2e: no demo server; run the specs with scripts/task.sh web:e2e");
  return url;
}

/** What the seed made that the server decided: the TOTP secrets and steps, the passkeys' keys, the apps' client IDs. */
export function secrets(): SeedState {
  return JSON.parse(readFileSync(process.env["E2E_DEMO_STATE"] ?? "", "utf8")) as SeedState;
}

/**
 * The newest link to the server's page at `path` (`/auth/verify_email`) in the mail to `address`, once the server
 * wrote it: a mail goes out in a job, after the screen that caused it has answered.
 */
export async function mailedLink(address: string, path: string): Promise<string> {
  const log = process.env["E2E_DEMO_LOG"];
  if (log === undefined || log === "")
    throw new Error("e2e: no log of the demo server; run the specs with scripts/task.sh web:e2e");
  let link: string | undefined;
  await expect
    .poll(() => (link = mailedLinkTo(log, address, path)), { message: `a mail to ${address} with a link to ${path}` })
    .toBeDefined();
  if (link === undefined) throw new Error(`e2e: no mail to ${address} links to ${path}`);
  return link;
}

type Fixtures = {
  /** Signs in as `p` with the password and waits until the home screen shows. */
  signIn: (p: Person) => Promise<void>;
  /** Makes the test's own people, posts and follow requests on the demo server. */
  fresh: Fresh;
};

export const test = base.extend<Fixtures>({
  signIn: async ({ page }, use) => {
    await use(async (p) => {
      await page.goto("/login");
      await page.getByRole("textbox", { name: "Email address", exact: true }).fill(p.email);
      await page.getByLabel("Password", { exact: true }).fill(p.password);
      await page.getByRole("button", { name: "Sign in", exact: true }).click();
      // The heading, not the feed: the home of a fresh person who follows no one shows no feed.
      await expect(page.getByRole("main").getByRole("heading", { level: 1, name: "Home", exact: true })).toBeVisible();
    });
  },
  fresh: async ({ baseURL }, use) => {
    const envFile = process.env["E2E_DEMO_ENV"];
    if (envFile === undefined || baseURL === undefined)
      throw new Error("e2e: no demo server; run the specs with scripts/task.sh web:e2e");
    const state = secrets();
    await use(
      new Fresh({
        server: baseURL,
        runMy: (args, input) => runMyOn(envFile, args, input),
        database: (JSON.parse(readFileSync(envFile, "utf8")) as Record<string, string>)["MY_DATABASE_URL"] ?? "",
        totp: { secrets: state.totp, steps: state.totpSteps },
      }),
    );
  },
  // A fixture, not `test.use` in this module: a module runs once per worker, so its `test.use` reaches only the spec
  // file that imported it first, and the worker's next file has no base URL.
  baseURL: async ({ baseURL }, use) => {
    await use(baseURL ?? demoURL());
  },
  // The browser of each test comes from an address of its own (198.18.0.0/15, for tests), which the demo server reads
  // from X-Forwarded-For: the server limits sign-ins and tokens per client IP, and the tests from one machine would
  // otherwise share one limit and run out of it, as people coming from one address would.
  extraHTTPHeaders: async ({ extraHTTPHeaders }, use) => {
    const [a = 0, b = 0, c = 0] = randomBytes(3);
    await use({ ...extraHTTPHeaders, "X-Forwarded-For": `198.${18 + (a & 1)}.${b}.${c}` });
  },
});
