// SPDX-License-Identifier: AGPL-3.0-only
// Integration tests of the server's maintenance (ADR-0084 §3, web-client/09-app-shell.md §9 "メンテナンス", README.md T4):
// while the client API and the OAuth endpoints answer with the maintenance problem, the app shows the maintenance in
// place of every page, keeps the session, tries again after Retry-After, and loads the page again once the server
// answers. The page cannot reload in jsdom, so renderApp counts the reloads.
import { act, screen, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { resumableSession } from "./test/msw/http";
import { server } from "./test/msw/server";
import { renderApp } from "./test/render";

let underMaintenance = false;
/** The members the maintenance problem carries besides the standard ones. */
let members: Record<string, string> = {};
let retryAfter = "600";

/** The server's maintenance mode, before every other handler: the client API and the OAuth endpoints answer 503. */
const maintenanceMode = http.all("*", ({ request }) => {
  const { pathname } = new URL(request.url);
  if (!underMaintenance || !(pathname.startsWith("/api/") || pathname.startsWith("/oauth/"))) return undefined;
  return HttpResponse.json(
    { type: "/api/v1/problems/maintenance", title: "Under maintenance", status: 503, ...members },
    { status: 503, headers: { "Content-Type": "application/problem+json", "Retry-After": retryAfter, "Cache-Control": "no-store" } },
  );
});

beforeEach(() => {
  underMaintenance = false;
  members = {};
  retryAfter = "600";
});

describe("the maintenance screen", () => {
  test("replaces the page with what the server says, with its heading focused, and keeps the session", async () => {
    server.use(resumableSession());
    const { session, api, locale } = await renderApp({ path: "/local" });
    await screen.findByRole("feed", { name: "Local timeline" });
    // The user's preferences have set the time zone the maintenance's end is shown in.
    await waitFor(() => {
      expect(locale.getSnapshot().timeZone).toBe("UTC");
    });
    server.use(maintenanceMode);
    underMaintenance = true;
    members = { message: "Upgrading the database.", until: "2026-10-25T12:00:00Z", status_page_url: "https://status.my.example/" };

    // The next request of the page meets the maintenance.
    await act(() => api.fetch.GET("/instance").catch(() => undefined));

    const heading = await screen.findByRole("heading", { level: 1, name: "Under maintenance" });
    expect(heading).toHaveFocus();
    expect(screen.getByRole("main")).toContainElement(heading);
    expect(screen.queryByRole("feed")).not.toBeInTheDocument();
    expect(screen.queryByRole("navigation")).not.toBeInTheDocument();
    const notice = screen.getByText("Upgrading the database.");
    expect(notice).toHaveAttribute("dir", "auto");
    expect(notice).toHaveAttribute("lang", "");
    // In the user's time zone (UTC in the preferences fixture), not the browser's.
    const until = new Intl.DateTimeFormat("en", { dateStyle: "full", timeStyle: "short", timeZone: "UTC" }).format(Date.UTC(2026, 9, 25, 12));
    expect(screen.getByText(`Expected to end: ${until}`)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /See the status page/ })).toHaveAttribute("href", "https://status.my.example/");
    expect(screen.getByRole("button", { name: "Reload" })).toBeInTheDocument();
    expect(document.title).toBe("Under maintenance – my example");
    expect(session.status).toBe("signed-in");
    expect(session.accessToken).not.toBeNull();
  });

  test("shows only what the server set", async () => {
    server.use(maintenanceMode);
    underMaintenance = true;
    await renderApp({ path: "/local" });

    expect(await screen.findByRole("heading", { level: 1, name: "Under maintenance" })).toBeInTheDocument();
    expect(screen.queryByText(/Expected to end/)).not.toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  test("Reload says that the maintenance goes on, and loads the page again once it has ended", async () => {
    server.use(maintenanceMode);
    underMaintenance = true;
    const { user, reloads } = await renderApp({ path: "/local" });
    const reload = await screen.findByRole("button", { name: "Reload" });

    await user.click(reload);
    expect(await screen.findByText("The server is still under maintenance.")).toBeInTheDocument();
    expect(reloads.count).toBe(0);

    underMaintenance = false;
    await user.click(reload);
    await waitFor(() => {
      expect(reloads.count).toBe(1);
    });
  });

  test("tries the server again after Retry-After, and loads the page again when it answers", async () => {
    server.use(maintenanceMode);
    underMaintenance = true;
    retryAfter = "1";
    const { reloads } = await renderApp({ path: "/local" });
    await screen.findByRole("heading", { level: 1, name: "Under maintenance" });

    underMaintenance = false;
    await waitFor(
      () => {
        expect(reloads.count).toBe(1);
      },
      { timeout: 4000 },
    );
  });
});
