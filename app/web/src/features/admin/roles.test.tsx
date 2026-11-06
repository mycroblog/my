// SPDX-License-Identifier: AGPL-3.0-only
// The roles screen through the whole app (web-client/README.md T4, FR-720): an administrator with manage_roles sees the
// roles in a table with their permissions counted by area, creates a role on its page, changes a role's position,
// permissions and badge on the role's page with one save, and deletes a role there after asking; the built-in roles
// and the roles above the viewer offer only what the server allows, and refusals stay next to their field or in the
// dialog. MSW plays the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { createdRoleId } from "./test/handlers";

const everyPermission: Schemas["Permission"][] = [
  "manage_reports",
  "manage_accounts",
  "manage_domains",
  "manage_rules",
  "manage_settings",
  "manage_roles",
  "manage_invites",
  "view_audit",
  "manage_jobs",
  "manage_login_recovery",
  "manage_custom_emojis",
  "manage_webhooks",
  "view_finance",
];
const admin: Schemas["AdminRole"] = {
  id: "0199f0e0-0000-7000-8000-0000000000a1",
  key: "admin",
  position: 1000,
  permissions: everyPermission,
};
const moderator: Schemas["AdminRole"] = {
  id: "0199f0e0-0000-7000-8000-0000000000a2",
  key: "moderator",
  position: 100,
  permissions: ["manage_reports", "manage_accounts", "view_audit"],
};
const helper: Schemas["AdminRole"] = {
  id: "0199f0e0-0000-7000-8000-0000000000a3",
  key: "helper",
  position: 50,
  permissions: ["manage_reports", "manage_jobs"],
};

function problem(reason: Schemas["RoleProblemReason"], detail: string, missing?: Schemas["Permission"][]) {
  return HttpResponse.json(
    {
      type: "/api/v1/problems/invalid-role",
      title: "The role cannot be saved.",
      status: 422,
      detail,
      reason,
      ...(missing === undefined ? {} : { missing_permissions: missing }),
    },
    { status: 422, headers: { "Content-Type": "application/problem+json" } },
  );
}

/** The roles of the instance, and the viewer's own roles. */
function rolesHandlers(roles: () => Schemas["AdminRole"][], own: Schemas["AdminRole"][] = [admin]) {
  return [
    http.get(`${apiBase}/admin/roles`, () => HttpResponse.json({ items: roles() })),
    http.get(`${apiBase}/admin/accounts/:id/roles`, () => HttpResponse.json({ items: own })),
  ];
}

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

describe("the roles screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(everyPermission));
  });

  test("tells a viewer without the permission why it is empty", async () => {
    server.use(operatorHandler(["manage_settings"]));
    await renderApp({ path: "/admin/roles" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
  });

  test("lists the roles in a table, each key a link to its page", async () => {
    server.use(...rolesHandlers(() => [admin, moderator, helper]));
    await renderApp({ path: "/admin/roles" });
    const table = await screen.findByRole("table", { name: "Roles of this server" });

    const names = within(table).getAllByRole("rowheader");
    expect(names.map((name) => name.textContent)).toEqual(["admin", "moderator", "helper"]);
    expect(within(names[2] as HTMLElement).getByRole("link", { name: "helper" })).toHaveAttribute(
      "href",
      "/admin/roles/helper",
    );
    const helperRow = within(table).getAllByRole("row")[3] as HTMLElement;
    expect(within(helperRow).getByText("50")).toBeInTheDocument();
    expect(within(helperRow).getByText("Moderation: 1")).toBeInTheDocument();
    expect(within(helperRow).getByText("Server administration: 1")).toBeInTheDocument();
    expect(within(helperRow).getByText("Member features: 0")).toBeInTheDocument();
    expect(within(helperRow).getByText("No badge")).toBeInTheDocument();
    // No form, no dialog and no buttons on the list: each role has its page.
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(within(table).queryByRole("button")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Create a role" })).toHaveAttribute("href", "/admin/roles/new");
  });

  test("shows a role's permissions by area on its page, and what the viewer may do with it", async () => {
    server.use(...rolesHandlers(() => [admin, moderator, helper]));
    const { router } = await renderApp({ path: "/admin/roles/moderator" });
    const facts = await screen.findByRole("region", { name: "About this role" });

    expect(within(facts).getByText("Reports, Accounts and registrations, Audit log")).toBeInTheDocument();
    // No server administration permission and no member permission.
    expect(within(facts).getAllByText("None")).toHaveLength(2);
    expect(within(facts).getByText(/it cannot be deleted/)).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Edit the role" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Delete the role" })).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Save" })).toHaveLength(1);

    // Admin changes only its badge (ADR-0070).
    await router.navigate({ to: "/admin/roles/$key", params: { key: "admin" } });
    const adminForm = await screen.findByRole("region", { name: "Edit the role" });
    expect(within(adminForm).queryByRole("textbox", { name: "Position" })).not.toBeInTheDocument();
    expect(within(adminForm).queryByRole("checkbox")).not.toBeInTheDocument();
    expect(within(adminForm).getByRole("textbox", { name: "Name" })).toBeInTheDocument();

    await router.navigate({ to: "/admin/roles/$key", params: { key: "helper" } });
    expect(await screen.findByRole("region", { name: "Delete the role" })).toBeInTheDocument();
  });

  test("offers nothing on the roles above the viewer's highest role, and only the viewer's permissions", async () => {
    server.use(
      operatorHandler(["manage_roles", "manage_reports", "manage_accounts", "view_audit"]),
      ...rolesHandlers(() => [admin, moderator, helper], [moderator]),
    );
    const { router } = await renderApp({ path: "/admin/roles/admin" });
    const facts = await screen.findByRole("region", { name: "About this role" });

    expect(
      within(facts).getByText("Above your highest role, ⁦moderator⁩, so you cannot change it."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Edit the role" })).not.toBeInTheDocument();
    expect(within(screen.getByRole("main")).queryByRole("button")).not.toBeInTheDocument();

    await router.navigate({ to: "/admin/roles/new" });
    expect(await screen.findByRole("checkbox", { name: "Reports" })).toBeEnabled();
    expect(screen.getByRole("checkbox", { name: "Failed jobs" })).toBeDisabled();
    expect(screen.getByText(/A whole number from 0 to 100\./)).toBeInTheDocument();
  });

  test("creates a role on its page, then opens the role's page", async () => {
    let sent: Schemas["CreateRoleRequest"] | undefined;
    server.use(
      ...rolesHandlers(() =>
        sent === undefined ? [admin, moderator] : [admin, moderator, { id: createdRoleId, ...sent }],
      ),
      http.post(`${apiBase}/admin/roles`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateRoleRequest"];
        return HttpResponse.json({ id: createdRoleId, ...sent });
      }),
    );
    const { user, router } = await renderApp({ path: "/admin/roles/new" });
    expect(await screen.findByRole("heading", { level: 1, name: "Create a role" })).toBeInTheDocument();

    await user.type(screen.getByRole("textbox", { name: "Key" }), "triage");
    await user.type(screen.getByRole("textbox", { name: "Position" }), "40");
    const main = screen.getByRole("main");
    const moderation = within(main).getByRole("group", { name: "Moderation" });
    await user.click(within(moderation).getByRole("checkbox", { name: "Audit log" }));
    await user.click(within(moderation).getByRole("checkbox", { name: "Reports" }));
    // A member permission is a group of its own, outside the staff's areas (ADR-0070).
    await user.click(
      within(within(main).getByRole("group", { name: "Member features" })).getByRole("checkbox", {
        name: "Read the finances",
      }),
    );
    await user.click(screen.getByRole("button", { name: "Create role" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Role" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/admin/roles/triage");
    expect(await screen.findByText(/^Created the role ⁦triage⁩\./)).toBeInTheDocument();
    expect(sent).toEqual({
      key: "triage",
      position: 40,
      permissions: ["manage_reports", "view_audit", "view_finance"],
    });
    expect(within(screen.getByRole("region", { name: "About this role" })).getByText("triage")).toBeInTheDocument();
  });

  test("checks the key and the position before sending, and shows a refusal on its field", async () => {
    let posts = 0;
    server.use(
      ...rolesHandlers(() => [admin, moderator]),
      http.post(`${apiBase}/admin/roles`, () => {
        posts++;
        return problem("key_taken", "Another role has this key.");
      }),
    );
    const { user } = await renderApp({ path: "/admin/roles/new" });
    const key = await screen.findByRole("textbox", { name: "Key" });
    const position = screen.getByRole("textbox", { name: "Position" });

    await user.type(key, "Triage Team");
    await user.type(position, "10");
    await user.click(screen.getByRole("button", { name: "Create role" }));
    expect(key).toHaveAccessibleDescription(/Enter up to 64 lowercase letters/);
    expect(key).toBeInvalid();

    // "new" is the address of this page, so no role can have it as its key.
    await user.clear(key);
    await user.type(key, "new");
    await user.click(screen.getByRole("button", { name: "Create role" }));
    expect(key).toHaveAccessibleDescription(/"new" cannot be a role key/);

    await user.clear(key);
    await user.type(key, "triage");
    await user.clear(position);
    await user.type(position, "1000");
    await user.click(screen.getByRole("button", { name: "Create role" }));
    expect(position).toHaveAccessibleDescription(/Enter a whole number from 0 to 999\./);
    expect(posts).toBe(0);

    await user.clear(position);
    await user.type(position, "10");
    await user.click(screen.getByRole("button", { name: "Create role" }));
    await waitFor(() => {
      expect(key).toHaveAccessibleDescription(/Another role has this key\./);
    });
    expect(posts).toBe(1);
    expect(key).toHaveValue("triage");
  });

  test("changes the moderator role on its page with one save", async () => {
    let current = moderator;
    let sent: Schemas["UpdateRoleRequest"] | undefined;
    server.use(
      ...rolesHandlers(() => [admin, current]),
      http.patch(`${apiBase}/admin/roles/:role`, async ({ request }) => {
        sent = (await request.json()) as Schemas["UpdateRoleRequest"];
        current = {
          ...moderator,
          position: sent.position ?? moderator.position,
          permissions: sent.permissions ?? moderator.permissions,
        };
        return HttpResponse.json(current);
      }),
    );
    const { user } = await renderApp({ path: "/admin/roles/moderator" });
    const form = await screen.findByRole("region", { name: "Edit the role" });

    const position = within(form).getByRole("textbox", { name: "Position" });
    await user.clear(position);
    await user.type(position, "200");
    await user.click(within(form).getByRole("checkbox", { name: "Audit log" }));
    await user.click(within(form).getByRole("checkbox", { name: "Invites" }));
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(await within(form).findByRole("status")).toHaveTextContent("Changed the role ⁦moderator⁩.");
    expect(sent).toEqual({
      position: 200,
      permissions: ["manage_reports", "manage_accounts", "manage_invites"],
    });
    expect(await within(screen.getByRole("region", { name: "About this role" })).findByText("200")).toBeInTheDocument();
  });

  test("keeps a refused change in the form", async () => {
    server.use(
      ...rolesHandlers(() => [admin, moderator]),
      http.patch(`${apiBase}/admin/roles/:role`, () =>
        problem("missing_permission", "You can give or take only the permissions you hold.", ["manage_jobs"]),
      ),
    );
    const { user } = await renderApp({ path: "/admin/roles/moderator" });
    const form = await screen.findByRole("region", { name: "Edit the role" });
    await user.click(within(form).getByRole("checkbox", { name: "Failed jobs" }));
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(await within(form).findByText(/^You can give or take only the permissions you hold\./)).toBeInTheDocument();
    expect(within(form).getByText("Permissions your roles do not grant: ⁦manage_jobs⁩")).toBeInTheDocument();
    expect(within(form).getByRole("checkbox", { name: "Failed jobs" })).toBeChecked();
  });

  test("deletes a role on its page after asking, says why the server refused, then opens the list", async () => {
    let roles = [admin, moderator, helper];
    let held = true;
    server.use(
      ...rolesHandlers(() => roles),
      http.delete(`${apiBase}/admin/roles/:role`, () => {
        if (held) return problem("held", "Revoke the role from everyone who holds it first.");
        roles = [admin, moderator];
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user, router } = await renderApp({ path: "/admin/roles/helper" });
    const region = await screen.findByRole("region", { name: "Delete the role" });
    await user.click(within(region).getByRole("button", { name: "Delete" }));
    let dialog = await screen.findByRole("alertdialog", { name: "Delete the role ⁦helper⁩?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await within(dialog).findByText(/Revoke the role from everyone who holds it first\./)).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });

    held = false;
    await user.click(within(region).getByRole("button", { name: "Delete" }));
    dialog = await screen.findByRole("alertdialog", { name: "Delete the role ⁦helper⁩?" });
    expect(within(dialog).queryByText(/Revoke the role/)).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByText(/^Deleted the role ⁦helper⁩\./)).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/admin/roles");
    await waitFor(() => {
      const table = screen.getByRole("table", { name: "Roles of this server" });
      expect(
        within(table)
          .getAllByRole("rowheader")
          .map((name) => name.textContent),
      ).toEqual(["admin", "moderator"]);
    });
  });
});

describe("the badge of a role", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(everyPermission));
  });

  test("is set with a name and a color, with a sample, in the role's form, and removed with an empty name", async () => {
    let current = helper;
    const sent: Schemas["UpdateRoleRequest"][] = [];
    server.use(
      ...rolesHandlers(() => [admin, moderator, current]),
      http.patch(`${apiBase}/admin/roles/:role`, async ({ request }) => {
        const change = (await request.json()) as Schemas["UpdateRoleRequest"];
        sent.push(change);
        const { badge: _, ...rest } = current;
        current =
          change.remove_badge === true
            ? rest
            : { ...current, ...(change.badge === undefined ? {} : { badge: change.badge }) };
        return HttpResponse.json(current);
      }),
    );
    const { user } = await renderApp({ path: "/admin/roles/helper" });
    const facts = await screen.findByRole("region", { name: "About this role" });
    expect(within(facts).getByText("No badge")).toBeInTheDocument();
    const form = screen.getByRole("region", { name: "Edit the role" });
    const name = within(form).getByRole("textbox", { name: "Name" });

    await user.type(name, "  Helper  ");
    await user.click(within(form).getByRole("radio", { name: "Teal" }));
    // The sample is drawn as the badge will be, from the trimmed name.
    expect(within(form).getByText("Helper")).toHaveAttribute("dir", "auto");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(await within(form).findByRole("status")).toHaveTextContent("Changed the role ⁦helper⁩.");
    expect(sent).toEqual([
      { permissions: ["manage_reports", "manage_jobs"], badge: { label: "Helper", color: "teal" } },
    ]);
    expect(await within(facts).findByText("Helper")).toHaveClass("bg-badge-teal");

    // An empty name removes it, which hides it from every profile.
    await user.clear(name);
    await user.click(within(form).getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(sent[1]).toEqual({
        permissions: ["manage_reports", "manage_jobs"],
        remove_badge: true,
      });
    });
    expect(await within(facts).findByText("No badge")).toBeInTheDocument();
  });

  test("keeps a refusal of the server on the name", async () => {
    server.use(
      ...rolesHandlers(() => [admin, moderator, helper]),
      http.patch(`${apiBase}/admin/roles/:role`, () =>
        problem("badge_label", "The badge name must be 1 to 32 characters on one line."),
      ),
    );
    const { user } = await renderApp({ path: "/admin/roles/helper" });
    const form = await screen.findByRole("region", { name: "Edit the role" });
    const name = within(form).getByRole("textbox", { name: "Name" });
    await user.type(name, "Helper");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(name).toHaveAccessibleDescription(/The badge name must be 1 to 32 characters on one line\./);
    });
    expect(name).toHaveValue("Helper");
  });
});
