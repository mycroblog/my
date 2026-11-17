// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline draws a post's text in the direction of the text itself, which the browser computes.

import { homeTimeline } from "./screens";
import { expect, person, test } from "./test";

test("a post in Arabic reads right to left, by the direction of its own text", async ({ page, signIn }) => {
  await signIn(person("yuki"));
  const text = homeTimeline(page).post(person("layla").displayName, "صباح الخير").paragraph("صباح الخير");
  await expect(text).toHaveAttribute("dir", "auto");
  await expect(text).toHaveCSS("direction", "rtl");
});
