// SPDX-License-Identifier: AGPL-3.0-only
// The main navigation through the whole app (web-client/README.md T4, ADR-0039): the local and the federated timeline as
// items of their own, the phones' tab bar with Menu, and the menu's sheet. jsdom applies no styles, so the items
// that only the tab bar or only the rail shows are all in the document; e2e/shell.spec.ts checks which ones each
// width shows.
import { act, screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { instanceFixture, preferencesFixture } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { navigationKey, themeKey } from "../../lib/storage";
import { renderApp } from "../../test/render";

/** GET /instance with the given public timelines turned on or off. */
function instanceWith(timelines: (typeof instanceFixture)["timelines"]) {
  return http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, timelines }));
}

function navigation() {
  return screen.getByRole("navigation", { name: "Main" });
}

// The user's items need a session; the tests of the signed-out navigation drop it with server.resetHandlers().
beforeEach(() => {
  server.use(signedInHandler);
});

describe("while signed out", () => {
  beforeEach(() => {
    server.resetHandlers();
  });

  test("offers only the pages usable without a session, signing in and signing up", async () => {
    const { user } = await renderApp({ path: "/local" });
    await screen.findByRole("heading", { level: 1, name: "Local timeline" });
    const list = within(navigation()).getByRole("list");
    await within(list).findByRole("link", { name: "Sign up" });

    // jsdom applies no styles, so Sign up, which only the rail shows, is here too.
    expect(
      within(list)
        .getAllByRole("link")
        .map((link) => [link.textContent, link.getAttribute("href")]),
    ).toEqual([
      ["Local", "/local"],
      ["Federated", "/public"],
      ["Search", "/search"],
      ["About", "/about"],
      ["Sign in", "/login?return_to=%2Flocal"],
      ["Sign up", "http://localhost:3000/auth/sign_up"],
    ]);
    expect(within(list).getByRole("link", { name: "Local" })).toHaveAttribute("aria-current", "page");

    // Below lg the items are behind the menu button (a disclosure, not a dialog), and Sign up stays in the bar.
    const menu = within(navigation()).getByRole("button", { name: "Menu" });
    expect(menu).toHaveAttribute("aria-expanded", "false");
    expect(menu).toHaveAttribute("aria-controls", list.id);
    expect(
      within(navigation())
        .getAllByRole("link", { name: "Sign up" })
        .map((link) => link.getAttribute("href")),
    ).toEqual(["http://localhost:3000/auth/sign_up", "http://localhost:3000/auth/sign_up"]);
    await user.click(menu);
    expect(menu).toHaveAttribute("aria-expanded", "true");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    // Escape closes the items and leaves focus on the button.
    within(list).getByRole("link", { name: "Search" }).focus();
    await user.keyboard("{Escape}");
    expect(menu).toHaveAttribute("aria-expanded", "false");
    expect(menu).toHaveFocus();

    // Following one of the items closes them too.
    await user.click(menu);
    await user.click(within(list).getByRole("link", { name: "Search" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Search" })).toBeInTheDocument();
    expect(menu).toHaveAttribute("aria-expanded", "false");
  });

  test("makes Sign in the call to action of the bar while the server takes no new accounts", async () => {
    server.use(
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({ ...instanceFixture, registrations: { mode: "closed" } }),
      ),
    );
    await renderApp({ path: "/local" });
    await screen.findByRole("heading", { level: 1, name: "Local timeline" });
    // jsdom applies no styles: the bar's call to action (below lg) and the list's item (from lg) are both here.
    await waitFor(() => {
      expect(within(navigation()).getAllByRole("link", { name: "Sign in" })).toHaveLength(2);
    });
    expect(within(navigation()).queryByRole("link", { name: "Sign up" })).not.toBeInTheDocument();
  });

  test("leaves out a timeline kept for signed-in users, and asks to sign in at its address", async () => {
    server.use(
      instanceWith({ local: "logged_in", federated: "everyone" }),
      http.get(`${apiBase}/timelines/local`, () => problem(401, "/api/v1/problems/unauthenticated", "Unauthenticated")),
    );
    await renderApp({ path: "/local" });

    expect(
      await screen.findByRole("heading", { level: 1, name: "The local timeline is for signed-in users" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Sign in to read the public posts of this server's accounts.")).toBeInTheDocument();
    // The page's own link and the navigation's both sign in and come back here.
    expect(screen.getAllByRole("link", { name: "Sign in" }).map((link) => link.getAttribute("href"))).toEqual([
      "/login?return_to=%2Flocal",
      "/login?return_to=%2Flocal",
    ]);
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
    await waitFor(() => {
      expect(within(navigation()).queryByRole("link", { name: "Local" })).not.toBeInTheDocument();
    });
    expect(within(navigation()).getByRole("link", { name: "Federated" })).toBeInTheDocument();
  });

  test("signs in from the page it is on, and comes back there", async () => {
    const { user, router } = await renderApp({ path: "/tags/cats" });
    await screen.findByRole("heading", { level: 1, name: "#cats" });

    const signIn = within(navigation()).getByRole("link", { name: "Sign in" });
    expect(signIn).toHaveAttribute("href", "/login?return_to=%2Ftags%2Fcats");
    await user.click(signIn);

    expect(await screen.findByRole("heading", { level: 1, name: "Sign in" })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ return_to: "/tags/cats" });
    // On the sign-in page, the item is the place, and keeps where to come back to.
    const current = within(navigation()).getByRole("link", { name: "Sign in" });
    expect(current).toHaveAttribute("aria-current", "page");
    expect(current).toHaveAttribute("href", "/login?return_to=%2Ftags%2Fcats");
  });

  test("leaves out Sign up while the server takes no new accounts", async () => {
    server.use(
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({ ...instanceFixture, registrations: { mode: "closed" } }),
      ),
    );
    await renderApp({ path: "/about" });
    await screen.findByRole("heading", { level: 1, name: "About my example" });
    await within(navigation()).findByRole("link", { name: "Federated" });

    expect(within(navigation()).getByRole("link", { name: "About" })).toHaveAttribute("aria-current", "page");
    expect(within(navigation()).queryByRole("link", { name: "Sign up" })).not.toBeInTheDocument();
  });

  test("sends the pages that need a session to sign in first, and back afterwards", async () => {
    for (const path of ["/home", "/notifications", "/profile", "/bookmarks", "/settings"]) {
      const { router, unmount } = await renderApp({ path });
      expect(await screen.findByRole("heading", { level: 1, name: "Sign in" })).toBeInTheDocument();
      expect(router.state.location.pathname).toBe("/login");
      expect(router.state.location.search).toEqual({ return_to: path });
      unmount();
    }
  });

  test("is in Japanese in the ja locale", async () => {
    await renderApp({ path: "/search", locale: "ja" });
    await screen.findByRole("heading", { level: 1, name: "検索" });
    const nav = screen.getByRole("navigation", { name: "メイン" });
    expect(await within(nav).findAllByRole("link", { name: "新規登録" })).toHaveLength(2);
    expect(within(nav).getByRole("link", { name: "サーバー情報" })).toHaveAttribute("href", "/about");
    expect(within(nav).getByRole("link", { name: "ログイン" })).toHaveAttribute("href", "/login?return_to=%2Fsearch");
  });
});

describe("the timelines' items", () => {
  test("open the local and the federated timeline as two screens, each the current item on its own, with no tabs", async () => {
    const { user } = await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });

    const local = within(navigation()).getByRole("link", { name: "Local" });
    expect(local).toHaveAttribute("href", "/local");
    expect(local).not.toHaveAttribute("aria-current");
    await user.click(local);

    const heading = await screen.findByRole("heading", { level: 1, name: "Local timeline" });
    await waitFor(() => {
      expect(heading).toHaveFocus();
    });
    expect(within(navigation()).getByRole("link", { name: "Local" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("feed", { name: "Local timeline" })).toBeInTheDocument();
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();

    const federated = await within(navigation()).findByRole("link", { name: "Federated" });
    expect(federated).toHaveAttribute("href", "/public");
    expect(federated).not.toHaveAttribute("aria-current");
    await user.click(federated);

    expect(await screen.findByRole("feed", { name: "Federated timeline" })).toBeInTheDocument();
    expect(within(navigation()).getByRole("link", { name: "Federated" })).toHaveAttribute("aria-current", "page");
    expect(within(navigation()).getByRole("link", { name: "Local" })).not.toHaveAttribute("aria-current");
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
  });

  test("leave out both timelines when the server serves neither", async () => {
    server.use(instanceWith({ local: "off", federated: "off" }));
    await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });
    await waitFor(() => {
      expect(within(navigation()).queryByRole("link", { name: "Local" })).not.toBeInTheDocument();
    });
    expect(within(navigation()).queryByRole("link", { name: "Federated" })).not.toBeInTheDocument();
    expect(
      within(within(navigation()).getByRole("list"))
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual(["/home", "/notifications", "/search", "/profile", "/bookmarks", "/settings"]);
  });

  test("offer the timelines kept for signed-in users", async () => {
    server.use(instanceWith({ local: "logged_in", federated: "logged_in" }));
    await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });

    expect(await within(navigation()).findByRole("link", { name: "Federated" })).toHaveAttribute("href", "/public");
    expect(within(navigation()).getByRole("link", { name: "Local" })).toHaveAttribute("href", "/local");
  });

  test("say that the local timeline is off when its address is a 404", async () => {
    server.use(
      instanceWith({ local: "off", federated: "off" }),
      http.get(`${apiBase}/timelines/local`, () => problem(404, "about:blank", "Not Found")),
    );
    await renderApp({ path: "/local" });

    expect(await screen.findByRole("heading", { level: 1, name: "The local timeline is off" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to the home timeline" })).toHaveAttribute("href", "/home");
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
  });

  test("are in Japanese in the ja locale", async () => {
    // The signed-in user's preferences decide the locale once they load: they are in Japanese too.
    server.use(http.get(`${apiBase}/preferences`, () => HttpResponse.json({ ...preferencesFixture, locale: "ja" })));
    await renderApp({ path: "/home", locale: "ja" });
    await screen.findByRole("heading", { level: 1, name: "ホーム" });
    const nav = screen.getByRole("navigation", { name: "メイン" });
    expect(within(nav).getByRole("link", { name: "ローカル" })).toHaveAttribute("href", "/local");
    expect(await within(nav).findByRole("link", { name: "連合" })).toHaveAttribute("href", "/public");
  });
});

describe("the user's arrangement", () => {
  test("orders and hides the items as kept on this browser, with the shown items after the first five in the menu's sheet", async () => {
    navigationKey.write({
      order: ["search", "notifications", "home", "profile", "public", "local", "bookmarks", "settings"],
      hidden: ["local"],
    });
    const { user } = await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });
    const list = within(navigation()).getByRole("list");
    await within(list).findByRole("link", { name: "Federated" });
    expect(
      within(list)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual(["/search", "/notifications", "/home", "/profile", "/public", "/bookmarks", "/settings"]);

    await user.click(within(navigation()).getByRole("button", { name: "Menu" }));
    const sheet = await screen.findByRole("dialog", { name: "Menu" });
    expect(
      within(sheet)
        .getAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["Bookmarks", "Settings", "my example", "More about this server"]);
  });

  test("keeps the place of a timeline the server does not offer among the first five", async () => {
    server.use(instanceWith({ local: "off", federated: "everyone" }));
    navigationKey.write({
      order: ["local", "home", "public", "notifications", "search", "profile", "bookmarks", "settings"],
      hidden: [],
    });
    const { user } = await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });
    await waitFor(() => {
      expect(within(navigation()).queryByRole("link", { name: "Local" })).not.toBeInTheDocument();
    });

    await user.click(within(navigation()).getByRole("button", { name: "Menu" }));
    const sheet = await screen.findByRole("dialog", { name: "Menu" });
    expect(
      within(sheet)
        .getAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["Profile", "Bookmarks", "Settings", "my example", "More about this server"]);
  });

  test("follows a change made in another tab", async () => {
    await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });
    expect(within(navigation()).getByRole("link", { name: "Search" })).toBeInTheDocument();

    const value = JSON.stringify({ v: 1, value: { order: [], hidden: ["search"] } });
    localStorage.setItem("my:display:navigation", value);
    act(() => {
      window.dispatchEvent(new StorageEvent("storage", { key: "my:display:navigation", newValue: value }));
    });
    expect(within(navigation()).queryByRole("link", { name: "Search" })).not.toBeInTheDocument();
  });
});

describe("the tab bar's Menu", () => {
  test("is a button, never the current item, that opens a sheet with the profile, the bookmarks and the settings", async () => {
    server.use(signedInHandler);
    const { user } = await renderApp({ path: "/settings" });
    await screen.findByRole("heading", { level: 1, name: "Settings" });

    const menu = within(navigation()).getByRole("button", { name: "Menu" });
    expect(menu).not.toHaveAttribute("aria-current");
    await user.click(menu);

    const sheet = await screen.findByRole("dialog", { name: "Menu" });
    expect(
      within(sheet)
        .getAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["Profile", "Bookmarks", "Settings", "my example", "More about this server"]);
    expect(within(sheet).getByRole("link", { name: "Settings" })).toHaveAttribute("aria-current", "page");
    expect(within(sheet).getByRole("link", { name: "Profile" })).not.toHaveAttribute("aria-current");

    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(menu).toHaveFocus();
    });
  });

  test("switches the color theme in place, and the sheet stays open", async () => {
    const { user } = await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });
    await user.click(within(navigation()).getByRole("button", { name: "Menu" }));
    const sheet = await screen.findByRole("dialog", { name: "Menu" });

    const theme = within(sheet).getByRole("radiogroup", { name: "Theme" });
    expect(theme).toHaveAttribute("aria-orientation", "horizontal");
    await user.click(within(theme).getByRole("radio", { name: "Dark" }));

    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    expect(themeKey.read()).toBe("dark");
    expect(screen.getByRole("dialog", { name: "Menu" })).toBeInTheDocument();
  });

  test("closes when one of its links is followed, and focus moves to the new page's heading", async () => {
    server.use(signedInHandler);
    const { user, router } = await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });

    await user.click(within(navigation()).getByRole("button", { name: "Menu" }));
    const sheet = await screen.findByRole("dialog", { name: "Menu" });
    await user.click(within(sheet).getByRole("link", { name: "Settings" }));

    const heading = await screen.findByRole("heading", { level: 1, name: "Settings" });
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(heading).toHaveFocus();
    });
    expect(router.state.location.pathname).toBe("/settings");

    // Back on the page it was opened from, the sheet stays closed.
    await user.click(within(navigation()).getByRole("link", { name: "Home" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Home" })).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  test("stays open while leaving unsaved changes is asked about, and closes once they are discarded", async () => {
    server.use(signedInHandler);
    const { user, router } = await renderApp({ path: "/settings/profile" });
    const displayName = await screen.findByRole("textbox", { name: "Display name" });
    await user.type(displayName, " Liddell");

    await user.click(within(navigation()).getByRole("button", { name: "Menu" }));
    await user.click(
      within(await screen.findByRole("dialog", { name: "Menu" })).getByRole("link", { name: "Settings" }),
    );
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Keep editing" }));
    expect(router.state.location.pathname).toBe("/settings/profile");
    expect(screen.getByRole("dialog", { name: "Menu" })).toBeInTheDocument();

    await user.click(within(screen.getByRole("dialog", { name: "Menu" })).getByRole("link", { name: "Settings" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Discard changes" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Settings" })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });

  test("marks the viewer's own profile as current in the sheet", async () => {
    server.use(signedInHandler);
    const { user } = await renderApp({ path: "/profile" });
    await screen.findByRole("heading", { level: 1, name: "Alice" });

    await user.click(within(navigation()).getByRole("button", { name: "Menu" }));
    const sheet = await screen.findByRole("dialog", { name: "Menu" });
    expect(within(sheet).getByRole("link", { name: "Profile" })).toHaveAttribute("aria-current", "page");
  });

  test("is in Japanese in the ja locale", async () => {
    server.use(
      signedInHandler,
      http.get(`${apiBase}/preferences`, () => HttpResponse.json({ ...preferencesFixture, locale: "ja" })),
    );
    const { user } = await renderApp({ path: "/home", locale: "ja" });
    const nav = await screen.findByRole("navigation", { name: "メイン" });
    await user.click(await within(nav).findByRole("button", { name: "メニュー" }));
    const sheet = await screen.findByRole("dialog", { name: "メニュー" });
    expect(within(sheet).getByRole("button", { name: "閉じる" })).toBeInTheDocument();
  });
});

describe("the sidebar's server and viewer", () => {
  test("names the server above the list and links the signed-in account below it, without changing the list", async () => {
    server.use(signedInHandler);
    await renderApp({ path: "/profile" });
    await screen.findByRole("heading", { level: 1, name: "Alice" });

    const nav = navigation();
    const viewer = await within(nav).findByRole("link", { name: "Alice @alice" });
    expect(viewer).toHaveAttribute("href", "/@alice");
    expect(viewer).toHaveAccessibleDescription("The account you are signed in with");
    // The list's Profile item marks the place; the viewer's link is a second way there.
    expect(viewer).not.toHaveAttribute("aria-current");
    expect(within(nav).getByRole("link", { name: "Profile" })).toHaveAttribute("aria-current", "page");

    // The server's name links to the root address, which opens the home timeline for a signed-in user.
    const name = within(nav).getByRole("link", { name: "my example" });
    expect(name).toHaveAttribute("href", "/");
    expect(name).toHaveAccessibleDescription("Top page of this server");
    expect(name).not.toHaveAttribute("aria-current");
    const list = within(nav).getByRole("list");
    expect(name.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(list.compareDocumentPosition(viewer) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(list).not.toContainElement(viewer);
    // The federated timeline joins the list once the instance says it serves it.
    await within(list).findByRole("link", { name: "Federated" });
    expect(
      within(list)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual(["/home", "/local", "/public", "/notifications", "/search", "/profile", "/bookmarks", "/settings"]);
  });

  test("shows no account while signed out, and the server's name leads to the landing page", async () => {
    server.resetHandlers();
    const { user, router } = await renderApp({ path: "/local" });
    await screen.findByRole("heading", { level: 1, name: "Local timeline" });
    expect(within(navigation()).queryByRole("link", { name: /@alice/ })).not.toBeInTheDocument();
    const name = within(navigation()).getByRole("link", { name: "my example" });
    expect(name).toHaveAccessibleDescription("Top page of this server");

    await user.click(name);
    expect(await screen.findByRole("heading", { level: 1, name: "my example" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/");
    // On the landing page, the name marks the place.
    expect(within(navigation()).getByRole("link", { name: "my example" })).toHaveAttribute("aria-current", "page");
  });

  test("names the server in the menu's sheet as a link to the root address", async () => {
    const { user } = await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });
    await user.click(await within(navigation()).findByRole("button", { name: "Menu" }));
    const sheet = await screen.findByRole("dialog", { name: "Menu" });
    const name = within(sheet).getByRole("link", { name: "my example" });
    expect(name).toHaveAttribute("href", "/");
    expect(name).toHaveAccessibleDescription("Top page of this server");
  });

  test("is in Japanese in the ja locale", async () => {
    server.use(
      signedInHandler,
      http.get(`${apiBase}/preferences`, () => HttpResponse.json({ ...preferencesFixture, locale: "ja" })),
    );
    await renderApp({ path: "/home", locale: "ja" });
    const nav = await screen.findByRole("navigation", { name: "メイン" });
    expect(await within(nav).findByRole("link", { name: "Alice @alice" })).toHaveAccessibleDescription(
      "ログインしているアカウント",
    );
    expect(within(nav).getByRole("link", { name: "my example" })).toHaveAccessibleDescription(
      "このサーバーのトップページ",
    );
  });
});
