// SPDX-License-Identifier: AGPL-3.0-only

// The preview of the draft theme in the app (#3266, ADR-0059), on the fake API: the draft's stylesheet lies over the
// page in place of the published theme's, which the shell links under /theme/, so that a token the draft no longer
// overrides shows its default while the preview is on; and the page is as it was once the preview stops.

import { afterEach, expect, test } from "bun:test";
import { waitFor } from "@testing-library/react";
import { main, renderApp } from "./test/app";
import { fakeApp } from "./test/server";

/** What the stylesheets the page adopted say, joined. */
const adopted = () =>
  document.adoptedStyleSheets.flatMap((sheet) => [...sheet.cssRules].map((rule) => rule.cssText)).join("\n");

afterEach(() => {
  for (const link of document.head.querySelectorAll('link[href^="/theme/"]')) link.remove();
});

test("the preview shows a token the draft resets at its default, the published theme's stylesheet turned off", async () => {
  // The published theme overrides Accent text; the draft saved since resets it.
  const on = fakeApp({ viewer: "hana", theme: { published: true } });
  // The shell's link of the published theme, which the DOM simulation does not load.
  const link = document.createElement("link");
  link.href = "/theme/published.css";
  document.head.append(link);
  link.rel = "stylesheet";
  const app = await renderApp("/admin/theme", { on });
  await app.data.api.PUT("/admin/theme/draft", { body: { tokens: [{ token: "radius-control", value: "1rem" }] } });

  await app.user.click(await main().findByRole("button", { name: "Preview" }));

  await waitFor(() => {
    if (!adopted().includes("--radius-control")) throw new Error("test: the draft's stylesheet is not adopted");
  });
  expect(adopted()).not.toContain("--color-accent-text");
  expect(link.disabled).toBe(true);

  await app.user.click(main().getByRole("button", { name: "Preview" }));

  await waitFor(() => {
    if (adopted() !== "") throw new Error("test: the draft's stylesheet is still adopted");
  });
  expect(link.disabled).toBe(false);
});
