// SPDX-License-Identifier: AGPL-3.0-only
// Takes the screenshots of the documentation site and the README from the static Storybook of the screens
// (storybook-static, built by `scripts/task.sh web:storybook-build`). `scripts/task.sh docs:screenshots` runs it in
// PLAYWRIGHT_IMAGE (scripts/docs-screenshots.sh), so that every run draws with the same browser and fonts. Each shot is
// the Showcase story of a screen (src/test/showcase.ts) at a desktop viewport, in the light and the dark theme,
// written as WebP to app/docs/src/assets/screenshots/<name>-<theme>.webp. Change the story, not this script, to change
// what a shot shows.
//
//   bun scripts/screenshots.ts [<name> ...]    every shot, or only the named ones (home, post, ...)
import { mkdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium, type Page } from "playwright";

/** A screenshot: the story it shows, and the name of its files. */
type Shot = { name: string; story: string };

const shots: Shot[] = [
  { name: "home", story: "screens-timeline-home--showcase" },
  { name: "post", story: "screens-timeline-post--showcase" },
  { name: "voice-room", story: "screens-voice-room--showcase" },
  { name: "supporters", story: "screens-about-supporters--showcase" },
  { name: "reports", story: "screens-moderation-reports--showcase" },
];

const themes = ["light", "dark"] as const;
const viewport = { width: 1280, height: 800 };
/** The quality of the WebP encoding, 0 to 1: the screens are flat colors and text, which stay sharp at 0.85. */
const quality = 0.85;

const site = fileURLToPath(new URL("../storybook-static", import.meta.url));
const out = fileURLToPath(new URL("../../docs/src/assets/screenshots/", import.meta.url));
const only = new Set(process.argv.slice(2));

const contentTypes: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript",
  ".mjs": "text/javascript",
  ".css": "text/css",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".webp": "image/webp",
  ".woff2": "font/woff2",
};

/** Serves a file of the static Storybook; MSW's Service Worker answers the API inside the page. */
function serve(request: IncomingMessage, response: ServerResponse): void {
  const path = normalize(join(site, decodeURIComponent(new URL(request.url ?? "/", "http://localhost").pathname)));
  if (!path.startsWith(site)) {
    response.writeHead(403).end();
    return;
  }
  try {
    const file = statSync(path).isDirectory() ? join(path, "index.html") : path;
    const body = readFileSync(file);
    response.writeHead(200, { "content-type": contentTypes[extname(file)] ?? "application/octet-stream" }).end(body);
  } catch {
    response.writeHead(404).end();
  }
}

/** Waits until Storybook has rendered the story and run its play function, and fails if the story failed. */
async function storyDone(page: Page): Promise<void> {
  const phase = await page.waitForFunction(
    () => {
      const preview = (window as { __STORYBOOK_PREVIEW__?: { currentRender?: { phase?: string } } })
        .__STORYBOOK_PREVIEW__;
      const current = preview?.currentRender?.phase;
      return current === "finished" || current === "errored" || current === "aborted" ? current : undefined;
    },
    undefined,
    { timeout: 60_000 },
  );
  const result = await phase.jsonValue();
  if (result !== "finished") throw new Error(`the story ended ${result}`);
}

/** Waits until the fonts and every picture in the viewport have loaded; the pictures below it may load lazily. */
async function picturesDrawn(page: Page): Promise<void> {
  await page.evaluate(() => document.fonts.ready);
  await page.waitForFunction(
    () =>
      [...document.images]
        .filter((image) => image.checkVisibility() && image.getBoundingClientRect().top < window.innerHeight)
        .every((image) => image.complete && image.naturalWidth > 0),
    undefined,
    { timeout: 30_000 },
  );
}

/** Encodes a PNG as WebP with the browser's own encoder, so that the image needs no other tool. */
async function toWebP(page: Page, png: Buffer): Promise<Buffer> {
  const dataURL = await page.evaluate(
    async ({ base64, quality }) => {
      const image = new Image();
      image.src = `data:image/png;base64,${base64}`;
      await image.decode();
      const canvas = document.createElement("canvas");
      canvas.width = image.naturalWidth;
      canvas.height = image.naturalHeight;
      canvas.getContext("2d")?.drawImage(image, 0, 0);
      return canvas.toDataURL("image/webp", quality);
    },
    { base64: png.toString("base64"), quality },
  );
  if (!dataURL.startsWith("data:image/webp;base64,")) throw new Error("the browser did not encode WebP");
  return Buffer.from(dataURL.slice(dataURL.indexOf(",") + 1), "base64");
}

try {
  statSync(join(site, "iframe.html"));
} catch {
  throw new Error(`no Storybook build in ${site}: run scripts/task.sh web:storybook-build`);
}

const server = createServer(serve);
await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
const address = server.address();
if (address === null || typeof address === "string") throw new Error("the Storybook server has no port");
const origin = `http://127.0.0.1:${address.port}/`;
mkdirSync(out, { recursive: true });
const browser = await chromium.launch();
try {
  const encoder = await browser.newPage();
  for (const shot of shots) {
    if (only.size > 0 && !only.has(shot.name)) continue;
    for (const theme of themes) {
      const page = await browser.newPage({ viewport, colorScheme: theme, reducedMotion: "reduce" });
      await page.goto(`${origin}iframe.html?id=${shot.story}&viewMode=story&globals=theme:${theme};locale:en`);
      await storyDone(page);
      await picturesDrawn(page);
      const png = await page.screenshot({ animations: "disabled", caret: "hide" });
      const path = join(out, `${shot.name}-${theme}.webp`);
      writeFileSync(path, await toWebP(encoder, png));
      console.log(`screenshots: ${path}`);
      await page.close();
    }
  }
} finally {
  await browser.close();
  server.close();
}
