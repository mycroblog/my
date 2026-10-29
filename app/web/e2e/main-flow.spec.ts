// SPDX-License-Identifier: AGPL-3.0-only
// The main flow of M2 (#27) against a real `my serve` on SQLite (e2e/server.ts), in every width × locale
// project (ADR-0015, ADR-0027): sign up, sign in through the server's screen (OAuth 2.0 with PKCE,
// ADR-0030), post with a content warning and an image with its description, see the post on the home and
// the local timeline, follow (an account that approves its followers too), and the notifications that
// follow. Two users take part, each in a browser context of its own. Every screen on the way is checked
// with axe, for horizontal scrolling and against an ARIA snapshot.
//
// The server allows only a few sign-ups per address in 30 minutes (identity.SignUpIPLimit), so only the
// en-320 project signs its first user up through the sign-up page and has the registration approved with
// `my registration approve`; the other users are created with `my admin create` (e2e/server-cli.ts).
// Every user is new to the test (a random name), so a retry and the other projects never meet its posts
// or follows. The pages are in the project's locale: the app shows itself in the user's locale, which the
// test sets through the client API once the user has signed in.
import { randomBytes } from "node:crypto";
import { crc32, deflateSync } from "node:zlib";
import type { Page } from "@playwright/test";
import {
  axeBuilder,
  expect,
  expectNoAxeViolations,
  expectNoHorizontalScroll,
  localeDir,
  openFromNavigation,
  test as base,
  newPostButton,
} from "./fixtures";
import { ownsServer, runMy } from "./server-cli.ts";
import { serverURL } from "./server-origin.ts";
import { readCatalog } from "./catalog.ts";

/** The second user's page, in a browser context of its own with the project's options. */
const test = base.extend<{ secondPage: Page }>({
  secondPage: async ({ browser }, use) => {
    const context = await browser.newContext();
    await use(await context.newPage());
    await context.close();
  },
});

// Without motion the colors do not transition (motion-safe:), so axe never samples a button in the middle
// of changing its colors, such as the follow button right after it was pressed.
test.use({ baseURL: serverURL, reducedMotion: "reduce" });

/** The project's locale. */
function projectLocale(): string {
  return test.info().project.use.locale ?? "en";
}

/** The client message `key` in the project's locale, with its `{name}` arguments replaced. */
function message(key: string, values: Record<string, string> = {}): string {
  const locale = projectLocale();
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text.replaceAll(/\{(\w+)\}/g, (_, name: string) => values[name] ?? `{${name}}`);
}

/** A regular expression that matches `text` literally. */
function literal(text: string): RegExp {
  return new RegExp(text.replaceAll(/[.*+?^${}()|[\]\\]/g, "\\$&"));
}

type User = { username: string; email: string; password: string };

function newUser(role: string): User {
  const username = `e2e_${role}_${randomBytes(4).toString("hex")}`;
  return { username, email: `${username}@example.com`, password: `e2e-password-${username}` };
}

/** Creates `user` with `my admin create`, the only way besides the sign-up page to make a local user. */
function createUser(user: User): void {
  runMy(["admin", "create", "-username", user.username, "-email", user.email], `${user.password}\n`);
}

/** A PNG image of `width` × `height` pixels in one color (the PNG specification, ISO/IEC 15948). */
function png(width: number, height: number): Buffer {
  const chunk = (type: string, data: Buffer) => {
    const length = Buffer.alloc(4);
    length.writeUInt32BE(data.length);
    const body = Buffer.concat([Buffer.from(type, "latin1"), data]);
    const crc = Buffer.alloc(4);
    crc.writeUInt32BE(crc32(body));
    return Buffer.concat([length, body, crc]);
  };
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header.writeUInt8(8, 8); // bit depth
  header.writeUInt8(2, 9); // truecolor
  const row = Buffer.concat([
    Buffer.from([0]),
    Buffer.from(Array.from({ length: width }, () => [0x2a, 0x7a, 0xb0]).flat()),
  ]);
  const pixels = Buffer.concat(Array.from({ length: height }, () => row));
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", header),
    chunk("IDAT", deflateSync(pixels)),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

/** Checks the page as every screen of the flow is checked: axe and no horizontal scrolling. */
async function expectQuality(page: Page): Promise<void> {
  await expectNoAxeViolations(() => axeBuilder(page));
  await expectNoHorizontalScroll(page);
}

/** The level-one heading of the page. */
const pageHeading = (page: Page) => page.getByRole("main").getByRole("heading", { level: 1 });

/**
 * Signs in to the app: /login sends the browser to the server's sign-in screen at once, which
 * returns to the app's callback with a code (OAuth 2.0 with PKCE; the web client is first party, so no
 * consent page). When the browser already has a session on the server, the screen is skipped. Then the
 * user's display language becomes the project's locale (PATCH /api/v1/preferences with the app's access
 * token), and the app reloads in it.
 */
async function signInToApp(page: Page, user: User, { hasSession = false } = {}): Promise<void> {
  const authorized = page.waitForRequest(
    (request) =>
      new URL(request.url()).pathname.startsWith("/api/") &&
      request.headers()["authorization"]?.startsWith("Bearer ") === true,
  );
  // /login is not a page: it opens the server's sign-in screen at once.
  await page.goto("/login");
  if (!hasSession) {
    await page.locator('input[type="email"][autocomplete="username"]').fill(user.email);
    await page.locator('input[type="password"][autocomplete="current-password"]').fill(user.password);
    await page.locator('main button[type="submit"]').first().click();
  }
  const authorization = (await authorized).headers()["authorization"] ?? "";
  await expect(page).toHaveURL(`${serverURL}/home`);

  const locale = projectLocale();
  const response = await page.request.patch(`${serverURL}/api/v1/preferences`, {
    headers: { Authorization: authorization },
    data: { locale },
  });
  expect(response.ok(), `PATCH /api/v1/preferences: ${String(response.status())}`).toBe(true);
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", locale);
  await expect(page.locator("html")).toHaveAttribute("dir", localeDir(locale));
  await expect(pageHeading(page)).toHaveText(message("home.title"));
}

/** Opens a section of the app from the main navigation, as a user does. */
async function navigate(page: Page, label: "home" | "local" | "notifications" | "settings"): Promise<void> {
  // Found by its address: the notifications link's name changes once unread notifications arrive
  // ("Notifications, 1 unread", a message of its own in every locale).
  await openFromNavigation(page, `/${label}`, false);
  await expect(pageHeading(page)).toHaveText(message(`${label}.title`));
}

/** Opens the new-post form from the navigation's button (the tab bar's or the rail's) and returns it. */
async function openComposer(page: Page) {
  await newPostButton(page, message("compose.open")).click();
  await expect(page.getByRole("dialog", { name: message("compose.title") })).toBeVisible();
  const form = page.getByRole("form", { name: message("compose.title") });
  await expect(form.getByRole("textbox", { name: message("compose.text.label") })).toBeVisible();
  return form;
}

/** Asserts that a toast says `text`; an earlier toast may still say it too. */
async function expectToast(page: Page, text: string): Promise<void> {
  await expect(
    page
      .getByRole("region", { name: message("shell.toast.region") })
      .getByText(text)
      .last(),
  ).toBeVisible();
}

/** The article of the feed whose text contains `text`. */
const articleWith = (page: Page, text: string) => page.getByRole("feed").getByRole("article").filter({ hasText: text });

/**
 * Finds the article with `text` in a timeline that the other projects post to as well (the local one):
 * their newer posts can come first, and the feed mounts only the articles near the viewport, so the
 * keyboard moves through the feed (Page Down, web-client/README.md Y6) until the article is there.
 */
async function findInFeed(page: Page, text: string) {
  const articles = page.getByRole("feed").getByRole("article");
  const target = articleWith(page, text);
  await expect(articles.first()).toBeVisible();
  if ((await target.count()) === 0) {
    await articles.first().focus();
    // Only the posts of the projects that run at the same time can be newer; 20 is more than run at once.
    for (let step = 0; step < 20 && (await target.count()) === 0; step++) {
      await page.keyboard.press("PageDown");
    }
  }
  await expect(target).toBeVisible();
  return target;
}

test.describe("the main flow", () => {
  test.skip(
    !ownsServer,
    "the flow creates users with bin/my, which needs the database of the server e2e/server.ts starts",
  );

  test("sign up, sign in, post, read the timelines, follow, and get notified", async ({
    page,
    secondPage: bobPage,
  }, testInfo) => {
    test.setTimeout(180_000);
    const run = randomBytes(3).toString("hex");
    const alice = newUser("a");
    const bob = newUser("b");
    const signsUp = testInfo.project.name === "en-320";

    await test.step("the first user signs up and is approved", async () => {
      if (!signsUp) {
        createUser(alice);
        return;
      }
      await page.goto("/auth/sign_up");
      await page.locator('input[autocomplete="nickname"]').fill(alice.username);
      await page.locator('input[type="email"][autocomplete="username"]').fill(alice.email);
      await page.locator('input[type="password"][autocomplete="new-password"]').fill(alice.password);
      await page.locator("textarea").fill("E2E of the main flow");
      await page.locator('main button[type="submit"]').first().click();
      // The registration waits for a moderator: the user is signed in to the server's pages only.
      await expect(page).toHaveURL(/\/auth\/sign_in$/);
      await expect(page.locator('form[action="/auth/sign_out"]')).toBeVisible();
      expect(runMy(["registration", "approve", alice.username])).toContain(alice.username);
    });

    await test.step("the first user signs in to the app", async () => {
      await signInToApp(page, alice, { hasSession: signsUp });
      await expectQuality(page);
    });

    createUser(bob);

    await test.step("the second user signs in and asks to approve followers", async () => {
      await signInToApp(bobPage, bob);
      await navigate(bobPage, "settings");
      await bobPage
        .getByRole("main")
        .getByRole("link", { name: literal(message("settings.profile.title")) })
        .first()
        .click();
      await expect(pageHeading(bobPage)).toHaveText(message("settings.profile.title"));
      await bobPage.getByText(message("settings.profile.locked"), { exact: true }).click();
      await bobPage.getByRole("button", { name: message("settings.profile.save") }).click();
      await expect(
        bobPage.getByRole("main").getByRole("status").getByText(message("settings.profile.saved")),
      ).toBeVisible();
      await expectQuality(bobPage);
    });

    const warning = `E2E warning ${run}`;
    const text = `E2E post ${run}`;
    const description = `E2E image ${run}`;

    await test.step("the first user posts with a content warning and an image with a description", async () => {
      const form = await openComposer(page);
      await form.getByRole("button", { name: message("compose.warning.toggle") }).click();
      await form.getByRole("textbox", { name: message("compose.warning.label") }).fill(warning);
      await form.getByRole("textbox", { name: message("compose.text.label") }).fill(text);
      const chooser = page.waitForEvent("filechooser");
      await form.getByRole("button", { name: message("compose.media.add") }).click();
      await (await chooser).setFiles({ name: "e2e.png", mimeType: "image/png", buffer: png(64, 48) });
      await form
        .getByRole("textbox", { name: message("compose.media.description.label", { position: "1" }) })
        .fill(description);
      // The post button stays pressable while the image uploads, and then only says why it cannot post (G23): post
      // once the image is ready.
      const attachments = form.getByRole("list", { name: message("compose.media.list") });
      for (const progress of ["compose.media.uploading", "compose.media.processing"])
        await expect(attachments.getByText(message(progress))).toHaveCount(0);
      await expectQuality(page);
      await form.getByRole("button", { name: message("compose.submit") }).click();
      await expectToast(page, message("compose.posted.first"));
    });

    await test.step("the post is on the first user's home and local timelines", async () => {
      for (const section of ["home", "local"] as const) {
        await navigate(page, section);
        const article = await findInFeed(page, warning);
        await article.getByRole("button", { name: message("timeline.content_warning.toggle") }).click();
        await expect(article.getByText(text)).toBeVisible();
        await expect(article.getByRole("img", { name: description })).toBeVisible();
        // The author's name (the picture's link is hidden), the time, the warning and its toggle, the post,
        // its image named by the description (a button that opens it in the image viewer), and the actions.
        await expect(article).toMatchAriaSnapshot(`
          - article:
            - link
            - link:
              - time
            - paragraph: ${warning}
            - button [expanded]
            - paragraph: ${text}
            - button:
              - img "${description}"
            - button
            - button
            - button
            - button
            - button
        `);
        await expectQuality(page);
      }
    });

    await test.step("the second user reads the post on the local timeline and follows its author", async () => {
      await navigate(bobPage, "local");
      const article = await findInFeed(bobPage, warning);
      await article
        .getByRole("link", { name: literal(alice.username) })
        .first()
        .click();
      await expect(pageHeading(bobPage)).toHaveText(alice.username);
      await bobPage.getByRole("button", { name: message("profile.follow") }).click();
      await expect(bobPage.getByRole("button", { name: message("profile.follow.following") })).toBeVisible();
      // The follow button and the menu, the name and the handle, the counts, and the tabs of the posts.
      await expect(bobPage.getByRole("main")).toMatchAriaSnapshot(`
        - button
        - button
        - heading "${alice.username}" [level=1]
        - paragraph: "@${alice.username}"
        - paragraph
        - list:
          - listitem:
            - link
          - listitem:
            - link
        - tablist:
          - tab [selected]
          - tab
          - tab
        - tabpanel:
          - heading [level=2]
          - paragraph
          - feed:
            - article
      `);
      await expectQuality(bobPage);
    });

    const followersOnly = `E2E followers-only post ${run}`;

    await test.step("a followers-only post reaches the follower's home timeline but not the local one", async () => {
      const form = await openComposer(page);
      await form.getByRole("textbox", { name: message("compose.text.label") }).fill(followersOnly);
      await form
        .getByRole("button", {
          name: message("compose.visibility.button", { visibility: message("compose.visibility.public") }),
        })
        .click();
      // React Aria hides the native radio under its drawing; the label is what the user presses.
      const choices = page.getByRole("radiogroup", { name: message("compose.visibility.title") });
      await choices.getByText(message("compose.visibility.private"), { exact: true }).click();
      await expect(choices.getByRole("radio", { name: literal(message("compose.visibility.private")) })).toBeChecked();
      await page.getByRole("button", { name: message("compose.visibility.done") }).click();
      await form.getByRole("button", { name: message("compose.submit") }).click();
      await expectToast(page, message("compose.posted"));

      await navigate(bobPage, "home");
      // The stream may deliver the post after the timeline opened: it then waits behind the "1 new post"
      // button (web-client/README.md Y7) instead of being in the feed from the first render.
      const newPosts = bobPage.getByRole("main").getByRole("status").getByRole("button");
      await expect(articleWith(bobPage, followersOnly).or(newPosts)).toBeVisible();
      if (await newPosts.isVisible()) await newPosts.click();
      await expect(articleWith(bobPage, followersOnly)).toBeVisible();
      await expectQuality(bobPage);
      await navigate(bobPage, "local");
      await findInFeed(bobPage, warning);
      await expect(articleWith(bobPage, followersOnly)).toHaveCount(0);
    });

    await test.step("the first user is notified of the follower and asks to follow back", async () => {
      await navigate(page, "notifications");
      const notification = page
        .getByRole("feed")
        .getByRole("article")
        .filter({ hasText: message("notifications.type.follow", { name: bob.username }) });
      await expect(notification).toBeVisible();
      await expect(page.getByRole("main")).toMatchAriaSnapshot(`
        - heading [level=1]
        - tablist:
          - tab [selected]
          - tab
          - tab
          - tab
          - tab
        - tabpanel:
          - feed:
            - article:
              - paragraph:
                - link
              - time
              - button
      `);
      await expectQuality(page);

      // The follow back is in the notification; the second user approves followers, so it is a request.
      await notification.getByRole("button", { name: message("profile.follow.request") }).click();
      await expect(notification.getByRole("button", { name: message("profile.follow.requested") })).toBeVisible();
      // The actor's name leads to their profile, which shows the same request.
      await notification.getByRole("link", { name: bob.username }).click();
      await expect(pageHeading(page)).toHaveText(bob.username);
      await expect(page.getByRole("button", { name: message("profile.follow.requested") })).toBeVisible();
      await expectQuality(page);
    });

    await test.step("the second user is notified of the request and accepts it", async () => {
      await navigate(bobPage, "notifications");
      await expect(
        bobPage
          .getByRole("feed")
          .getByRole("article")
          .filter({ hasText: message("notifications.type.follow_request", { name: alice.username }) }),
      ).toBeVisible();
      await expectQuality(bobPage);
      await navigate(bobPage, "settings");
      await bobPage
        .getByRole("main")
        .getByRole("link", { name: literal(message("settings.follow_requests.title")) })
        .first()
        .click();
      await expect(pageHeading(bobPage)).toHaveText(message("settings.follow_requests.title"));
      await expect(bobPage.getByRole("main")).toMatchAriaSnapshot(`
        - heading [level=1]
        - link
        - list:
          - listitem:
            - paragraph
            - button
            - button
      `);
      await expectQuality(bobPage);
      await bobPage.getByRole("button", { name: literal(message("settings.follow_requests.accept")) }).click();
      await expect(bobPage.getByText(message("settings.follow_requests.empty"))).toBeVisible();
    });

    await test.step("the first user is notified that the request was accepted", async () => {
      await navigate(page, "notifications");
      await expect(
        page
          .getByRole("feed")
          .getByRole("article")
          .filter({ hasText: message("notifications.type.follow_accepted", { name: bob.username }) }),
      ).toBeVisible();
      await expectQuality(page);
    });
  });
});
