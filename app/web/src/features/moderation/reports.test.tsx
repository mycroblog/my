// SPDX-License-Identifier: AGPL-3.0-only
// The report queue and a report's page through the whole app (web-client/README.md T4): a moderator with
// manage_reports (and manage_accounts for actions) lists the reports, takes one, writes a note, and handles it
// by an action on its target, which resolves it, or without one, with MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import {
  accountFixture,
  otherAccountFixture,
  remoteAccountFixture,
  type Schemas,
  statusFixture,
  viewerFixture,
} from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { watchAppearances } from "../../test/appearances";
import { renderApp } from "../../test/render";
import { newAccountAction, reportFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_reports", "manage_accounts", "view_audit"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Report = Schemas["AdminReportDetail"];
type NewAction = Schemas["CreateAccountActionRequest"];

/** A name inside a sentence, as the screens isolate it (U+2068 … U+2069). */
function isolated(name: string): string {
  return `⁨${name}⁩`;
}

const moderator = accountFixture(5, "Mona", "my.example");

const spam = reportFixture();
const older = reportFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000c0000",
  category: "violation",
  comment: "",
  filed_at: "2026-10-15T09:00:00Z",
  state_changed_at: "2026-10-15T09:00:00Z",
  assignee_account_id: moderator.id,
  reporter_account_id: viewerFixture.id,
  reporter_account: viewerFixture,
  target_account_id: remoteAccountFixture.id,
  target_account: remoteAccountFixture,
  status_ids: [],
  rule_ids: ["0192f0c4-7a8e-7000-8000-0000000f0001"],
});
const dismissed = reportFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000b0009",
  state: "resolved",
  outcome: "dismissed",
  state_changed_at: "2026-10-14T00:00:00Z",
  filed_at: "2026-10-13T09:00:00Z",
});

/** Plays the reports of the server: lists by state, actions that resolve their report; records the requests. */
function reportsServer(initial: Report[]) {
  let reports = initial;
  const lists: string[] = [];
  const actions: { accountId: string; body: NewAction }[] = [];
  const changes: string[] = [];
  const update = (id: string, change: (report: Report) => Report): Report | undefined => {
    const found = reports.find((report) => report.id === id);
    if (found === undefined) return undefined;
    const next = change(found);
    reports = reports.map((report) => (report.id === id ? next : report));
    return next;
  };
  server.use(
    http.get(`${apiBase}/admin/reports`, ({ request }) => {
      const query = new URL(request.url).searchParams;
      lists.push(query.toString());
      const state = query.get("state");
      return HttpResponse.json({ items: reports.filter((report) => state === null || report.state === state) });
    }),
    http.get(`${apiBase}/admin/reports/:id`, ({ params }) => {
      const found = reports.find((report) => report.id === params["id"]);
      return found === undefined ? problem(404, "about:blank", "Not Found") : HttpResponse.json(found);
    }),
    http.post(`${apiBase}/admin/reports/:id/:operation`, async ({ request, params }) => {
      const operation = String(params["operation"]);
      const body =
        operation === "resolve" || operation === "notes" ? ((await request.json()) as Record<string, string>) : {};
      changes.push(`${operation}${body["outcome"] === undefined ? "" : ` ${body["outcome"]}`}`);
      const next = update(String(params["id"]), (report) => {
        switch (operation) {
          case "assign_to_self":
            return { ...report, assignee_account_id: viewerFixture.id };
          case "unassign": {
            const next = { ...report };
            delete next.assignee_account_id;
            return next;
          }
          case "resolve":
            return {
              ...report,
              state: "resolved",
              outcome: body["outcome"] as Schemas["AdminReportOutcome"],
              state_changed_at: "2026-10-17T00:00:00Z",
            };
          case "reopen": {
            const next: Report = { ...report, state: "open", state_changed_at: "2026-10-17T01:00:00Z" };
            delete next.outcome;
            return next;
          }
          default:
            return {
              ...report,
              notes: [
                ...report.notes,
                {
                  id: `0192f0c4-7a8e-7000-8000-0000000e000${String(report.notes.length)}`,
                  author_account_id: viewerFixture.id,
                  body: body["body"] ?? "",
                  created_at: "2026-10-17T00:00:00Z",
                },
              ],
            };
        }
      });
      return next === undefined ? problem(404, "about:blank", "Not Found") : HttpResponse.json(next);
    }),
    http.post(`${apiBase}/admin/accounts/:id/actions`, async ({ request, params }) => {
      const body = (await request.json()) as NewAction;
      const accountId = String(params["id"]);
      actions.push({ accountId, body });
      if (body.report_id !== undefined) {
        update(body.report_id, (report) => ({
          ...report,
          state: "resolved",
          outcome: "actioned",
          state_changed_at: "2026-10-17T00:00:00Z",
          target_history: { ...report.target_history, actions_count: report.target_history.actions_count + 1 },
        }));
      }
      return HttpResponse.json(newAccountAction(accountId, body));
    }),
  );
  return { lists, actions, changes };
}

/** The rows of the queue's table, without its header row. */
function rows() {
  return within(screen.getByRole("table")).queryAllByRole("row").slice(1);
}

function region(name: string) {
  return screen.getByRole("region", { name });
}

describe("the report queue", () => {
  test("lists the open reports newest first, with the target, reporter, category and assignee, and the resolved ones in their tab", async () => {
    const api = reportsServer([spam, older, dismissed]);
    server.use(
      http.get(`${apiBase}/accounts/:id`, ({ params }) =>
        params["id"] === moderator.id ? HttpResponse.json(moderator) : undefined,
      ),
    );
    const { user, router } = await renderApp({ path: "/moderation/reports" });

    expect(await screen.findByRole("heading", { level: 1, name: "Reports" })).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: `Report on ${isolated("@bob")}` })).toHaveAttribute(
      "href",
      `/moderation/reports/${spam.id}`,
    );
    expect(rows().map((row) => within(row).getByRole("link").textContent)).toEqual([
      `Report on ${isolated("@bob")}`,
      `Report on ${isolated("@carol@remote.example")}`,
    ]);
    const first = within(rows()[0] as HTMLElement);
    expect(first.getByText("Spam")).toBeInTheDocument();
    expect(first.getByText("@carol@remote.example")).toBeInTheDocument();
    expect(first.getByText("Oct 16, 2026, 9:00 AM")).toBeInTheDocument();
    expect(first.getByText("Nobody")).toBeInTheDocument();
    expect(first.getByText("1 post")).toBeInTheDocument();
    // The reporter's comment, in its own direction.
    expect(first.getByText("Posts the same link again and again.")).toHaveAttribute("dir", "auto");
    const second = within(rows()[1] as HTMLElement);
    expect(second.getByText("Breaks the server rules")).toBeInTheDocument();
    expect(await second.findByText("Mona")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Reports/ }));
    await user.click(await screen.findByRole("option", { name: "Resolved" }));
    expect(await screen.findByText("No action")).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ state: "resolved" });
    expect(rows()).toHaveLength(1);
    expect(api.lists).toEqual(["limit=40&state=open", "limit=40&state=resolved"]);
  });

  test("pages by cursor with previous and next, one page of the table at a time, by keyboard", async () => {
    const cursors: (string | null)[] = [];
    server.use(
      http.get(`${apiBase}/admin/reports`, ({ request }) => {
        const cursor = new URL(request.url).searchParams.get("cursor");
        cursors.push(cursor);
        return HttpResponse.json(cursor === null ? { items: [spam], next_cursor: "next" } : { items: [older] });
      }),
    );
    const { user } = await renderApp({ path: "/moderation/reports" });

    const pages = await screen.findByRole("navigation", { name: "Pages" });
    expect(within(pages).getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(rows()).toHaveLength(1);
    await user.click(within(pages).getByRole("button", { name: "Next" }));

    expect(
      await screen.findByRole("link", { name: `Report on ${isolated("@carol@remote.example")}` }),
    ).toBeInTheDocument();
    expect(rows()).toHaveLength(1);
    expect(within(pages).getByRole("button", { name: "Next" })).toBeDisabled();
    await user.click(within(pages).getByRole("button", { name: "Previous" }));
    expect(await screen.findByRole("link", { name: `Report on ${isolated("@bob")}` })).toBeInTheDocument();
    expect(cursors).toEqual([null, "next"]);
  });

  test("an empty queue says what shows up there", async () => {
    reportsServer([]);
    await renderApp({ path: "/moderation/reports" });

    expect(await screen.findByRole("heading", { level: 2, name: "No open reports" })).toBeInTheDocument();
  });

  test("is in the moderation menu of a moderator", async () => {
    await renderApp({ path: "/moderation" });

    const menu = await screen.findByRole("region", { name: "Moderation" });
    expect(within(menu).getByRole("link", { name: "Reports" })).toHaveAttribute("href", "/moderation/reports");
  });
});

describe("a report's page", () => {
  test("shows the report, its posts, its target with the target's history, the assignee and the notes on one page", async () => {
    const report = reportFixture({
      status_ids: [
        "0192f0c4-7a8e-7000-8000-0000000000b1",
        "0192f0c4-7a8e-7000-8000-0000000000b2",
        "0192f0c4-7a8e-7000-8000-0000000000b3",
      ],
      statuses: [
        { status: { ...statusFixture("0192f0c4-7a8e-7000-8000-0000000000b1"), visibility: "direct" } },
        { status: statusFixture("0192f0c4-7a8e-7000-8000-0000000000b2"), deleted_at: "2026-10-16T11:00:00Z" },
      ],
      category: "violation",
      rule_ids: ["0192f0c4-7a8e-7000-8000-0000000f0001", "0192f0c4-7a8e-7000-8000-0000000f0009"],
      forwarded: true,
      uri: "https://remote.example/flags/1",
      notes: [
        { id: "0192f0c4-7a8e-7000-8000-0000000e0001", body: "Seen this before.", created_at: "2026-10-16T10:00:00Z" },
      ],
      target_history: { reports_count: 2, actions_count: 1 },
    });
    reportsServer([report]);
    server.use(
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({
          domain: "my.example",
          name: "my example",
          version: "0.0.0-test",
          source_url: "https://github.com/mycroblog/my",
          registrations: { mode: "approval" },
          limits: { max_post_characters: 500, require_media_description: false },
          timelines: { local: "everyone", federated: "everyone" },
          sends_email: true,
          search_indexing: "deny",
          ai_usage: "deny",
          rules: [{ id: "0192f0c4-7a8e-7000-8000-0000000f0001", text: "No spam" }],
        }),
      ),
      http.get(`${apiBase}/admin/accounts/:id/actions`, () =>
        HttpResponse.json({
          items: [
            {
              id: "0192f0c4-7a8e-7000-8000-0000000d0000",
              account_id: otherAccountFixture.id,
              kind: "warn",
              reason_category: "spam",
              public_reason: "Please stop.",
              issuer: "moderator",
              notify_target: true,
              state: "active",
              issued_at: "2026-10-01T00:00:00Z",
              report_id: "0192f0c4-7a8e-7000-8000-0000000c0099",
            },
          ],
        }),
      ),
    );
    await renderApp({ path: `/moderation/reports/${report.id}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Report" })).toBeInTheDocument();
    const summary = region("Report");
    expect(within(summary).getByText("Open")).toBeInTheDocument();
    expect(within(summary).getByText("Breaks the server rules")).toBeInTheDocument();
    expect(within(summary).getByText("Another server")).toBeInTheDocument();
    expect(within(summary).getByText("To the account's server")).toBeInTheDocument();
    expect(await within(summary).findByText("No spam")).toHaveAttribute("lang", "");
    expect(within(summary).getByText("A rule no longer in effect")).toBeInTheDocument();

    // The posts, whatever their visibility, without the actions on them: a direct one, one its author deleted, and
    // one the server no longer has.
    const posts = region("Reported posts");
    expect(within(posts).getAllByRole("article")).toHaveLength(2);
    expect(within(posts).queryByRole("button", { name: /Favourite|Boost|Reply/ })).not.toBeInTheDocument();
    expect(within(posts).getByText(/^Deleted .+\. Kept for this report\.$/)).toBeInTheDocument();
    expect(within(posts).getByText("The server no longer has this post.")).toBeInTheDocument();

    const target = region("Reported account");
    expect(within(target).getByText("@bob")).toBeInTheDocument();
    expect(within(target).getByRole("link", { name: "Open the profile" })).toHaveAttribute("href", "/@bob");
    expect(within(target).getByRole("link", { name: "Open the account's admin page" })).toHaveAttribute(
      "href",
      `/moderation/accounts/${otherAccountFixture.id}`,
    );
    expect(
      within(target).getByRole("link", { name: "Show operations on the account in the audit log" }),
    ).toHaveAttribute("href", `/moderation/audit-log?target_kind=account&target=${otherAccountFixture.id}`);
    expect(within(target).getByText("2 reports")).toBeInTheDocument();
    expect(within(target).getByText("1 action")).toBeInTheDocument();

    const history = region("Actions on the account");
    expect(await within(history).findByRole("heading", { level: 3, name: "Warn" })).toBeInTheDocument();
    expect(within(history).getByRole("link", { name: "Open the report" })).toHaveAttribute(
      "href",
      "/moderation/reports/0192f0c4-7a8e-7000-8000-0000000c0099",
    );
    expect(within(history).getByText("Please stop.")).toHaveAttribute("dir", "auto");

    expect(within(region("Assignee")).getByText("Nobody")).toBeInTheDocument();
    const notes = region("Notes");
    expect(within(notes).getByText("Seen this before.")).toHaveAttribute("dir", "auto");
    expect(within(notes).getByText("Command line")).toBeInTheDocument();
  });

  test("takes the report, leaves it, and writes notes", async () => {
    const api = reportsServer([spam]);
    const { user } = await renderApp({ path: `/moderation/reports/${spam.id}` });

    const assignee = await screen.findByRole("region", { name: "Assignee" });
    const take = within(assignee).getByRole("button", { name: "Assign to me" });
    await user.click(take);
    expect(await within(assignee).findByText("You handle this report now.")).toBeInTheDocument();
    expect(within(assignee).getByText("You")).toBeInTheDocument();
    // The button stays where it was, so focus stays on it.
    expect(within(assignee).getByRole("button", { name: "Take over" })).toBe(take);
    await user.click(within(assignee).getByRole("button", { name: "Unassign" }));
    expect(await within(assignee).findByText("Nobody handles this report now.")).toBeInTheDocument();
    expect(within(assignee).getByText("Nobody")).toBeInTheDocument();

    const notes = region("Notes");
    await user.click(within(notes).getByRole("button", { name: "Add note" }));
    expect(within(notes).getByRole("textbox", { name: "New note" })).toBeInvalid();
    expect(api.changes).toEqual(["assign_to_self", "unassign"]);

    await user.type(within(notes).getByRole("textbox", { name: "New note" }), "Checked the links: all spam.");
    await user.click(within(notes).getByRole("button", { name: "Add note" }));
    expect(await within(notes).findByText("Checked the links: all spam.", { selector: "p" })).toBeInTheDocument();
    expect(within(notes).getByText("Note added.")).toBeInTheDocument();
    expect(within(notes).getByRole("textbox", { name: "New note" })).toHaveValue("");
    expect(api.changes).toEqual(["assign_to_self", "unassign", "notes"]);
  });

  test("a note the server refuses says why under its field and keeps the text", async () => {
    reportsServer([spam]);
    server.use(
      http.post(`${apiBase}/admin/reports/:id/notes`, () => problem(503, "about:blank", "Service Unavailable")),
    );
    const { user } = await renderApp({ path: `/moderation/reports/${spam.id}` });

    const field = await screen.findByRole("textbox", { name: "New note" });
    await user.type(field, "A note");
    await user.click(screen.getByRole("button", { name: "Add note" }));

    expect(await screen.findByText("Could not add the note: Service Unavailable")).toBeInTheDocument();
    expect(field).toHaveValue("A note");
    expect(field).toBeInvalid();
  });

  test("an action on the target resolves the report, which leaves the open reports", async () => {
    const api = reportsServer([spam, older]);
    const { user } = await renderApp({ path: "/moderation/reports" });

    await user.click(await screen.findByRole("link", { name: `Report on ${isolated("@bob")}` }));
    expect(await screen.findByRole("heading", { level: 1, name: "Report" })).toBeInTheDocument();
    const handling = region("Handling");
    await user.click(await within(handling).findByRole("radio", { name: "Limit" }));
    // The category starts from the report's.
    expect(within(handling).getByRole("button", { name: /Reason/ })).toHaveTextContent("Spam");
    await user.type(within(handling).getByRole("textbox", { name: "Public reason" }), "Spam links.");
    await user.click(within(handling).getByRole("button", { name: "Take action and resolve" }));

    expect(await within(handling).findByText("Action taken (Limit). The report is resolved.")).toBeInTheDocument();
    expect(api.actions).toEqual([
      {
        accountId: otherAccountFixture.id,
        body: {
          kind: "limit",
          reason_category: "spam",
          public_reason: "Spam links.",
          report_id: spam.id,
          notify_target: true,
        },
      },
    ]);
    expect(await within(handling).findByRole("button", { name: "Reopen" })).toBeInTheDocument();
    expect(within(handling).getByText("Action taken")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole("heading", { level: 2, name: "Handling" })).toHaveFocus();
    });

    await user.click(screen.getByRole("link", { name: "Back to the reports" }));
    expect(
      await screen.findByRole("link", { name: `Report on ${isolated("@carol@remote.example")}` }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: `Report on ${isolated("@bob")}` })).not.toBeInTheDocument();
  });

  // #464, ADR-0045: the action marks the queue stale, and the route fetches it again before showing it, so the
  // queue never shows the report as it was before the action, however slowly the server answers.
  test("back on the queue after an action, the resolved report is gone at once, however slowly the lists come", async () => {
    const api = reportsServer([spam, older]);
    const { user } = await renderApp({ path: "/moderation/reports" });
    await user.click(await screen.findByRole("link", { name: `Report on ${isolated("@bob")}` }));
    const handling = await screen.findByRole("region", { name: "Handling" });
    await user.click(await within(handling).findByRole("radio", { name: "Limit" }));
    // From now on every list of reports takes a while; the handler falls through to reportsServer's.
    server.use(
      http.get(`${apiBase}/admin/reports`, async () => {
        await delay(300);
      }),
    );
    await user.click(within(handling).getByRole("button", { name: "Take action and resolve" }));
    expect(await within(handling).findByText("Action taken (Limit). The report is resolved.")).toBeInTheDocument();

    const staleQueue = watchAppearances(
      () => screen.queryByRole("link", { name: `Report on ${isolated("@bob")}` }) !== null,
    );
    await user.click(screen.getByRole("link", { name: "Back to the reports" }));
    expect(
      await screen.findByRole("link", { name: `Report on ${isolated("@carol@remote.example")}` }),
    ).toBeInTheDocument();
    expect(staleQueue()).toBe(false);
    expect(api.lists).toEqual(["limit=40&state=open", "limit=40&state=open"]);
  });

  test("back on the queue after resolving without an action, the report is gone at once, however slowly the lists come", async () => {
    reportsServer([spam, older]);
    const { user } = await renderApp({ path: "/moderation/reports" });
    await user.click(await screen.findByRole("link", { name: `Report on ${isolated("@bob")}` }));
    server.use(
      http.get(`${apiBase}/admin/reports`, async () => {
        await delay(300);
      }),
    );
    await user.click(await screen.findByRole("button", { name: "Resolve without action" }));
    expect(await within(region("Handling")).findByText("The report is resolved: No action.")).toBeInTheDocument();

    const staleQueue = watchAppearances(
      () => screen.queryByRole("link", { name: `Report on ${isolated("@bob")}` }) !== null,
    );
    await user.click(screen.getByRole("link", { name: "Back to the reports" }));
    expect(
      await screen.findByRole("link", { name: `Report on ${isolated("@carol@remote.example")}` }),
    ).toBeInTheDocument();
    expect(staleQueue()).toBe(false);
  });

  test("a ban asks first in an alert dialog; cancelling sends nothing", async () => {
    const api = reportsServer([spam]);
    const { user } = await renderApp({ path: `/moderation/reports/${spam.id}` });

    await user.click(await screen.findByRole("radio", { name: "Ban" }));
    // A ban lasts for good: no duration.
    expect(screen.queryByRole("button", { name: /Duration/ })).not.toBeInTheDocument();
    const submit = screen.getByRole("button", { name: "Take action and resolve" });
    await user.click(submit);

    const dialog = await screen.findByRole("alertdialog", { name: `Ban ${isolated("@bob")}?` });
    expect(dialog).toHaveAccessibleDescription(/This cannot be undone/);
    await waitFor(() => {
      expect(within(dialog).getByRole("button", { name: "Cancel" })).toHaveFocus();
    });
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(submit).toHaveFocus();
    });
    expect(api.actions).toEqual([]);

    await user.click(submit);
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Ban" }));
    expect(await screen.findByText("Action taken (Ban). The report is resolved.")).toBeInTheDocument();
    expect(api.actions.map(({ body }) => body.kind)).toEqual(["ban"]);
  });

  test("deleting posts asks first, sends the chosen posts, and needs one", async () => {
    const report = reportFixture({
      status_ids: ["0192f0c4-7a8e-7000-8000-0000000000b1", "0192f0c4-7a8e-7000-8000-0000000000b2"],
    });
    const api = reportsServer([report]);
    const { user } = await renderApp({ path: `/moderation/reports/${report.id}` });

    await user.click(await screen.findByRole("radio", { name: "Delete posts" }));
    const posts = screen.getByRole("group", { name: "Posts" });
    const boxes = await within(posts).findAllByRole("checkbox");
    expect(boxes).toHaveLength(2);
    for (const box of boxes) expect(box).toBeChecked();
    for (const box of boxes) await user.click(box);
    await user.click(screen.getByRole("button", { name: "Take action and resolve" }));
    expect(await within(posts).findByRole("alert")).toHaveTextContent("Choose at least one post.");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();

    await user.click(boxes[1] as HTMLElement);
    await user.click(screen.getByRole("button", { name: "Take action and resolve" }));
    const dialog = await screen.findByRole("alertdialog", { name: `Delete 1 post of ${isolated("@bob")}?` });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await screen.findByText("Action taken (Delete posts). The report is resolved.")).toBeInTheDocument();
    expect(api.actions.map(({ body }) => body)).toEqual([
      {
        kind: "delete_statuses",
        reason_category: "spam",
        report_id: report.id,
        notify_target: true,
        status_ids: ["0192f0c4-7a8e-7000-8000-0000000000b2"],
      },
    ]);
  });

  test("a deleted post is shown but not offered to an action on posts", async () => {
    const report = reportFixture({
      status_ids: ["0192f0c4-7a8e-7000-8000-0000000000b1", "0192f0c4-7a8e-7000-8000-0000000000b2"],
      statuses: [
        { status: statusFixture("0192f0c4-7a8e-7000-8000-0000000000b1") },
        { status: statusFixture("0192f0c4-7a8e-7000-8000-0000000000b2"), deleted_at: "2026-10-16T11:00:00Z" },
      ],
    });
    reportsServer([report]);
    const { user } = await renderApp({ path: `/moderation/reports/${report.id}` });

    expect(within(await screen.findByRole("region", { name: "Reported posts" })).getAllByRole("article")).toHaveLength(
      2,
    );
    await user.click(await screen.findByRole("radio", { name: "Delete posts" }));
    expect(within(screen.getByRole("group", { name: "Posts" })).getAllByRole("checkbox")).toHaveLength(1);
  });

  test("an action that fails in its confirmation says why there, which stays open", async () => {
    reportsServer([spam]);
    server.use(
      http.post(`${apiBase}/admin/accounts/:id/actions`, () => problem(503, "about:blank", "Service Unavailable")),
    );
    const { user } = await renderApp({ path: `/moderation/reports/${spam.id}` });

    await user.click(await screen.findByRole("radio", { name: "Ban" }));
    await user.click(screen.getByRole("button", { name: "Take action and resolve" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Ban" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "Could not take the action: Service Unavailable",
    );
  });

  test("an account of another server takes only the actions for it, and is not told", async () => {
    reportsServer([older]);
    const { user } = await renderApp({ path: `/moderation/reports/${older.id}` });

    const kinds = await screen.findByRole("radiogroup", { name: "Action" });
    expect(
      within(kinds)
        .getAllByRole("radio")
        .map((radio) => radio.getAttribute("value")),
    ).toEqual(["force_sensitive", "limit", "suspend"]);
    expect(screen.queryByRole("checkbox", { name: "Tell the account" })).not.toBeInTheDocument();
    await user.click(within(kinds).getByRole("radio", { name: "Suspend" }));
    // A suspension can be lifted: it takes effect without asking.
    await user.click(screen.getByRole("button", { name: "Take action and resolve" }));
    expect(await screen.findByText("Action taken (Suspend). The report is resolved.")).toBeInTheDocument();
  });

  test("resolves without an action and reopens, keeping focus in the region", async () => {
    const api = reportsServer([spam]);
    const { user } = await renderApp({ path: `/moderation/reports/${spam.id}` });

    await user.click(await screen.findByRole("button", { name: "Resolve without action" }));
    const handling = region("Handling");
    expect(await within(handling).findByText("The report is resolved: No action.")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole("heading", { level: 2, name: "Handling" })).toHaveFocus();
    });
    expect(within(region("Report")).getByText("Resolved")).toBeInTheDocument();

    await user.click(within(handling).getByRole("button", { name: "Reopen" }));
    expect(await within(handling).findByText("The report is open again.")).toBeInTheDocument();
    expect(within(handling).getByRole("button", { name: "Resolve without action" })).toBeInTheDocument();
    expect(api.changes).toEqual(["resolve dismissed", "reopen"]);
  });

  test("a change that fails is told in a toast, and the report stays as it was", async () => {
    reportsServer([spam]);
    server.use(
      http.post(`${apiBase}/admin/reports/:id/resolve`, () => problem(503, "about:blank", "Service Unavailable")),
    );
    const { user } = await renderApp({ path: `/moderation/reports/${spam.id}` });

    await user.click(await screen.findByRole("button", { name: "Resolve as actioned" }));

    expect(await screen.findByText("Could not resolve the report: Service Unavailable")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Resolve as actioned" })).toBeInTheDocument();
  });

  test("a moderator without manage_accounts resolves without the action form", async () => {
    server.use(operatorHandler(["manage_reports"]));
    reportsServer([spam]);
    await renderApp({ path: `/moderation/reports/${spam.id}` });

    expect(
      await screen.findByText("Taking an action on the account needs the manage_accounts permission."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("radiogroup", { name: "Action" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Actions on the account" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Open the account's admin page" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Resolve without action" })).toBeInTheDocument();
  });

  test("an unknown report is not found", async () => {
    reportsServer([]);
    await renderApp({ path: "/moderation/reports/0192f0c4-7a8e-7000-8000-0000000c0404" });

    expect(await screen.findByRole("heading", { level: 1, name: "Report" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 2, name: /not found/i })).toBeInTheDocument();
  });
});

describe("permissions", () => {
  test("a user without manage_reports is told why, and nothing is asked of the server", async () => {
    server.use(operatorHandler(["manage_domains"]));
    const api = reportsServer([spam]);
    await renderApp({ path: "/moderation/reports" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.getByText(/the permission manage_reports/)).toBeInTheDocument();
    expect(api.lists).toEqual([]);
  });

  test("a refusal of the server for a missing permission is shown as such", async () => {
    server.use(
      http.get(`${apiBase}/admin/reports/:id`, () => problem(403, "/api/v1/problems/missing-permission", "Forbidden")),
    );
    await renderApp({ path: `/moderation/reports/${spam.id}` });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Report" })).toBeInTheDocument();
  });
});
