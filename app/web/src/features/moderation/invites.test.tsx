// SPDX-License-Identifier: AGPL-3.0-only
// The invites screen through the whole app (web-client/README.md T4): a moderator with manage_invites creates an invite
// with a limit and an expiry or without them, reads its code and link, copies a link and revokes an invite (focus
// moves to its heading); the screen says when the registration mode does not take invites; a viewer without the
// permission sees why; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { inviteFixture, newInvite } from "./test/handlers";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

/** Plays the invites of the server, newest first; records the new ones and the revocations. */
function invitesServer(initial: readonly Schemas["AdminInvite"][]) {
  let invites = [...initial];
  const created: Schemas["CreateInviteRequest"][] = [];
  const revoked: string[] = [];
  server.use(
    http.get(`${apiBase}/admin/invites`, () => HttpResponse.json({ items: invites })),
    http.post(`${apiBase}/admin/invites`, async ({ request }) => {
      const body = (await request.json()) as Schemas["CreateInviteRequest"];
      created.push(body);
      const invite = newInvite(body);
      invites = [invite, ...invites];
      return HttpResponse.json(invite);
    }),
    http.post(`${apiBase}/admin/invites/:id/revoke`, ({ params }) => {
      const id = String(params["id"]);
      revoked.push(id);
      invites = invites.map((invite) => (invite.id === id ? { ...invite, state: "revoked" as const } : invite));
      return HttpResponse.json(invites.find((invite) => invite.id === id));
    }),
  );
  return { created, revoked };
}

describe("the invites", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_invites"]));
    server.use(
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({ ...instanceFixture, registrations: { mode: "invite" } }),
      ),
    );
  });

  test("lists the invites with how they were used", async () => {
    const {
      max_uses: _,
      expires_at: __,
      ...unlimitedInvite
    } = inviteFixture({ id: "0192f0c4-7a8e-7000-8000-0000000006a2", code: "ZZZZYYYYXXXXWWWW", uses: 3 });
    invitesServer([inviteFixture({ uses: 1, state: "exhausted" }), unlimitedInvite]);
    await renderApp({ path: "/moderation/invites" });

    expect(await screen.findByRole("heading", { level: 1, name: "Invites" })).toBeInTheDocument();
    expect(screen.queryByText(/does not take sign-ups with an invite/)).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "Invites" })).toBeInTheDocument();
    const [exhausted, unlimited] = within(screen.getByRole("table", { name: "Invites" }))
      .getAllByRole("row")
      .slice(1);
    if (exhausted === undefined || unlimited === undefined) throw new Error("expected two invites");
    expect(within(exhausted).getByRole("rowheader", { name: "ABCDEFGHJKLMNPQR" })).toBeInTheDocument();
    expect(within(exhausted).getByText("Used up")).toBeInTheDocument();
    expect(within(exhausted).getByText("1 of 1")).toBeInTheDocument();
    expect(within(exhausted).queryByRole("button")).not.toBeInTheDocument();
    expect(within(unlimited).getByRole("rowheader", { name: "ZZZZYYYYXXXXWWWW" })).toBeInTheDocument();
    expect(within(unlimited).getByText("3 (no limit)")).toBeInTheDocument();
    expect(within(unlimited).getByText("Never")).toBeInTheDocument();
  });

  test("creates an invite for one person within a week by default, and shows its code and link", async () => {
    const invites = invitesServer([]);
    const { user } = await renderApp({ path: "/moderation/invites" });
    expect(await screen.findByText("No invites yet")).toBeInTheDocument();
    const form = screen.getByRole("region", { name: "Create an invite" });
    await user.click(within(form).getByRole("button", { name: "Create the invite" }));

    expect(await within(form).findByText("Created the invite NEWCODE234567ABC.")).toBeInTheDocument();
    expect(within(form).getByText("https://my.example/auth/sign_up?invite=NEWCODE234567ABC")).toHaveAttribute(
      "dir",
      "ltr",
    );
    expect(invites.created).toEqual([{ max_uses: 1, expires_in: 7 * 86_400 }]);
    expect(await screen.findByRole("rowheader", { name: "NEWCODE234567ABC" })).toBeInTheDocument();
  });

  test("creates an invite without a limit that never expires", async () => {
    const invites = invitesServer([]);
    const { user } = await renderApp({ path: "/moderation/invites" });
    const form = await screen.findByRole("region", { name: "Create an invite" });
    await user.click(within(form).getByRole("button", { name: /Number of sign-ups/ }));
    await user.click(await screen.findByRole("option", { name: "No limit" }));
    await user.click(within(form).getByRole("button", { name: /Expires after/ }));
    await user.click(await screen.findByRole("option", { name: "Never" }));
    await user.click(within(form).getByRole("button", { name: "Create the invite" }));

    expect(await within(form).findByText(/^Created the invite/)).toBeInTheDocument();
    expect(invites.created).toEqual([{}]);
  });

  test("revokes an invite, and focus moves to its row header", async () => {
    const invites = invitesServer([inviteFixture()]);
    const { user } = await renderApp({ path: "/moderation/invites" });
    await user.click(await screen.findByRole("button", { name: /^Revoke the invite ABCDEFGHJKLMNPQR/ }));

    expect(
      await screen.findByText("Revoked the invite ABCDEFGHJKLMNPQR. Nobody can sign up with it anymore."),
    ).toBeInTheDocument();
    const heading = screen.getByRole("rowheader", { name: "ABCDEFGHJKLMNPQR" });
    await waitFor(() => {
      expect(heading).toHaveFocus();
    });
    expect(within(screen.getByRole("table", { name: "Invites" })).getByText("Revoked")).toBeInTheDocument();
    expect(invites.revoked).toEqual([inviteFixture().id]);
  });

  test("is in the moderation menu", async () => {
    invitesServer([]);
    const { user } = await renderApp({ path: "/moderation" });
    await user.click(await within(await screen.findByRole("main")).findByRole("link", { name: "Invites" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Invites" })).toBeInTheDocument();
  });
});

test("says that invites let nobody sign up while the registration mode is not invite", async () => {
  server.use(signedInHandler, operatorHandler(["manage_invites"]));
  await renderApp({ path: "/moderation/invites" });

  expect(await screen.findByText(/does not take sign-ups with an invite now/)).toBeInTheDocument();
});

test("a viewer without manage_invites sees why instead of the invites, and nothing is read", async () => {
  server.use(signedInHandler, operatorHandler(["manage_reports"]));
  let reads = 0;
  server.use(
    http.get(`${apiBase}/admin/invites`, () => {
      reads++;
      return HttpResponse.json({ items: [] });
    }),
  );
  await renderApp({ path: "/moderation/invites" });

  expect(await screen.findByRole("heading", { level: 1, name: "Invites" })).toBeInTheDocument();
  expect(await screen.findByText(/manage_invites/)).toBeInTheDocument();
  expect(reads).toBe(0);
});
