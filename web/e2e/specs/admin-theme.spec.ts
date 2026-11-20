// SPDX-License-Identifier: AGPL-3.0-only

// The server's theme on the real server (#3266, FR-605): an administrator saves a draft of the theme, previews it on
// the app's own screens, publishes it, and restores the theme published before it. The theme is the whole server's,
// so this spec runs in the project of the specs that change the whole server, after every other spec and one test at
// a time (playwright.config.ts); it starts by publishing the default theme, so that there is one to restore, and ends
// with the default theme in effect again. What each screen shows in every state, a pair under its minimum refused
// before sending, and what the server refuses, are the stories' and the route tests' on the fake.

import { ThemeAdmin } from "./screens";
import { expect, test } from "./test";

/** The default of Accent text in light, and the darker one the draft takes. */
const accentText = { default: "#a64646", draft: "#7a2e2e" };

test("an administrator previews a draft of the theme on the app's screens, publishes it, and restores the one before", async ({
  page,
  signIn,
  fresh,
}) => {
  const admin = await fresh.person({ admin: true });
  await signIn(admin);
  const theme = new ThemeAdmin(page);

  try {
    // The default theme first, published, which the restore at the end comes back to.
    await theme.open();
    await theme.editDraft();
    await theme.color("Accent text", "Light").fill(accentText.default);
    await theme.saveDraft();
    await expect(theme.saved).toBeVisible();
    await theme.open();
    await theme.publish();
    await expect(theme.said("Published.")).toBeVisible();

    // A darker accent text, saved as the draft and previewed on the home timeline.
    await theme.editDraft();
    await theme.color("Accent text", "Light").fill(accentText.draft);
    await theme.saveDraft();
    await expect(theme.saved).toBeVisible();
    await theme.open();
    await theme.button("Preview").click();
    await theme.goHome();
    await expect(theme.previewNotice).toBeVisible();
    await expect.poll(() => theme.property("--color-accent-text")).toContain(accentText.draft);

    // Published, then the default theme restored.
    await theme.backToTheme();
    await theme.publish();
    await expect(theme.said("Published.")).toBeVisible();
    await expect(theme.previewNotice).toBeHidden();
    await theme.restore();
    await expect(theme.said("Restored.")).toBeVisible();
    await expect(page.getByRole("main").getByText("None: the default theme", { exact: true })).toBeVisible();
  } finally {
    await fresh.defaultTheme(admin);
  }
});
