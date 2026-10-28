// SPDX-License-Identifier: AGPL-3.0-only
// Integration tests of the app shell (web-client/README.md T4): the real router, providers and layout.
import { screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, test } from "vitest";
import { signedInHandler } from "./test/msw/http";
import { server } from "./test/msw/server";
import { renderApp } from "./test/render";

describe("app shell", () => {
  // The home timeline and the user's navigation need a session; routes/-layout/Navigation.test.tsx covers its absence.
  beforeEach(() => {
    server.use(signedInHandler);
  });

  test("opens at the home page with the navigation and one level-one heading in the main landmark", async () => {
    await renderApp();

    const heading = await screen.findByRole("heading", { level: 1, name: "Home" });
    expect(within(screen.getByRole("main")).getAllByRole("heading", { level: 1 })).toEqual([heading]);
    const nav = screen.getByRole("navigation", { name: "Main" });
    expect(within(nav).getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Local" })).not.toHaveAttribute("aria-current");
  });

  test("names the document after the first page without moving focus to it", async () => {
    await renderApp();

    const heading = await screen.findByRole("heading", { level: 1, name: "Home" });
    expect(document.title).toBe("Home – my example");
    expect(heading).not.toHaveFocus();
  });

  test("moves focus to the new page's heading and renames the document on navigation", async () => {
    const { user } = await renderApp();
    await screen.findByRole("heading", { level: 1, name: "Home" });

    const nav = screen.getByRole("navigation", { name: "Main" });
    await user.click(within(nav).getByRole("link", { name: "Local" }));

    const heading = await screen.findByRole("heading", { level: 1, name: "Local timeline" });
    // Focus moves when the router reports the page rendered, after the heading is in the document.
    await waitFor(() => {
      expect(heading).toHaveFocus();
    });
    expect(document.title).toBe("Local timeline – my example");
    expect(within(nav).getByRole("link", { name: "Local" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Home" })).not.toHaveAttribute("aria-current");
  });

  test("navigates from the keyboard", async () => {
    const { user } = await renderApp({ path: "/local" });
    await screen.findByRole("heading", { level: 1, name: "Local timeline" });

    within(screen.getByRole("navigation", { name: "Main" }))
      .getByRole("link", { name: "Settings" })
      .focus();
    await user.keyboard("{Enter}");

    const heading = await screen.findByRole("heading", { level: 1, name: "Settings" });
    await waitFor(() => {
      expect(heading).toHaveFocus();
    });
    expect(document.title).toBe("Settings – my example");
  });

  test("the skip link moves focus to the main landmark", async () => {
    const { user } = await renderApp();
    await screen.findByRole("heading", { level: 1, name: "Home" });

    await user.tab();
    const skip = screen.getByRole("link", { name: "Skip to main content" });
    expect(skip).toHaveFocus();
    await user.keyboard("{Enter}");

    expect(screen.getByRole("main")).toHaveFocus();
  });

  test("shows a not-found page inside the layout for an unknown address", async () => {
    await renderApp({ path: "/no-such-page" });

    expect(await screen.findByRole("heading", { level: 1, name: "Page not found" })).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "Main" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to the home timeline" })).toHaveAttribute("href", "/home");
    expect(document.title).toBe("Page not found – my example");
  });

  test("leads people who are not signed in from the not-found page to the landing page", async () => {
    server.resetHandlers();
    await renderApp({ path: "/no-such-page" });

    expect(await screen.findByRole("heading", { level: 1, name: "Page not found" })).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "Go to the top page" })).toHaveAttribute("href", "/");
    expect(screen.queryByRole("link", { name: "Go to the home timeline" })).not.toBeInTheDocument();
  });

  test("renders in the locale it is given, falling back to the closest catalog", async () => {
    await renderApp({ locale: "ja-JP" });

    expect(await screen.findByRole("heading", { level: 1, name: "ホーム" })).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "メイン" })).toBeInTheDocument();
  });
});
