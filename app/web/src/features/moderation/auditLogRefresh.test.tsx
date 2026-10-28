// SPDX-License-Identifier: AGPL-3.0-only
// The audit log after each audited write of the admin screens (#475, web-client/README.md S1, ADR-0045): every write that
// the server records marks the cached log stale, and the route fetches it again before showing it (S7), or going
// back to the log would show it without the operation until a slow server answered it again. Each case opens the
// log, goes to the write's screen, makes the write while the log takes a while to come, and goes back.
import { screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { otherAccountFixture, type Schemas, viewerFixture } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import {
  accountActionFixture,
  appealFixture,
  domainActionFixture,
  registrationFixture,
  reportFixture,
} from "./test/handlers";

beforeEach(() => {
  server.use(
    signedInHandler,
    operatorHandler([
      "manage_accounts",
      "manage_reports",
      "manage_domains",
      "manage_settings",
      "manage_rules",
      "view_audit",
    ]),
  );
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

/** U+2066 LEFT-TO-RIGHT ISOLATE … U+2069 POP DIRECTIONAL ISOLATE: a handle inside a sentence. */
function isolated(handle: string): string {
  return `⁦${handle}⁩`;
}

const report = reportFixture();
const appeal = appealFixture();

type Write = {
  /** The audited operation, as the audit log lists it. */
  action: Schemas["AdminAuditAction"];
  /** The request of the write, which the server records. */
  method: "post" | "put" | "patch";
  path: string;
  /** The screen of the write. */
  page: string;
  /** What the server holds before the write, beyond the default handlers. */
  setup?: () => void;
  /** Makes the write through the screen, and waits until the screen shows it done. */
  write: (user: UserEvent) => Promise<void>;
};

async function takeAccountAction(user: UserEvent) {
  const form = await screen.findByRole("region", { name: "Take action" });
  await user.click(within(form).getByRole("radio", { name: "Restrict posting" }));
  await user.click(within(form).getByRole("button", { name: "Take action" }));
  expect(await within(form).findByText("Action taken (Restrict posting).")).toBeInTheDocument();
}

async function changeReport(user: UserEvent, region: string, button: string, done: string) {
  const handling = await screen.findByRole("region", { name: region });
  await user.click(await within(handling).findByRole("button", { name: button }));
  expect(await within(handling).findByText(done)).toBeInTheDocument();
}

async function decide(user: UserEvent, button: string, confirm: string, done: string) {
  await user.click(await screen.findByRole("button", { name: button }));
  await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: confirm }));
  expect(await screen.findByText(done)).toBeInTheDocument();
}

const writes: Write[] = [
  {
    action: "account_action_issued",
    method: "post",
    path: "/admin/accounts/:id/actions",
    page: `/moderation/accounts/${otherAccountFixture.id}`,
    write: takeAccountAction,
  },
  {
    action: "account_action_lifted",
    method: "post",
    path: "/admin/account_actions/:id/lift",
    page: `/moderation/accounts/${otherAccountFixture.id}`,
    setup: () => {
      server.use(
        http.get(`${apiBase}/admin/accounts/:id/actions`, () => HttpResponse.json({ items: [accountActionFixture()] })),
      );
    },
    write: async (user) => {
      const history = await screen.findByRole("region", { name: "Actions on the account" });
      await user.click(await within(history).findByRole("button", { name: /^Lift Limit, taken / }));
      await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Lift" }));
      expect(await within(history).findByRole("status")).toHaveTextContent("Limit is lifted.");
    },
  },
  {
    action: "revealed_private_data",
    method: "post",
    path: "/admin/accounts/:id/reveal",
    page: `/moderation/accounts/${otherAccountFixture.id}`,
    write: async (user) => {
      const privateData = await screen.findByRole("region", { name: "Email and IP addresses" });
      await user.click(within(privateData).getByRole("button", { name: "Show email and IP addresses" }));
      expect(await within(privateData).findByText("bob@example.com")).toBeInTheDocument();
    },
  },
  {
    action: "domain_action_issued",
    method: "post",
    path: "/admin/domain_actions",
    page: "/moderation/domain-actions",
    write: async (user) => {
      await user.type(await screen.findByRole("textbox", { name: "Domain" }), "spam.example");
      await user.click(screen.getByRole("radio", { name: "Limit" }));
      await user.click(screen.getByRole("button", { name: "Take action" }));
      expect(await screen.findByText(`Limit is in effect for ${isolated("spam.example")}.`)).toBeInTheDocument();
    },
  },
  {
    action: "domain_action_lifted",
    method: "post",
    path: "/admin/domain_actions/:id/lift",
    page: "/moderation/domain-actions",
    setup: () => {
      server.use(
        http.get(`${apiBase}/admin/domain_actions`, () => HttpResponse.json({ items: [domainActionFixture()] })),
      );
    },
    write: async (user) => {
      await user.click(await screen.findByRole("button", { name: /^Lift Limit for/ }));
      expect(await screen.findByText(/^Limit for .+ is lifted\.$/)).toBeInTheDocument();
    },
  },
  {
    action: "report_assigned",
    method: "post",
    path: "/admin/reports/:id/assign_to_self",
    page: `/moderation/reports/${report.id}`,
    write: (user) => changeReport(user, "Assignee", "Assign to me", "You handle this report now."),
  },
  {
    action: "report_unassigned",
    method: "post",
    path: "/admin/reports/:id/unassign",
    page: `/moderation/reports/${report.id}`,
    setup: () => {
      server.use(
        http.get(`${apiBase}/admin/reports/:id`, () =>
          HttpResponse.json({ ...report, assignee_account_id: viewerFixture.id }),
        ),
      );
    },
    write: (user) => changeReport(user, "Assignee", "Unassign", "Nobody handles this report now."),
  },
  {
    action: "report_resolved",
    method: "post",
    path: "/admin/reports/:id/resolve",
    page: `/moderation/reports/${report.id}`,
    write: (user) => changeReport(user, "Handling", "Resolve without action", "The report is resolved: No action."),
  },
  {
    action: "report_reopened",
    method: "post",
    path: "/admin/reports/:id/reopen",
    page: `/moderation/reports/${report.id}`,
    setup: () => {
      server.use(
        http.get(`${apiBase}/admin/reports/:id`, () =>
          HttpResponse.json({
            ...report,
            state: "resolved",
            outcome: "dismissed",
            state_changed_at: "2026-10-17T00:00:00Z",
          }),
        ),
      );
    },
    write: (user) => changeReport(user, "Handling", "Reopen", "The report is open again."),
  },
  {
    action: "report_note_added",
    method: "post",
    path: "/admin/reports/:id/notes",
    page: `/moderation/reports/${report.id}`,
    write: async (user) => {
      const notes = await screen.findByRole("region", { name: "Notes" });
      await user.type(within(notes).getByRole("textbox", { name: "New note" }), "Checked the links.");
      await user.click(within(notes).getByRole("button", { name: "Add note" }));
      expect(await within(notes).findByText("Note added.")).toBeInTheDocument();
    },
  },
  {
    action: "registration_approved",
    method: "post",
    path: "/admin/registrations/:userId/approve",
    page: "/moderation/registrations",
    setup: () => {
      server.use(
        http.get(`${apiBase}/admin/registrations`, () => HttpResponse.json({ items: [registrationFixture()] })),
      );
    },
    write: (user) =>
      decide(user, `Approve ${isolated("@bob")}`, "Approve", `${isolated("@bob")} is approved and can sign in.`),
  },
  {
    action: "registration_rejected",
    method: "post",
    path: "/admin/registrations/:userId/reject",
    page: "/moderation/registrations",
    setup: () => {
      server.use(
        http.get(`${apiBase}/admin/registrations`, () => HttpResponse.json({ items: [registrationFixture()] })),
      );
    },
    write: (user) => decide(user, `Reject ${isolated("@bob")}`, "Reject", `${isolated("@bob")} is rejected.`),
  },
  {
    action: "appeal_upheld",
    method: "post",
    path: "/admin/appeals/:id/uphold",
    page: `/moderation/appeals/${appeal.id}`,
    write: (user) => decide(user, "Uphold", "Uphold", "The appeal is upheld. The action stays."),
  },
  {
    action: "appeal_overturned",
    method: "post",
    path: "/admin/appeals/:id/overturn",
    page: `/moderation/appeals/${appeal.id}`,
    write: (user) => decide(user, "Overturn", "Overturn", "The appeal is overturned and the action is lifted."),
  },
  {
    action: "text_changed",
    method: "put",
    path: "/admin/instance_texts/:key",
    page: "/admin/settings",
    write: async (user) => {
      await user.type(await screen.findByRole("textbox", { name: "Name" }), "Example Social");
      await user.click(screen.getByRole("button", { name: "Save" }));
      expect(await screen.findByText("Saved the name.")).toBeInTheDocument();
    },
  },
  {
    action: "setting_changed",
    method: "patch",
    path: "/admin/settings",
    page: "/admin/settings",
    write: async (user) => {
      await user.click(await screen.findByRole("radio", { name: "With an invite" }));
      await user.click(screen.getByRole("button", { name: "Save the settings" }));
      expect(await screen.findByText(/^Saved the settings\./)).toBeInTheDocument();
    },
  },
  {
    action: "rule_added",
    method: "post",
    path: "/admin/rules",
    page: "/admin/rules",
    write: async (user) => {
      await user.type(await screen.findByRole("textbox", { name: "Rule" }), "Be kind");
      await user.click(screen.getByRole("button", { name: "Add the rule" }));
      expect(await screen.findByText(/^Added the rule/)).toBeInTheDocument();
    },
  },
];

describe("the audit log after an audited write", () => {
  test.each(writes)(
    "back on the log after $action, the operation is in it at once, however slowly the log comes",
    async ({ action, method, path, page, setup, write }) => {
      let entries: Schemas["AdminAuditEntry"][] = [];
      server.use(
        http.get(`${apiBase}/admin/audit_log`, () => HttpResponse.json({ items: entries })),
        // The server records the write; the handler falls through to the one that answers it.
        http[method](`${apiBase}${path}`, () => {
          entries = [
            {
              id: "0192f0c4-7a8e-7000-8000-0000000a0475",
              actor_kind: "user",
              actor_account_id: viewerFixture.id,
              action,
              target_kind: "account",
              target_id: otherAccountFixture.id,
              target_label: "@bob",
              target_exists: true,
              occurred_at: "2026-10-17T09:30:00Z",
            },
          ];
        }),
      );
      setup?.();
      const { user, router } = await renderApp({ path: "/moderation/audit-log" });
      expect(await screen.findByRole("heading", { level: 2, name: "No operations yet" })).toBeInTheDocument();
      expect(screen.queryByRole("table")).not.toBeInTheDocument();

      await router.navigate({ href: page });
      // From now on the audit log takes a while; the handler falls through to the one above.
      server.use(
        http.get(`${apiBase}/admin/audit_log`, async () => {
          await delay(300);
        }),
      );
      await write(user);
      expect(entries).toHaveLength(1);

      await router.navigate({ to: "/moderation/audit-log" });
      const after = await screen.findByRole("table");
      expect(within(after).getAllByRole("rowheader").length).toBeGreaterThan(0);
    },
  );
});
