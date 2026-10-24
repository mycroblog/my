// SPDX-License-Identifier: AGPL-3.0-only
// The settings pages in every width × locale project (ADR-0027): the display language switches the whole app
// and <html lang dir> at once, form errors are tied to their fields, and the pages pass axe without
// scrolling sideways. `vite preview` has no API, so each test plays a signed-in user's server with
// page.route; names change with the locale, so elements are found by role, structure, autocomplete tokens and,
// where a role is not unique, by their names in the project's catalog.
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, localeDir, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

/** The signed-in user's account, as GET /accounts/verify_credentials answers. */
const viewer = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
  discoverable: true,
  indexable: true,
  ai_opt_out: false,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: [],
};

/** The role whose badge the profile settings show and hide (ADR-0071). */
const supporterBadgeId = "0192f0c4-7a8e-7000-8000-0000000000b1";

/** Another account, as the lists of the settings show it. */
const bob = { ...viewer, id: "0192f0c4-7a8e-7000-8000-000000000002", username: "bob", acct: "bob@remote.example", display_name: "Bob" };

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: "application/json", headers: { "Cache-Control": "no-store" }, body: JSON.stringify(body) });
}

/**
 * What the played server is like beyond the defaults: the viewer's permissions, whether it sends email, and
 * whether the viewer has chosen their time zone (UTC) yet.
 */
type ServerOptions = { permissions?: string[]; sendsEmail?: boolean; timeZoneChosen?: boolean };

/**
 * Plays the server of a signed-in user whose preferences start in `locale`: the refresh endpoint resumes the
 * session, and the preferences remember their changes (across reloads too). Returns the changes the app sent.
 */
async function signedInServer(page: Page, locale: string, { permissions = [], sendsEmail = true, timeZoneChosen = true }: ServerOptions = {}) {
  let preferences = {
    locale,
    time_zone: "UTC",
    time_zone_chosen: timeZoneChosen,
    default_visibility: "public",
    default_post_language: "und",
    require_media_description: false,
    email_moderator_notifications: false,
    timeline_auto_insert: false,
    timeline_languages: [] as string[],
  };
  let autoDeletePolicy = {
    enabled: false,
    min_age_days: 30,
    keep_pinned: true,
    keep_bookmarked: true,
    keep_favourited: true,
    keep_media: false,
    keep_polls: false,
    keep_visibilities: ["direct"],
    keep_min_boosts: 0,
    keep_min_favourites: 0,
  };
  // The badges of the viewer's roles (ADR-0071): a long name, and one in a right-to-left script.
  let badgeChoices = [
    { role_id: supporterBadgeId, label: "Supporter of the medium plan", color: "green", displayed: false },
    { role_id: "0192f0c4-7a8e-7000-8000-0000000000b2", label: "داعم", color: "pink", displayed: true },
  ];
  // The user's own OAuth apps (ADR-0090): one registered already, with a long name.
  const oauthApps: Record<string, unknown>[] = [
    {
      id: "0192f0c4-7a8e-7000-8000-000000000bb1",
      client_id: "my_ci_a-very-long-client-identifier-for-the-narrowest-screens",
      name: "A bot with a very long name for the narrowest phone screens",
      redirect_uris: ["http://127.0.0.1/callback"],
      scope: "read write",
      client_type: "confidential",
      disabled: false,
      created_at: "2026-10-16T09:30:00Z",
    },
  ];
  const changes: unknown[] = [];
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow" }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
    if (request.headers()["authorization"] !== "Bearer e2e-token" && path !== "/instance") return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    switch (`${request.method()} ${path}`) {
      case "GET /instance":
        return json(route, { domain: "my.example", name: "my example", version: "0.0.0", source_url: "https://github.com/mycroblog/my", registrations: { mode: "open" }, federation: { mode: "both" }, limits: { max_post_characters: 500, require_media_description: false }, timelines: { local: "everyone", federated: "everyone" }, sends_email: sendsEmail, rules: [], search_indexing: "deny", ai_usage: "deny" });
      case "GET /accounts/verify_credentials":
        return json(route, { ...viewer, permissions });
      case "GET /preferences":
        return json(route, preferences);
      case "PATCH /preferences": {
        const change = request.postDataJSON() as Partial<typeof preferences>;
        changes.push(change);
        preferences = { ...preferences, ...change, time_zone_chosen: preferences.time_zone_chosen || change.time_zone !== undefined };
        return json(route, preferences);
      }
      case "GET /blocks":
        return json(route, { accounts: [bob] });
      case "GET /mutes":
        return json(route, { mutes: [{ account: bob, notifications: true }] });
      case "GET /domain_mutes":
        // A long domain, and one in IDNA A-labels that the page shows with its U-labels too.
        return json(route, {
          domain_mutes: [
            { domain: "a-very-long-subdomain-name-for-the-narrowest-screens.spam.example", created_at: "2026-10-17T10:00:00Z" },
            { domain: "xn--r8jz45g.xn--zckzah", created_at: "2026-10-18T10:00:00Z" },
          ],
        });
      case "GET /follow_requests":
        return json(route, { accounts: [bob] });
      case "GET /exports":
        return json(route, {
          items: [
            { id: "0192f0c4-7a8e-7000-8000-0000000008e2", state: "pending", requested_at: "2026-10-17T10:00:00Z" },
            {
              id: "0192f0c4-7a8e-7000-8000-0000000008e1",
              state: "ready",
              requested_at: "2026-10-16T09:30:00Z",
              completed_at: "2026-10-16T09:35:00Z",
              expires_at: "2026-10-23T09:35:00Z",
              file_size: 2_500_000,
            },
          ],
        });
      case "GET /imports":
        return json(route, {
          items: [
            {
              id: "0192f0c4-7a8e-7000-8000-0000000009a1",
              kind: "following",
              mode: "overwrite",
              state: "in_progress",
              created_at: "2026-10-21T09:30:00Z",
              total: 1200,
              processed: 340,
              failed: 2,
            },
          ],
        });
      case "POST /imports/preview":
        return json(route, {
          entries: 1200,
          present: 30,
          removals: 4,
          skipped: [{ line: 7, value: "a-very-long-line-that-is-not-an-address-and-goes-on-and-on@", reason: "invalid" }],
        });
      case "GET /preferences/badges":
        return json(route, { items: badgeChoices });
      case `PUT /preferences/badges/${supporterBadgeId}`: {
        const { displayed } = request.postDataJSON() as { displayed: boolean };
        changes.push({ badge: displayed });
        badgeChoices = badgeChoices.map((c) => (c.role_id === supporterBadgeId ? { ...c, displayed } : c));
        return json(route, badgeChoices.find((c) => c.role_id === supporterBadgeId));
      }
      case "GET /preferences/auto_delete":
        return json(route, autoDeletePolicy);
      case "PUT /preferences/auto_delete":
        autoDeletePolicy = request.postDataJSON() as typeof autoDeletePolicy;
        return json(route, autoDeletePolicy);
      case "GET /two_factor":
        return json(route, { enabled: false });
      case "GET /accounts/password":
        return json(route, { has_password: true });
      case "GET /passkeys":
        return json(route, {
          items: [{ id: "0192f0c4-7a8e-7000-8000-000000000ab1", name: "A password manager with a long name on a narrow phone", created_at: "2026-10-16T09:30:00Z" }],
        });
      case "GET /oauth_apps":
        return json(route, { items: oauthApps });
      case "POST /oauth_apps": {
        const body = request.postDataJSON() as { name: string; redirect_uris: string[]; scope: string; client_type: string };
        const app = { id: "0192f0c4-7a8e-7000-8000-000000000bb2", client_id: "my_ci_registered", ...body, disabled: false, created_at: "2026-10-21T09:30:00Z" };
        oauthApps.push(app);
        return json(route, { app, client_secret: "my_cs_e2e-secret-shown-once" }, 201);
      }
      case "POST /two_factor/totp/setup":
        return json(route, totpSetup);
      default:
        return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
    }
  });
  return changes;
}

// The browser is in the time zone of the played preferences (UTC), unless a test says otherwise.
test.use({ timezoneId: "UTC" });

/** The project's locale. */
function projectLocale(): string {
  return test.info().project.use.locale ?? "en";
}

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const catalog = readCatalog(projectLocale());
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${projectLocale()}`);
  return text;
}

/** The switch that also sends the moderator notifications by email. */
function moderatorEmailSwitch(page: Page) {
  return page.getByRole("main").getByRole("switch", { name: message("settings.moderator_notifications.email") });
}

/** The button that switches the time zone to the browser's, shown only when they differ. */
function useDeviceButton(page: Page) {
  return page.getByRole("main").getByRole("button", { name: message("settings.time_zone.use_device"), exact: true });
}

const pages = [
  "/settings",
  "/settings/profile",
  "/settings/preferences",
  "/settings/follow-requests",
  "/settings/blocks",
  "/settings/mutes",
  "/settings/domain-mutes",
  "/settings/password",
  "/settings/two-factor",
  "/settings/passkeys",
  "/settings/apps",
  "/settings/auto-delete",
  "/settings/account",
  "/settings/import-export",
] as const;

// Where the column is too narrow for the menu beside the page, the settings page still holds its own content under
// the menu, and a section's page leads back to it.
test("the settings page keeps signing out under the menu, and a section leads back to it", async ({ page }) => {
  await signedInServer(page, projectLocale());
  await page.goto("/settings");
  const main = page.getByRole("main");
  await expect(main.getByRole("navigation").getByRole("link").first()).toBeVisible();
  await expect(main.getByRole("button", { name: message("session.sign_out"), exact: true })).toBeVisible();
  await page.goto("/settings/profile");
  const back = main.getByRole("link", { name: message("settings.back"), exact: true });
  // The way back is shown only while the menu is not beside the page.
  if ((page.viewportSize()?.width ?? 0) < 768) {
    await expect(back).toBeVisible();
    await back.click();
    await expect(page).toHaveURL(/\/settings$/);
    await expect(main.getByRole("button", { name: message("session.sign_out"), exact: true })).toBeVisible();
  } else {
    await expect(back).toBeHidden();
  }
});

/** A TOTP setup whose QR code is the three finder patterns of a 21-module code with its quiet zone. */
const totpSetup = {
  secret: "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP",
  uri: "otpauth://totp/my.example:alice@my.example?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP",
  qr_code: { size: 29, path: "M4 4h7v7h-7zM18 4h7v7h-7zM4 18h7v7h-7z" },
  setup_token: "e2e-setup",
  expires_at: "2026-10-20T00:15:00Z",
};

test("two-factor setup shows the QR code and the key without axe violations or horizontal scroll", async ({ page, makeAxeBuilder }) => {
  await signedInServer(page, projectLocale());
  await page.goto("/settings/two-factor");
  await page.getByRole("main").getByRole("button", { name: message("settings.two_factor.setup.start"), exact: true }).click();
  await expect(page.getByRole("main").getByRole("img")).toBeVisible();
  await expect(page.getByRole("main").getByRole("textbox")).toBeVisible();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});

test("registering an app shows its secret once without axe violations or horizontal scroll", async ({ page, makeAxeBuilder }) => {
  await signedInServer(page, projectLocale());
  await page.goto("/settings/apps");
  const main = page.getByRole("main");
  const form = main.getByRole("region", { name: message("settings.apps.new.title") });
  await form.getByRole("textbox", { name: message("settings.apps.name") }).first().fill("E2E bot");
  await form.getByRole("textbox", { name: message("settings.apps.redirect_uris") }).fill("http://127.0.0.1/callback");
  // By keyboard: the radio's input is visually hidden under its drawn circle.
  const confidential = form.getByRole("radio", { name: message("settings.apps.client_type.confidential") });
  await confidential.focus();
  await page.keyboard.press("Space");
  await expect(confidential).toBeChecked();
  await form.getByRole("button", { name: message("settings.apps.register"), exact: true }).click();
  const notice = main.getByRole("region", { name: message("settings.apps.secret.title").replace("{name}", "E2E bot") });
  await expect(notice.getByText("my_cs_e2e-secret-shown-once")).toBeVisible();
  await expect(notice).toBeFocused();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
  // The secret is not kept: a new visit does not show it again.
  await page.reload();
  await expect(main.getByRole("region", { name: message("settings.apps.list.title") })).toBeVisible();
  await expect(page.getByText("my_cs_e2e-secret-shown-once")).toHaveCount(0);
});

test("checking a file to import shows what it does without axe violations or horizontal scroll", async ({ page, makeAxeBuilder }) => {
  await signedInServer(page, projectLocale());
  await page.goto("/settings/import-export");
  const main = page.getByRole("main");
  // By keyboard: the radio's input is visually hidden under its drawn circle.
  const overwrite = main.getByRole("radio", { name: message("settings.import_export.import.mode.overwrite") });
  await overwrite.focus();
  await page.keyboard.press("Space");
  await expect(overwrite).toBeChecked();
  // FileTrigger's input is hidden; Playwright sets its files as the file picker would.
  await main.locator('input[type="file"]').setInputFiles({ name: "following_accounts.csv", mimeType: "text/csv", buffer: Buffer.from("bob@remote.example\n") });
  await main.getByRole("button", { name: message("settings.import_export.import.preview"), exact: true }).click();
  const preview = main.getByRole("region", { name: message("settings.import_export.import.preview.title") });
  await expect(preview.getByRole("button", { name: message("settings.import_export.import.start"), exact: true })).toBeVisible();
  await expect(preview).toMatchAriaSnapshot(`
    - region:
      - heading [level=3]
      - term
      - definition
      - term
      - definition
      - term
      - definition
      - heading [level=4]
      - list:
        - listitem
      - button
  `);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});

test("the automatic deletion of old posts is turned on and saved without axe violations or horizontal scroll", async ({ page, makeAxeBuilder }) => {
  await signedInServer(page, projectLocale());
  await page.goto("/settings/auto-delete");
  const main = page.getByRole("main");
  // By keyboard: the inputs of the switch and the checkbox are visually hidden under what they draw.
  const enabled = main.getByRole("switch", { name: message("settings.auto_delete.enabled") });
  await enabled.focus();
  await page.keyboard.press("Space");
  await expect(enabled).toBeChecked();
  const keepMedia = main.getByRole("checkbox", { name: message("settings.auto_delete.keep.media") });
  await keepMedia.focus();
  await page.keyboard.press("Space");
  await expect(keepMedia).toBeChecked();
  await main.getByRole("button", { name: message("settings.auto_delete.save"), exact: true }).click();
  // The status names the age in the plural form of the locale; its words before the number are enough.
  await expect(main.getByRole("status").getByText(message("settings.auto_delete.saved_on").replace(/\{.*$/s, "").trim())).toBeVisible();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});

for (const path of pages) {
  test.describe(`page ${path}`, () => {
    test.beforeEach(async ({ page }) => {
      await signedInServer(page, projectLocale());
      await page.goto(path);
      await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    });

    test("has no axe violations", async ({ makeAxeBuilder }) => {
      await expectNoAxeViolations(makeAxeBuilder);
    });

    test("does not scroll horizontally", async ({ page }) => {
      await expectNoHorizontalScroll(page);
    });
  });
}

test.describe("display language", () => {
  test("switches the app and <html lang dir> at once, and is saved", async ({ page }) => {
    const locale = projectLocale();
    const changes = await signedInServer(page, locale);
    await page.goto("/settings/preferences");
    const heading = page.getByRole("main").getByRole("heading", { level: 1 });
    await expect(heading).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("lang", locale);
    await expect(page.locator("html")).toHaveAttribute("dir", localeDir(locale));
    const before = await heading.textContent();

    // The form: the display language and time zone selects, the theme radio group, the autoplay switch, the
    // visibility radio group, the default post language select, the switch that asks for descriptions, and the
    // languages group.
    await expect(page.getByRole("main")).toMatchAriaSnapshot(`
      - heading [level=1]
      - link
      - paragraph
      - button
      - button
      - radiogroup:
        - radio [checked]
        - radio
        - radio
      - switch
      - radiogroup:
        - radio [checked]
        - radio
        - radio
        - radio
        - radio
      - button
      - switch
      - group:
        - list
    `);
    // A viewer without moderator permissions has no moderator notifications to switch.
    await expect(moderatorEmailSwitch(page)).toHaveCount(0);

    // The names of the display languages are in their own language, whatever the page's locale.
    const target = locale === "ja" ? { locale: "en", name: "English" } : { locale: "ja", name: "日本語" };
    const select = page.locator('main button[aria-haspopup="listbox"]').first();
    await select.click();
    await page.getByRole("option", { name: target.name }).click();

    await expect(page.locator("html")).toHaveAttribute("lang", target.locale);
    await expect(page.locator("html")).toHaveAttribute("dir", "ltr");
    await expect(heading).not.toHaveText(before ?? "");
    await expect(select).toContainText(target.name);
    expect(changes).toEqual([{ locale: target.locale }]);
  });
});

test.describe("theme", () => {
  test("switches at once, passes axe in both themes, and stays after a reload without reaching the server", async ({ page, makeAxeBuilder }) => {
    await page.emulateMedia({ colorScheme: "light" });
    const changes = await signedInServer(page, projectLocale());
    await page.goto("/settings/preferences");
    // The theme: the OS setting (checked), light and dark.
    const theme = page.getByRole("main").getByRole("radiogroup", { name: message("settings.theme.label"), exact: true });
    await expect(theme.getByRole("radio").first()).toBeChecked();
    const html = page.locator("html");
    await expect(html).not.toHaveAttribute("data-theme");

    // The APG radio group: the arrow keys move and check (Up is the previous one in both directions, and wraps).
    await theme.getByRole("radio").first().focus();
    await page.keyboard.press("ArrowUp");
    await expect(theme.getByRole("radio").nth(2)).toBeChecked();
    await expect(html).toHaveAttribute("data-theme", "dark");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await page.keyboard.press("ArrowUp");
    await expect(html).toHaveAttribute("data-theme", "light");
    await expectNoAxeViolations(makeAxeBuilder);

    await page.reload();
    await expect(theme.getByRole("radio").nth(1)).toBeChecked();
    await expect(html).toHaveAttribute("data-theme", "light");
    expect(changes).toEqual([]);
  });
});

test.describe("navigation", () => {
  test("is arranged from the keyboard alone, passes axe, and stays after a reload", async ({ page, makeAxeBuilder }) => {
    await signedInServer(page, projectLocale());
    await page.goto("/settings/preferences");
    const section = page.getByRole("main").getByRole("region", { name: message("settings.navigation.title") });
    const name = (item: string) => message(`shell.nav.${item}`);
    const moveUp = (item: string) => section.getByRole("button", { name: message("settings.navigation.move_up").replace("{name}", name(item)), exact: true });
    /** The addresses of the navigation's links in their order, the ones the width hides included. */
    const order = () => page.getByRole("navigation").getByRole("list").locator("a").evaluateAll((links) => links.map((link) => link.getAttribute("href")));
    await expect(section.getByRole("checkbox")).toHaveCount(8);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Enter on Move up moves Search before Notifications, and the focus stays on the moved item's button.
    await moveUp("search").focus();
    await page.keyboard.press("Enter");
    await expect(moveUp("search")).toBeFocused();
    await expect.poll(order).toEqual(["/home", "/local", "/public", "/search", "/notifications", "/profile", "/bookmarks", "/settings"]);

    // Space on a checkbox hides Profile.
    await section.getByRole("checkbox", { name: name("profile"), exact: true }).focus();
    await page.keyboard.press("Space");
    await expect(section.getByRole("checkbox", { name: name("profile"), exact: true })).not.toBeChecked();
    await expect.poll(order).toEqual(["/home", "/local", "/public", "/search", "/notifications", "/bookmarks", "/settings"]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await page.reload();
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    await expect.poll(order).toEqual(["/home", "/local", "/public", "/search", "/notifications", "/bookmarks", "/settings"]);
    await expect(section.getByRole("checkbox", { name: name("profile"), exact: true })).not.toBeChecked();
  });
});

test.describe("profile form", () => {
  test("ties a validation error to its field and moves focus to it", async ({ page, makeAxeBuilder }) => {
    await signedInServer(page, projectLocale());
    await page.goto("/settings/profile");
    const displayName = page.locator('input[autocomplete="nickname"]');
    await expect(displayName).toBeVisible();

    await displayName.fill("a".repeat(31));
    await page.locator('main button[type="submit"]').click();

    await expect(displayName).toBeFocused();
    await expect(displayName).toHaveAttribute("aria-invalid", "true");
    const describedBy = (await displayName.getAttribute("aria-describedby")) ?? "";
    const descriptions = page.locator(describedBy.split(" ").map((id) => `[id="${id}"]`).join(", "));
    // The description and the error message.
    await expect(descriptions).toHaveCount(2);
    await expect(descriptions.last()).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("asks before leaving with unsaved changes", async ({ page, makeAxeBuilder }) => {
    await signedInServer(page, projectLocale());
    await page.goto("/settings/profile");
    const displayName = page.locator('input[autocomplete="nickname"]');
    await expect(displayName).toBeVisible();
    await displayName.fill("Alice Liddell");
    // Phones reach the settings from the menu's sheet (ADR-0039), which stays open while the question is asked.
    const nav = page.getByRole("navigation");
    let settingsLink = nav.locator('a[href="/settings"]').filter({ visible: true });
    if ((await settingsLink.count()) === 0) {
      await nav.getByRole("button").click();
      settingsLink = page.getByRole("dialog").locator('a[href="/settings"]');
    }

    await settingsLink.click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    // The question, what leaving does, and "Keep editing" (focused) before "Discard changes".
    await expect(dialog).toMatchAriaSnapshot(`
      - alertdialog:
        - heading [level=2]
        - paragraph
        - button
        - button
    `);
    await expect(dialog.getByRole("button").first()).toBeFocused();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(page).toHaveURL(/\/settings\/profile$/);
    await expect(displayName).toHaveValue("Alice Liddell");

    await settingsLink.click();
    await dialog.getByRole("button").last().click();
    await expect(page).toHaveURL(/\/settings$/);
  });
});

test.describe("post defaults", () => {
  test("the default post language and the request for descriptions are saved from the keyboard", async ({ page, makeAxeBuilder }) => {
    const changes = await signedInServer(page, projectLocale());
    await page.goto("/settings/preferences");
    const language = page.getByRole("main").getByRole("button", { name: message("settings.default_post_language.label") });
    await expect(language).toContainText(message("compose.language.und"));
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // A select opens on Enter (React Aria Select, the APG listbox pattern); the languages are named in the display language.
    const japanese = new Intl.DisplayNames([projectLocale()], { type: "language" }).of("ja") ?? "ja";
    await language.focus();
    await page.keyboard.press("Enter");
    await page.getByRole("option", { name: japanese, exact: true }).click();
    await expect(language).toContainText(japanese);

    // Space toggles a switch (WAI-ARIA APG, Switch pattern).
    const ask = page.getByRole("main").getByRole("switch", { name: message("settings.require_media_description.label") });
    await expect(ask).not.toBeChecked();
    await ask.focus();
    await page.keyboard.press("Space");
    await expect(ask).toBeChecked();
    await expect.poll(() => changes).toEqual([{ default_post_language: "ja" }, { require_media_description: true }]);

    await page.reload();
    await expect(page.getByRole("main").getByRole("button", { name: message("settings.default_post_language.label") })).toContainText(japanese);
    await expect(page.getByRole("main").getByRole("switch", { name: message("settings.require_media_description.label") })).toBeChecked();
  });
});

test.describe("moderator notifications by email", () => {
  test("a moderator switches them on, and they stay on after a reload", async ({ page, makeAxeBuilder }) => {
    const changes = await signedInServer(page, projectLocale(), { permissions: ["manage_reports"] });
    await page.goto("/settings/preferences");
    const toggle = moderatorEmailSwitch(page);
    await expect(toggle).not.toBeChecked();
    // The section of the moderator notifications follows the languages group, under its own heading.
    await expect(page.getByRole("main")).toMatchAriaSnapshot(`
      - group:
        - list
      - region:
        - heading [level=2]
        - switch
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Space toggles a switch (WAI-ARIA APG, Switch pattern); the input under the label takes the focus.
    await toggle.focus();
    await page.keyboard.press("Space");
    await expect(toggle).toBeChecked();
    await expect.poll(() => changes).toEqual([{ email_moderator_notifications: true }]);

    await page.reload();
    await expect(moderatorEmailSwitch(page)).toBeChecked();
  });

  test("says why there is no switch when the server sends no email", async ({ page, makeAxeBuilder }) => {
    await signedInServer(page, projectLocale(), { permissions: ["manage_accounts"], sendsEmail: false });
    await page.goto("/settings/preferences");
    const section = page.getByRole("main").getByRole("region", { name: message("settings.moderator_notifications.title"), exact: true });
    await expect(section).toMatchAriaSnapshot(`
      - region:
        - heading [level=2]
        - paragraph
    `);
    await expect(section.getByRole("switch")).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("badges on the profile", () => {
  test("a badge is shown from the keyboard, each switch named by its badge, and stays after a reload", async ({ page, makeAxeBuilder }) => {
    const changes = await signedInServer(page, projectLocale());
    await page.goto("/settings/profile");
    const section = page.getByRole("main").getByRole("region", { name: message("settings.profile.badges"), exact: true });
    // The badges' names are the staff's text in any script, the same in every locale (ADR-0071).
    await expect(section).toMatchAriaSnapshot(`
      - region:
        - heading [level=2]
        - paragraph
        - list:
          - listitem:
            - switch "Supporter of the medium plan"
          - listitem:
            - switch "داعم" [checked]
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    const supporter = section.getByRole("switch", { name: "Supporter of the medium plan" });
    await supporter.focus();
    await page.keyboard.press("Space");
    await expect(supporter).toBeChecked();
    await expect.poll(() => changes).toEqual([{ badge: true }]);

    await page.reload();
    await expect(page.getByRole("main").getByRole("switch", { name: "Supporter of the medium plan" })).toBeChecked();
  });
});

test.describe("time zone", () => {
  test("names the zones in the display language with their offset, and describes them by their IANA name", async ({ page, makeAxeBuilder }) => {
    await signedInServer(page, projectLocale());
    await page.goto("/settings/preferences");
    const select = page.locator('main button[aria-haspopup="listbox"]').nth(1);
    // The closed select shows the stored UTC's offset and CLDR name, without the description.
    await expect(select).toContainText("+00:00");

    await select.click();
    const tokyo = page.getByRole("option").filter({ hasText: "Asia/Tokyo" });
    await expect(tokyo).toHaveAccessibleName(/\+09:00/);
    await expect(tokyo).not.toHaveAccessibleName(/Asia\/Tokyo/);
    await expect(tokyo).toHaveAccessibleDescription("Asia/Tokyo");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("time zone of the device", () => {
  test.use({ timezoneId: "Asia/Tokyo" });

  test("a new account takes the browser's time zone", async ({ page, makeAxeBuilder }) => {
    const changes = await signedInServer(page, projectLocale(), { timeZoneChosen: false });
    await page.goto("/settings/preferences");

    await expect.poll(() => changes).toEqual([{ time_zone: "Asia/Tokyo" }]);
    const select = page.locator('main button[aria-haspopup="listbox"]').nth(1);
    await expect(select).toContainText("+09:00");
    // The zone is the browser's, so no note tells of a difference and offers to switch to it.
    await expect(useDeviceButton(page)).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("tells that the chosen zone is not the browser's, and switches to it from the keyboard", async ({ page, makeAxeBuilder }) => {
    const changes = await signedInServer(page, projectLocale());
    await page.goto("/settings/preferences");
    const select = page.locator('main button[aria-haspopup="listbox"]').nth(1);
    await expect(select).toContainText("+00:00");
    // The display language and time zone selects, then the note of the browser's zone with its button.
    await expect(page.getByRole("main")).toMatchAriaSnapshot(`
      - paragraph
      - button
      - button
      - paragraph: /\\+09:00/
      - button
      - radiogroup
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    const useDevice = useDeviceButton(page);
    await useDevice.focus();
    await page.keyboard.press("Enter");

    await expect.poll(() => changes).toEqual([{ time_zone: "Asia/Tokyo" }]);
    await expect(select).toContainText("+09:00");
    await expect(select).toBeFocused();
    await expect(useDevice).toHaveCount(0);
  });
});
