// SPDX-License-Identifier: AGPL-3.0-only
// Addresses several segments deep, against a real `my serve` (e2e/server.ts). The build is relative to
// itself (Vite base "./"), but the server writes the shell's tags from the build manifest as root paths
// (internal/interface/web, ADR-0028) and the chunks import each other relative to their own URL, so the
// app loads at any depth. `vite preview` serves the build's own index.html instead, which the fixture
// rewrites (e2e/fixtures.ts); this spec checks that the real server needs no such help.
import { expect, test } from "./fixtures";
import { serverURL } from "./server-origin.ts";

test.use({ baseURL: serverURL });

/** Pages below the root: a hashtag, a post address of the federated URI shape, and an unknown deep path. */
const paths = ["/tags/cats", "/@alice/status/0192f0c4-7a8e-7000-8000-000000000001", "/a/b/c/d"] as const;

for (const path of paths) {
  test(`${path} loads the app with root-relative assets`, async ({ page }) => {
    /** Scripts and stylesheets of the build that did not load. */
    const failed: string[] = [];
    const isAsset = (type: string) => type === "script" || type === "stylesheet";
    page.on("response", (response) => {
      if (isAsset(response.request().resourceType()) && response.status() >= 400)
        failed.push(`${String(response.status())} ${response.url()}`);
    });
    page.on("requestfailed", (request) => {
      if (isAsset(request.resourceType())) failed.push(`failed ${request.url()}`);
    });

    const response = await page.goto(path);

    // The shell as the server sent it, before the app adds tags of its own: every asset is a root path.
    const html = (await response?.text()) ?? "";
    const urls = [...html.matchAll(/<(?:script|link)\b[^>]*\b(?:src|href)="([^"]+)"/g)].map((match) => match[1] ?? "");
    expect(urls.filter((url) => url.startsWith("/assets/")).length).toBeGreaterThan(0);
    for (const url of urls) expect(url, "a shell tag of `my serve`").toMatch(/^\/[^/]/);

    // The entry and the route's lazily loaded chunk both load: the page's heading renders.
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    expect(failed).toEqual([]);
  });
}
