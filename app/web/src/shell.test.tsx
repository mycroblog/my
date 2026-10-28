// SPDX-License-Identifier: AGPL-3.0-only
// Integration tests of what the app shell reads on every page (web-client/09-app-shell.md §9, web-client/README.md T4): the viewer
// (GET /accounts/verify_credentials), the preferences (GET /preferences), the instance (GET /instance) and the
// notification marker (GET /notifications/marker). When one of them fails, a 429 of the rate limit included, the
// page goes on with the last value or a default, in the locale the app was last displayed in; a 429 says when
// the app loads again (Retry-After) and loads again then.
import { act, screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { preferencesFixture } from "./test/fixtures";
import { apiBase, problem, resumableSession } from "./test/msw/http";
import { server } from "./test/msw/server";
import { renderApp } from "./test/render";
import { loadStartI18n } from "./lib/locale";

const rateLimitedText = /^Too many requests were sent to the server\. What failed to load loads again at .+\.$/;

/** A 429 of the server's rate limit that lets a request through again after `seconds`. */
function rateLimited(seconds: number) {
  return problem(429, "about:blank", "Too many requests.", { headers: { "Retry-After": String(seconds) } });
}

/** Counts the requests to `path` of the client API. */
function countRequests(path: string): { readonly count: number } {
  const counter = { count: 0 };
  server.events.on("request:start", ({ request }) => {
    if (new URL(request.url).pathname === `${apiBase}${path}`) counter.count++;
  });
  return counter;
}

/** The bar at the top of the app that says when the rate-limited requests load again, or null. */
function rateLimitBar(): HTMLElement | null {
  return screen.queryAllByRole("status").find((region) => within(region).queryByText(rateLimitedText) !== null) ?? null;
}

beforeEach(() => {
  // The tests share one document; the server's shell writes the instance's locale into <html lang>.
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
  server.events.removeAllListeners("request:start");
});

describe("the display locale", () => {
  test("stays the user's while GET /preferences is rate-limited after a reload", async () => {
    server.use(
      resumableSession(),
      http.get(`${apiBase}/preferences`, () => HttpResponse.json({ ...preferencesFixture, locale: "ja" })),
    );
    // The instance's locale is English; the user's preferences say Japanese.
    const { unmount, session } = await renderApp({ path: "/home", locale: (await loadStartI18n(document)).locale });
    await screen.findByRole("heading", { level: 1, name: "ホーム" });
    expect(document.documentElement).toHaveAttribute("lang", "ja");

    // The page is loaded again from the server's shell, in the instance's locale, and the preferences are refused.
    unmount();
    // A reload leaves no tab behind to talk to over the session's BroadcastChannel.
    session.dispose();
    document.documentElement.lang = "en";
    server.use(http.get(`${apiBase}/preferences`, () => rateLimited(30)));
    const preferences = countRequests("/preferences");
    await renderApp({ path: "/home", locale: (await loadStartI18n(document)).locale });

    expect(await screen.findByRole("heading", { level: 1, name: "ホーム" })).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "メイン" })).toBeInTheDocument();
    // The new-post button mounts once the viewer has loaded; its form reads the same preferences, but does not
    // ask again for what the rate limit refused, so the bar stays as it is instead of going and coming back.
    expect(await screen.findAllByRole("button", { name: "投稿を書く" })).not.toHaveLength(0);
    const bar = await screen.findByText(/^サーバーへのリクエストが多すぎます。/);
    expect(preferences.count).toBe(1);
    expect(bar).toBeInTheDocument();
    expect(document.documentElement).toHaveAttribute("lang", "ja");
    expect(screen.queryByText("問題が発生しました")).not.toBeInTheDocument();
    expect(screen.queryByText("Something went wrong")).not.toBeInTheDocument();
  });

  test("starts in the instance's locale from <html lang> on the first visit", async () => {
    expect((await loadStartI18n(document)).locale).toBe("en");
    document.documentElement.lang = "ja";
    expect((await loadStartI18n(document)).locale).toBe("ja");
  });
});

describe("a request of the shell that fails", () => {
  const requests = [
    { name: "the viewer", path: "/accounts/verify_credentials" },
    { name: "the preferences", path: "/preferences" },
    { name: "the instance", path: "/instance" },
    { name: "the notification marker", path: "/notifications/marker" },
  ] as const;

  test.each(
    requests.flatMap((request) => [
      {
        ...request,
        failure: "a server error",
        respond: () => problem(500, "about:blank", "Internal Server Error"),
        limited: false,
      },
      { ...request, failure: "a 429", respond: () => rateLimited(30), limited: true },
    ]),
  )("leaves the page and the navigation in place: $name with $failure", async ({ path, respond, limited }) => {
    server.use(resumableSession(), http.get(`${apiBase}${path}`, respond));
    await renderApp({ path: "/local" });

    expect(await screen.findByRole("feed", { name: "Local timeline" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Local timeline" })).toBeInTheDocument();
    const navigation = screen.getByRole("navigation", { name: "Main" });
    expect(within(navigation).getByRole("link", { name: "Home" })).toBeInTheDocument();
    expect(within(navigation).getByRole("link", { name: "Local" })).toHaveAttribute("aria-current", "page");
    expect(screen.queryByText("Something went wrong")).not.toBeInTheDocument();
    if (limited) {
      await waitFor(() => {
        expect(rateLimitBar()).not.toBeNull();
      });
    } else {
      expect(rateLimitBar()).toBeNull();
    }
  });

  test("keeps the values loaded before while a refetch is rate-limited", async () => {
    server.use(resumableSession());
    const { queryClient } = await renderApp({ path: "/settings" });
    const account = await screen.findByRole("region", { name: "Account" });
    await within(account).findByText("Alice");
    const navigation = screen.getByRole("navigation", { name: "Main" });
    await within(navigation).findByRole("link", { name: "Notifications, 1 unread" });

    server.use(
      http.get(`${apiBase}/accounts/verify_credentials`, () => rateLimited(30)),
      http.get(`${apiBase}/notifications/marker`, () => rateLimited(30)),
      http.get(`${apiBase}/instance`, () => rateLimited(30)),
      http.get(`${apiBase}/preferences`, () => rateLimited(30)),
    );
    await act(() => queryClient.invalidateQueries());

    await waitFor(() => {
      expect(rateLimitBar()).not.toBeNull();
    });
    expect(within(account).getByText("Alice")).toBeInTheDocument();
    expect(within(account).queryByText("Could not load your account")).not.toBeInTheDocument();
    expect(within(navigation).getByRole("link", { name: "Notifications, 1 unread" })).toBeInTheDocument();
    expect(within(navigation).getByRole("link", { name: "Profile" })).toBeInTheDocument();
  });
});

describe("a 429 of the rate limit", () => {
  test("says when the app loads again, does not retry before then, and loads again at that time", async () => {
    let limited = true;
    server.use(
      resumableSession(),
      http.get(`${apiBase}/notifications/marker`, () => (limited ? rateLimited(2) : undefined)),
    );
    const markers = countRequests("/notifications/marker");
    await renderApp({ path: "/local" });
    await screen.findByRole("feed", { name: "Local timeline" });

    await waitFor(() => {
      expect(rateLimitBar()).not.toBeNull();
    });
    // The time is the moment Retry-After names, as the time of day with seconds.
    expect(rateLimitBar()).toHaveTextContent(/at \d{1,2}:\d{2}:\d{2}/);
    expect(markers.count).toBe(1);
    const navigation = screen.getByRole("navigation", { name: "Main" });
    expect(within(navigation).getByRole("link", { name: "Notifications" })).toBeInTheDocument();

    limited = false;
    // About two seconds later, the marker loads again by itself and the bar goes.
    expect(await within(navigation).findByRole("link", { name: "Notifications, 1 unread" })).toBeInTheDocument();
    expect(markers.count).toBe(2);
    expect(rateLimitBar()).toBeNull();
  });
});

describe("the account section of the settings page", () => {
  test("shows that the account could not be loaded after one more try, and loads it on retry", async () => {
    let failing = true;
    server.use(
      resumableSession(),
      http.get(`${apiBase}/accounts/verify_credentials`, () =>
        failing ? problem(500, "about:blank", "Internal Server Error") : undefined,
      ),
    );
    const viewers = countRequests("/accounts/verify_credentials");
    const { user } = await renderApp({ path: "/settings" });
    const account = await screen.findByRole("region", { name: "Account" });

    expect(
      await within(account).findByRole("heading", { level: 3, name: "Could not load your account" }),
    ).toBeInTheDocument();
    expect(within(account).getByText("Internal Server Error")).toBeInTheDocument();
    // The first request and one more try.
    expect(viewers.count).toBe(2);

    failing = false;
    await user.click(within(account).getByRole("button", { name: "Try again" }));

    expect(await within(account).findByText("Alice")).toBeInTheDocument();
    expect(within(account).queryByText("Could not load your account")).not.toBeInTheDocument();
  });
});
