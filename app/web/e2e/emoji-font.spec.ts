// SPDX-License-Identifier: AGPL-3.0-only
// The Noto Color Emoji web font (ADR-0095 §3) in Chromium, Firefox and WebKit: the emoji-font-* projects of
// playwright.config.ts run this spec, and only they. A page loads the splits of @my/ui/emoji.css only when it shows an
// emoji of their unicode-range, and each browser picks the format its tech() supports: COLRv1 in Chromium and Firefox,
// OpenType-SVG in WebKit. It plays the landing page against `vite preview`, whose client API it answers with page.route.
import type { Page } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, test } from "./fixtures";

type Instance = components["schemas"]["Instance"];
type Status = components["schemas"]["Status"];

const account: Status["account"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  acct: "bob",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000002",
  display_name: "Bob",
  note: "",
  fields: [],
  emojis: [],
  badges: [],
  url: "https://my.example/@bob",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
};

function status(n: number, content: string): Status {
  return {
    id: `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`,
    uri: `https://my.example/ap/statuses/${String(n)}`,
    account,
    content,
    spoiler_text: "",
    sensitive: false,
    visibility: "public",
    language: "en",
    created_at: new Date(Date.now() - n * 7 * 60_000).toISOString(),
    media_attachments: [],
    mentions: [],
    tags: [],
    emojis: [],
    replies_count: 0,
    boosts_count: 0,
    favourites_count: 0,
    reactions: [],
  };
}

const instance: Instance = {
  domain: "my.example",
  name: "my example",
  short_description: "A small server for gardeners.",
  description: "We grow vegetables, herbs and flowers.",
  version: "0.0.0-e2e",
  source_url: "https://github.com/mycroblog/my",
  registrations: { mode: "approval" },
  limits: { max_post_characters: 500, require_media_description: false },
  timelines: { local: "everyone", federated: "everyone" },
  federation: { mode: "both" },
  sends_email: true,
  search_indexing: "deny",
  ai_usage: "deny",
  finance_visibility: "private",
  income_visibility: "private",
  supporters_enabled: false,
  supporter_list_visibility: "off",
  voice_enabled: false,
  voice_room_starters: "permitted",
  rules: [],
};

/** The emoji font files of the build: assets/noto-emoji-<format>-<split>-<hash>.woff2. */
const emojiFont = /\/noto-emoji-(colrv1|svg)-([a-z-]+)-[\w-]+\.woff2$/;

/** Opens the landing page with its two recent posts, and returns the emoji font files the page requests, as they come. */
async function openLanding(page: Page, posts: readonly [string, string]): Promise<string[]> {
  const requested: string[] = [];
  page.on("request", (request) => {
    const match = emojiFont.exec(new URL(request.url()).pathname);
    if (match !== null) requested.push(`${match[1] ?? ""}/${match[2] ?? ""}`);
  });
  await page.route("**/api/v1/instance", (route) => route.fulfill({ json: instance }));
  await page.route("**/api/v1/timelines/local*", (route) =>
    route.fulfill({ json: { items: [status(1, posts[0]), status(2, posts[1])] } }),
  );
  await page.goto("/");
  await expect(page.getByRole("main").getByRole("article")).toHaveCount(2);
  await page.evaluate(() => document.fonts.ready.then(() => undefined));
  return requested;
}

/** The format of the splits that the browser of the running project draws with. */
function expectedFormat(): "colrv1" | "svg" {
  return test.info().project.use.browserName === "webkit" ? "svg" : "colrv1";
}

test.describe("the emoji font", () => {
  test("is not loaded by a page without emoji, even with the text symbols left out of its range", async ({ page }) => {
    const requested = await openLanding(page, ["<p>The tomatoes are ripe!</p>", "<p>#1 tip: water at 7am © Bob ™ ↔ ▶</p>"]);
    // A font the page needs is requested before document.fonts settles; give a late one a moment to show.
    await page.waitForTimeout(500);
    expect(requested).toEqual([]);
  });

  test("loads only the splits of the emoji the page shows, in the format the browser supports", async ({ page }) => {
    const requested = await openLanding(page, ["<p>Sprouts today 🌱</p>", "<p>Thank you 😊</p>"]);
    await expect.poll(() => [...new Set(requested)].sort()).toEqual(
      [`${expectedFormat()}/animals-nature`, `${expectedFormat()}/smileys-emotion`].sort(),
    );
    const loaded = await page.evaluate(() =>
      [...document.fonts].filter((face) => face.family.replaceAll('"', "") === "my Noto Color Emoji" && face.status === "loaded").length,
    );
    expect(loaded).toBe(2);
  });
});
