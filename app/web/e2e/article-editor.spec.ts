// SPDX-License-Identifier: AGPL-3.0-only
// The article editor (web-client/screens/article-editor.md) against a real `my serve` on SQLite (e2e/server.ts), in
// every width project (the spec is in `widthSpecs`): a user writes an article with a heading and bold text, drops an
// image into the body, sees it in the server's preview, and publishes the autosaved draft; the published article's
// body is the HTML the preview showed. A second test saves the draft from "another device" (the client API) meanwhile:
// the editor overwrites nothing, keeps the text, and saves it as a new draft. The editor is checked with axe and for
// horizontal scrolling. Every user is new to the test (a random name), so the projects never meet each other's drafts.
import { randomBytes } from "node:crypto";
import { crc32, deflateSync } from "node:zlib";
import type { Locator, Page } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { ownsServer, runMy } from "./server-cli.ts";
import { serverURL } from "./server-origin.ts";
import { readCatalog } from "./catalog.ts";

test.use({ baseURL: serverURL });

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

const pageHeading = (page: Page) => page.getByRole("main").getByRole("heading", { level: 1 });

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
    Buffer.from(Array.from({ length: width }, () => [0xc0, 0x40, 0x30]).flat()),
  ]);
  const pixels = Buffer.concat(Array.from({ length: height }, () => row));
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", header),
    chunk("IDAT", deflateSync(pixels)),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

/** The Authorization header of the app's next request to the client API. */
function nextAuthorization(page: Page): Promise<string> {
  return page
    .waitForRequest(
      (request) =>
        new URL(request.url()).pathname.startsWith("/api/") &&
        request.headers()["authorization"]?.startsWith("Bearer ") === true,
    )
    .then((request) => request.headers()["authorization"] ?? "");
}

/**
 * Creates a new user with `my admin create`, signs in through the server's sign-in screen, and sets the display
 * language to the project's locale.
 */
async function signInNewUser(page: Page): Promise<string> {
  const username = `e2e_article_${randomBytes(4).toString("hex")}`;
  const email = `${username}@example.com`;
  const password = `e2e-password-${username}`;
  runMy(["admin", "create", "-username", username, "-email", email], `${password}\n`);
  const authorized = nextAuthorization(page);
  await page.goto("/login");
  await page.locator('input[type="email"][autocomplete="username"]').fill(email);
  await page.locator('input[type="password"][autocomplete="current-password"]').fill(password);
  await page.locator('main button[type="submit"]').first().click();
  const authorization = await authorized;
  await expect(page).toHaveURL(`${serverURL}/home`);
  const locale = test.info().project.use.locale ?? "en";
  const response = await page.request.patch(`${serverURL}/api/v1/preferences`, {
    headers: { Authorization: authorization },
    data: { locale },
  });
  expect(response.ok(), `PATCH /api/v1/preferences: ${String(response.status())}`).toBe(true);
  return username;
}

/** Opens a new article in the editor. */
async function openEditor(page: Page): Promise<void> {
  await page.goto("/articles/new");
  await expect(page.locator("html")).toHaveAttribute("lang", test.info().project.use.locale ?? "en");
  await expect(pageHeading(page)).toHaveText(message("articles.editor.title"));
}

/**
 * The Authorization header of the editor's first save of the draft (POST /api/v1/drafts), for the client API calls of
 * "another device": the app takes a new access token each time it loads, and the page before may still send the old one.
 */
function firstSaveAuthorization(page: Page): Promise<string> {
  return page
    .waitForRequest((request) => request.method() === "POST" && new URL(request.url()).pathname === "/api/v1/drafts")
    .then((request) => request.headers()["authorization"] ?? "");
}

/** Drops a PNG file on `target` as a file manager would. */
async function dropImage(page: Page, target: Locator): Promise<void> {
  const bytes = png(320, 180).toString("base64");
  const dataTransfer = await page.evaluateHandle((base64) => {
    const transfer = new DataTransfer();
    const data = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
    transfer.items.add(new File([data], "tomato.png", { type: "image/png" }));
    return transfer;
  }, bytes);
  await target.dispatchEvent("drop", { dataTransfer });
}

test.beforeEach(() => {
  test.skip(
    !ownsServer,
    "the test creates its user with bin/my, which needs the database of the server e2e/server.ts starts",
  );
  test.setTimeout(120_000);
});

test("an article written with an image in its body is published as the preview showed it", async ({
  page,
  makeAxeBuilder,
}) => {
  const username = await signInNewUser(page);
  await openEditor(page);
  const main = page.getByRole("main");
  const form = main.getByRole("form", { name: message("articles.editor.title") });
  const preview = main.getByRole("region", { name: message("articles.preview.heading") });
  const body = form.getByRole("textbox", { name: message("articles.field.body"), exact: true });

  const title = `Tomatoes on a balcony ${username}`;
  await form.getByRole("textbox", { name: message("articles.field.title"), exact: true }).fill(title);
  await body.fill("# Soil\n\nUse a **deep pot**, and mix compost into the soil.");
  await expect(preview.getByRole("heading", { level: 2, name: "Soil" })).toBeVisible();
  await expect(preview.locator("strong", { hasText: "deep pot" })).toBeVisible();

  // An image dropped into the body uploads, goes in at the caret, and shows in the preview once processed.
  await body.focus();
  await body.press("ControlOrMeta+End");
  await dropImage(page, body);
  await expect(body).toHaveValue(/!\[\]\(media:[^)]+\)$/u);
  const described = (await body.inputValue()).replace("![](", "![A red square](");
  await body.fill(described);
  await expect(preview.getByRole("img", { name: "A red square" })).toBeVisible();

  await expect(form.getByText(message("articles.autosave.saved"), { exact: true })).toBeVisible();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
  // The body is the innermost element of its direction that holds its heading.
  const previewBody = preview
    .locator('div[dir="auto"]')
    .filter({ has: page.getByRole("heading", { name: "Soil" }) })
    .last();
  const previewed = await previewBody.innerHTML();

  await form.getByRole("button", { name: message("articles.publish"), exact: true }).click();

  await expect(page).toHaveURL(new RegExp(`/@${username}/status/[^/]+$`, "u"));
  await expect(pageHeading(page)).toHaveText(title);
  const published = main
    .locator('div[dir="auto"]')
    .filter({ has: page.getByRole("heading", { name: "Soil" }) })
    .last();
  await expect(published.getByRole("img", { name: "A red square" })).toBeVisible();
  expect(await published.innerHTML()).toBe(previewed);
});

test("a draft saved on another device meanwhile is not overwritten, and the text is saved as a new draft", async ({
  page,
  makeAxeBuilder,
}) => {
  const username = await signInNewUser(page);
  await openEditor(page);
  const form = page.getByRole("main").getByRole("form", { name: message("articles.editor.title") });
  const body = form.getByRole("textbox", { name: message("articles.field.body"), exact: true });
  const firstSave = firstSaveAuthorization(page);
  await form.getByRole("textbox", { name: message("articles.field.title"), exact: true }).fill(`Draft ${username}`);
  await body.fill("Written on this device.");
  const authorization = await firstSave;
  await expect(form.getByText(message("articles.autosave.saved"), { exact: true })).toBeVisible();

  // Another device saves the draft over the version it read.
  const headers = { Authorization: authorization };
  const list = await page.request.get(`${serverURL}/api/v1/drafts`, { headers });
  expect(list.ok()).toBe(true);
  const [draft] = ((await list.json()) as { items: { id: string }[] }).items;
  if (draft === undefined) throw new Error("the editor saved no draft");
  const read = await page.request.get(`${serverURL}/api/v1/drafts/${draft.id}`, { headers });
  const saved = await page.request.put(`${serverURL}/api/v1/drafts/${draft.id}`, {
    headers: { ...headers, "If-Match": read.headers()["etag"] ?? "" },
    data: {
      text: "Written on another device.",
      visibility: "public",
      article: { title: `Draft ${username}` },
    },
  });
  expect(saved.ok(), `PUT /api/v1/drafts/{id}: ${String(saved.status())}`).toBe(true);

  await body.fill("Written on this device, and more.");
  const conflict = form.getByRole("alert").filter({ hasText: message("articles.autosave.conflict") });
  await expect(conflict).toBeVisible();
  await expect(body).toHaveValue("Written on this device, and more.");
  await expectNoAxeViolations(makeAxeBuilder);

  await conflict.getByRole("button", { name: message("articles.autosave.save_new") }).click();
  await expect(form.getByText(message("articles.autosave.saved"), { exact: true })).toBeVisible();
  const drafts = await page.request.get(`${serverURL}/api/v1/drafts`, { headers });
  const texts = ((await drafts.json()) as { items: { text: string }[] }).items.map((d) => d.text).toSorted();
  expect(texts).toEqual(["Written on another device.", "Written on this device, and more."]);
});
