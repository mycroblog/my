// SPDX-License-Identifier: AGPL-3.0-only
// The errors screens through the whole app (web-client/README.md T4, ADR-0103 §8): an administrator with manage_errors
// lists the error groups and filters them in the URL, opens one, copies the IDs of its latest occurrence, reads its
// hourly counts and history, and resolves, mutes and unmutes it; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { errorGroupDetailFixture, errorGroupFixture } from "./test/handlers";

const groupId = "0199f0e0-0000-7000-8000-0000000000e1";
const otherId = "0199f0e0-0000-7000-8000-0000000000e2";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
  server.use(signedInHandler, operatorHandler(["manage_errors"]));
});

describe("the errors screen", () => {
  test("tells a viewer without the permission why it is empty, without asking for the groups", async () => {
    const asked: string[] = [];
    server.use(
      operatorHandler(["manage_settings"]),
      http.get(`${apiBase}/admin/error_groups`, ({ request }) => {
        asked.push(request.url);
        return HttpResponse.json({ items: [] });
      }),
    );
    await renderApp({ path: "/admin/errors" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(asked).toEqual([]);
  });

  test("is in the administration menu of a viewer with manage_errors", async () => {
    await renderApp({ path: "/admin" });

    expect(await within(await screen.findByRole("main")).findByRole("link", { name: /^Errors/u })).toHaveAttribute(
      "href",
      "/admin/errors",
    );
  });

  test("lists the open groups, and filters them by state and source in the URL", async () => {
    const queries: string[] = [];
    server.use(
      http.get(`${apiBase}/admin/error_groups`, ({ request }) => {
        const params = new URL(request.url).searchParams;
        params.delete("limit");
        queries.push(params.toString());
        if (params.get("state") === "muted") return HttpResponse.json({ items: [] });
        return HttpResponse.json({
          items: [
            errorGroupFixture(groupId),
            errorGroupFixture(otherId, {
              source: "web",
              kind: "error",
              type: "TypeError",
              where: "/@{$acct}",
              count_24h: 1,
            }),
          ],
        });
      }),
    );
    const { user, router } = await renderApp({ path: "/admin/errors" });

    const table = await screen.findByRole("table", { name: "Error groups" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);
    const first = within(rows[0] as HTMLElement);
    expect(first.getByRole("link", { name: "runtime.Error" })).toHaveAttribute("href", `/admin/errors/${groupId}`);
    expect(first.getByText("client_api GET /api/v1/statuses/{id}")).toHaveAttribute("dir", "ltr");
    expect(first.getByText("Open")).toBeInTheDocument();
    expect(first.getByText("Server")).toBeInTheDocument();
    expect(first.getByText("3 times in 24 hours")).toBeInTheDocument();
    expect(first.getByText("12 times in 7 days")).toBeInTheDocument();
    expect(first.getByText("Version ⁦0.3.1⁩")).toBeInTheDocument();
    expect(first.getByText("Version ⁦0.3.0⁩")).toBeInTheDocument();
    const second = within(rows[1] as HTMLElement);
    expect(second.getByText("Web client")).toBeInTheDocument();
    expect(second.getByText("1 time in 24 hours")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Source/u }));
    await user.click(await screen.findByRole("option", { name: "Web client" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ source: "web" });
    });
    await user.click(screen.getByRole("button", { name: /State/u }));
    await user.click(await screen.findByRole("option", { name: "Muted" }));
    expect(await screen.findByRole("heading", { name: "No errors match these filters" })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ state: "muted", source: "web" });
    expect(queries).toEqual(["state=open", "state=open&source=web", "state=muted&source=web"]);
  });

  test("says so when no group is open", async () => {
    await renderApp({ path: "/admin/errors" });

    expect(await screen.findByRole("heading", { name: "No open errors" })).toBeInTheDocument();
  });
});

describe("the page of an error group", () => {
  test("shows the group, its latest occurrence with copyable IDs, its hourly counts and its history", async () => {
    server.use(
      http.get(`${apiBase}/admin/error_groups/:id`, () =>
        HttpResponse.json(
          errorGroupDetailFixture(groupId, {
            state: "resolved",
            resolved_version: "0.3.0",
            events: [
              { kind: "resolved", actor: "user", version: "0.3.0", occurred_at: "2026-10-20T03:00:00Z" },
              { kind: "muted", actor: "cli", occurred_at: "2026-10-20T04:00:00Z" },
            ],
          }),
        ),
      ),
    );
    const { user } = await renderApp({ path: `/admin/errors/${groupId}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Error" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "All errors" })).toHaveAttribute("href", "/admin/errors");
    const state = screen.getByRole("region", { name: "State" });
    expect(within(state).getByText("Resolved")).toBeInTheDocument();
    expect(within(state).getByText("0.3.0")).toBeInTheDocument();
    // A resolved group can be muted, and is not resolved again.
    expect(within(state).queryByRole("button", { name: "Resolve" })).not.toBeInTheDocument();
    expect(within(state).getByRole("button", { name: "Mute" })).toBeInTheDocument();

    const details = screen.getByRole("region", { name: "About this error" });
    expect(within(details).getByText("0123456789abcdef0123456789abcdef")).toBeInTheDocument();
    expect(within(details).getByText("Crash")).toBeInTheDocument();

    const sample = screen.getByRole("region", { name: "Latest occurrence" });
    expect(within(sample).getByText(/internal\/app\/status\/get\.go:42/u)).toBeInTheDocument();
    await user.click(within(sample).getByRole("button", { name: "Copy the request ID" }));
    // user-event's clipboard stands in for the browser's.
    expect(await navigator.clipboard.readText()).toBe("01JB0000000000000000000000");
    expect(await screen.findByText("Copied")).toBeInTheDocument();
    await user.click(within(sample).getByRole("button", { name: "Copy the trace ID" }));
    expect(await navigator.clipboard.readText()).toBe("4bf92f3577b34da6a3ce929d0e0e4736");

    const hourly = screen.getByRole("table", { name: "Occurrences by hour" });
    const hours = within(hourly).getAllByRole("row").slice(1);
    // Newest first.
    expect(hours.map((row) => within(row).getAllByRole("cell")[0]?.textContent)).toEqual(["3", "9"]);

    const history = screen.getByRole("table", { name: "History" });
    const changes = within(history)
      .getAllByRole("rowheader")
      .map((cell) => cell.textContent);
    expect(changes).toEqual(["Muted", "Resolved (version ⁦0.3.0⁩)"]);
    expect(within(history).getByText("The command line")).toBeInTheDocument();
  });

  test("resolves an open group, says so, and moves focus to the state's heading", async () => {
    let state: "open" | "resolved" = "open";
    const sent: string[] = [];
    server.use(
      http.get(`${apiBase}/admin/error_groups/:id`, () =>
        HttpResponse.json(
          errorGroupDetailFixture(
            groupId,
            state === "resolved"
              ? {
                  state,
                  resolved_version: "0.3.1",
                  events: [{ kind: "resolved", actor: "user", version: "0.3.1", occurred_at: "2026-10-21T06:00:00Z" }],
                }
              : {},
          ),
        ),
      ),
      http.post(`${apiBase}/admin/error_groups/:id/resolve`, ({ params }) => {
        sent.push(String(params["id"]));
        state = "resolved";
        return HttpResponse.json(errorGroupFixture(groupId, { state, resolved_version: "0.3.1" }));
      }),
    );
    const { user } = await renderApp({ path: `/admin/errors/${groupId}` });

    const section = await screen.findByRole("region", { name: "State" });
    await user.click(within(section).getByRole("button", { name: "Resolve" }));

    expect(await within(section).findByRole("status")).toHaveTextContent("The error is resolved.");
    expect(sent).toEqual([groupId]);
    expect(within(section).getByRole("heading", { name: "State" })).toHaveFocus();
    expect(within(section).queryByRole("button", { name: "Resolve" })).not.toBeInTheDocument();
    expect(within(screen.getByRole("table", { name: "History" })).getByRole("rowheader")).toHaveTextContent(
      "Resolved (version ⁦0.3.1⁩)",
    );
  });

  test("mutes and unmutes a group", async () => {
    let state: "open" | "muted" = "open";
    server.use(
      http.get(`${apiBase}/admin/error_groups/:id`, () =>
        HttpResponse.json(errorGroupDetailFixture(groupId, { state })),
      ),
      http.post(`${apiBase}/admin/error_groups/:id/mute`, () => {
        state = "muted";
        return HttpResponse.json(errorGroupFixture(groupId, { state }));
      }),
      http.post(`${apiBase}/admin/error_groups/:id/unmute`, () => {
        state = "open";
        return HttpResponse.json(errorGroupFixture(groupId, { state }));
      }),
    );
    const { user } = await renderApp({ path: `/admin/errors/${groupId}` });

    const section = await screen.findByRole("region", { name: "State" });
    await user.click(within(section).getByRole("button", { name: "Mute" }));
    expect(await within(section).findByRole("status")).toHaveTextContent("The error is muted.");
    expect(within(section).getByText("Muted")).toBeInTheDocument();
    expect(within(section).queryByRole("button", { name: "Resolve" })).not.toBeInTheDocument();

    await user.click(within(section).getByRole("button", { name: "Unmute" }));
    expect(await within(section).findByRole("status")).toHaveTextContent("The error is unmuted.");
    expect(within(section).getByRole("button", { name: "Resolve" })).toBeInTheDocument();
  });

  test("keeps the group and its buttons when an action fails, and says why", async () => {
    server.use(
      http.post(`${apiBase}/admin/error_groups/:id/mute`, () =>
        HttpResponse.json(
          { type: "about:blank", title: "Service Unavailable", status: 503, detail: "Try again later." },
          { status: 503, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: `/admin/errors/${groupId}` });

    const section = await screen.findByRole("region", { name: "State" });
    await user.click(within(section).getByRole("button", { name: "Mute" }));

    expect(await screen.findByText(/^The error could not be changed/u)).toBeInTheDocument();
    expect(within(section).getByText("Open")).toBeInTheDocument();
    expect(within(section).getByRole("button", { name: "Mute" })).toBeInTheDocument();
  });

  test("says when the occurrence had no IDs and no occurrence is kept", async () => {
    server.use(
      http.get(`${apiBase}/admin/error_groups/:id`, ({ params }) => {
        if (params["id"] === otherId) {
          const { sample: _, ...rest } = errorGroupDetailFixture(otherId);
          return HttpResponse.json(rest);
        }
        const sample = {
          type: "TypeError",
          message: "x is undefined",
          stack: "TypeError: x is undefined",
          version: "0.3.1",
          role: "web",
          occurred_at: "2026-10-21T05:30:00Z",
        };
        return HttpResponse.json(errorGroupDetailFixture(groupId, { sample, hourly_counts: [] }));
      }),
    );
    const { router } = await renderApp({ path: `/admin/errors/${groupId}` });

    const sample = await screen.findByRole("region", { name: "Latest occurrence" });
    expect(within(sample).getAllByText("None")).toHaveLength(2);
    expect(within(sample).queryByRole("button", { name: "Copy the request ID" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "No occurrences in the last 90 days" })).toBeInTheDocument();

    await router.navigate({ to: "/admin/errors/$id", params: { id: otherId } });
    expect(await screen.findByText("No occurrence is kept for this group.")).toBeInTheDocument();
  });
});

describe("an admin_error_group notification", () => {
  test("leads to the group's page, or to the list for the groups beyond the hour's limit", async () => {
    const items: Schemas["Notification"][] = [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e1",
        type: "admin_error_group",
        created_at: "2026-10-16T09:02:00Z",
        error_group: { change: "others", count: 4 },
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000e0",
        type: "admin_error_group",
        created_at: "2026-10-16T09:01:00Z",
        error_group: { change: "regressed", group_id: otherId },
      },
      {
        id: "0192f0c4-7a8e-7000-8000-0000000000df",
        type: "admin_error_group",
        created_at: "2026-10-16T09:00:00Z",
        error_group: { change: "created", group_id: groupId },
      },
    ];
    server.use(http.get(`${apiBase}/notifications`, () => HttpResponse.json({ items })));

    await renderApp({ path: "/notifications" });

    const feed = await screen.findByRole("feed", { name: "Notifications" });
    const created = within(feed).getByRole("article", { name: /A new error was reported/u });
    expect(within(created).getByRole("link", { name: "Open the error" })).toHaveAttribute(
      "href",
      `/admin/errors/${groupId}`,
    );
    const regressed = within(feed).getByRole("article", { name: /A resolved error happened again/u });
    expect(within(regressed).getByRole("link", { name: "Open the error" })).toHaveAttribute(
      "href",
      `/admin/errors/${otherId}`,
    );
    const others = within(feed).getByRole("article", { name: /More new or recurring errors/u });
    expect(within(others).getByRole("link", { name: "Open the errors" })).toHaveAttribute("href", "/admin/errors");
  });
});
