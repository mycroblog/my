// SPDX-License-Identifier: AGPL-3.0-only
// Granting and revoking the roles of a local account on its admin page through the whole app (web-client/README.md T4,
// FR-720): a viewer with manage_roles sees every role of the instance, changes the ones at or below their highest
// role after a confirmation, and reads why the others cannot be changed and why the last admin keeps admin, with
// MSW playing the role API.
import { screen, waitFor, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { otherAccountFixture, type Schemas, viewerFixture } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { adminAccountDetailFixture, adminRoleFixture, moderatorRoleFixture, rolesFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_accounts", "manage_roles", "view_audit"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Role = Schemas["AdminRole"];

/** A key or handle inside a sentence, as the screens isolate them. */
const ltr = (text: string) => `⁦${text}⁩`;
const handle = `⁨@bob⁩`;

/**
 * Plays the role API for Bob holding `held` and the viewer holding `own`, and Bob's admin page with his role keys;
 * returns the role changes the app sent, as "METHOD role".
 */
function rolesServer(held: Role[], own: Role[] = [adminRoleFixture]) {
  let bobRoles = held;
  const changes: string[] = [];
  const answer = () => HttpResponse.json({ items: rolesFixture.filter((role) => bobRoles.some((other) => other.id === role.id)) });
  server.use(
    http.get(`${apiBase}/admin/accounts/:id`, () => HttpResponse.json(adminAccountDetailFixture(otherAccountFixture, { roles: bobRoles.map((role) => role.key) }))),
    http.get(`${apiBase}/admin/accounts/:id/roles`, ({ params }) => (params["id"] === viewerFixture.id ? HttpResponse.json({ items: own }) : answer())),
    http.put(`${apiBase}/admin/accounts/:id/roles/:role`, ({ params }) => {
      changes.push(`PUT ${String(params["role"])}`);
      bobRoles = [...bobRoles, ...rolesFixture.filter((role) => role.key === params["role"])];
      return answer();
    }),
    http.delete(`${apiBase}/admin/accounts/:id/roles/:role`, ({ params }) => {
      changes.push(`DELETE ${String(params["role"])}`);
      bobRoles = bobRoles.filter((role) => role.key !== params["role"]);
      return answer();
    }),
  );
  return changes;
}

/** The row of the role `key` in the roles region. */
function roleRow(roles: HTMLElement, key: string) {
  const row = within(roles)
    .getAllByRole("listitem")
    .find((item) => within(item).queryByRole("heading", { level: 3, name: key }) !== null);
  if (row === undefined) throw new Error(`no row of ${key}`);
  return row;
}

async function openRoles() {
  const { user } = await renderApp({ path: `/moderation/accounts/${otherAccountFixture.id}` });
  const roles = await screen.findByRole("region", { name: "Roles" });
  await within(roles).findByRole("heading", { level: 3, name: "admin" });
  return { user, roles };
}

describe("an account's roles", () => {
  test("lists every role with its permissions and whether the account holds it", async () => {
    rolesServer([moderatorRoleFixture]);
    const { roles } = await openRoles();

    const admin = within(roleRow(roles, "admin"));
    expect(admin.getByText("Does not have the role")).toBeInTheDocument();
    expect(admin.getByRole("button", { name: `Grant ${ltr("admin")}` })).toBeInTheDocument();
    expect(admin.getByText(/^manage_reports, manage_accounts/)).toHaveAttribute("dir", "ltr");
    const moderator = within(roleRow(roles, "moderator"));
    expect(moderator.getByText("Has the role")).toBeInTheDocument();
    expect(moderator.getByText("manage_reports, manage_accounts, view_audit")).toBeInTheDocument();
    expect(moderator.getByRole("button", { name: `Revoke ${ltr("moderator")}` })).toBeInTheDocument();
  });

  test("grants a role after its confirmation, says so, and moves focus to the region's heading", async () => {
    const changes = rolesServer([]);
    const { user, roles } = await openRoles();

    await user.click(within(roleRow(roles, "moderator")).getByRole("button", { name: `Grant ${ltr("moderator")}` }));
    const dialog = await screen.findByRole("alertdialog", { name: `Grant ${ltr("moderator")} to ${handle}?` });
    await waitFor(() => {
      expect(within(dialog).getByRole("button", { name: "Cancel" })).toHaveFocus();
    });
    expect(changes).toEqual([]);
    await user.click(within(dialog).getByRole("button", { name: "Grant" }));

    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(changes).toEqual(["PUT moderator"]);
    expect(within(roles).getByRole("status")).toHaveTextContent(`${ltr("moderator")} is granted to ${handle}. The audit log records the change.`);
    expect(within(roleRow(roles, "moderator")).getByText("Has the role")).toBeInTheDocument();
    expect(within(roleRow(roles, "moderator")).getByRole("button", { name: `Revoke ${ltr("moderator")}` })).toBeInTheDocument();
    await waitFor(() => {
      expect(within(roles).getByRole("heading", { level: 2, name: "Roles" })).toHaveFocus();
    });
    // The account is read again: its summary lists the role.
    expect(await within(screen.getByRole("region", { name: "Account" })).findByText("moderator")).toBeInTheDocument();
  });

  // #471: the audit log, shown from its cache as the route loads it, must not show the operations as they were
  // before the change while a slow server answers it again.
  test("back on the audit log after a grant, the change is in it at once, however slowly the log comes", async () => {
    const changes = rolesServer([]);
    let entries: Schemas["AdminAuditEntry"][] = [];
    server.use(
      http.get(`${apiBase}/admin/audit_log`, () => HttpResponse.json({ items: entries })),
      http.put(`${apiBase}/admin/accounts/:id/roles/:role`, () => {
        entries = [
          {
            id: "0192f0c4-7a8e-7000-8000-0000000a0009",
            actor_kind: "user",
            actor_account_id: viewerFixture.id,
            action: "role_granted",
            target_kind: "user",
            target_id: otherAccountFixture.id,
            target_label: "@bob",
            target_exists: true,
            occurred_at: "2026-10-17T09:30:00Z",
          },
        ];
        // Falls through to rolesServer's, which grants the role.
      }),
    );
    const { user, router } = await renderApp({ path: "/moderation/audit-log" });
    expect(await screen.findByRole("heading", { level: 2, name: "No operations yet" })).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    await router.navigate({ to: "/moderation/accounts/$id", params: { id: otherAccountFixture.id } });
    const roles = await screen.findByRole("region", { name: "Roles" });
    await user.click(await within(roles).findByRole("button", { name: `Grant ${ltr("moderator")}` }));
    // From now on the audit log takes a while; the handler falls through to the one above.
    server.use(
      http.get(`${apiBase}/admin/audit_log`, async () => {
        await delay(300);
      }),
    );
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Grant" }));
    expect(await within(roles).findByText(`${ltr("moderator")} is granted to ${handle}. The audit log records the change.`)).toBeInTheDocument();
    expect(changes).toEqual(["PUT moderator"]);

    await router.navigate({ to: "/moderation/audit-log" });
    const after = await screen.findByRole("table");
    expect(within(after).getAllByRole("rowheader").length).toBeGreaterThan(0);
  });

  test("revokes a role after its confirmation; cancel changes nothing and returns focus to the button", async () => {
    const changes = rolesServer([moderatorRoleFixture]);
    const { user, roles } = await openRoles();

    const revoke = within(roleRow(roles, "moderator")).getByRole("button", { name: `Revoke ${ltr("moderator")}` });
    await user.click(revoke);
    expect(await screen.findByRole("alertdialog", { name: `Revoke ${ltr("moderator")} from ${handle}?` })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(revoke).toHaveFocus();
    });
    expect(changes).toEqual([]);

    await user.click(revoke);
    const dialog = await screen.findByRole("alertdialog", { name: `Revoke ${ltr("moderator")} from ${handle}?` });
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    expect(await within(roles).findByText(`${ltr("moderator")} is revoked from ${handle}. The audit log records the change.`)).toBeInTheDocument();
    expect(changes).toEqual(["DELETE moderator"]);
    expect(within(roleRow(roles, "moderator")).getByText("Does not have the role")).toBeInTheDocument();
    await waitFor(() => {
      expect(within(roles).getByRole("heading", { level: 2, name: "Roles" })).toHaveFocus();
    });
  });

  test("a role above the viewer's highest one has no button, and says why", async () => {
    rolesServer([], [moderatorRoleFixture]);
    const { roles } = await openRoles();

    const admin = within(roleRow(roles, "admin"));
    expect(admin.queryByRole("button")).not.toBeInTheDocument();
    expect(admin.getByText(`This role is above your highest role, ${ltr("moderator")}, so you cannot grant or revoke it.`)).toBeInTheDocument();
    expect(within(roleRow(roles, "moderator")).getByRole("button", { name: `Grant ${ltr("moderator")}` })).toBeInTheDocument();
  });

  test("a role with a permission the viewer lacks has no grant button, and says which", async () => {
    const lead: Role = { id: "0192f0c4-7a8e-7000-8000-000000000a09", key: "lead", position: 50, permissions: ["manage_reports", "manage_webhooks"] };
    const reporter: Role = { ...moderatorRoleFixture, id: "0192f0c4-7a8e-7000-8000-000000000a08", key: "reporter", position: 200, permissions: ["manage_reports"] };
    rolesServer([], [reporter]);
    server.use(http.get(`${apiBase}/admin/roles`, () => HttpResponse.json({ items: [adminRoleFixture, reporter, moderatorRoleFixture, lead] })));
    const { roles } = await openRoles();

    const row = within(roleRow(roles, "lead"));
    expect(row.queryByRole("button")).not.toBeInTheDocument();
    expect(row.getByText("You cannot grant this role: it needs permissions you do not have (manage_webhooks).")).toBeInTheDocument();
    // moderator also needs manage_accounts and view_audit.
    expect(within(roleRow(roles, "moderator")).queryByRole("button")).not.toBeInTheDocument();
    expect(within(roleRow(roles, "reporter")).getByRole("button", { name: `Grant ${ltr("reporter")}` })).toBeInTheDocument();
  });

  test("a change the server refuses for a role above the viewer's stays in the confirmation with the reason", async () => {
    rolesServer([]);
    server.use(http.put(`${apiBase}/admin/accounts/:id/roles/:role`, () => problem(403, "about:blank", "Forbidden")));
    const { user, roles } = await openRoles();

    await user.click(within(roleRow(roles, "admin")).getByRole("button", { name: `Grant ${ltr("admin")}` }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Grant" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "The role could not be changed: it is above your highest role, or it has permissions you do not hold. You can grant and revoke only the roles at or below your highest role, and grant only roles whose permissions you all hold.",
    );
  });

  test("the last admin keeps admin: the confirmation says so before and after the server refuses", async () => {
    rolesServer([adminRoleFixture]);
    server.use(http.delete(`${apiBase}/admin/accounts/:id/roles/:role`, () => problem(409, "about:blank", "Conflict")));
    const { user, roles } = await openRoles();

    await user.click(within(roleRow(roles, "admin")).getByRole("button", { name: `Revoke ${ltr("admin")}` }));
    const dialog = await screen.findByRole("alertdialog");
    expect(dialog).toHaveAccessibleDescription(/cannot be revoked from its only holder/);
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "The role could not be revoked: this account is the only admin of this server. Grant admin to another account first.",
    );
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(within(roleRow(roles, "admin")).getByText("Has the role")).toBeInTheDocument();
  });

  test("a failure to load the roles stays in the region with a retry", async () => {
    rolesServer([]);
    let failing = true;
    server.use(http.get(`${apiBase}/admin/roles`, () => (failing ? problem(503, "about:blank", "Service Unavailable") : HttpResponse.json({ items: rolesFixture }))));
    const { user } = await renderApp({ path: `/moderation/accounts/${otherAccountFixture.id}` });

    const roles = await screen.findByRole("region", { name: "Roles" });
    expect(await within(roles).findByRole("heading", { level: 3, name: "The roles could not be loaded" })).toBeInTheDocument();
    failing = false;
    await user.click(within(roles).getByRole("button", { name: "Try again" }));
    expect(await within(roles).findByRole("heading", { level: 3, name: "admin" })).toBeInTheDocument();
  });

  test("a viewer without manage_roles has no roles region and asks for no roles", async () => {
    server.use(operatorHandler(["manage_accounts"]));
    const requests: string[] = [];
    server.events.on("request:start", ({ request }) => {
      if (request.url.includes("/roles")) requests.push(request.url);
    });
    await renderApp({ path: `/moderation/accounts/${otherAccountFixture.id}` });

    expect(await screen.findByRole("region", { name: "Account" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Roles" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Grant |^Revoke / })).not.toBeInTheDocument();
    expect(requests).toEqual([]);
  });
});
