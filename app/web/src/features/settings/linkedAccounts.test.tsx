// SPDX-License-Identifier: AGPL-3.0-only
// The linked accounts page through the whole app (web-client/README.md T4): the list, unlinking (and the last way to
// sign in), linking after the password, the outcome that the provider's round trip brings back in the fragment, and
// a user with neither a password nor a passkey who confirms the deletion of the account with a linked provider, with
// MSW playing the server.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, oauthPaths, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

const corp = "019a0000-0000-7000-8000-0000000000c1";
const lab = "019a0000-0000-7000-8000-0000000000c2";
const corpLink: Schemas["LinkedIdentity"] = {
  id: "019a0000-0000-7000-8000-0000000000d1",
  provider_id: corp,
  provider_kind: "oidc",
  provider_name: "Corp SSO",
  linked_at: "2026-10-01T09:00:00Z",
  available: true,
};
const providers: Schemas["LinkableLoginProvider"][] = [
  { id: corp, kind: "oidc", display_name: "Corp SSO", allow_link: true },
  { id: lab, kind: "oidc", display_name: "Lab ID", allow_link: true },
];

/** The server's linked accounts of the viewer, which unlinking changes, with the requests it received. */
function linkServer(initial: Schemas["LinkedIdentity"][], { last = false } = {}) {
  let items = initial;
  const links: Schemas["LinkIdentityRequest"][] = [];
  const confirmations: Schemas["ExternalConfirmationRequest"][] = [];
  server.use(
    http.get(`${apiBase}/linked_identities`, () => HttpResponse.json({ items, providers })),
    http.delete(`${apiBase}/linked_identities/:id`, ({ params }) => {
      if (last) return problem(409, "/api/v1/problems/last-sign-in-method", "This is your last way to sign in.");
      items = items.filter((l) => l.id !== params["id"]);
      return new HttpResponse(null, { status: 204 });
    }),
    http.post(`${apiBase}/linked_identities`, async ({ request }) => {
      const body = (await request.json()) as Schemas["LinkIdentityRequest"];
      links.push(body);
      if (body.password !== undefined && body.password !== "correct horse") {
        return problem(422, "/api/v1/problems/wrong-password", "The password is wrong.");
      }
      return HttpResponse.json({
        start_url: `https://social.example/auth/external/${body.provider_id}/start?intent=x`,
        expires_at: "2026-10-20T00:10:00Z",
      });
    }),
    http.post(`${apiBase}/linked_identities/confirmation`, async ({ request }) => {
      const body = (await request.json()) as Schemas["ExternalConfirmationRequest"];
      confirmations.push(body);
      return HttpResponse.json({
        start_url: `https://social.example/auth/external/${body.provider_id}/start?intent=y`,
        expires_at: "2026-10-20T00:10:00Z",
      });
    }),
  );
  return { links, confirmations };
}

describe("linked accounts", () => {
  test("are in the settings menu", async () => {
    await renderApp({ path: "/settings" });
    const link = await screen.findByRole("link", { name: "Linked accounts" });
    expect(link).toHaveAttribute("href", "/settings/linked-accounts");
  });

  test("lists the linked accounts and links another after the password, leaving for the provider", async () => {
    const api = linkServer([corpLink]);
    const { user, leftTo } = await renderApp({ path: "/settings/linked-accounts" });

    const list = await screen.findByRole("region", { name: "Your linked accounts" });
    expect(within(list).getByText("Corp SSO")).toHaveAttribute("dir", "auto");
    // Only the providers without a link are offered.
    const add = screen.getByRole("region", { name: "Link an account" });
    expect(within(add).queryByRole("button", { name: "Link Corp SSO" })).not.toBeInTheDocument();
    await user.click(within(add).getByRole("button", { name: "Link Lab ID" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Link your account at Lab ID" });
    expect(dialog).toHaveAccessibleDescription("Enter your password to continue to Lab ID.");
    await user.type(within(dialog).getByLabelText(/Current password/), "wrong");
    await user.click(within(dialog).getByRole("button", { name: "Continue" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("The password is wrong.");

    await user.clear(within(dialog).getByLabelText(/Current password/));
    await user.type(within(dialog).getByLabelText(/Current password/), "correct horse");
    await user.click(within(dialog).getByRole("button", { name: "Continue" }));
    await waitFor(() => {
      expect(leftTo.map(String)).toEqual([`https://social.example/auth/external/${lab}/start?intent=x`]);
    });
    expect(api.links.at(-1)).toEqual({
      provider_id: lab,
      password: "correct horse",
      return_to: "/settings/linked-accounts",
    });
  });

  test("says what the provider's round trip did, and takes the outcome out of the address", async () => {
    linkServer([corpLink]);
    const { router } = await renderApp({ path: `/settings/linked-accounts#linked=${corp}&provider=${corp}` });
    expect(await screen.findByText("Linked your account at Corp SSO.")).toBeInTheDocument();
    await waitFor(() => {
      expect(router.state.location.hash).toBe("");
    });
  });

  test("says why a link failed", async () => {
    linkServer([]);
    await renderApp({ path: `/settings/linked-accounts#external_error=linked_elsewhere&provider=${lab}` });
    expect(await screen.findByText("That account is linked to another user of this server.")).toHaveAttribute(
      "role",
      "alert",
    );
  });

  test("unlinks an account after a dialog, and says when it is the last way to sign in", async () => {
    linkServer([corpLink], { last: true });
    const { user } = await renderApp({ path: "/settings/linked-accounts" });
    const list = await screen.findByRole("region", { name: "Your linked accounts" });
    await user.click(within(list).getByRole("button", { name: "Unlink: Corp SSO" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Unlink your account at Corp SSO?" });
    await user.click(within(dialog).getByRole("button", { name: "Unlink" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "This is your last way to sign in. Set a password, add a passkey, or link another account first.",
    );
  });

  test("unlinks an account", async () => {
    linkServer([corpLink]);
    const { user } = await renderApp({ path: "/settings/linked-accounts" });
    const list = await screen.findByRole("region", { name: "Your linked accounts" });
    await user.click(within(list).getByRole("button", { name: "Unlink: Corp SSO" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Unlink your account at Corp SSO?" });
    await user.click(within(dialog).getByRole("button", { name: "Unlink" }));
    expect(await within(list).findByText("You have not linked any account.")).toBeInTheDocument();
  });
});

describe("a user who signs in only with a linked account", () => {
  test("confirms the deletion of the account by signing in again at the provider", async () => {
    const api = linkServer([corpLink]);
    const bodies: Schemas["DeleteOwnAccountRequest"][] = [];
    server.use(
      http.get(`${apiBase}/accounts/password`, () => HttpResponse.json({ has_password: false })),
      http.post(`${apiBase}/accounts/delete`, async ({ request }) => {
        bodies.push((await request.json()) as Schemas["DeleteOwnAccountRequest"]);
        return new HttpResponse(null, { status: 204 });
      }),
      http.post(oauthPaths.revoke, () => new HttpResponse(null, { status: 200 })),
    );
    // Before the round trip, the page offers the provider instead of the deletion.
    const view = await renderApp({ path: "/settings/account" });
    const section = await screen.findByRole("region", { name: "Delete your account" });
    expect(within(section).queryByRole("button", { name: "Delete account…" })).not.toBeInTheDocument();
    await view.user.click(within(section).getByRole("button", { name: "Sign in again with Corp SSO" }));
    await waitFor(() => {
      expect(view.leftTo.map(String)).toEqual([`https://social.example/auth/external/${corp}/start?intent=y`]);
    });
    expect(api.confirmations).toEqual([
      { provider_id: corp, operation: "account_deletion", return_to: "/settings/account" },
    ]);
    view.unmount();

    // The provider sends the browser back with the confirmation.
    const { user } = await renderApp({
      path: `/settings/account#external_confirmation=tok-1&operation=account_deletion&provider=${corp}`,
    });
    const back = await screen.findByRole("region", { name: "Delete your account" });
    expect(within(back).getByText("You signed in again with Corp SSO. Finish within 10 minutes.")).toBeInTheDocument();
    await user.click(within(back).getByRole("button", { name: "Delete account…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete your account?" });
    expect(within(dialog).queryByLabelText(/Password/)).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Delete account" }));

    // Signed out, the landing page opens (/login would start signing in again).
    expect(await screen.findByRole("heading", { level: 1, name: "my example" })).toBeInTheDocument();
    expect(bodies).toEqual([{ external_confirmation: "tok-1" }]);
  });

  test("says why the confirmation failed", async () => {
    linkServer([corpLink]);
    server.use(http.get(`${apiBase}/accounts/password`, () => HttpResponse.json({ has_password: false })));
    await renderApp({ path: `/settings/account#external_error=mismatch&operation=account_deletion&provider=${corp}` });
    const section = await screen.findByRole("region", { name: "Delete your account" });
    expect(await within(section).findByRole("alert")).toHaveTextContent(
      "You signed in with an account that is not linked to yours. Try again with your linked account.",
    );
  });
});
