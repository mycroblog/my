// SPDX-License-Identifier: AGPL-3.0-only
// The settings pages through the whole app (web-client/README.md T4): a signed-in session from the default
// handlers, MSW playing the server with the state each test needs, and the user's own interactions.
import { screen, waitFor, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test, vi } from "vitest";
import type { Preferences, PreferencesChange, ProfileChange } from "./api/me";
import {
  accountFixture,
  instanceFixture,
  mediaFixture,
  preferencesFixture,
  relationshipFixture,
  type Schemas,
  statusPage,
  viewerFixture,
  viewerStatusFixture,
} from "../../test/fixtures";
import { apiBase, problem, signedInHandler, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { nodeUploadFile } from "../../test/multipart";
import { autoplayKey, navigationKey, themeKey } from "../../lib/storage";
import { renderApp } from "../../test/render";

/**
 * Makes `zone` the time zone of this device: the zone of every Intl.DateTimeFormat that names none, as in a
 * browser set to it. Undone after each test (`unstubGlobals` in vitest.config.ts).
 */
function stubDeviceTimeZone(zone: string) {
  class DeviceDateTimeFormat extends Intl.DateTimeFormat {
    constructor(locales?: Intl.LocalesArgument, options?: Intl.DateTimeFormatOptions) {
      super(locales, { ...options, timeZone: options?.timeZone ?? zone });
    }
  }
  vi.stubGlobal("Intl", Object.assign(Object.create(Intl) as typeof Intl, { DateTimeFormat: DeviceDateTimeFormat }));
}

beforeEach(() => {
  server.use(signedInHandler);
  stubDeviceTimeZone("UTC");
  // The tests share one document; a locale change sets its lang and dir.
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

/** The server's preferences of the viewer, which PATCH changes, with the requests it received. */
function preferencesServer(initial: Partial<Preferences> = {}) {
  let preferences: Preferences = { ...preferencesFixture, ...initial };
  const changes: PreferencesChange[] = [];
  const languages: (string | null)[] = [];
  server.use(
    http.get(`${apiBase}/preferences`, ({ request }) => {
      languages.push(request.headers.get("Accept-Language"));
      return HttpResponse.json(preferences);
    }),
    http.patch(`${apiBase}/preferences`, async ({ request }) => {
      const change = (await request.json()) as PreferencesChange;
      changes.push(change);
      preferences = { ...preferences, ...change, time_zone_chosen: preferences.time_zone_chosen || change.time_zone !== undefined };
      return HttpResponse.json(preferences);
    }),
  );
  return { changes, languages, get current() { return preferences; } };
}

describe("preferences", () => {
  test("switching the display language saves it and displays the app in it at once", async () => {
    const prefs = preferencesServer();
    const { user } = await renderApp({ path: "/settings/preferences" });
    expect(await screen.findByRole("heading", { level: 1, name: "Preferences" })).toBeInTheDocument();
    expect(document.documentElement).toHaveAttribute("lang", "en");

    await user.click(screen.getByRole("button", { name: /Display language/ }));
    await user.click(await screen.findByRole("option", { name: "日本語" }));

    expect(await screen.findByRole("heading", { level: 1, name: "表示と投稿" })).toBeInTheDocument();
    expect(document.documentElement).toHaveAttribute("lang", "ja");
    expect(document.documentElement).toHaveAttribute("dir", "ltr");
    expect(prefs.changes).toEqual([{ locale: "ja" }]);
    expect(screen.getByRole("button", { name: /表示言語/ })).toHaveTextContent("日本語");
    // The server's localized data is fetched again in the new locale.
    await waitFor(() => {
      expect(prefs.languages.at(-1)).toBe("ja");
    });
  });

  test("the app starts in the signed-in user's locale", async () => {
    preferencesServer({ locale: "ja-JP" });
    await renderApp({ path: "/settings" });

    expect(await screen.findByRole("heading", { level: 1, name: "設定" })).toBeInTheDocument();
    expect(document.documentElement).toHaveAttribute("lang", "ja");
  });

  test("the default visibility and the timeline languages are saved as they change", async () => {
    const prefs = preferencesServer();
    const { user } = await renderApp({ path: "/settings/preferences" });
    const visibility = await screen.findByRole("radiogroup", { name: "Who sees your posts by default" });

    await user.click(within(visibility).getByRole("radio", { name: "Followers" }));
    expect(await within(visibility).findByRole("radio", { name: "Followers" })).toBeChecked();
    const languages = screen.getByRole("group", { name: "Languages of your timelines" });
    await user.click(within(languages).getByRole("checkbox", { name: "Japanese" }));
    await user.click(within(languages).getByRole("checkbox", { name: "German" }));

    await waitFor(() => {
      expect(prefs.changes).toEqual([{ default_visibility: "private" }, { timeline_languages: ["ja"] }, { timeline_languages: ["de", "ja"] }]);
    });
    expect(within(languages).getByRole("checkbox", { name: "German" })).toBeChecked();
    expect((await screen.findAllByText("Saved")).length).toBeGreaterThan(0);
  });

  test("the default post language is saved and the post form starts in it", async () => {
    const prefs = preferencesServer({ default_post_language: "und" });
    const { user } = await renderApp({ path: "/settings/preferences" });
    const select = await screen.findByRole("button", { name: /Default language of your posts/ });
    expect(select).toHaveTextContent("Not specified");
    expect(select).toHaveAccessibleDescription(expect.stringContaining("You can still choose for each post.") as string);

    await user.click(select);
    const options = await screen.findAllByRole("option");
    expect(options[0]).toHaveAccessibleName("Not specified");
    await user.click(screen.getByRole("option", { name: "Japanese" }));

    await waitFor(() => {
      expect(prefs.changes).toEqual([{ default_post_language: "ja" }]);
    });
    expect(screen.getByRole("button", { name: /Default language of your posts/ })).toHaveTextContent("Japanese");
    // The settings page has no new-post button; the home timeline has one.
    await user.click(screen.getByRole("link", { name: "Home" }));
    await user.click((await screen.findAllByRole("button", { name: "Write a post" }))[0] as HTMLElement);
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(within(sheet).getByRole("button", { name: "Japanese Language" })).toBeInTheDocument();
  });

  test("the automatic insertion of new posts is off by default and switched on", async () => {
    const prefs = preferencesServer({ timeline_auto_insert: false });
    const { user } = await renderApp({ path: "/settings/preferences" });
    const auto = await screen.findByRole("switch", { name: "Show new posts automatically" });
    expect(auto).not.toBeChecked();

    await user.click(auto);

    await waitFor(() => {
      expect(prefs.current.timeline_auto_insert).toBe(true);
    });
    expect(auto).toBeChecked();
  });

  test("the request for a description of every attachment is switched on and off", async () => {
    const prefs = preferencesServer({ require_media_description: false });
    const { user } = await renderApp({ path: "/settings/preferences" });
    const ask = await screen.findByRole("switch", { name: "Ask me for a description of every attachment" });
    expect(ask).not.toBeChecked();

    await user.click(ask);
    await waitFor(() => {
      expect(ask).toBeChecked();
    });
    await waitFor(() => {
      expect(prefs.current.require_media_description).toBe(true);
    });
    await user.click(ask);

    await waitFor(() => {
      expect(prefs.changes).toEqual([{ require_media_description: true }, { require_media_description: false }]);
    });
    expect(ask).not.toBeChecked();
  });

  test("a change the server refuses is reported on its field", async () => {
    preferencesServer();
    server.use(
      http.patch(`${apiBase}/preferences`, () =>
        HttpResponse.json(
          { type: "about:blank", title: "Unprocessable", status: 422, detail: "time_zone is not a valid value." },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/settings/preferences" });

    await user.click(await screen.findByRole("button", { name: /Time zone/ }));
    await user.click(await screen.findByRole("option", { name: "(UTC+09:00) Japan Standard Time" }));

    const timeZone = screen.getByRole("button", { name: /Time zone/ });
    await waitFor(() => {
      expect(timeZone).toHaveAccessibleDescription(expect.stringContaining("time_zone is not a valid value.") as string);
    });
    expect(timeZone).toHaveTextContent("(UTC+00:00) Coordinated Universal Time");
  });

  test("time zones are named in the display language with their offset, and described by their IANA name", async () => {
    const prefs = preferencesServer({ locale: "ja", time_zone: "Asia/Tokyo" });
    const { user } = await renderApp({ path: "/settings/preferences" });
    // The select's name ends with its label; the button to use the device's time zone also names it.
    const timeZone = await screen.findByRole("button", { name: /タイムゾーン$/ });
    expect(timeZone).toHaveTextContent("（UTC+09:00）日本標準時");

    await user.click(timeZone);
    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).getByRole("option", { name: "（UTC+09:00）日本標準時" })).toHaveAccessibleDescription("Asia/Tokyo");
    // Zones of one name are told apart by their IANA name.
    expect(within(listbox).getByRole("option", { description: "America/New_York" })).toHaveAccessibleName(/^（UTC−0[45]:00）米国東部時間$/);
    expect(within(listbox).getByRole("option", { description: "America/Detroit" })).toHaveAccessibleName(/^（UTC−0[45]:00）米国東部時間$/);
    // Nepal is ahead of UTC by 5 hours 45 minutes all year; its IANA name depends on the ICU data (Asia/Katmandu).
    await user.click(within(listbox).getByRole("option", { name: "（UTC+05:45）ネパール時間" }));

    await waitFor(() => {
      expect(prefs.changes).toEqual([{ time_zone: expect.stringMatching(/^Asia\/Kat(h)?mandu$/) as string }]);
    });
    expect(timeZone).toHaveTextContent("（UTC+05:45）ネパール時間");
  });
});

describe("theme", () => {
  test("applies at once with the arrow keys, is kept on this browser, and sends nothing to the server", async () => {
    const prefs = preferencesServer();
    const { user } = await renderApp({ path: "/settings/preferences" });
    const theme = await screen.findByRole("radiogroup", { name: "Theme" });
    expect(theme).toHaveAccessibleDescription("Applies at once and is kept on this browser.");
    const [system, light, dark] = within(theme).getAllByRole("radio");
    expect(system).toHaveAccessibleName("Match the system");
    expect(light).toHaveAccessibleName("Light");
    expect(dark).toHaveAccessibleName("Dark");
    expect(within(theme).getByRole("radio", { name: "Match the system" })).toBeChecked();

    await user.click(within(theme).getByRole("radio", { name: "Light" }));
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    expect(themeKey.read()).toBe("light");
    // The APG radio group: an arrow key moves to the next option and checks it.
    await user.keyboard("{ArrowDown}");
    expect(within(theme).getByRole("radio", { name: "Dark" })).toBeChecked();
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    expect(themeKey.read()).toBe("dark");

    await user.click(within(theme).getByRole("radio", { name: "Match the system" }));
    expect(document.documentElement).not.toHaveAttribute("data-theme");
    expect(localStorage.getItem("my:display:theme")).toBeNull();
    expect(prefs.changes).toEqual([]);
  });

  test("the page opens with the theme the document is displayed in", async () => {
    preferencesServer();
    document.documentElement.dataset["theme"] = "dark";
    await renderApp({ path: "/settings/preferences" });

    const theme = await screen.findByRole("radiogroup", { name: "Theme" });
    expect(within(theme).getByRole("radio", { name: "Dark" })).toBeChecked();
  });
});

describe("autoplay of animations", () => {
  test("is on at first, applies at once, is kept on this browser, and sends nothing to the server", async () => {
    const prefs = preferencesServer();
    const { user } = await renderApp({ path: "/settings/preferences" });
    const autoplay = await screen.findByRole("switch", { name: "Play animations automatically" });
    expect(autoplay).toBeChecked();
    expect(autoplay).toHaveAccessibleDescription(expect.stringContaining("Kept on this browser") as string);

    await user.click(autoplay);
    expect(autoplay).not.toBeChecked();
    expect(autoplayKey.read()).toBe(false);
    await user.click(autoplay);
    expect(autoplay).toBeChecked();
    expect(localStorage.getItem("my:display:autoplay")).toBeNull();
    expect(prefs.changes).toEqual([]);
  });
});

describe("navigation", () => {
  test("moves and hides items from the keyboard, keeps them on this browser, and the navigation follows", async () => {
    const prefs = preferencesServer();
    const { user } = await renderApp({ path: "/settings/preferences" });
    const section = await screen.findByRole("region", { name: "Navigation" });
    expect(section).toHaveAccessibleDescription(expect.stringContaining("the first 5 shown items are in the tab bar") as string);
    expect(within(section).getAllByRole("checkbox")).toHaveLength(8);
    expect(within(section).getByRole("checkbox", { name: "Settings" })).toBeDisabled();
    expect(within(section).getByRole("checkbox", { name: "Search" })).toHaveAccessibleDescription("In the tab bar on phones");
    expect(within(section).getByRole("checkbox", { name: "Profile" })).toHaveAccessibleDescription("In the menu on phones");
    expect(within(section).getByRole("button", { name: "Move Home up" })).toBeDisabled();

    // Enter on Move up moves the item; the focus stays on the moved item's button and the move is announced.
    within(section).getByRole("button", { name: "Move Profile up" }).focus();
    await user.keyboard("{Enter}");
    await waitFor(() => {
      expect(within(section).getByRole("button", { name: "Move Profile up" })).toHaveFocus();
    });
    expect(await screen.findByText("Profile moved to position 5 of 8")).toBeInTheDocument();
    expect(within(section).getByRole("checkbox", { name: "Profile" })).toHaveAccessibleDescription("In the tab bar on phones");
    expect(within(section).getByRole("checkbox", { name: "Search" })).toHaveAccessibleDescription("In the menu on phones");

    // Down on the item before the last moves it to the end, and the focus goes to its Move up, Move down being disabled.
    within(section).getByRole("button", { name: "Move Bookmarks down" }).focus();
    await user.keyboard("{Enter}");
    await waitFor(() => {
      expect(within(section).getByRole("button", { name: "Move Bookmarks up" })).toHaveFocus();
    });
    expect(within(section).getByRole("button", { name: "Move Bookmarks down" })).toBeDisabled();

    // Space on a checkbox hides its item.
    within(section).getByRole("checkbox", { name: "Local" }).focus();
    await user.keyboard(" ");
    expect(within(section).getByRole("checkbox", { name: "Local" })).not.toBeChecked();
    expect(within(section).getByRole("checkbox", { name: "Local" })).toHaveAccessibleDescription("Hidden");

    expect(navigationKey.read()).toEqual({
      order: ["home", "local", "public", "notifications", "profile", "search", "settings", "bookmarks"],
      hidden: ["local"],
    });
    const nav = screen.getByRole("navigation", { name: "Main" });
    expect(within(within(nav).getByRole("list")).getAllByRole("link").map((link) => link.getAttribute("href"))).toEqual([
      "/home",
      "/public",
      "/notifications",
      "/profile",
      "/search",
      "/settings",
      "/bookmarks",
    ]);
    expect(prefs.changes).toEqual([]);

    // Restoring the default forgets the arrangement.
    await user.click(within(section).getByRole("button", { name: "Restore the default" }));
    expect(localStorage.getItem("my:display:navigation")).toBeNull();
    expect(within(section).getByRole("checkbox", { name: "Local" })).toBeChecked();
    expect(within(section).getAllByRole("checkbox", { checked: true })).toHaveLength(8);
  });

  test("says which timeline this server does not offer", async () => {
    preferencesServer();
    server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, timelines: { local: "everyone", federated: "off" } })));
    await renderApp({ path: "/settings/preferences" });
    const section = await screen.findByRole("region", { name: "Navigation" });
    await waitFor(() => {
      expect(within(section).getByRole("checkbox", { name: "Federated" })).toHaveAccessibleDescription("Not offered by this server");
    });
    expect(within(section).getByRole("checkbox", { name: "Local" })).toHaveAccessibleDescription("In the tab bar on phones");
  });

  test("is in Japanese in the ja locale", async () => {
    preferencesServer({ locale: "ja" });
    await renderApp({ path: "/settings/preferences", locale: "ja" });
    const section = await screen.findByRole("region", { name: "ナビゲーション" });
    expect(within(section).getByRole("button", { name: "ホームを下へ移動" })).toBeInTheDocument();
  });
});

describe("time zone of the device", () => {
  test("a new account takes this device's time zone", async () => {
    stubDeviceTimeZone("Asia/Tokyo");
    const prefs = preferencesServer({ time_zone: "UTC", time_zone_chosen: false });
    await renderApp({ path: "/settings/preferences" });

    await waitFor(() => {
      expect(prefs.changes).toEqual([{ time_zone: "Asia/Tokyo" }]);
    });
    const timeZone = screen.getByRole("button", { name: /Time zone$/ });
    await waitFor(() => {
      expect(timeZone).toHaveTextContent("(UTC+09:00) Japan Standard Time");
    });
    expect(screen.queryByRole("button", { name: "Use this device's time zone" })).not.toBeInTheDocument();
  });

  test("a chosen time zone is kept, and the settings tell that this device is in another one", async () => {
    stubDeviceTimeZone("America/New_York");
    const prefs = preferencesServer({ time_zone: "Asia/Tokyo" });
    const { user } = await renderApp({ path: "/settings/preferences" });

    expect(await screen.findByText(/^This device is in a different time zone: \(UTC−0[45]:00\) Eastern Time\.$/)).toBeInTheDocument();
    const timeZone = screen.getByRole("button", { name: /Time zone$/ });
    expect(timeZone).toHaveTextContent("(UTC+09:00) Japan Standard Time");
    expect(prefs.changes).toEqual([]);

    await user.click(screen.getByRole("button", { name: "Use this device's time zone" }));

    await waitFor(() => {
      expect(prefs.changes).toEqual([{ time_zone: "America/New_York" }]);
    });
    expect(timeZone).toHaveTextContent(/^\(UTC−0[45]:00\) Eastern Time/);
    expect(timeZone).toHaveFocus();
    expect(screen.queryByText(/different time zone/)).not.toBeInTheDocument();
  });

  test("an alias of the device's zone is the same zone", async () => {
    stubDeviceTimeZone("Asia/Calcutta");
    preferencesServer({ time_zone: "Asia/Kolkata" });
    await renderApp({ path: "/settings/preferences" });

    expect(await screen.findByRole("button", { name: /Time zone$/ })).toHaveTextContent("(UTC+05:30) India Standard Time");
    expect(screen.queryByText(/different time zone/)).not.toBeInTheDocument();
  });
});

describe("moderator notifications by email", () => {
  /** The viewer's account with the permissions `permissions`. */
  function viewerWith(permissions: Schemas["Permission"][]) {
    server.use(
      http.get(`${apiBase}/accounts/verify_credentials`, ({ request }) => withViewer(request, () => HttpResponse.json({ ...viewerFixture, permissions }))),
    );
  }

  test("a moderator switches them on, and the change is saved", async () => {
    viewerWith(["manage_reports"]);
    const prefs = preferencesServer();
    const { user } = await renderApp({ path: "/settings/preferences" });
    const section = await screen.findByRole("region", { name: "Moderator notifications" });
    const toggle = within(section).getByRole("switch", { name: "Also send them by email" });
    expect(toggle).not.toBeChecked();
    expect(toggle).toHaveAccessibleDescription(expect.stringContaining("that your role handles") as string);

    await user.click(toggle);

    await waitFor(() => {
      expect(prefs.changes).toEqual([{ email_moderator_notifications: true }]);
    });
    expect(toggle).toBeChecked();
    expect(prefs.current.email_moderator_notifications).toBe(true);
  });

  test("the page opens with the saved value as the switch's state", async () => {
    viewerWith(["manage_accounts"]);
    preferencesServer({ email_moderator_notifications: true, locale: "ja" });
    await renderApp({ path: "/settings/preferences" });

    const section = await screen.findByRole("region", { name: "モデレーターへの通知" });
    expect(within(section).getByRole("switch", { name: "メールでも受け取る" })).toBeChecked();
  });

  test("is not offered to a viewer whose roles get no moderator notifications", async () => {
    viewerWith(["manage_domains", "view_audit"]);
    preferencesServer();
    await renderApp({ path: "/settings/preferences" });

    expect(await screen.findByRole("heading", { level: 1, name: "Preferences" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Moderator notifications" })).not.toBeInTheDocument();
    // The switches left are this browser's autoplay of animations, the request for descriptions and the automatic new posts.
    expect(screen.getAllByRole("switch")).toEqual([
      screen.getByRole("switch", { name: "Play animations automatically" }),
      screen.getByRole("switch", { name: "Ask me for a description of every attachment" }),
      screen.getByRole("switch", { name: "Show new posts automatically" }),
    ]);
  });

  test("says why there is no switch when the server sends no email", async () => {
    viewerWith(["manage_reports", "manage_accounts"]);
    preferencesServer();
    server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, sends_email: false })));
    await renderApp({ path: "/settings/preferences" });

    const section = await screen.findByRole("region", { name: "Moderator notifications" });
    expect(await within(section).findByText("This server does not send email, so the moderator notifications come only in the app.")).toBeInTheDocument();
    expect(within(section).queryByRole("switch")).not.toBeInTheDocument();
  });
});

describe("profile", () => {
  /** The server's update_credentials operation, with the changes it received. */
  function profileServer() {
    const changes: ProfileChange[] = [];
    server.use(
      http.patch(`${apiBase}/accounts/update_credentials`, async ({ request }) => {
        const change = (await request.json()) as ProfileChange;
        changes.push(change);
        return HttpResponse.json({ ...viewerFixture, display_name: change.display_name ?? viewerFixture.display_name, locked: change.locked ?? false });
      }),
    );
    return changes;
  }

  test("a value over its limit is reported on its field, which gets focus, and nothing is sent", async () => {
    const changes = profileServer();
    const { user } = await renderApp({ path: "/settings/profile" });
    const displayName = await screen.findByRole("textbox", { name: "Display name" });

    await user.clear(displayName);
    await user.type(displayName, "a".repeat(31));
    await user.click(screen.getByRole("button", { name: "Save profile" }));

    expect(displayName).toHaveFocus();
    expect(displayName).toHaveAttribute("aria-invalid", "true");
    expect(displayName).toHaveAccessibleDescription(expect.stringContaining("Use at most 30 characters.") as string);
    expect(changes).toEqual([]);
  });

  test("a field with content needs a label", async () => {
    profileServer();
    const { user } = await renderApp({ path: "/settings/profile" });
    await user.click(await screen.findByRole("button", { name: "Add a field" }));
    const field = screen.getByRole("group", { name: "Field 1" });

    await user.type(within(field).getByRole("textbox", { name: "Content" }), "https://example.com");
    await user.click(screen.getByRole("button", { name: "Save profile" }));

    const label = within(field).getByRole("textbox", { name: "Label" });
    expect(label).toHaveFocus();
    expect(label).toHaveAccessibleDescription("Give this field a label.");
  });

  test("saves the profile with a new avatar, a field and approval of followers", async () => {
    const changes = profileServer();
    const uploads: string[] = [];
    server.use(
      http.post(`${apiBase}/media`, async ({ request }) => {
        // The raw multipart body: undici's parser cannot make its File objects under jsdom.
        const body = await request.text();
        uploads.push(/name="file"; filename="([^"]*)"/.exec(body)?.[1] ?? "");
        return HttpResponse.json(mediaFixture("0192f0c4-7a8e-7000-8000-00000000bbbb", "ready"), { status: 202 });
      }),
    );
    const { user } = await renderApp({ path: "/settings/profile" });
    const displayName = await screen.findByRole("textbox", { name: "Display name" });

    await user.clear(displayName);
    await user.type(displayName, "Alice in Chains");
    await user.type(screen.getByRole("textbox", { name: "Bio" }), "Hello");
    await user.click(screen.getByRole("button", { name: "Add a field" }));
    const field = screen.getByRole("group", { name: "Field 1" });
    await user.type(within(field).getByRole("textbox", { name: "Label" }), "Site");
    await user.type(within(field).getByRole("textbox", { name: "Content" }), "https://alice.example");
    await user.click(screen.getByRole("checkbox", { name: "Approve followers" }));
    const avatar = screen.getByRole("region", { name: "Avatar" });
    // FileTrigger's input is hidden; user-event uploads through it as the file picker would.
    // eslint-disable-next-line testing-library/no-node-access -- the hidden file input has no role.
    const input = avatar.querySelector<HTMLInputElement>('input[type="file"]');
    if (input === null) throw new Error("no file input");
    await user.upload(input, nodeUploadFile("png", "me.png", "image/png"));
    expect(await within(avatar).findByRole("button", { name: "Remove image" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Save profile" }));

    expect((await screen.findAllByText("Your profile has been saved.")).length).toBeGreaterThan(0);
    expect(uploads).toEqual(["me.png"]);
    expect(changes).toEqual([
      {
        display_name: "Alice in Chains",
        note: "Hello",
        fields: [{ name: "Site", value: "https://alice.example" }],
        locked: true,
        bot: false,
        indexable: true,
        ai_opt_out: false,
        avatar_media_id: "0192f0c4-7a8e-7000-8000-00000000bbbb",
      },
    ]);
  });

  test("saves whether search and AI may use the profile, and says what the server already denies", async () => {
    const changes = profileServer();
    server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, search_indexing: "deny", ai_usage: "deny_training" })));
    const { user } = await renderApp({ path: "/settings/profile" });
    const crawlers = await screen.findByRole("region", { name: "Search engines and AI" });
    const indexable = within(crawlers).getByRole("checkbox", { name: "Show my profile and public posts in search" });
    expect(indexable).toBeChecked();
    expect(
      await within(crawlers).findByText("This server asks search engines not to index any page, so they do not index yours even if you allow it."),
    ).toBeInTheDocument();
    expect(
      within(crawlers).getByText("This server already denies AI training on every page; this also denies the use by AI search and assistants."),
    ).toBeInTheDocument();

    await user.click(indexable);
    await user.click(within(crawlers).getByRole("checkbox", { name: "Deny AI training and use" }));
    await user.click(screen.getByRole("button", { name: "Save profile" }));

    expect((await screen.findAllByText("Your profile has been saved.")).length).toBeGreaterThan(0);
    expect(changes).toEqual([expect.objectContaining({ indexable: false, ai_opt_out: true })]);
  });

  test("says nothing about the server when it allows search and AI", async () => {
    profileServer();
    server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, search_indexing: "allow", ai_usage: "allow" })));
    await renderApp({ path: "/settings/profile" });
    const crawlers = await screen.findByRole("region", { name: "Search engines and AI" });
    expect(within(crawlers).getByRole("checkbox", { name: "Deny AI training and use" })).not.toBeChecked();
    expect(within(crawlers).queryByText(/^This server/u)).not.toBeInTheDocument();
  });

  test("leaving with unsaved changes asks first: keeping them stays, discarding them leaves", async () => {
    profileServer();
    const { user, router } = await renderApp({ path: "/settings/profile" });
    const displayName = await screen.findByRole("textbox", { name: "Display name" });
    await user.type(displayName, " Liddell");
    const settings = screen.getByRole("link", { name: "Settings" });

    await user.click(settings);
    const dialog = await screen.findByRole("alertdialog", { name: "Leave without saving your profile?" });
    expect(dialog).toHaveAccessibleDescription("Your changes to your profile will be lost.");
    expect(within(dialog).getByRole("button", { name: "Keep editing" })).toHaveFocus();
    await user.click(within(dialog).getByRole("button", { name: "Keep editing" }));

    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/settings/profile");
    expect(screen.getByRole("textbox", { name: "Display name" })).toHaveValue("Alice Liddell");
    await waitFor(() => {
      expect(settings).toHaveFocus();
    });

    await user.click(settings);
    await user.click(await screen.findByRole("button", { name: "Discard changes" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Settings" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/settings");
  });

  test("Esc in the question stays on the form", async () => {
    profileServer();
    const { user, router } = await renderApp({ path: "/settings/profile" });
    await user.click(await screen.findByRole("checkbox", { name: "Approve followers" }));

    await user.click(screen.getByRole("link", { name: "Settings" }));
    expect(await screen.findByRole("alertdialog")).toBeInTheDocument();
    await user.keyboard("{Escape}");

    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(router.state.location.pathname).toBe("/settings/profile");
    expect(screen.getByRole("checkbox", { name: "Approve followers" })).toBeChecked();
  });

  test("leaving without changes, or after undoing or saving them, does not ask", async () => {
    profileServer();
    const { user, router } = await renderApp({ path: "/settings/profile" });
    const displayName = await screen.findByRole("textbox", { name: "Display name" });
    // Undone by hand: nothing is left to save.
    await user.type(displayName, "!");
    await user.type(displayName, "{Backspace}");
    await user.click(screen.getByRole("link", { name: "Settings" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Settings" })).toBeInTheDocument();

    await router.navigate({ to: "/settings/profile" });
    await user.type(await screen.findByRole("textbox", { name: "Display name" }), " Liddell");
    await user.click(screen.getByRole("button", { name: "Save profile" }));
    expect((await screen.findAllByText("Your profile has been saved.")).length).toBeGreaterThan(0);
    await user.click(screen.getByRole("link", { name: "Settings" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Settings" })).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  test("the profile page and the author of the viewer's posts show the saved profile at once, before a slow server answers again", async () => {
    // The server's own account of the viewer, which the save changes; once saved, every read is slow, as on a
    // real network, so a page that waited for its refetch would show the old name first (#505).
    let account: Schemas["Account"] = { ...viewerFixture, followers_count: 1, following_count: 1 };
    let saved = false;
    const slow = async () => {
      if (saved) await delay(2000);
    };
    server.use(
      http.get(`${apiBase}/accounts/lookup`, async () => {
        await slow();
        return HttpResponse.json(account);
      }),
      http.get(`${apiBase}/accounts/:id/statuses`, async () => {
        await slow();
        return HttpResponse.json(statusPage([viewerStatusFixture({ account })]));
      }),
      http.patch(`${apiBase}/accounts/update_credentials`, async ({ request }) => {
        const change = (await request.json()) as ProfileChange;
        account = { ...account, display_name: change.display_name ?? account.display_name };
        saved = true;
        return HttpResponse.json({ ...viewerFixture, ...account });
      }),
    );
    const { user, router } = await renderApp({ path: "/profile" });
    expect(await screen.findByRole("heading", { level: 1, name: "Alice" })).toBeInTheDocument();
    expect(within(screen.getByRole("feed", { name: "Posts" })).getByRole("article", { name: /^Alice/ })).toBeInTheDocument();

    await router.navigate({ to: "/settings/profile" });
    const displayName = await screen.findByRole("textbox", { name: "Display name" });
    await user.clear(displayName);
    await user.type(displayName, "Alice in Chains");
    await user.click(screen.getByRole("button", { name: "Save profile" }));
    expect((await screen.findAllByText("Your profile has been saved.")).length).toBeGreaterThan(0);
    await router.navigate({ to: "/profile" });

    // The first render of the page has the new name, long before the slow reads could answer.
    const heading = await screen.findByRole("heading", { level: 1 }, { timeout: 500 });
    expect(heading).toHaveTextContent("Alice in Chains");
    expect(within(screen.getByRole("feed", { name: "Posts" })).getByRole("article", { name: /^Alice in Chains/ })).toBeInTheDocument();
  });

  test("a save the server refuses says why", async () => {
    server.use(
      http.patch(`${apiBase}/accounts/update_credentials`, () =>
        HttpResponse.json(
          { type: "about:blank", title: "Unprocessable", status: 422, detail: "fields is not a valid value." },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/settings/profile" });

    await user.click(await screen.findByRole("button", { name: "Save profile" }));

    expect(await screen.findByText("fields is not a valid value.")).toHaveAttribute("role", "alert");
  });
});

describe("blocks, mutes and follow requests", () => {
  const bob = accountFixture(2, "Bob");
  const carol = accountFixture(3, "Carol");

  test("unblocking takes the account off the list and keeps focus in the list", async () => {
    let blocked = [bob, carol];
    const unblocked: string[] = [];
    server.use(
      http.get(`${apiBase}/blocks`, () => HttpResponse.json({ accounts: blocked })),
      http.post(`${apiBase}/accounts/:id/unblock`, ({ params }) => {
        const id = String(params["id"]);
        unblocked.push(id);
        blocked = blocked.filter((account) => account.id !== id);
        return HttpResponse.json(relationshipFixture(id));
      }),
    );
    const { user } = await renderApp({ path: "/settings/blocks" });
    expect(await screen.findByText("@bob@remote.example")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Unblock: Bob" }));

    await waitFor(() => {
      expect(screen.queryByText("Bob")).not.toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Unblock: Carol" })).toHaveFocus();
    expect(unblocked).toEqual([bob.id]);
    expect(await screen.findByText("You unblocked Bob.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Unblock: Carol" }));
    expect(await screen.findByRole("heading", { level: 2, name: "You have not blocked anyone" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1 })).toHaveFocus();
  });

  test("lists mutes with when they end, and loads more pages", async () => {
    server.use(
      http.get(`${apiBase}/mutes`, ({ request }) =>
        new URL(request.url).searchParams.get("cursor") === "next"
          ? HttpResponse.json({ mutes: [{ account: carol, notifications: false }] })
          : HttpResponse.json({ mutes: [{ account: bob, notifications: true, expires_at: "2026-11-01T09:30:00Z" }], next_cursor: "next" }),
      ),
    );
    const { user } = await renderApp({ path: "/settings/mutes" });

    expect(await screen.findByText("Bob")).toBeInTheDocument();
    expect(screen.getByText(/^Until Nov 1, 2026/)).toBeInTheDocument();
    expect(screen.getByText("Notifications hidden too")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Show more" }));
    expect(await screen.findByText("Carol")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Unmute: Carol" })).toBeInTheDocument();
  });

  test("accepting and rejecting follow requests", async () => {
    const answered: string[] = [];
    server.use(
      http.get(`${apiBase}/follow_requests`, () => HttpResponse.json({ accounts: [bob, carol] })),
      http.post(`${apiBase}/follow_requests/:id/authorize`, ({ params }) => {
        answered.push(`accept ${String(params["id"])}`);
        return HttpResponse.json({ ...relationshipFixture(String(params["id"])), followed_by: true });
      }),
      http.post(`${apiBase}/follow_requests/:id/reject`, ({ params }) => {
        answered.push(`reject ${String(params["id"])}`);
        return HttpResponse.json(relationshipFixture(String(params["id"])));
      }),
    );
    const { user } = await renderApp({ path: "/settings/follow-requests" });

    await user.click(await screen.findByRole("button", { name: "Accept: Bob" }));
    expect(await screen.findByText("Bob now follows you.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Accept: Carol" })).toHaveFocus();
    await user.click(screen.getByRole("button", { name: "Reject: Carol" }));

    expect(await screen.findByRole("heading", { level: 2, name: "No follow requests" })).toBeInTheDocument();
    expect(answered).toEqual([`accept ${bob.id}`, `reject ${carol.id}`]);
  });

  test("a failed action keeps the account and says why", async () => {
    server.use(
      http.get(`${apiBase}/blocks`, () => HttpResponse.json({ accounts: [bob] })),
      http.post(`${apiBase}/accounts/:id/unblock`, () => problem(404, "about:blank", "Not found")),
    );
    const { user } = await renderApp({ path: "/settings/blocks" });

    await user.click(await screen.findByRole("button", { name: "Unblock: Bob" }));

    expect(await screen.findByText("Not found")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Unblock: Bob" })).toBeInTheDocument();
  });
});

describe("signed out", () => {
  test("a settings page sends the browser to sign in first, and back afterwards", async () => {
    server.resetHandlers();
    const { router } = await renderApp({ path: "/settings/blocks" });

    expect(await screen.findByRole("heading", { level: 1, name: "Sign in" })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ return_to: "/settings/blocks" });
  });
});

describe("the settings page", () => {
  test("lists the settings pages", async () => {
    await renderApp({ path: "/settings" });

    const menu = await screen.findByRole("navigation", { name: "Settings pages" });
    expect(within(menu).getAllByRole("link").map((link) => link.getAttribute("href"))).toEqual([
      "/settings/profile",
      "/settings/preferences",
      "/settings/password",
      "/settings/two-factor",
      "/settings/passkeys",
      "/settings/auto-delete",
      "/settings/follow-requests",
      "/settings/blocks",
      "/settings/mutes",
      "/settings/domain-mutes",
      "/settings/import-export",
      "/settings/apps",
      "/settings/account-status",
      "/settings/account",
    ]);
  });
});
