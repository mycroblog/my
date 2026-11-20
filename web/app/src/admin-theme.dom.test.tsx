// SPDX-License-Identifier: AGPL-3.0-only

// The instance theme in the app (#3266, FR-605, ADR-0059), on the fake API. The preview of the draft: its stylesheet
// lies over the page in place of the published theme's, which the shell links under /theme/, so that a token the
// draft no longer overrides shows its default while the preview is on, on every screen of the tab, with a banner that
// leads back; and the page is as it was once the preview stops. The draft's form: every token, each color in light
// and dark with the verdicts of its pairs, the defaults as placeholders, and Save refused before sending for a pair
// under its minimum and a color half typed. Publishing and restoring, a theme the server refuses, and who reaches the
// screen. What each screen shows at rest is the stories' (admin-theme.stories.tsx); the flow on the real server is
// admin-theme.spec.ts's.

import { afterEach, expect, test } from "bun:test";
import { themeTokens } from "@my/design-tokens/theme";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
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

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

/** The app as hana, the scenario's administrator, at `at` on a server of `options`, once the screen is read. */
async function atTheme(at: string, options: FakeOptions = {}, on = fakeApp({ viewer: "hana", ...options })) {
  const app = await renderApp(at, { on });
  await main().findByRole("heading", { level: 1 });
  await settled(app);
  return app;
}

const button = (name: string) => main().getByRole("button", { name });
const section = (name: string) => main().getByRole("region", { name });
/** The field of the color `name` in `scheme` (`Accent text`, `Light`), as #rrggbb. */
const colorField = (name: string, scheme: "Light" | "Dark") =>
  main().getByRole("textbox", { name: `${name} ${scheme}` });
/** What describes `field`, each part's words: its hint or error, then each item of the list of its pairs. */
const describing = (field: HTMLElement) =>
  (field.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .flatMap((id) => {
      const part = document.getElementById(id);
      if (part === null) return [];
      const items = within(part).queryAllByRole("listitem");
      return items.length > 0 ? items.map(words) : [words(part)];
    })
    .filter((said) => said !== "");
/** What the screen's alerts say. */
const alerts = () => screen.queryAllByRole("alert").map(words);
/** The tokens a save sent, by name. */
const byName = <T extends { token: string }>(tokens: readonly T[]) =>
  [...tokens].sort((a, b) => a.token.localeCompare(b.token));

// The theme's screen.

test.each([
  ["no draft", {}, "No draft is saved. Start one from the theme in effect."],
  ["a draft not published", { theme: { draft: true } }, "StateNot published yet"],
  ["the draft published", { theme: { published: true } }, "StateSame as the published theme"],
] as const)("with %s the draft's section says where it stands", async (_, options, says) => {
  await atTheme("/admin/theme", options);

  expect(words(section("Draft"))).toContain(says);
});

test("an override an upgrade left without effect is listed, with the next save said to drop it", async () => {
  await atTheme("/admin/theme", { theme: { stale: true } });

  expect(words(section("Draft"))).toContain("No longer applies1 change: color-link. The next save drops it.");
});

// The draft's form.

test("the draft's form has a field for each of the theme's tokens, a color in light and in dark, the others with their defaults as placeholders", async () => {
  await atTheme("/admin/theme/draft");
  const tokens = themeTokens();
  const colors = tokens.filter((t) => t.kind === "color");
  const others = tokens.filter((t) => t.kind !== "color");
  expect(tokens).toHaveLength(31);

  expect(main().getAllByRole("textbox", { name: / Light$/ })).toHaveLength(colors.length);
  expect(main().getAllByRole("textbox", { name: / Dark$/ })).toHaveLength(colors.length);
  const fields = main()
    .getAllByRole("textbox")
    .filter((field) => field.getAttribute("name") !== null);
  expect(fields.map((field) => field.getAttribute("name")).sort()).toEqual(others.map((t) => t.name).sort());
  // Each length, number and duration waits empty over its default; the families have none to show.
  const corners = main().getByRole("textbox", { name: "Corners of controls" });
  expect(corners).toHaveProperty("value", "");
  expect(corners.getAttribute("placeholder")).toBe("0.375rem");
  expect(main().getByRole("textbox", { name: "Spinner turn" }).getAttribute("placeholder")).toBe("800ms");
});

test("each color in light and in dark says of each of its pairs whether it meets WCAG AA, and a color typed is judged as it is typed", async () => {
  const app = await atTheme("/admin/theme/draft");
  const fields = [
    ...main().getAllByRole("textbox", { name: / Light$/ }),
    ...main().getAllByRole("textbox", { name: / Dark$/ }),
  ];
  const verdicts = fields.flatMap(describing);
  expect(verdicts.length).toBeGreaterThan(fields.length);
  for (const verdict of verdicts) expect(verdict).toMatch(/^Aa(On|Under) .+Meets AA \d+(\.\d+)?:1$/);
  expect(describing(colorField("Accent text", "Light"))).toEqual(["AaOn Page backgroundMeets AA 5.85:1"]);

  await app.user.clear(colorField("Accent text", "Light"));
  await app.user.type(colorField("Accent text", "Light"), "#ff6b6b");

  await waitFor(() => {
    expect(describing(colorField("Accent text", "Light"))).toEqual([
      "AaOn Page backgroundBelow AA 2.77:1, needs 4.5:1",
    ]);
  });
  expect(colorField("Accent text", "Light").getAttribute("aria-invalid")).toBe("true");
});

test("a pair under its minimum refuses Save before sending, and takes the focus", async () => {
  const on = fakeApp({ viewer: "hana", theme: { low: true } });
  const saved = on.fake.script("PUT /admin/theme/draft");
  const app = await atTheme("/admin/theme/draft", {}, on);
  const accent = colorField("Accent text", "Light");
  expect(accent.getAttribute("aria-invalid")).toBe("true");

  await app.user.click(button("Save"));

  await waitFor(() => {
    expect(alerts()).toContain("Nothing was saved. Fix what is marked and save again.");
  });
  expect(app.focused()).toBe(accent);
  expect(saved.seen).toHaveLength(0);
});

test("a color half typed refuses Save before sending, says the form of a color under it, and takes the focus", async () => {
  const on = fakeApp({ viewer: "hana", theme: { draft: true } });
  const saved = on.fake.script("PUT /admin/theme/draft");
  const app = await atTheme("/admin/theme/draft", {}, on);
  const text = colorField("Text", "Light");

  await app.user.clear(text);
  await app.user.type(text, "#12");
  await app.user.click(button("Save"));

  await waitFor(() => {
    expect(text.getAttribute("aria-invalid")).toBe("true");
  });
  expect(describing(text)[0]).toBe("Enter a color as # and 6 characters from 0 to 9 and a to f, such as #ff6b6b.");
  expect(app.focused()).toBe(text);
  expect(saved.seen).toHaveLength(0);
});

test("the form opens on the draft, and a token emptied goes back to its default: the save sends the others alone", async () => {
  const on = fakeApp({ viewer: "hana", theme: { draft: true } });
  const saved = on.fake.script("PUT /admin/theme/draft");
  const app = await atTheme("/admin/theme/draft", {}, on);
  expect(colorField("Accent text", "Light")).toHaveProperty("value", "#8f3434");
  const corners = main().getByRole("textbox", { name: "Corners of controls" });
  expect(corners).toHaveProperty("value", "0.75rem");

  await app.user.clear(corners);
  await app.user.click(button("Save"));

  expect(byName((await saved.arrived()).body.tokens)).toEqual([
    { token: "color-accent-text", light: "#8f3434", dark: "#ff8a80" },
  ]);
});

test("a save of a draft with an override an upgrade left without effect sends it no more", async () => {
  const on = fakeApp({ viewer: "hana", theme: { stale: true } });
  const saved = on.fake.script("PUT /admin/theme/draft");
  const app = await atTheme("/admin/theme/draft", {}, on);

  await app.user.click(button("Save"));

  const tokens = (await saved.arrived()).body.tokens.map((t) => t.token);
  expect(tokens).not.toContain("color-link");
  expect(tokens.sort()).toEqual(["color-accent-text", "radius-control"]);
});

// The preview.

test("the preview lies over every screen of the tab, with a banner whose link leads back to the theme", async () => {
  const app = await atTheme("/admin/theme", { theme: { draft: true } });
  const notice = () => screen.queryAllByRole("region", { name: "Server and connection status" }).map(words).join(" ");

  await app.user.click(button("Preview"));
  await waitFor(() => {
    expect(adopted()).toContain("#8f3434");
  });
  expect(words(section("Draft"))).toContain("Every screen of this tab shows the draft until you press Preview again.");

  await app.navigate("/lists");
  expect(notice()).toContain("This tab shows the draft theme.");
  expect(adopted()).toContain("--color-accent-text: light-dark(#8f3434, #ff8a80)");
  await app.navigate("/admin/closure");
  expect(notice()).toContain("This tab shows the draft theme.");
  expect(adopted()).toContain("#8f3434");

  const back = within(screen.getByRole("region", { name: "Server and connection status" })).getByRole("link", {
    name: "Theme",
  });
  await app.user.click(back);
  await app.arrived("/admin/theme");
  await app.user.click(await main().findByRole("button", { name: "Preview" }));

  await waitFor(() => {
    expect(notice()).not.toContain("This tab shows the draft theme.");
  });
  expect(adopted()).toBe("");
});

// Publishing and restoring.

test("Publish publishes the draft, and the screen says it is the theme in effect", async () => {
  const on = fakeApp({ viewer: "hana", theme: { draft: true } });
  const published = on.fake.script("POST /admin/theme/publish");
  const app = await atTheme("/admin/theme", {}, on);

  await app.user.click(button("Publish"));

  await published.arrived();
  await waitFor(() => {
    expect(words(section("Draft"))).toContain("StateSame as the published theme");
  });
  expect(words(section("In effect"))).toContain("Changes2 from the default");
  expect(screen.getAllByRole("status").map(words).join(" ")).toMatch(/Published\. /);
});

test("Restore the previous theme puts the one published before in effect", async () => {
  const on = fakeApp({ viewer: "hana", theme: { published: true } });
  const restored = on.fake.script("POST /admin/theme/restore");
  const app = await atTheme("/admin/theme", {}, on);

  await app.user.click(button("Restore the previous theme"));

  await restored.arrived();
  await waitFor(() => {
    expect(words(section("In effect"))).toContain("ChangesNone: the default theme");
  });
  expect(words(section("In effect"))).toContain(
    "Restored. Visitors see it within a few minutes, and pages already open once they are reloaded.",
  );
  expect(words(section("Draft"))).toContain("StateNot published yet");
});

test("a draft the server refuses to publish is said in the draft's section, with a link to the draft", async () => {
  // The draft's Accent text is under its minimum: the fake refuses it as the server does (invalid-theme).
  const on = fakeApp({ viewer: "hana", theme: { low: true } });
  const published = on.fake.script("POST /admin/theme/publish");
  const app = await atTheme("/admin/theme", {}, on);

  await app.user.click(button("Publish"));

  await published.arrived();
  const said = await waitFor(() => {
    const status = within(section("Draft"))
      .getAllByRole("status")
      .find((s) => words(s) !== "");
    if (status === undefined) throw new Error("test: the draft's section says nothing");
    return status;
  });
  expect(words(said)).not.toBe("Edit the draft");
  expect(within(said).getByRole("link").getAttribute("href")).toBe("/admin/theme/draft");
  expect(words(section("Draft"))).toContain("StateNot published yet");
});

test("a theme the server refuses to restore is said in its section, with a link to the draft", async () => {
  // The theme published before no longer meets a rule (an upgrade changed a default its pairs are checked with).
  const refusal = {
    type: "/api/v1/problems/invalid-theme",
    title: "Invalid theme",
    status: 422,
    theme_errors: [
      {
        code: "low_contrast" as const,
        token: "color-accent-text",
        background: "color-page",
        scheme: "light" as const,
        ratio: 2.77,
        minimum: 4.5,
      },
    ],
  };
  const on = fakeApp({ viewer: "hana", theme: { published: true } });
  on.fake.script("POST /admin/theme/restore", { fail: refusal });
  const app = await atTheme("/admin/theme", {}, on);

  await app.user.click(button("Restore the previous theme"));

  const said = await waitFor(() => {
    const status = within(section("In effect"))
      .getAllByRole("status")
      .find((s) => words(s) !== "");
    if (status === undefined) throw new Error("test: the section in effect says nothing");
    return status;
  });
  expect(within(said).getByRole("link").getAttribute("href")).toBe("/admin/theme/draft");
  expect(words(section("In effect"))).toContain("Changes2 from the default");
});

// Who reaches it.

test("a reader without manage_settings is refused the theme's screen, and the administration does not list it", async () => {
  const on = fakeApp({ viewer: "hana" });
  on.fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({ ...account, permissions: account.permissions.filter((p) => p !== "manage_settings") }),
  });
  const app = await renderApp("/admin/theme", { on });
  await main().findByRole("alert");
  expect(main().queryAllByRole("heading", { name: "Theme" })).toHaveLength(0);

  await app.navigate("/admin");
  await main().findByRole("navigation", { name: "Administration screens" });
  expect(main().queryAllByRole("link", { name: /^Theme/ })).toHaveLength(0);
});
