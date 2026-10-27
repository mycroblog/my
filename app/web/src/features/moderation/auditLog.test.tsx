// SPDX-License-Identifier: AGPL-3.0-only
// The audit log screen through the whole app (web-client/README.md T4): a signed-in operator with view_audit, MSW
// playing GET /admin/audit_log, and the filters and loaded pages in the URL.
import { screen, waitFor, within } from "@testing-library/react";
import type { RouterHistory } from "@tanstack/react-router";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { accountFixture, type Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { auditTargetHref } from "./hooks/useAuditLog";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

/** The audit log requests the app sent, as "path?query". */
function recordAuditRequests() {
  const requests: string[] = [];
  server.events.on("request:start", ({ request }) => {
    const url = new URL(request.url);
    if (url.pathname === `${apiBase}/admin/audit_log`) requests.push(`${url.pathname.slice(apiBase.length)}${url.search}`);
  });
  return requests;
}

const moderator = accountFixture(5, "Mona", "my.example");

const suspension: Schemas["AdminAuditEntry"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000a0003",
  actor_kind: "user",
  actor_account_id: moderator.id,
  actor_account: moderator,
  action: "account_action_issued",
  target_kind: "account",
  target_id: "0192f0c4-7a8e-7000-8000-000000000003",
  target_label: "@carol@remote.example",
  target_exists: true,
  source_event_id: "0192f0c4-7a8e-7000-8000-0000000b0001",
  occurred_at: "2026-10-17T09:30:00Z",
};

const domainLimit: Schemas["AdminAuditEntry"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000a0002",
  actor_kind: "user",
  actor_account_id: moderator.id,
  actor_account: moderator,
  action: "domain_action_issued",
  target_kind: "domain",
  target_id: "spam.example",
  target_label: "spam.example",
  target_exists: true,
  occurred_at: "2026-10-16T09:30:00Z",
};

const settingByCli: Schemas["AdminAuditEntry"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000a0001",
  actor_kind: "cli",
  action: "setting_changed",
  target_kind: "setting",
  target_id: "registration_mode",
  target_label: "registration_mode",
  target_exists: true,
  occurred_at: "2026-10-15T09:30:00Z",
};

const byDeletedUser: Schemas["AdminAuditEntry"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000a0000",
  actor_kind: "user",
  action: "report_resolved",
  target_kind: "report",
  target_id: "0192f0c4-7a8e-7000-8000-0000000c0001",
  target_label: "@dave",
  // The report has been erased past its retention.
  target_exists: false,
  occurred_at: "2026-10-14T09:30:00Z",
};

const allEntries = [suspension, domainLimit, settingByCli, byDeletedUser];

/** Answers the audit log as the server does: the entries that match the query, two per page. */
const auditLogHandler = http.get(`${apiBase}/admin/audit_log`, ({ request }) => {
  const query = new URL(request.url).searchParams;
  const matching = allEntries.filter(
    (entry) =>
      (query.get("actor_account_id") === null || entry.actor_account_id === query.get("actor_account_id")) &&
      (query.get("action") === null || entry.action === query.get("action")) &&
      (query.get("target_kind") === null || entry.target_kind === query.get("target_kind")) &&
      (query.get("target_id") === null || entry.target_id === query.get("target_id")),
  );
  const start = query.get("cursor") === null ? 0 : matching.findIndex((entry) => entry.id === query.get("cursor")) + 1;
  const items = matching.slice(start, start + 2);
  const last = items.at(-1);
  return HttpResponse.json(start + 2 < matching.length && last !== undefined ? { items, next_cursor: last.id } : { items });
});

/** The rows of the table of operations, without its header row. */
function rows() {
  return within(screen.getByRole("table")).queryAllByRole("row").slice(1);
}

/** The pagination under the table. */
function pagination() {
  return screen.getByRole("navigation", { name: "Pages" });
}

/** A name inside a sentence, as the screen isolates it (U+2068 … U+2069). */
function isolated(name: string): string {
  return `\u2068${name}\u2069`;
}

function row(operation: string) {
  const found = rows().find((item) => within(item).queryByRole("rowheader", { name: operation }) !== null);
  if (found === undefined) throw new Error(`no row ${operation}`);
  return found;
}

describe("the audit log", () => {
  test("lists the operations newest first, with who, what, when, and links to their targets", async () => {
    server.use(operatorHandler(["view_audit"]), auditLogHandler);
    const requests = recordAuditRequests();
    const { user } = await renderApp({ path: "/moderation/audit-log" });

    expect(await screen.findByRole("heading", { level: 1, name: "Audit log" })).toBeInTheDocument();
    expect(await screen.findByRole("rowheader", { name: "Account action taken" })).toBeInTheDocument();
    expect(rows().map((item) => within(item).getByRole("rowheader").textContent)).toEqual(["Account action taken", "Domain action taken"]);

    const first = within(row("Account action taken"));
    expect(first.getByText("Mona")).toBeInTheDocument();
    expect(first.getByText("@mona")).toBeInTheDocument();
    expect(first.getByText("Oct 17, 2026, 9:30 AM")).toBeInTheDocument();
    expect(first.getByRole("link", { name: "@carol@remote.example" })).toHaveAttribute("href", "/moderation/accounts/0192f0c4-7a8e-7000-8000-000000000003");
    expect(within(row("Domain action taken")).getByRole("link", { name: "spam.example" })).toHaveAttribute("href", "/moderation/domain-actions?filter=all&domain=spam.example");

    expect(within(pagination()).getByRole("button", { name: "Previous" })).toBeDisabled();
    await user.click(within(pagination()).getByRole("button", { name: "Next" }));
    expect(await screen.findByRole("rowheader", { name: "Setting changed" })).toBeInTheDocument();
    // A setting has no screen: its label only. The command line and a deleted account have no account to show.
    const setting = within(row("Setting changed"));
    expect(setting.getByText("registration_mode")).not.toHaveAttribute("href");
    expect(setting.queryByRole("link", { name: "registration_mode" })).not.toBeInTheDocument();
    expect(setting.getByText("Command line")).toBeInTheDocument();
    expect(setting.queryByRole("link", { name: /Show only operations by/ })).not.toBeInTheDocument();
    const resolved = within(row("Report resolved"));
    expect(resolved.getByText("A deleted account")).toBeInTheDocument();
    // An erased report has no screen to link to: its label only.
    expect(resolved.getByText("@dave")).not.toHaveAttribute("href");
    expect(resolved.queryByRole("link", { name: "@dave" })).not.toBeInTheDocument();
    expect(within(pagination()).getByRole("button", { name: "Next" })).toBeDisabled();
    await user.click(within(pagination()).getByRole("button", { name: "Previous" }));
    expect(await screen.findByRole("rowheader", { name: "Account action taken" })).toBeInTheDocument();
    expect(requests).toEqual(["/admin/audit_log?limit=40", `/admin/audit_log?limit=40&cursor=${domainLimit.id}`]);
  });

  test("keeps the loaded pages in the URL and loads them all again from it", async () => {
    server.use(operatorHandler(["view_audit"]), auditLogHandler);
    const { user, router } = await renderApp({ path: "/moderation/audit-log" });

    await user.click(within(await screen.findByRole("navigation", { name: "Pages" })).getByRole("button", { name: "Next" }));
    expect(await screen.findByRole("rowheader", { name: "Setting changed" })).toBeInTheDocument();
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ pages: 2 });
    });
    // Loading more replaced the entry of the history: going back leaves the screen.
    expect((router.history as RouterHistory).canGoBack()).toBe(false);
  });

  test("opens with the pages of the URL", async () => {
    server.use(operatorHandler(["view_audit"]), auditLogHandler);
    const requests = recordAuditRequests();
    const { user } = await renderApp({ path: "/moderation/audit-log?pages=2" });

    // Both pages are loaded; the table shows one at a time.
    expect(await screen.findByRole("rowheader", { name: "Account action taken" })).toBeInTheDocument();
    expect(rows()).toHaveLength(2);
    await user.click(within(pagination()).getByRole("button", { name: "Next" }));
    expect(await screen.findByRole("rowheader", { name: "Report resolved" })).toBeInTheDocument();
    expect(rows()).toHaveLength(2);
    expect(requests).toEqual(["/admin/audit_log?limit=40", `/admin/audit_log?limit=40&cursor=${domainLimit.id}`]);
  });

  test("filters by the kind of operation and of target in the URL", async () => {
    server.use(operatorHandler(["view_audit"]), auditLogHandler);
    const requests = recordAuditRequests();
    const { user, router } = await renderApp({ path: "/moderation/audit-log" });

    await user.click(await screen.findByRole("button", { name: /Operation/ }));
    await user.click(await screen.findByRole("option", { name: "Domain action taken" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ action: "domain_action_issued" });
    });
    await waitFor(() => {
      expect(rows()).toHaveLength(1);
    });
    expect(within(row("Domain action taken")).getByRole("link", { name: "spam.example" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Target/ }));
    await user.click(await screen.findByRole("option", { name: "Setting" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ action: "domain_action_issued", target_kind: "setting" });
    });
    expect(await screen.findByRole("heading", { level: 2, name: "No operations match the filters" })).toBeInTheDocument();

    const [clear] = screen.getAllByRole("link", { name: "Clear filters" });
    if (clear === undefined) throw new Error("no clear link");
    await user.click(clear);
    await waitFor(() => {
      expect(router.state.location.search).toEqual({});
    });
    expect(await screen.findByRole("rowheader", { name: "Account action taken" })).toBeInTheDocument();
    expect(requests).toEqual([
      "/admin/audit_log?limit=40",
      "/admin/audit_log?limit=40&action=domain_action_issued",
      "/admin/audit_log?limit=40&action=domain_action_issued&target_kind=setting",
    ]);
  });

  test("filters by a row's actor and target, keeping focus on the pressed link", async () => {
    server.use(operatorHandler(["view_audit"]), auditLogHandler);
    const requests = recordAuditRequests();
    const { user, router } = await renderApp({ path: "/moderation/audit-log" });

    const byMona = (await screen.findAllByRole("link", { name: `Show only operations by ${isolated("Mona")}` }))[0];
    if (byMona === undefined) throw new Error("no actor link");
    await user.click(byMona);
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ actor: moderator.id });
    });
    // The row stays, and its link now lets go of the filter.
    await waitFor(() => {
      expect(within(row("Account action taken")).getByRole("link", { name: "Show operations by all people" })).toHaveFocus();
    });
    const filters = screen.getByRole("group", { name: "Filters" });
    // The chosen actor and target are listed under the selects.
    expect(screen.getAllByRole("definition")[0]).toHaveTextContent("Mona");

    await user.click(within(row("Domain action taken")).getByRole("link", { name: `Show only operations on ${isolated("spam.example")}` }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ actor: moderator.id, target_kind: "domain", target: "spam.example" });
    });
    await waitFor(() => {
      expect(rows()).toHaveLength(1);
    });
    expect(screen.getAllByRole("definition")[1]).toHaveTextContent("spam.example");
    expect(screen.getByRole("button", { name: /Target/ })).toHaveTextContent("Domain");

    // Letting go of a filter from the filters keeps focus among them.
    await user.click(screen.getByRole("link", { name: "Show all people" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ target_kind: "domain", target: "spam.example" });
    });
    await waitFor(() => {
      expect(within(filters).getAllByRole("button")[0]).toHaveFocus();
    });
    expect(requests).toEqual([
      "/admin/audit_log?limit=40",
      `/admin/audit_log?limit=40&actor_account_id=${moderator.id}`,
      `/admin/audit_log?limit=40&actor_account_id=${moderator.id}&target_kind=domain&target_id=spam.example`,
      "/admin/audit_log?limit=40&target_kind=domain&target_id=spam.example",
    ]);
  });

  test("ignores a target without its kind and unknown kinds in the URL", async () => {
    server.use(operatorHandler(["view_audit"]), auditLogHandler);
    const requests = recordAuditRequests();
    await renderApp({ path: "/moderation/audit-log?target=spam.example&action=unknown&pages=1000" });

    expect(await screen.findByRole("rowheader", { name: "Account action taken" })).toBeInTheDocument();
    expect(requests).toEqual(["/admin/audit_log?limit=40"]);
  });

  test("says why the next page failed, keeps the loaded entries, and tries again", async () => {
    let fail = true;
    server.use(
      operatorHandler(["view_audit"]),
      http.get(`${apiBase}/admin/audit_log`, ({ request }) => {
        if (new URL(request.url).searchParams.get("cursor") === null) return HttpResponse.json({ items: [suspension], next_cursor: suspension.id });
        if (fail) return problem(500, "about:blank", "The database is unavailable.");
        return HttpResponse.json({ items: [domainLimit] });
      }),
    );
    const { user, router } = await renderApp({ path: "/moderation/audit-log" });

    await user.click(within(await screen.findByRole("navigation", { name: "Pages" })).getByRole("button", { name: "Next" }));
    expect(await screen.findByText("Could not load more operations")).toBeInTheDocument();
    expect(screen.getByText("The database is unavailable.")).toBeInTheDocument();
    expect(rows()).toHaveLength(1);
    expect(router.state.location.search).toEqual({});

    fail = false;
    await user.click(within(pagination()).getByRole("button", { name: "Next" }));
    expect(await screen.findByRole("rowheader", { name: "Domain action taken" })).toBeInTheDocument();
    expect(screen.queryByText("Could not load more operations")).not.toBeInTheDocument();
  });

  test("says when nothing has been recorded", async () => {
    server.use(operatorHandler(["view_audit"]));
    await renderApp({ path: "/moderation/audit-log" });

    expect(await screen.findByRole("heading", { level: 2, name: "No operations yet" })).toBeInTheDocument();
  });

  test("says why a user without view_audit cannot use it, and asks the server for nothing", async () => {
    server.use(operatorHandler(["manage_domains"]));
    const requests = recordAuditRequests();
    await renderApp({ path: "/moderation/audit-log" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.getByText(/the permission view_audit/)).toBeInTheDocument();
    expect(requests).toEqual([]);
  });

  test("shows a refusal of the server for a missing permission as such", async () => {
    server.use(
      operatorHandler(["view_audit"]),
      http.get(`${apiBase}/admin/audit_log`, () => problem(403, "/api/v1/problems/missing-permission", "Forbidden")),
    );
    await renderApp({ path: "/moderation/audit-log" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Audit log" })).toBeInTheDocument();
  });
});

describe("the link of an entry's target", () => {
  test("goes to the admin screen of an account, a report, a domain or a sign-up block while the target is there", () => {
    expect(auditTargetHref({ target_kind: "account", target_id: "a1", target_label: "a1", target_exists: true })).toBe("/moderation/accounts/a1");
    expect(auditTargetHref({ target_kind: "report", target_id: "r1", target_label: "r1", target_exists: true })).toBe("/moderation/reports/r1");
    expect(auditTargetHref({ target_kind: "domain", target_id: "spam.example", target_label: "spam.example", target_exists: true })).toBe(
      "/moderation/domain-actions?filter=all&domain=spam.example",
    );
    expect(auditTargetHref({ target_kind: "signup_block", target_id: "0192f0c4-7a8e-7000-8000-0000000001a1", target_label: "0192f0c4-7a8e-7000-8000-0000000001a1", target_exists: true })).toBe("/moderation/signup-blocks?filter=all");
    expect(auditTargetHref({ target_kind: "setting", target_id: "registration_mode", target_label: "registration_mode", target_exists: true })).toBeUndefined();
  });

  test("goes to the custom emoji screen searched by the emoji's shortcode, of this server or of its domain", () => {
    expect(auditTargetHref({ target_kind: "custom_emoji", target_id: "e1", target_label: ":blobcat:", target_exists: true })).toBe("/admin/custom-emojis?shortcode=blobcat");
    expect(auditTargetHref({ target_kind: "custom_emoji", target_id: "e2", target_label: ":blobfox:@remote.example", target_exists: true })).toBe(
      "/admin/custom-emojis?origin=remote&shortcode=blobfox&domain=remote.example",
    );
  });

  test("goes to the list of roles or invites, and to the page of a webhook", () => {
    expect(auditTargetHref({ target_kind: "role", target_id: "role1", target_label: "helper", target_exists: true })).toBe("/admin/roles");
    expect(auditTargetHref({ target_kind: "invite", target_id: "inv1", target_label: "ABCD…", target_exists: true })).toBe("/moderation/invites");
    expect(auditTargetHref({ target_kind: "webhook", target_id: "w1", target_label: "Chat (hooks.example)", target_exists: true })).toBe("/admin/webhooks/w1");
    expect(auditTargetHref({ target_kind: "login_provider", target_id: "lp1", target_label: "Google", target_exists: true })).toBe("/admin/login-providers");
  });

  test("is not there for a target that is gone or deleted", () => {
    expect(auditTargetHref({ target_kind: "account", target_id: "a1", target_label: "a1", target_exists: false })).toBeUndefined();
    expect(auditTargetHref({ target_kind: "report", target_id: "r1", target_label: "r1", target_exists: false })).toBeUndefined();
    expect(auditTargetHref({ target_kind: "role", target_id: "role1", target_label: "helper", target_exists: false })).toBeUndefined();
    expect(auditTargetHref({ target_kind: "custom_emoji", target_id: "e1", target_label: ":blobcat:", target_exists: false })).toBeUndefined();
    expect(auditTargetHref({ target_kind: "webhook", target_id: "w1", target_label: "Chat (hooks.example)", target_exists: false })).toBeUndefined();
  });
});
