// SPDX-License-Identifier: AGPL-3.0-only

// The test of the specs: Playwright's, with the demo server as its base URL, and its fixtures:
//
// - `signIn(person)` signs in through the server's sign-in screen with the person's password and waits for the home
//   screen's heading (a fresh person's home may show no feed).
// - `fresh` makes what a test changes, new on every run of it (src/fresh.ts), so that the specs pass in any order and
//   with `--repeat-each`: a test that follows, favourites, accepts or posts acts on its own subjects, never on the
//   scenario's people and posts, which every test reads.
//   - `fresh.person({ locked?, totp?, follows?, followedBy? })` a new person with a password, a display name unique
//     to them ("Fresh 1a2b3c4d") and a username `fresh_…`, locked or not, following and followed by the people given
//     (each follow accepted). With `totp: true` it enables two-factor authentication last and returns `totp: { secret,
//     step }`: sign them in on the screen with `totpCode(p.totp.secret, { avoidStep: p.totp.step })`.
//   - `fresh.post(author, { text, visibility?, image?, spoiler? })` a new post of `author` (a person of the scenario or
//     a fresh one) whose text is `text` and a word unique to it; it returns `{ id, text }`, and `text`, the whole
//     text, finds the post on the screen.
//   - `fresh.followRequest(from, to)` a follow request of `from` to the locked `to`, waiting for `to` to decide it.
//
// And what a spec needs of the scenario: `person(username)` the person to sign in as, `announcements` the server's
// announcements (the last published is the newest), `secrets()` what the seed made that the server decided (a TOTP
// secret and the step its enrollment used, a passkey's key, an app's client ID), and `totpCode(secret, { avoidStep })`
// a code of an authenticator app that the server has not taken: the current step's, or the next one's when the
// current step is `avoidStep`.

import { randomBytes } from "node:crypto";
import { readFileSync } from "node:fs";
import { expect, test as base } from "@playwright/test";
import { Fresh } from "../src/fresh";
import { announcements, personOf, type Person } from "../src/scenario";
import type { SeedState } from "../src/seed/seed";
import { totpCode } from "../src/seed/totp";
import { runMyOn } from "../src/server/demo";

export { announcements, expect, personOf as person, totpCode };
export type { FreshPerson, FreshPost } from "../src/fresh";

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
