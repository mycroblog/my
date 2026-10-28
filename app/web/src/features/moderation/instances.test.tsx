// SPDX-License-Identifier: AGPL-3.0-only
// The remote servers' screens through the whole app (web-client/README.md T4, federation.md §7.3): a signed-in session
// from the default handlers, a viewer whose roles have manage_domains (or not), and MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { expectSignInStarted } from "../../test/signIn";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

/** The admin requests the app sent, as "METHOD path?query". */
function recordAdminRequests() {
  const requests: string[] = [];
  server.events.on("request:start", ({ request }) => {
    const url = new URL(request.url);
    if (url.pathname.startsWith(`${apiBase}/admin/`))
      requests.push(`${request.method} ${url.pathname.slice(apiBase.length)}${url.search}`);
  });
  return requests;
}

const paused: Schemas["AdminInstanceHealth"] = {
  domain: "down.example",
  state: "unavailable",
  since: "2026-10-10T08:00:00Z",
  failure_days: 7,
  last_failure_day: "2026-10-10",
};
const flaky: Schemas["AdminInstanceHealth"] = {
  domain: "flaky.example",
  state: "available",
  failure_days: 2,
  last_failure_day: "2026-10-16",
};

describe("remote servers", () => {
  test("lists the servers with their state and failures, filters them by state in the URL, and loads more", async () => {
    server.use(
      operatorHandler(["manage_domains"]),
      http.get(`${apiBase}/admin/instances`, ({ request }) => {
        const query = new URL(request.url).searchParams;
        if (query.get("state") === "unavailable") return HttpResponse.json({ items: [paused] });
        return query.get("cursor") === "next"
          ? HttpResponse.json({ items: [flaky] })
          : HttpResponse.json({ items: [paused], next_cursor: "next" });
      }),
    );
    const requests = recordAdminRequests();
    const { user, router } = await renderApp({ path: "/moderation/instances" });

    expect(await screen.findByRole("heading", { level: 1, name: "Remote servers" })).toBeInTheDocument();
    const [row] = within(await screen.findByRole("table"))
      .getAllByRole("row")
      .slice(1);
    if (row === undefined) throw new Error("no server is listed");
    const details = within(row);
    expect(details.getByRole("rowheader", { name: "down.example" })).toBeInTheDocument();
    expect(details.getByRole("link", { name: "down.example" })).toHaveAttribute(
      "href",
      "/moderation/instances/down.example",
    );
    expect(details.getByText("Paused")).toBeInTheDocument();
    expect(details.getByText("7 days")).toBeInTheDocument();
    expect(details.getByText("Oct 10, 2026")).toBeInTheDocument();

    await user.click(within(screen.getByRole("navigation", { name: "Pages" })).getByRole("button", { name: "Next" }));
    expect(await screen.findByRole("rowheader", { name: "flaky.example" })).toBeInTheDocument();
    expect(screen.getByText("2 days")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Server state/ }));
    await user.click(await screen.findByRole("option", { name: "Paused" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ state: "unavailable" });
    });
    expect(await screen.findByRole("rowheader", { name: "down.example" })).toBeInTheDocument();
    expect(screen.queryByRole("rowheader", { name: "flaky.example" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Server state/ })).toHaveTextContent("Paused");
    expect(requests).toEqual([
      "GET /admin/instances?limit=40",
      "GET /admin/instances?limit=40&cursor=next",
      "GET /admin/instances?limit=40&state=unavailable",
    ]);
  });

  test("says when no server has failed deliveries", async () => {
    server.use(operatorHandler(["manage_domains"]));
    await renderApp({ path: "/moderation/instances" });

    expect(
      await screen.findByRole("heading", { level: 2, name: "No servers with failed deliveries" }),
    ).toBeInTheDocument();
  });

  test("a server's page shows why it is paused and its history, and resumes deliveries", async () => {
    let health = paused;
    let events: Schemas["AdminAvailabilityEvent"][] = [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e2",
        kind: "marked_unavailable",
        reason: "failure_days",
        occurred_at: "2026-10-10T08:00:00Z",
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e1",
        kind: "marked_available",
        reason: "probe_succeeded",
        occurred_at: "2026-09-01T00:00:00Z",
      },
    ];
    server.use(
      operatorHandler(["manage_domains"]),
      http.get(`${apiBase}/admin/instances/:domain`, () => HttpResponse.json(health)),
      http.get(`${apiBase}/admin/instances/:domain/availability_events`, () => HttpResponse.json({ items: events })),
      http.post(`${apiBase}/admin/instances/:domain/mark_available`, () => {
        health = { domain: paused.domain, state: "available", since: "2026-10-17T00:00:00Z", failure_days: 0 };
        events = [
          {
            id: "0192f0c4-7a8e-7000-8000-0000000000e3",
            kind: "marked_available",
            reason: "operator",
            occurred_at: "2026-10-17T00:00:00Z",
          },
          ...events,
        ];
        return HttpResponse.json(health);
      }),
    );
    const requests = recordAdminRequests();
    const { user } = await renderApp({ path: "/moderation/instances/down.example" });

    expect(await screen.findByRole("heading", { level: 1, name: "down.example" })).toBeInTheDocument();
    expect(screen.getByText("Paused")).toBeInTheDocument();
    // The reason of the newest event, and in the history.
    expect(screen.getAllByText("Deliveries failed on 7 different days.")).toHaveLength(2);
    expect(screen.getByRole("heading", { level: 2, name: "History" })).toBeInTheDocument();
    const history = screen.getByRole("table", { name: "History" });
    expect(
      within(history)
        .getAllByRole("rowheader")
        .map((header) => header.textContent),
    ).toEqual(["Deliveries paused", "Deliveries resumed"]);
    expect(within(history).getByText("The daily check reached the server.")).toBeInTheDocument();

    const resume = screen.getByRole("button", { name: "Resume deliveries" });
    expect(resume).toHaveAccessibleDescription("Clears the failed days, so that the server gets every delivery again.");
    await user.click(resume);

    expect(await screen.findByText("Deliveries to down.example resume.")).toBeInTheDocument();
    expect(screen.getByText("Delivering")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Resume deliveries" })).not.toBeInTheDocument();
    expect(await within(history).findByText("An operator resumed deliveries.")).toBeInTheDocument();
    expect(requests).toContain("POST /admin/instances/down.example/mark_available");
  });

  // #471: the list, shown from its cache as the route loads it, must not show the server as it was before
  // deliveries resumed while a slow server answers the list again.
  test("back on the list after resuming deliveries, the server is gone from it at once, however slowly the list comes", async () => {
    let failing = [paused, flaky];
    server.use(
      operatorHandler(["manage_domains"]),
      http.get(`${apiBase}/admin/instances`, () => HttpResponse.json({ items: failing })),
      http.get(`${apiBase}/admin/instances/:domain`, ({ params }) =>
        HttpResponse.json(failing.find((health) => health.domain === params.domain)),
      ),
      http.get(`${apiBase}/admin/instances/:domain/availability_events`, () => HttpResponse.json({ items: [] })),
      http.post(`${apiBase}/admin/instances/:domain/mark_available`, () => {
        failing = [flaky];
        return HttpResponse.json({
          domain: paused.domain,
          state: "available",
          since: "2026-10-17T00:00:00Z",
          failure_days: 0,
        });
      }),
    );
    const requests = recordAdminRequests();
    const { user } = await renderApp({ path: "/moderation/instances" });
    await user.click(await screen.findByRole("link", { name: "down.example" }));
    const resume = await screen.findByRole("button", { name: "Resume deliveries" });
    // From now on the list takes a while; the handler falls through to the one above.
    server.use(
      http.get(`${apiBase}/admin/instances`, async () => {
        await delay(300);
      }),
    );
    await user.click(resume);
    expect(await screen.findByText("Deliveries to down.example resume.")).toBeInTheDocument();

    await user.click(screen.getByRole("link", { name: "Back to remote servers" }));
    expect(await screen.findByRole("rowheader", { name: "flaky.example" })).toBeInTheDocument();
    expect(screen.queryByRole("rowheader", { name: "down.example" })).not.toBeInTheDocument();
    expect(requests.filter((request) => request.startsWith("GET /admin/instances?"))).toEqual([
      "GET /admin/instances?limit=40",
      "GET /admin/instances?limit=40",
    ]);
  });

  test("a failure to resume deliveries keeps the server's state and says why", async () => {
    server.use(
      operatorHandler(["manage_domains"]),
      http.get(`${apiBase}/admin/instances/:domain`, () => HttpResponse.json(paused)),
      http.post(`${apiBase}/admin/instances/:domain/mark_available`, () =>
        problem(503, "about:blank", "Service Unavailable"),
      ),
    );
    const { user } = await renderApp({ path: "/moderation/instances/down.example" });

    await user.click(await screen.findByRole("button", { name: "Resume deliveries" }));

    expect(await screen.findByText("Could not resume deliveries: Service Unavailable")).toBeInTheDocument();
    expect(screen.getByText("Paused")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Resume deliveries" })).toBeInTheDocument();
  });
});

describe("permissions", () => {
  test("a refusal of the server for a missing permission is shown as such", async () => {
    server.use(
      operatorHandler(["manage_domains"]),
      http.get(`${apiBase}/admin/instances`, () => problem(403, "/api/v1/problems/missing-permission", "Forbidden")),
    );
    await renderApp({ path: "/moderation/instances" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Remote servers" })).toBeInTheDocument();
  });

  test("a signed-out user is sent to sign in first", async () => {
    server.resetHandlers();
    const { leftTo } = await renderApp({ path: "/moderation/instances" });

    await expectSignInStarted(leftTo, "/moderation/instances");
  });
});
