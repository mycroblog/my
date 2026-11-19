// SPDX-License-Identifier: AGPL-3.0-only

// A custom emoji from its request to its approval, on the real server (#3263, FR-915, ADR-0070 §2): a person whose role
// lets them ask for custom emoji asks for one in their settings, with a shortcode unique to the run and an image; an
// administrator, in a browser of their own, finds the request under the administration's Custom emoji, tab Requests,
// and approves it; the person then finds it approved among their requests and among their own emoji. What each screen
// shows in every state is the stories' on the fake (web/app, admin-custom-emojis.stories.tsx and
// settings-custom-emojis.stories.tsx).

import { randomUUID } from "node:crypto";
import type { Page } from "@playwright/test";
import { drawPNG } from "../src/seed/png";
import { EmojiRequests, Frame, OwnEmoji, SignIn, type Upload } from "./screens";
import { expect, test, type FreshPerson } from "./test";

/** A small PNG to pick in the file chooser: a circle on a square, an emoji's shape. */
const png: Upload = {
  name: "emoji.png",
  mimeType: "image/png",
  buffer: Buffer.from(
    drawPNG({
      width: 64,
      height: 64,
      top: [250, 214, 120],
      bottom: [236, 160, 80],
      shapes: [{ kind: "circle", x: 32, y: 32, r: 18, color: [90, 60, 40] }],
    }),
  ),
};

/** Signs in as `p` on `page`, a browser of its own, and waits for the home screen. */
async function signInOn(page: Page, p: FreshPerson): Promise<void> {
  const screen = new SignIn(page);
  await screen.open();
  await screen.withPassword(p.email, p.password);
  await expect(new Frame(page).heading("Home")).toBeVisible();
}

test("a person asks for a custom emoji, an administrator approves the request, and the emoji is the person's own", async ({
  page,
  browser,
  baseURL,
  signIn,
  fresh,
}) => {
  const artist = await fresh.person({ role: { key: "e2e_emoji_artist", permissions: ["create_custom_emojis"] } });
  const admin = await fresh.person({ admin: true });
  const shortcode = `e2e_${randomUUID().replaceAll("-", "").slice(0, 16)}`;

  // The person asks for it in their settings; it waits for a decision.
  await signIn(artist);
  const own = new OwnEmoji(page);
  await own.open();
  await own.ask(shortcode, png);
  await expect(own.request(shortcode)).toContainText("Waiting for a decision");

  // The administrator, in a browser of their own, approves it from the requests.
  const theirs = await browser.newContext({
    ...(baseURL === undefined ? {} : { baseURL }),
    extraHTTPHeaders: { "X-Forwarded-For": "198.19.251.2" },
  });
  try {
    const staff = await theirs.newPage();
    await signInOn(staff, admin);
    const requests = new EmojiRequests(staff);
    await requests.open();
    await expect(requests.row(shortcode)).toBeVisible();
    await requests.approve(shortcode);
    await expect(requests.said(`:${shortcode}: is approved and made.`)).toBeVisible();
  } finally {
    await theirs.close();
  }

  // The person finds it approved, and among their own emoji.
  await page.reload();
  await expect(own.request(shortcode)).toContainText("Approved");
  await expect(own.emoji(shortcode)).toBeVisible();
});
