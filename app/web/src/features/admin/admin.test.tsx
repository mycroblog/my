// SPDX-License-Identifier: AGPL-3.0-only
// The server administration screens and the staff's menus through the whole app (web-client/README.md T4, ADR-0058): a
// signed-in session from the default handlers, a viewer whose roles have the screen's permission (or not), and
// MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { expectSelectedTabControlsPanel } from "../../test/tabs";

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

describe("the staff's entry on the settings page and the mode's menu", () => {
  test("are not there for a user without staff permissions", async () => {
    await renderApp({ path: "/settings" });

    expect(await screen.findByRole("navigation", { name: "Settings pages" })).toBeInTheDocument();
    // The account section loads with the viewer; the menus would be shown by then.
    expect(await within(screen.getByRole("main")).findByText("@alice")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /^Admin and moderation/ })).not.toBeInTheDocument();
  });

  test("Administration links to the server administration screens whose permissions the user's roles have", async () => {
    server.use(operatorHandler(["manage_jobs"]));
    await renderApp({ path: "/admin" });

    const menu = await screen.findByRole("region", { name: "Administration" });
    expect(
      within(menu)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual(["/admin", "/admin/jobs"]);
    expect(await within(screen.getByRole("main")).findByRole("link", { name: "Jobs" })).toHaveAccessibleDescription(
      "Background jobs waiting, running, retrying and failed, why they failed, and running failed ones again.",
    );
    expect(screen.queryByRole("region", { name: "Moderation" })).not.toBeInTheDocument();
  });

  test("a moderator has Moderation alone, with the screens of their permissions", async () => {
    server.use(operatorHandler(["manage_reports", "manage_accounts", "view_audit"]));
    await renderApp({ path: "/moderation" });

    const menu = await screen.findByRole("region", { name: "Moderation" });
    expect(
      within(menu)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual([
      "/moderation",
      "/moderation/reports",
      "/moderation/appeals",
      "/moderation/accounts",
      "/moderation/registrations",
      "/moderation/signup-blocks",
      "/moderation/audit-log",
    ]);
    const main = within(screen.getByRole("main"));
    expect(await main.findByRole("link", { name: "Audit log" })).toHaveAccessibleDescription(
      "Who did what in the administration, and when.",
    );
    expect(main.getByRole("link", { name: "Registrations" })).toHaveAccessibleDescription(
      "Sign-ups that await approval, and approving or rejecting them.",
    );
    expect(main.getByRole("link", { name: "Appeals" })).toHaveAccessibleDescription(
      "Review appeals of moderation actions and uphold or overturn them.",
    );
    expect(screen.queryByRole("region", { name: "Administration" })).not.toBeInTheDocument();
  });

  test("an administrator has both, Moderation first", async () => {
    server.use(
      operatorHandler([
        "manage_reports",
        "manage_accounts",
        "manage_domains",
        "manage_rules",
        "manage_settings",
        "manage_roles",
        "manage_invites",
        "view_audit",
        "manage_jobs",
      ]),
    );
    await renderApp({ path: "/moderation" });

    const moderation = await screen.findByRole("region", { name: "Moderation" });
    const admin = screen.getByRole("region", { name: "Administration" });
    expect(moderation.compareDocumentPosition(admin) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(
      within(moderation)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual([
      "/moderation",
      "/moderation/reports",
      "/moderation/appeals",
      "/moderation/accounts",
      "/moderation/registrations",
      "/moderation/signup-blocks",
      "/moderation/invites",
      "/moderation/domain-actions",
      "/moderation/instances",
      "/moderation/audit-log",
    ]);
    expect(
      within(admin)
        .getAllByRole("link")
        .map((link) => link.getAttribute("href")),
    ).toEqual([
      "/admin",
      "/admin/settings",
      "/admin/rules",
      "/admin/theme",
      "/admin/documents",
      "/admin/relays",
      "/admin/roles",
      "/admin/jobs",
    ]);
  });
});

describe("the queue dashboard", () => {
  const deliver: Schemas["AdminJob"] = {
    id: "0192f0c4-7a8e-7000-8000-0000000000d1",
    type: "federation.deliver",
    state: "dead",
    payload: '{"delivery_id":"x"}',
    idempotency_key: "deliver:x",
    attempts: 9,
    last_error: "502 Bad Gateway",
    created_at: "2026-10-13T10:00:00Z",
    failed_at: "2026-10-17T10:00:00Z",
  };
  const cleanup: Schemas["AdminJob"] = {
    id: "0192f0c4-7a8e-7000-8000-0000000000d0",
    type: "kv.cleanup",
    state: "dead",
    payload: "null",
    attempts: 3,
    last_error: "db down",
    created_at: "2026-10-16T10:00:00Z",
    failed_at: "2026-10-16T11:00:00Z",
  };
  const retrying: Schemas["AdminJob"] = {
    id: "0192f0c4-7a8e-7000-8000-0000000000e1",
    type: "federation.deliver",
    state: "retrying",
    payload: '{"delivery_id":"y"}',
    ordering_key: "inbox:a.example",
    attempts: 2,
    last_error: "503 Service Unavailable",
    run_at: "2026-10-17T10:05:00Z",
    created_at: "2026-10-17T09:00:00Z",
  };
  const running: Schemas["AdminJob"] = {
    id: "0192f0c4-7a8e-7000-8000-0000000000e2",
    type: "media.process",
    state: "running",
    payload: '{"media_id":"m"}',
    attempts: 1,
    run_at: "2026-10-17T10:00:00Z",
    worker: "worker-7f3a",
    locked_until: "2026-10-17T10:05:00Z",
    created_at: "2026-10-17T10:00:00Z",
  };
  const queue: Schemas["AdminQueue"] = {
    adapter: "db",
    pending_listing: true,
    counts: [
      { state: "running", type: "media.process", count: 1 },
      { state: "retrying", type: "federation.deliver", count: 1 },
      { state: "dead", type: "federation.deliver", count: 1 },
      { state: "dead", type: "kv.cleanup", count: 1 },
    ],
    oldest_ready_at: "2026-10-17T09:30:00Z",
  };

  test("shows the queue's counts and its pending jobs, and the attempt history of a job when it is opened", async () => {
    const requests = recordAdminRequests();
    server.use(
      operatorHandler(["manage_jobs"]),
      http.get(`${apiBase}/admin/queue`, () => HttpResponse.json(queue)),
      http.get(`${apiBase}/admin/jobs`, ({ request }) =>
        HttpResponse.json({
          items: new URL(request.url).searchParams.get("state") === "dead" ? [deliver, cleanup] : [running, retrying],
        }),
      ),
      http.get(`${apiBase}/admin/jobs/:id/attempts`, () =>
        HttpResponse.json({
          items: [
            {
              attempt: 2,
              type: "federation.deliver",
              outcome: "retry",
              error: "503 Service Unavailable",
              worker: "worker-1",
              started_at: "2026-10-17T10:00:00Z",
              finished_at: "2026-10-17T10:00:01Z",
            },
            {
              attempt: 1,
              type: "federation.deliver",
              outcome: "retry",
              error: "connection refused",
              worker: "worker-2",
              started_at: "2026-10-17T09:00:00Z",
              finished_at: "2026-10-17T09:00:01Z",
            },
          ],
        }),
      ),
    );
    const { user } = await renderApp({ path: "/admin/jobs" });

    expect(await screen.findByRole("heading", { level: 1, name: "Jobs" })).toBeInTheDocument();
    const summary = screen.getByRole("region", { name: "Queue" });
    expect(within(summary).getByText("db")).toBeInTheDocument();
    const types = within(summary).getByRole("list", { name: "Jobs by type" });
    expect(
      within(types)
        .getAllByRole("heading", { level: 3 })
        .map((heading) => heading.textContent),
    ).toEqual(["media.process", "federation.deliver", "kv.cleanup"]);
    expect(screen.getByRole("tab", { name: "Pending", selected: true })).toBeInTheDocument();

    const jobs = screen.getByRole("tabpanel", { name: "Pending" });
    expect(within(jobs).getByRole("heading", { level: 3, name: "media.process" })).toBeInTheDocument();
    expect(within(jobs).getByText("worker-7f3a")).toBeInTheDocument();
    expect(within(jobs).getByText("503 Service Unavailable")).toBeInTheDocument();
    expect(within(jobs).queryByRole("button", { name: /^Run again/ })).not.toBeInTheDocument();
    expect(requests.filter((request) => request.includes("/attempts"))).toEqual([]);

    const history = within(jobs).getByRole("button", { name: "Attempt history: federation.deliver" });
    expect(history).toHaveAttribute("aria-expanded", "false");
    await user.click(history);
    expect(history).toHaveAttribute("aria-expanded", "true");
    expect(await within(jobs).findByRole("heading", { level: 4, name: "Attempt 2: retried" })).toBeInTheDocument();
    expect(within(jobs).getByText("connection refused")).toBeInTheDocument();
    expect(requests.filter((request) => request.includes("/attempts"))).toEqual([
      `GET /admin/jobs/${retrying.id}/attempts?limit=40`,
    ]);

    await user.click(history);
    expect(history).toHaveAttribute("aria-expanded", "false");
    expect(within(jobs).queryByText("connection refused")).not.toBeInTheDocument();
  });

  test("runs failed jobs again: each leaves the list, focus moves to the next one, and the result is announced", async () => {
    let dead = [deliver, cleanup];
    const retried: string[] = [];
    server.use(
      operatorHandler(["manage_jobs"]),
      http.get(`${apiBase}/admin/queue`, () => HttpResponse.json(queue)),
      http.get(`${apiBase}/admin/jobs`, () => HttpResponse.json({ items: dead })),
      http.post(`${apiBase}/admin/jobs/:id/retry`, ({ params }) => {
        const id = String(params["id"]);
        retried.push(id);
        dead = dead.filter((job) => job.id !== id);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/jobs?state=dead" });

    expect(await screen.findByRole("tab", { name: "Failed", selected: true })).toBeInTheDocument();
    const jobs = screen.getByRole("tabpanel", { name: "Failed" });
    expect(within(jobs).getByRole("heading", { level: 3, name: "federation.deliver" })).toBeInTheDocument();
    expect(within(jobs).getByText("502 Bad Gateway")).toBeInTheDocument();
    expect(within(jobs).getByText("deliver:x")).toBeInTheDocument();
    expect(within(jobs).getByText('{"delivery_id":"x"}')).toBeInTheDocument();

    await user.click(
      within(jobs).getByRole("button", { name: /^Run again: federation\.deliver, failed Oct 17, 2026/ }),
    );

    await waitFor(() => {
      expect(within(jobs).queryByRole("heading", { level: 3, name: "federation.deliver" })).not.toBeInTheDocument();
    });
    expect(within(jobs).getByRole("button", { name: /^Run again: kv\.cleanup/ })).toHaveFocus();
    expect(await screen.findByText("The job federation.deliver is queued to run again.")).toBeInTheDocument();

    await user.click(within(jobs).getByRole("button", { name: /^Run again: kv\.cleanup/ }));
    expect(await screen.findByRole("heading", { level: 3, name: "No failed jobs" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1 })).toHaveFocus();
    expect(retried).toEqual([deliver.id, cleanup.id]);
  });

  test("a job that cannot be run again stays, and the failure says why", async () => {
    server.use(
      operatorHandler(["manage_jobs"]),
      http.get(`${apiBase}/admin/jobs`, () => HttpResponse.json({ items: [deliver] })),
      http.post(`${apiBase}/admin/jobs/:id/retry`, () => problem(404, "about:blank", "Not Found")),
    );
    const { user } = await renderApp({ path: "/admin/jobs?state=dead" });

    await user.click(await screen.findByRole("button", { name: /^Run again: federation\.deliver/ }));

    expect(await screen.findByText("Could not run the job again: Not Found")).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 3, name: "federation.deliver" })).toBeInTheDocument();
  });

  test("the tabs put the state into the URL", async () => {
    const requests = recordAdminRequests();
    server.use(operatorHandler(["manage_jobs"]));
    const { user, router } = await renderApp({ path: "/admin/jobs" });

    await user.click(await screen.findByRole("tab", { name: "Retrying" }));
    expect(await screen.findByRole("tabpanel", { name: "Retrying" })).toBeInTheDocument();
    expectSelectedTabControlsPanel("Retrying");
    expect(router.state.location.search).toEqual({ state: "retrying" });
    expect(requests).toContain("GET /admin/jobs?limit=40&state=retrying");
  });

  test("a queue that cannot list its pending jobs says so, and still lists the failed ones", async () => {
    server.use(
      operatorHandler(["manage_jobs"]),
      http.get(`${apiBase}/admin/queue`, () =>
        HttpResponse.json({ adapter: "sqs", pending_listing: false, counts: [] }),
      ),
      http.get(`${apiBase}/admin/jobs`, () => HttpResponse.json({ items: [deliver] })),
    );
    const { user } = await renderApp({ path: "/admin/jobs" });

    expect(
      await screen.findByRole("heading", { level: 3, name: "This queue cannot list these jobs" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/This queue cannot list its waiting, running and retrying jobs/)).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "Failed" }));
    const jobs = await screen.findByRole("tabpanel", { name: "Failed" });
    expect(await within(jobs).findByRole("heading", { level: 3, name: "federation.deliver" })).toBeInTheDocument();
  });
});

describe("permissions", () => {
  test("a screen says why a user without its permission cannot use it, and asks the server for nothing", async () => {
    server.use(operatorHandler(["manage_domains"]));
    const requests = recordAdminRequests();
    await renderApp({ path: "/admin/jobs" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.getByText(/the permission manage_jobs/)).toBeInTheDocument();
    expect(requests).toEqual([]);
  });
});
