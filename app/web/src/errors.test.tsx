// SPDX-License-Identifier: AGPL-3.0-only
// Integration tests of the error boundaries (web-client/README.md E1, web-client/16-look-states-and-wording.md G16): the app's layout, error components and
// router options, over a route tree of the test's own with a page whose loader fails, and over the app's routes
// with an API that fails in each of the ways the error tells apart; and the report of a page that fails.
import { createRootRouteWithContext, createRoute, createRouter, type RouterHistory } from "@tanstack/react-router";
import { act, screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test, vi } from "vitest";
import { ErrorReporter } from "./lib/errorReporting";
import type { RouterContext } from "./lib/query";
import { createAppRouter, routerOptions } from "./router";
import { AppLayout } from "./routes/-layout/AppLayout";
import { PlaceholderPage } from "./routes/-layout/PlaceholderPage";
import { NotFound, RootError } from "./routes/-layout/RouteStates";
import { preferencesFixture, statusPage, timelineStatusFixture } from "./test/fixtures";
import { apiBase, problem, resumableSession } from "./test/msw/http";
import { server } from "./test/msw/server";
import { renderApp } from "./test/render";

function HomePage() {
  return <PlaceholderPage titleKey="client:home.title" />;
}

function SettingsPage() {
  return <PlaceholderPage titleKey="client:settings.title" />;
}

/** A router whose "/broken" page fails to load `failures` times, then shows the settings placeholder. */
function routerWithBrokenPage(failures: number) {
  let remaining = failures;
  const rootRoute = createRootRouteWithContext<RouterContext>()({
    component: AppLayout,
    errorComponent: RootError,
    notFoundComponent: NotFound,
  });
  const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: "/", component: HomePage });
  const brokenRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/broken",
    staticData: { title: "client:settings.title" },
    loader: () => {
      if (remaining > 0) {
        remaining--;
        throw new Error("the page failed to load");
      }
    },
    component: SettingsPage,
  });
  const routeTree = rootRoute.addChildren([indexRoute, brokenRoute]);
  return ({ history, ...context }: RouterContext & { history: RouterHistory }) =>
    createRouter({ ...routerOptions, routeTree, context, history });
}

test("a page that fails shows the error inside the layout under the page's heading, focused and titled, and recovers on retry", async () => {
  const { user, router } = await renderApp({ path: "/", createRouter: routerWithBrokenPage(1) });
  await screen.findByRole("heading", { level: 1, name: "Home" });

  await act(() => router.navigate({ href: "/broken" }));

  const heading = await screen.findByRole("heading", { level: 2, name: "This page could not be shown" });
  const pageHeading = screen.getByRole("heading", { level: 1, name: "Settings" });
  // Focus moves when the router reports the page rendered, after the heading is in the document.
  await waitFor(() => {
    expect(pageHeading).toHaveFocus();
  });
  expect(document.title).toBe("Settings – my example");
  // A fault of the page itself: its message is not for the user.
  expect(screen.getByText("Try again, or come back later.")).toBeInTheDocument();
  expect(screen.queryByText("the page failed to load")).not.toBeInTheDocument();
  // The layout stays, so the user can go elsewhere.
  expect(screen.getByRole("navigation", { name: "Main" })).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Try again" }));

  await waitFor(() => {
    expect(heading).not.toBeInTheDocument();
  });
  expect(await screen.findByRole("heading", { level: 1, name: "Settings" })).toBeInTheDocument();
  expect(screen.queryByRole("heading", { name: "This page could not be shown" })).not.toBeInTheDocument();
});

test("a page that fails on the first load shows the error without moving focus", async () => {
  await renderApp({ path: "/broken", createRouter: routerWithBrokenPage(1) });

  const heading = await screen.findByRole("heading", { level: 1, name: "Settings" });
  expect(heading).not.toHaveFocus();
  expect(screen.getByRole("heading", { level: 2, name: "This page could not be shown" })).toBeInTheDocument();
  expect(screen.getByRole("navigation", { name: "Main" })).toBeInTheDocument();
});

describe("a page whose API fails", () => {
  /** Opens the search page signed in, then goes to the home timeline, whose first page `respond` answers. */
  async function openHome(respond: () => Response, locale = "en") {
    server.use(resumableSession(), http.get(`${apiBase}/timelines/home`, respond));
    const { user, router } = await renderApp({ path: "/search", locale });
    await screen.findByRole("heading", { level: 1 });
    await act(() => router.navigate({ to: "/home" }));
    return { user };
  }

  test.each([
    {
      kind: "a server error",
      respond: () => problem(500, "about:blank", "Internal Server Error"),
      heading: "Could not load this page",
      reason: "Internal Server Error",
      retry: true,
    },
    {
      kind: "no response",
      respond: () => HttpResponse.error(),
      heading: "Could not connect to the server",
      reason: "Could not reach the server. Check your connection and try again.",
      retry: true,
    },
    {
      kind: "a 404",
      respond: () => problem(404, "about:blank", "Timeline not found"),
      heading: "What this page shows was not found",
      reason: "Timeline not found",
      retry: false,
    },
    {
      kind: "a 403",
      respond: () => problem(403, "/api/v1/problems/forbidden", "You may not read this timeline"),
      heading: "You do not have permission to see this page",
      reason: "You may not read this timeline",
      retry: false,
    },
  ])(
    "for $kind keeps the page's heading and title, focused, and says what happened and why",
    async ({ respond, heading, reason, retry }) => {
      await openHome(respond);

      const pageHeading = await screen.findByRole("heading", { level: 1, name: "Home" });
      await waitFor(() => {
        expect(pageHeading).toHaveFocus();
      });
      expect(document.title).toBe("Home – my example");
      const errorHeading = await screen.findByRole("heading", { level: 2, name: heading });
      const main = screen.getByRole("main");
      expect(within(main).getByText(reason)).toBeInTheDocument();
      // A failure the page opened with is read, not announced (G16).
      expect(within(main).queryByRole("alert")).not.toBeInTheDocument();
      expect(within(main).queryByRole("button", { name: "Try again" }) !== null).toBe(retry);
      expect(screen.queryByRole("heading", { name: "Something went wrong" })).not.toBeInTheDocument();
      expect(errorHeading).toBeVisible();
    },
  );

  test("says in Japanese that it could not connect to the server", async () => {
    // The signed-in user's display language is Japanese too.
    server.use(http.get(`${apiBase}/preferences`, () => HttpResponse.json({ ...preferencesFixture, locale: "ja" })));
    await openHome(() => HttpResponse.error(), "ja");

    expect(await screen.findByRole("heading", { level: 1, name: "ホーム" })).toBeInTheDocument();
    expect(
      await screen.findByRole("heading", { level: 2, name: "サーバーに接続できませんでした" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("サーバーに接続できませんでした。接続を確かめて、もう一度試してください。"),
    ).toBeInTheDocument();
    expect(document.title).toBe("ホーム – my example");
  });

  test("keeps focus on the retry that fails again and alerts its reason, then loads the page and focuses its heading", async () => {
    let responses = [() => problem(500, "about:blank", "Internal Server Error"), () => HttpResponse.error()];
    const { user } = await openHome(() => {
      const [respond, ...rest] = responses;
      if (respond === undefined) return HttpResponse.json(statusPage([timelineStatusFixture]));
      responses = rest;
      return respond();
    });
    await screen.findByRole("heading", { level: 2, name: "Could not load this page" });

    const retry = screen.getByRole("button", { name: "Try again" });
    await user.click(retry);

    // The result of the user's action: an alert in place of the error, with the button still focused.
    const alert = await within(screen.getByRole("main")).findByRole("alert");
    expect(
      within(alert).getByRole("heading", { level: 2, name: "Could not connect to the server" }),
    ).toBeInTheDocument();
    expect(
      within(alert).getByText("Could not reach the server. Check your connection and try again."),
    ).toBeInTheDocument();
    expect(retry).toHaveFocus();

    await user.click(retry);

    expect(await screen.findByRole("feed", { name: "Home" })).toBeInTheDocument();
    const pageHeading = screen.getByRole("heading", { level: 1, name: "Home" });
    await waitFor(() => {
      expect(pageHeading).toHaveFocus();
    });
    expect(within(screen.getByRole("main")).queryByRole("alert")).not.toBeInTheDocument();
  });
});

test("reports a page that fails as a route_error with the pattern of its route (ADR-0103 §6)", async () => {
  // A local timeline whose post cannot be displayed: the page fails while it renders.
  server.use(http.get(`${apiBase}/timelines/local`, () => HttpResponse.json({ items: [null] })));
  const reports: unknown[] = [];
  server.use(
    http.post(`${apiBase}/client_errors`, async ({ request }) => {
      reports.push(await request.json());
      return new HttpResponse(null, { status: 204 });
    }),
  );
  // React and the router log the error.
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  vi.spyOn(console, "warn").mockImplementation(() => undefined);

  // The app's router, with a reporter that takes the test's stacks, which name files rather than URLs, for the app's own.
  await renderApp({
    path: "/local",
    createRouter: (options) =>
      createAppRouter({
        ...options,
        errors: new ErrorReporter({ api: options.api, apiOrigin: window.location.origin, isOwnStack: () => true }),
      }),
  });

  await waitFor(() => {
    expect(reports).toHaveLength(1);
  });
  expect(reports[0]).toMatchObject({ kind: "route_error", route: "/local" });
});
