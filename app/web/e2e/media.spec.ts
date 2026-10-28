// SPDX-License-Identifier: AGPL-3.0-only
// Videos and audio files in posts in every width × locale project (FR-105, ADR-0054 §6, ADR-0027): an uploaded video
// with its poster, an audio file without a description, and a video of another server through the media proxy play in
// their cells with the browser's controls, never by themselves. `vite preview` has no API, so the spec answers the
// token refresh, the client API, and the media files with `page.route`, as a signed-in user's server would.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, imageVariants, test } from "./fixtures";

type Account = components["schemas"]["Account"];
type Status = components["schemas"]["Status"];

/** A one-second VP8 video of one blue frame and a one-second silent Opus file. */
const videoFile = Buffer.from(
  "GkXfo59ChoEBQveBAULygQRC84EIQoKEd2VibUKHgQJChYECGFOAZwEAAAAAAAIpEU2bdLpNu4tTq4QVSalmU6yBoU27i1OrhBZUrmtTrIHYTbuMU6uEElTDZ1OsggEfTbuMU6uEHFO7a1OsggIT7AEAAAAAAABZAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAVSalmsirXsYMPQkBNgI1MYXZmNTguNDUuMTAwV0GNTGF2ZjU4LjQ1LjEwMESJiECPQAAAAAAAFlSua8KuAQAAAAAAADnXgQFzxYiPgJbYbBAl7pyBACK1nIN1bmSGhVZfVlA4g4EBI+ODhDuaygDgAQAAAAAAAAawgaC6gVoSVMNnQJlzcwEAAAAAAAAnY8CAZ8gBAAAAAAAAGkWjh0VOQ09ERVJEh41MYXZmNTguNDUuMTAwc3MBAAAAAAAAXmPAi2PFiI+AlthsECXuZ8gBAAAAAAAAIUWjh0VOQ09ERVJEh5RMYXZjNTguOTEuMTAwIGxpYnZweGfIokWjiERVUkFUSU9ORIeUMDA6MDA6MDEuMDAwMDAwMDAwAAAfQ7Z10OeBAKPLgQAAgLAFAJ0BKqAAWgABhwiFhYiZhIgIAgJ1qgIGeZak569GXiOevRl4jnr0ZeI569GXiOevRl4jlAD+7gpnySPVzHQfit9QM0gAHFO7a5G7j7OBALeK94EB8YIBvvCBAw==",
  "base64",
);
const audioFile = Buffer.from(
  "T2dnUwACAAAAAAAAAABf1EMQAAAAAL7MFqUBE09wdXNIZWFkAQE4AYC7AAAAAABPZ2dTAAAAAAAAAAAAAF/UQxABAAAAIiMbKgE+T3B1c1RhZ3MNAAAATGF2ZjU4LjQ1LjEwMAEAAAAdAAAAZW5jb2Rlcj1MYXZjNTguOTEuMTAwIGxpYm9wdXNPZ2dTAACAuwAAAAAAAF/UQxACAAAAWsUo5TIHBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBggL5FMVS/IIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsZPZ2dTAAS4vAAAAAAAAF/UQxADAAAAEH17vgEGCAbgsw7G",
  "base64",
);

/** A poster as an SVG, served from the page's origin like the media files. */
const posterFile =
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 90"><rect width="160" height="90" fill="#4f6d8f"/></svg>';

const alice: Account = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000001",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  badges: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
};

/** Where the spec serves the media files: the page's own origin, which the CSP's media-src allows. */
const mediaBase = "/e2e-media";

const postId = "0192f0c4-7a8e-7000-8000-0000000006c1";
const post: Status = {
  id: postId,
  uri: `https://my.example/ap/statuses/${postId}`,
  url: `https://my.example/@alice/status/${postId}`,
  account: alice,
  content: "<p>Our cat, and the song she likes.</p>",
  spoiler_text: "",
  sensitive: false,
  visibility: "public",
  language: "en",
  created_at: new Date(Date.now() - 60 * 60_000).toISOString(),
  media_attachments: [
    {
      id: "0192f0c4-7a8e-7000-8000-0000000006d1",
      type: "video",
      state: "ready",
      description: "A cat jumps onto a sofa",
      meta: { focus: { x: 0, y: 0 }, width: 160, height: 90, duration: 1 },
      variants: imageVariants(`${mediaBase}/poster.svg`),
      url: `${mediaBase}/clip.webm`,
      preview_url: `${mediaBase}/poster.svg`,
    },
    {
      id: "0192f0c4-7a8e-7000-8000-0000000006d2",
      type: "audio",
      state: "ready",
      meta: { focus: { x: 0, y: 0 }, duration: 1 },
      url: `${mediaBase}/song.ogg`,
    },
    {
      // A video of another server, through the media proxy: no poster.
      id: "0192f0c4-7a8e-7000-8000-0000000006d3",
      type: "video",
      state: "ready",
      description: "A remote clip",
      meta: { focus: { x: 0, y: 0 } },
      url: `${mediaBase}/remote.webm`,
      remote_url: "https://remote.example/files/clip.webm",
    },
  ],
  mentions: [],
  tags: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
  favourited: false,
  boosted: false,
  bookmarked: false,
};

/** Answers the client API as Alice's session with the post, and the media files. */
async function serveApi(page: Page): Promise<void> {
  const json = (route: Route, body: unknown, statusCode = 200) =>
    route.fulfill({
      status: statusCode,
      contentType: statusCode < 400 ? "application/json" : "application/problem+json",
      body: JSON.stringify(body),
    });
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }),
  );
  await page.route(`**${mediaBase}/**`, (route) => {
    const name = new URL(route.request().url()).pathname.slice(mediaBase.length + 1);
    if (name === "poster.svg") return route.fulfill({ contentType: "image/svg+xml", body: posterFile });
    if (name === "song.ogg") return route.fulfill({ contentType: "audio/ogg", body: audioFile });
    return route.fulfill({ contentType: "video/webm", body: videoFile });
  });
  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname.replace("/api/v1", "");
    if (path === "/accounts/verify_credentials") return json(route, alice);
    if (path === "/instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0-e2e",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "open" },
        federation: { mode: "both" },
        limits: { max_post_characters: 500 },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (path === "/preferences") {
      // The user's locale is the project's: the app displays itself in the signed-in user's locale.
      return json(route, {
        locale: test.info().project.use.locale ?? "en",
        time_zone: "UTC",
        default_visibility: "public",
        default_post_language: "en",
        require_media_description: false,
        timeline_languages: [],
      });
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [post] });
    if (path === `/statuses/${postId}`) return json(route, post);
    if (path === `/statuses/${postId}/context`) return json(route, { ancestors: [], descendants: [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
}

test.describe("a post with videos and audio", () => {
  test.beforeEach(async ({ page }) => {
    await serveApi(page);
    await page.goto(`/@alice/status/${postId}`);
    await expect(page.getByRole("main").getByRole("article")).toBeVisible();
  });

  test("plays them in their cells with controls, never by themselves, without axe violations or sideways scrolling", async ({
    page,
    makeAxeBuilder,
  }) => {
    const article = page.getByRole("main").getByRole("article");
    const videos = article.locator("video");
    const audio = article.locator("audio");
    await expect(videos).toHaveCount(2);
    await expect(audio).toHaveCount(1);
    for (const player of [videos.first(), videos.nth(1)]) {
      await expect(player).toHaveAttribute("controls", "");
    }
    // The audio file has the page's own player (a play button and a seek slider) in place of the browser's controls.
    await expect(audio).not.toHaveAttribute("controls");
    const player = audio.locator("xpath=..");
    await expect(player.getByRole("button")).toHaveCount(1);
    await expect(player.getByRole("slider")).toHaveCount(1);
    for (const element of [videos.first(), videos.nth(1), audio]) {
      await expect(element).not.toHaveAttribute("autoplay");
      await expect(element).toHaveJSProperty("paused", true);
    }
    await expect(videos.first()).toHaveAccessibleName("A cat jumps onto a sofa");
    await expect(videos.first()).toHaveAttribute("poster", `${mediaBase}/poster.svg`);
    await expect(videos.nth(1)).toHaveAccessibleName("A remote clip");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
