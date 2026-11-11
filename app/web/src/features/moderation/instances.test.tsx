// SPDX-License-Identifier: AGPL-3.0-only
// The remote servers' screens through the whole app (web-client/README.md T4, federation.md §7.3): a signed-in session
// from the default handlers, a viewer whose roles have manage_domains (or not), and MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { otherAccountFixture, type Schemas } from "../../test/fixtures";
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
  state_changed_at: "2026-10-10T08:00:00Z",
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

  test("opens the page of an unlisted server by its domain, a handle on it, or a URL on it, by keyboard", async () => {
    server.use(operatorHandler(["manage_domains"]));
    const requests = recordAdminRequests();
    const { user, router } = await renderApp({ path: "/moderation/instances" });

    const form = await screen.findByRole("search", { name: "Open a server" });
    const field = within(form).getByRole("textbox", { name: "Server" });
    await user.type(field, "https://Calm.Example./@alice{Enter}");
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/moderation/instances/calm.example");
    });
    expect(await screen.findByRole("heading", { level: 1, name: "calm.example" })).toBeInTheDocument();
    expect(requests).toContain("GET /admin/instances/calm.example");

    await router.navigate({ to: "/moderation/instances" });
    await user.type(await screen.findByRole("textbox", { name: "Server" }), "@bob@例え.jp{Enter}");
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/moderation/instances/xn--r8jz45g.jp");
    });
  });

  test("the form to open a server says what it needs and stays on the list", async () => {
    server.use(operatorHandler(["manage_domains"]));
    const { user, router } = await renderApp({ path: "/moderation/instances" });

    const form = await screen.findByRole("search", { name: "Open a server" });
    await user.click(within(form).getByRole("button", { name: "Open" }));
    expect(await within(form).findByText("Enter the server's domain.")).toBeInTheDocument();

    await user.type(within(form).getByRole("textbox", { name: "Server" }), "not a host");
    await user.click(within(form).getByRole("button", { name: "Open" }));
    expect(await within(form).findByText("Enter a domain, a handle, or a URL.")).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/moderation/instances");
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
        health = {
          domain: paused.domain,
          state: "available",
          state_changed_at: "2026-10-17T00:00:00Z",
          failure_days: 0,
        };
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
          state_changed_at: "2026-10-17T00:00:00Z",
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

const accepted: Schemas["AdminDelivery"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000d1",
  domain: "flaky.example",
  activity_type: "Accept",
  activity_created_at: "2026-11-10T01:37:12Z",
  created_at: "2026-11-10T01:37:12Z",
  shared_inbox: false,
  sender: { account_id: otherAccountFixture.id, username: "bob" },
  activity_uri: "https://my.example/activities/accept-1",
  inbox_url: "https://flaky.example/users/8ak9qsd92w/inbox",
  attempts: [
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000a1",
      outcome: "succeeded",
      signature_scheme: "cavage",
      status_code: 202,
      attempted_at: "2026-11-10T01:37:12Z",
    },
  ],
};
const masked: Schemas["AdminDelivery"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000d2",
  domain: "flaky.example",
  activity_type: "Create",
  activity_created_at: "2026-11-09T00:00:00Z",
  created_at: "2026-11-09T00:00:00Z",
  shared_inbox: false,
  mask: "direct",
  attempts: [
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000a2",
      outcome: "retryable_failure",
      error: "dial tcp: i/o timeout",
      attempted_at: "2026-11-09T00:00:00Z",
    },
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000a3",
      outcome: "permanent_failure",
      signature_scheme: "rfc9421",
      status_code: 410,
      attempted_at: "2026-11-09T01:00:00Z",
    },
  ],
};

describe("deliveries", () => {
  test("a server's page shows the deliveries with their attempts, masked ones without their sender, URI or inbox, and loads more", async () => {
    server.use(
      operatorHandler(["manage_domains"]),
      http.get(`${apiBase}/admin/instances/:domain/deliveries`, ({ request }) =>
        new URL(request.url).searchParams.get("cursor") === "next"
          ? HttpResponse.json({ items: [masked] })
          : HttpResponse.json({ items: [accepted], next_cursor: "next" }),
      ),
    );
    const requests = recordAdminRequests();
    const { user } = await renderApp({ path: "/moderation/instances/flaky.example" });

    const region = within(await screen.findByRole("region", { name: "Deliveries" }));
    const [first] = await region.findAllByRole("listitem");
    if (first === undefined) throw new Error("no delivery is listed");
    const delivery = within(first);
    expect(delivery.getByRole("heading", { level: 3, name: "Accept" })).toBeInTheDocument();
    expect(delivery.getByText("@bob")).toBeInTheDocument();
    expect(delivery.getByText("https://flaky.example/users/8ak9qsd92w/inbox")).toBeInTheDocument();
    expect(delivery.getByText("https://my.example/activities/accept-1")).toBeInTheDocument();
    const attempts = within(delivery.getByRole("list", { name: "Attempts" }));
    expect(attempts.getByText("Accepted")).toBeInTheDocument();
    expect(attempts.getByText("HTTP 202")).toBeInTheDocument();
    expect(attempts.getByText("Cavage signature")).toBeInTheDocument();
    expect(region.queryByRole("button", { name: "Show masked fields" })).not.toBeInTheDocument();

    await user.click(region.getByRole("button", { name: "Show more" }));
    await region.findByRole("heading", { level: 3, name: "Create" });
    const created = region
      .getAllByRole("listitem")
      .find((item) => within(item).queryByRole("heading", { level: 3, name: "Create" }) !== null);
    if (created === undefined) throw new Error("the masked delivery is not listed");
    const second = within(created);
    expect(second.getAllByText("Masked")).toHaveLength(3);
    expect(
      second.getByText(
        "The sender, the activity ID and the inbox are masked: the activity is about a post for mentioned people only.",
      ),
    ).toBeInTheDocument();
    expect(second.getByText("Failed, tried again")).toBeInTheDocument();
    expect(second.getByText("dial tcp: i/o timeout")).toBeInTheDocument();
    expect(second.getByText("Refused")).toBeInTheDocument();
    expect(second.getByText("HTTP 410")).toBeInTheDocument();
    expect(second.getByText("RFC 9421 signature")).toBeInTheDocument();
    expect(second.getByRole("button", { name: "Show masked fields" })).toHaveAccessibleDescription(
      "Showing them is recorded in the audit log.",
    );
    expect(requests.filter((request) => request.includes("/deliveries"))).toEqual([
      "GET /admin/instances/flaky.example/deliveries?limit=40",
      "GET /admin/instances/flaky.example/deliveries?limit=40&cursor=next",
    ]);
  });

  test("revealing a masked delivery asks the server, shows the fields, and says that it is recorded", async () => {
    server.use(
      operatorHandler(["manage_domains"]),
      http.get(`${apiBase}/admin/instances/:domain/deliveries`, () => HttpResponse.json({ items: [masked] })),
      http.post(`${apiBase}/admin/instances/:domain/deliveries/:id/reveal`, () =>
        HttpResponse.json({
          id: masked.id,
          mask: "direct",
          sender: { account_id: otherAccountFixture.id, username: "bob" },
          activity_uri: "https://my.example/activities/create-2",
          inbox_url: "https://flaky.example/users/carol/inbox",
        }),
      ),
    );
    const requests = recordAdminRequests();
    const { user } = await renderApp({ path: "/moderation/instances/flaky.example" });

    await user.click(await screen.findByRole("button", { name: "Show masked fields" }));

    expect(
      await screen.findByText("The masked fields are shown. Seeing them is recorded in the audit log."),
    ).toBeInTheDocument();
    expect(screen.getByText("@bob")).toBeInTheDocument();
    expect(screen.getByText("https://my.example/activities/create-2")).toBeInTheDocument();
    expect(screen.getByText("https://flaky.example/users/carol/inbox")).toBeInTheDocument();
    expect(screen.queryByText("Masked")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show masked fields" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 3, name: "Create" })).toHaveFocus();
    expect(requests).toContain(`POST /admin/instances/flaky.example/deliveries/${masked.id}/reveal`);
  });

  test("a failed reveal keeps the delivery masked and says why", async () => {
    server.use(
      operatorHandler(["manage_domains"]),
      http.get(`${apiBase}/admin/instances/:domain/deliveries`, () => HttpResponse.json({ items: [masked] })),
      http.post(`${apiBase}/admin/instances/:domain/deliveries/:id/reveal`, () =>
        problem(503, "about:blank", "Service Unavailable"),
      ),
    );
    const { user } = await renderApp({ path: "/moderation/instances/flaky.example" });

    await user.click(await screen.findByRole("button", { name: "Show masked fields" }));

    expect(await screen.findByText("Could not show the masked fields: Service Unavailable")).toBeInTheDocument();
    expect(screen.getAllByText("Masked")).toHaveLength(3);
    expect(screen.getByRole("button", { name: "Show masked fields" })).toBeInTheDocument();
  });

  test("the filter by account says it is recorded before it runs, and the filter by activity ID does not", async () => {
    server.use(
      operatorHandler(["manage_domains"]),
      http.get(`${apiBase}/accounts/lookup`, ({ request }) =>
        new URL(request.url).searchParams.get("acct") === "bob"
          ? HttpResponse.json(otherAccountFixture)
          : problem(404, "/api/v1/problems/not-found", "Not Found"),
      ),
      http.get(`${apiBase}/admin/instances/:domain/deliveries`, ({ request }) => {
        const query = new URL(request.url).searchParams;
        if (query.get("account_id") === otherAccountFixture.id) return HttpResponse.json({ items: [accepted] });
        if (query.get("activity_uri") !== null) return HttpResponse.json({ items: [] });
        return HttpResponse.json({ items: [accepted, masked] });
      }),
    );
    const requests = recordAdminRequests();
    const { user } = await renderApp({ path: "/moderation/instances/flaky.example" });

    const form = within(await screen.findByRole("search", { name: "Filter deliveries" }));
    const account = form.getByRole("textbox", { name: "Local account" });
    const activity = form.getByRole("textbox", { name: "Activity ID" });
    expect(account).toHaveAccessibleDescription(
      "The username of an account on this server. Each filter by an account is recorded in the audit log.",
    );
    expect(activity).not.toHaveAccessibleDescription(/audit log/u);
    expect(await screen.findByRole("heading", { level: 3, name: "Create" })).toBeInTheDocument();

    await user.type(activity, "https://my.example/activities/none");
    await user.click(form.getByRole("button", { name: "Filter" }));
    expect(
      await screen.findByRole("heading", { level: 3, name: "No deliveries match the filter" }),
    ).toBeInTheDocument();

    await user.click(form.getByRole("button", { name: "Clear filter" }));
    expect(await screen.findByRole("heading", { level: 3, name: "Create" })).toBeInTheDocument();
    expect(activity).toHaveValue("");

    await user.type(account, "bob@flaky.example");
    await user.click(form.getByRole("button", { name: "Filter" }));
    expect(
      await screen.findByText("Enter the username of an account on this server, without a domain."),
    ).toBeInTheDocument();

    await user.clear(account);
    await user.type(account, "@bob");
    await user.click(form.getByRole("button", { name: "Filter" }));
    await waitFor(() => {
      expect(screen.queryByRole("heading", { level: 3, name: "Create" })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("heading", { level: 3, name: "Accept" })).toBeInTheDocument();

    await user.clear(account);
    await user.type(account, "nobody");
    await user.click(form.getByRole("button", { name: "Filter" }));
    expect(await screen.findByText("No account on this server is named @nobody.")).toBeInTheDocument();

    expect(requests.filter((request) => request.includes("/deliveries"))).toEqual([
      "GET /admin/instances/flaky.example/deliveries?limit=40",
      `GET /admin/instances/flaky.example/deliveries?limit=40&activity_uri=${encodeURIComponent("https://my.example/activities/none")}`,
      `GET /admin/instances/flaky.example/deliveries?limit=40&account_id=${otherAccountFixture.id}`,
    ]);
  });

  test("says when nothing was delivered to the server", async () => {
    server.use(operatorHandler(["manage_domains"]));
    await renderApp({ path: "/moderation/instances/flaky.example" });

    expect(await screen.findByRole("heading", { level: 3, name: "No deliveries to this server" })).toBeInTheDocument();
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
