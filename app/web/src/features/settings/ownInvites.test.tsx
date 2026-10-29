// SPDX-License-Identifier: AGPL-3.0-only
// The user's page of their own invites through the whole app (web-client/README.md T4, ADR-0108 §2, §5): the settings
// list it only for a user who may create invites; the form offers only the choices within the caps and starts at the
// server's defaults; a refused invite says why; a user who holds as many invites as allowed is told so; a user who may
// no longer create invites still lists and revokes theirs; revoking moves focus to the invite's heading; with MSW
// playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { ownInviteFixture, ownInvitePageFixture } from "./test/handlers";

/** Plays the user's invites, newest first, with `page` beyond them; records the new ones and the revocations. */
function invitesServer(
  initial: readonly Schemas["OwnInvite"][],
  page: Partial<Omit<Schemas["OwnInvitePage"], "items">> = {},
) {
  let invites = [...initial];
  const created: Schemas["CreateOwnInviteRequest"][] = [];
  const revoked: string[] = [];
  server.use(
    http.get(`${apiBase}/invites`, () => HttpResponse.json(ownInvitePageFixture(invites, page))),
    http.post(`${apiBase}/invites`, async ({ request }) => {
      const body = (await request.json()) as Schemas["CreateOwnInviteRequest"];
      created.push(body);
      const invite = ownInviteFixture("0192f0c4-7a8e-7000-8000-0000000009b1", "NEWCODE234567ABC", {
        max_uses: body.max_uses ?? 1,
      });
      invites = [invite, ...invites];
      return HttpResponse.json(invite);
    }),
    http.post(`${apiBase}/invites/:id/revoke`, ({ params }) => {
      const id = String(params["id"]);
      revoked.push(id);
      invites = invites.map((invite) => (invite.id === id ? { ...invite, state: "revoked" as const } : invite));
      return HttpResponse.json(invites.find((invite) => invite.id === id));
    }),
  );
  return { created, revoked };
}

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
  server.use(signedInHandler);
});

describe("the user's invites page", () => {
  test("is not in the settings for a user who may not create invites", async () => {
    invitesServer([], { can_create: false });
    await renderApp({ path: "/settings" });

    expect(await screen.findByRole("link", { name: "Import and export" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Your invites" })).not.toBeInTheDocument();
  });

  test("is in the settings for a user who may create invites", async () => {
    invitesServer([]);
    await renderApp({ path: "/settings" });

    expect(await screen.findByRole("link", { name: "Your invites" })).toHaveAttribute("href", "/settings/invites");
  });

  test("lists the invites with their states and how they were used", async () => {
    invitesServer([
      ownInviteFixture("i1", "AAAABBBBCCCCDDDD", { max_uses: 5, uses: 2 }),
      ownInviteFixture("i2", "EEEEFFFFGGGGHHHH", { state: "creator_ineligible" }),
      ownInviteFixture("i3", "JJJJKKKKLLLLMMMM", { state: "exhausted", uses: 1 }),
    ]);
    await renderApp({ path: "/settings/invites" });

    const list = await screen.findByRole("region", { name: "Invites you created" });
    const [first, second, third] = within(list).getAllByRole("listitem");
    if (first === undefined || second === undefined || third === undefined) throw new Error("expected three invites");
    expect(within(first).getByRole("heading", { level: 3, name: "AAAABBBBCCCCDDDD" })).toBeInTheDocument();
    expect(within(first).getByText("Usable")).toBeInTheDocument();
    expect(within(first).getByText("2 of 5")).toBeInTheDocument();
    expect(within(first).getByRole("button", { name: "Copy link: AAAABBBBCCCCDDDD" })).toBeEnabled();
    expect(within(second).getByText("Not usable while you cannot invite")).toBeInTheDocument();
    expect(within(second).queryByRole("button")).not.toBeInTheDocument();
    expect(within(third).getByText("Used up")).toBeInTheDocument();
    expect(screen.getByText("You have 1 of 5 usable invites.")).toBeInTheDocument();
  });

  test("offers only the choices within the caps, starting at the server's defaults, and creates the invite", async () => {
    const invites = invitesServer([], {
      limits: {
        max_active: 5,
        max_uses: 3,
        max_expires_in: 2 * 86_400,
        default_max_uses: 1,
        default_expires_in: 86_400,
      },
    });
    const { user } = await renderApp({ path: "/settings/invites" });
    const form = await screen.findByRole("region", { name: "Create an invite" });
    expect(within(form).getByText("How many people may sign up with the invite, up to 3.")).toBeInTheDocument();

    await user.click(within(form).getByRole("button", { name: /Sign-ups/ }));
    expect((await screen.findAllByRole("option")).map((option) => option.textContent)).toEqual([
      "1 person",
      "2 people",
      "3 people",
    ]);
    await user.click(screen.getByRole("option", { name: "3 people" }));
    await user.click(within(form).getByRole("button", { name: /Works for/ }));
    expect((await screen.findAllByRole("option")).map((option) => option.textContent)).toEqual([
      "1 hour",
      "6 hours",
      "12 hours",
      "1 day",
      "2 days",
    ]);
    await user.click(screen.getByRole("option", { name: "2 days" }));
    await user.click(within(form).getByRole("button", { name: "Create invite" }));

    expect(await within(form).findByText("Created the invite NEWCODE234567ABC.")).toBeInTheDocument();
    expect(within(form).getByText("https://my.example/auth/sign_up?invite=NEWCODE234567ABC")).toHaveAttribute(
      "dir",
      "ltr",
    );
    expect(invites.created).toEqual([{ max_uses: 3, expires_in: 2 * 86_400 }]);
    expect(await screen.findByRole("heading", { level: 3, name: "NEWCODE234567ABC" })).toBeInTheDocument();
  });

  test("sends the server's defaults when nothing is chosen", async () => {
    const invites = invitesServer([]);
    const { user } = await renderApp({ path: "/settings/invites" });
    const form = await screen.findByRole("region", { name: "Create an invite" });
    await user.click(within(form).getByRole("button", { name: "Create invite" }));

    expect(await within(form).findByText(/^Created the invite/)).toBeInTheDocument();
    expect(invites.created).toEqual([{ max_uses: 1, expires_in: 7 * 86_400 }]);
  });

  test("says why the server refused an invite, and keeps the choices", async () => {
    invitesServer([]);
    server.use(
      http.post(`${apiBase}/invites`, () =>
        problem(422, "/api/v1/problems/validation", "max_uses is over the limit of the server's settings."),
      ),
    );
    const { user } = await renderApp({ path: "/settings/invites" });
    const form = await screen.findByRole("region", { name: "Create an invite" });
    await user.click(within(form).getByRole("button", { name: "Create invite" }));

    expect(await within(form).findByRole("alert")).toHaveTextContent(
      "Could not create the invite: max_uses is over the limit of the server's settings.",
    );
    expect(within(form).getByRole("button", { name: /Sign-ups/ })).toHaveTextContent("1 person");
  });

  test("tells a user who holds as many usable invites as allowed, and shows why the server refuses another", async () => {
    invitesServer([ownInviteFixture("i1", "AAAABBBBCCCCDDDD")], {
      limits: {
        max_active: 1,
        max_uses: 10,
        max_expires_in: 30 * 86_400,
        default_max_uses: 1,
        default_expires_in: 7 * 86_400,
      },
    });
    server.use(
      http.post(`${apiBase}/invites`, () =>
        problem(409, "/api/v1/problems/too-many-invites", "You already hold as many usable invites as allowed."),
      ),
    );
    const { user } = await renderApp({ path: "/settings/invites" });
    const form = await screen.findByRole("region", { name: "Create an invite" });

    expect(within(form).getByText("You have 1 of 1 usable invites.")).toBeInTheDocument();
    expect(within(form).getByText(/^You have as many usable invites as you may\./)).toBeInTheDocument();
    await user.click(within(form).getByRole("button", { name: "Create invite" }));
    expect(await within(form).findByRole("alert")).toHaveTextContent(
      "You already hold as many usable invites as allowed.",
    );
  });

  test("a user who may no longer create invites sees why instead of the form, and still revokes theirs", async () => {
    const invites = invitesServer([ownInviteFixture("i1", "AAAABBBBCCCCDDDD")], { can_create: false });
    const { user } = await renderApp({ path: "/settings/invites" });
    const form = await screen.findByRole("region", { name: "Create an invite" });

    expect(within(form).getByText(/^You cannot create invites now\./)).toBeInTheDocument();
    expect(within(form).queryByRole("button")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^Revoke the invite AAAABBBBCCCCDDDD/ }));
    expect(await screen.findByText(/^Revoked the invite AAAABBBBCCCCDDDD\./)).toBeInTheDocument();
    expect(invites.revoked).toEqual(["i1"]);
  });

  test("revokes an invite, and focus moves to its heading", async () => {
    const invites = invitesServer([ownInviteFixture("i1", "AAAABBBBCCCCDDDD")]);
    const { user } = await renderApp({ path: "/settings/invites" });
    await user.click(await screen.findByRole("button", { name: /^Revoke the invite AAAABBBBCCCCDDDD/ }));

    expect(
      await screen.findByText(
        "Revoked the invite AAAABBBBCCCCDDDD. The people who signed up with it keep their accounts.",
      ),
    ).toBeInTheDocument();
    const heading = screen.getByRole("heading", { level: 3, name: "AAAABBBBCCCCDDDD" });
    await waitFor(() => {
      expect(heading).toHaveFocus();
    });
    const list = screen.getByRole("region", { name: "Invites you created" });
    expect(within(list).getByText("Revoked")).toBeInTheDocument();
    expect(invites.revoked).toEqual(["i1"]);
    expect(await screen.findByText("You have 0 of 5 usable invites.")).toBeInTheDocument();
  });

  test("says when a revocation fails, and the invite stays usable", async () => {
    invitesServer([ownInviteFixture("i1", "AAAABBBBCCCCDDDD")]);
    server.use(http.post(`${apiBase}/invites/:id/revoke`, () => problem(404, "about:blank", "Not found.")));
    const { user } = await renderApp({ path: "/settings/invites" });
    await user.click(await screen.findByRole("button", { name: /^Revoke the invite AAAABBBBCCCCDDDD/ }));

    expect(await screen.findByText("Could not revoke the invite: Not found.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Revoke the invite AAAABBBBCCCCDDDD/ })).toBeInTheDocument();
  });
});
