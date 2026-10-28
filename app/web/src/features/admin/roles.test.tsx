// SPDX-License-Identifier: AGPL-3.0-only
// The roles screen through the whole app (web-client/README.md T4, FR-720): an administrator with manage_roles sees the
// roles with their permissions by area, creates a role, changes the moderator role in a dialog and deletes a role
// after asking; the built-in roles and the roles above the viewer offer only what the server allows, and refusals
// stay next to their field or in the dialog. MSW plays the API.
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

  test("lists the roles with their permissions by area, and what the viewer may do with each", async () => {
    server.use(...rolesHandlers(() => [admin, moderator, helper]));
    await renderApp({ path: "/admin/roles" });
    const list = await screen.findByRole("region", { name: "Roles of this server" });

    const rows = within(list).getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    const [adminRow, moderatorRow, helperRow] = rows as [HTMLElement, HTMLElement, HTMLElement];
    expect(within(adminRow).getByRole("heading", { level: 3, name: "admin" })).toBeInTheDocument();
    expect(within(adminRow).getByText(/Only its badge can be changed/)).toBeInTheDocument();
    // Admin changes only its badge (ADR-0071).
    expect(
      within(adminRow)
        .getAllByRole("button")
        .map((b) => b.getAttribute("aria-label")),
    ).toEqual(["Change the badge of ⁦admin⁩"]);
    expect(within(moderatorRow).getByText("Reports, Accounts and registrations, Audit log")).toBeInTheDocument();
    // No server administration permission and no member permission.
    expect(within(moderatorRow).getAllByText("None")).toHaveLength(2);
    expect(within(moderatorRow).getByRole("button", { name: "Edit ⁦moderator⁩" })).toBeInTheDocument();
    expect(within(moderatorRow).queryByRole("button", { name: /Delete/ })).not.toBeInTheDocument();
    expect(within(helperRow).getByText("50")).toBeInTheDocument();
    expect(within(helperRow).getByText("Failed jobs")).toBeInTheDocument();
    expect(within(helperRow).getByRole("button", { name: "Delete ⁦helper⁩" })).toBeInTheDocument();
  });

  test("offers nothing on the roles above the viewer's highest role, and only the viewer's permissions", async () => {
    server.use(
      operatorHandler(["manage_roles", "manage_reports", "manage_accounts", "view_audit"]),
      ...rolesHandlers(() => [admin, moderator, helper], [moderator]),
    );
    await renderApp({ path: "/admin/roles" });
    const list = await screen.findByRole("region", { name: "Roles of this server" });

    const adminRow = within(list).getAllByRole("listitem")[0] as HTMLElement;
    expect(
      within(adminRow).getByText("Above your highest role, ⁦moderator⁩, so you cannot change it."),
    ).toBeInTheDocument();
    const form = screen.getByRole("region", { name: "Create a role" });
    expect(within(form).getByRole("checkbox", { name: "Reports" })).toBeEnabled();
    expect(within(form).getByRole("checkbox", { name: "Failed jobs" })).toBeDisabled();
    expect(within(form).getByText(/A whole number from 0 to 100\./)).toBeInTheDocument();
  });

  test("creates a role and lists it", async () => {
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
    const { user } = await renderApp({ path: "/admin/roles" });
    const form = await screen.findByRole("region", { name: "Create a role" });

    await user.type(within(form).getByRole("textbox", { name: "Key" }), "triage");
    await user.type(within(form).getByRole("textbox", { name: "Position" }), "40");
    const moderation = within(form).getByRole("group", { name: "Moderation" });
    await user.click(within(moderation).getByRole("checkbox", { name: "Audit log" }));
    await user.click(within(moderation).getByRole("checkbox", { name: "Reports" }));
    // A member permission is a group of its own, outside the staff's areas (ADR-0070).
    await user.click(
      within(within(form).getByRole("group", { name: "Member features" })).getByRole("checkbox", {
        name: "Read the finances",
      }),
    );
    await user.click(within(form).getByRole("button", { name: "Create role" }));

    expect(await within(form).findByRole("status")).toHaveTextContent("Created the role ⁦triage⁩.");
    expect(sent).toEqual({
      key: "triage",
      position: 40,
      permissions: ["manage_reports", "view_audit", "view_finance"],
    });
    expect(within(form).getByRole("textbox", { name: "Key" })).toHaveValue("");
    const list = screen.getByRole("region", { name: "Roles of this server" });
    expect(await within(list).findByRole("heading", { level: 3, name: "triage" })).toBeInTheDocument();
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
    const { user } = await renderApp({ path: "/admin/roles" });
    const form = await screen.findByRole("region", { name: "Create a role" });
    const key = within(form).getByRole("textbox", { name: "Key" });
    const position = within(form).getByRole("textbox", { name: "Position" });

    await user.type(key, "Triage Team");
    await user.type(position, "10");
    await user.click(within(form).getByRole("button", { name: "Create role" }));
    expect(key).toHaveAccessibleDescription(/Enter up to 64 lowercase letters/);
    expect(key).toBeInvalid();

    await user.clear(key);
    await user.type(key, "triage");
    await user.clear(position);
    await user.type(position, "1000");
    await user.click(within(form).getByRole("button", { name: "Create role" }));
    expect(position).toHaveAccessibleDescription(/Enter a whole number from 0 to 999\./);
    expect(posts).toBe(0);

    await user.clear(position);
    await user.type(position, "10");
    await user.click(within(form).getByRole("button", { name: "Create role" }));
    await waitFor(() => {
      expect(key).toHaveAccessibleDescription(/Another role has this key\./);
    });
    expect(posts).toBe(1);
    expect(key).toHaveValue("triage");
  });

  test("changes the moderator role in a dialog", async () => {
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
    const { user } = await renderApp({ path: "/admin/roles" });
    await user.click(await screen.findByRole("button", { name: "Edit ⁦moderator⁩" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit the role ⁦moderator⁩" });

    const position = within(dialog).getByRole("textbox", { name: "Position" });
    await user.clear(position);
    await user.type(position, "200");
    await user.click(within(dialog).getByRole("checkbox", { name: "Audit log" }));
    await user.click(within(dialog).getByRole("checkbox", { name: "Invites" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(sent).toEqual({ position: 200, permissions: ["manage_reports", "manage_accounts", "manage_invites"] });
    const list = screen.getByRole("region", { name: "Roles of this server" });
    expect(within(list).getByRole("status")).toHaveTextContent("Changed the role ⁦moderator⁩.");
    expect(await within(list).findByText("200")).toBeInTheDocument();
  });

  test("keeps a refused change in the dialog", async () => {
    server.use(
      ...rolesHandlers(() => [admin, moderator]),
      http.patch(`${apiBase}/admin/roles/:role`, () =>
        problem("missing_permission", "You can give or take only the permissions you hold.", ["manage_jobs"]),
      ),
    );
    const { user } = await renderApp({ path: "/admin/roles" });
    await user.click(await screen.findByRole("button", { name: "Edit ⁦moderator⁩" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit the role ⁦moderator⁩" });
    await user.click(within(dialog).getByRole("checkbox", { name: "Failed jobs" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      /^You can give or take only the permissions you hold\./,
    );
    expect(within(dialog).getByText("Permissions your roles do not grant: ⁦manage_jobs⁩")).toBeInTheDocument();
  });

  test("deletes a role after asking, and says why the server refused one", async () => {
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
    const { user } = await renderApp({ path: "/admin/roles" });
    await user.click(await screen.findByRole("button", { name: "Delete ⁦helper⁩" }));
    let dialog = await screen.findByRole("alertdialog", { name: "Delete the role ⁦helper⁩?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await within(dialog).findByText(/Revoke the role from everyone who holds it first\./)).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });

    held = false;
    await user.click(screen.getByRole("button", { name: "Delete ⁦helper⁩" }));
    dialog = await screen.findByRole("alertdialog", { name: "Delete the role ⁦helper⁩?" });
    expect(within(dialog).queryByText(/Revoke the role/)).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    const list = screen.getByRole("region", { name: "Roles of this server" });
    expect(await within(list).findByRole("status")).toHaveTextContent("Deleted the role ⁦helper⁩.");
    expect(within(list).queryByRole("heading", { level: 3, name: "helper" })).not.toBeInTheDocument();
  });
});

describe("the badge of a role", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(everyPermission));
  });

  test("is set with a name and a color, with a sample, and shown in the list", async () => {
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
    const { user } = await renderApp({ path: "/admin/roles" });
    const list = await screen.findByRole("region", { name: "Roles of this server" });
    const helperRow = within(list).getAllByRole("listitem")[2] as HTMLElement;
    expect(within(helperRow).getByText("No badge")).toBeInTheDocument();

    await user.click(within(helperRow).getByRole("button", { name: "Change the badge of ⁦helper⁩" }));
    const dialog = await screen.findByRole("dialog", { name: "Badge of ⁦helper⁩" });
    const name = within(dialog).getByRole("textbox", { name: "Name" });
    // Nothing is sent without a name.
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(name).toHaveAccessibleDescription(/Enter a name of 1 to 32 characters\./);
    expect(sent).toEqual([]);

    await user.type(name, "  Helper  ");
    await user.click(within(dialog).getByRole("radio", { name: "Teal" }));
    // The sample is drawn as the badge will be, from the trimmed name.
    expect(within(dialog).getByText("Helper")).toHaveAttribute("dir", "auto");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(sent).toEqual([{ badge: { label: "Helper", color: "teal" } }]);
    expect(await within(helperRow).findByText("Helper")).toHaveClass("bg-badge-teal");

    // Removing it hides it from every profile.
    await user.click(within(helperRow).getByRole("button", { name: "Change the badge of ⁦helper⁩" }));
    const again = await screen.findByRole("dialog", { name: "Badge of ⁦helper⁩" });
    expect(within(again).getByRole("textbox", { name: "Name" })).toHaveValue("Helper");
    expect(within(again).getByRole("radio", { name: "Teal" })).toBeChecked();
    await user.click(within(again).getByRole("button", { name: "Remove the badge" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(sent[1]).toEqual({ remove_badge: true });
    expect(await within(helperRow).findByText("No badge")).toBeInTheDocument();
  });

  test("keeps a refusal of the server on the name", async () => {
    server.use(
      ...rolesHandlers(() => [admin, moderator, helper]),
      http.patch(`${apiBase}/admin/roles/:role`, () =>
        problem("badge_label", "The badge name must be 1 to 32 characters on one line."),
      ),
    );
    const { user } = await renderApp({ path: "/admin/roles" });
    await user.click(await screen.findByRole("button", { name: "Change the badge of ⁦helper⁩" }));
    const dialog = await screen.findByRole("dialog", { name: "Badge of ⁦helper⁩" });
    const name = within(dialog).getByRole("textbox", { name: "Name" });
    await user.type(name, "Helper");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(name).toHaveAccessibleDescription(/The badge name must be 1 to 32 characters on one line\./);
    });
    expect(dialog).toBeInTheDocument();
  });
});
