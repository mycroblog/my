// SPDX-License-Identifier: AGPL-3.0-only
// The settings while sign-in is limited to the login providers, through the whole app (web-client/README.md T4): the
// password and passkey pages stop offering what could not sign in and keep what removes a credential, and the linked
// accounts page, which still confirms with the password, says why it matters. MSW plays the server.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import { instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { installPasskeys, removePasskeys } from "../../test/webauthn";

const phone: Schemas["Passkey"] = {
  id: "019a0000-0000-7000-8000-0000000000a1",
  name: "Phone",
  created_at: "2026-10-01T09:00:00Z",
};
const corp = "019a0000-0000-7000-8000-0000000000c1";

/** The server of a viewer with a password and a passkey, limited or not. */
function limitServer(limited: boolean, { hasPassword = true } = {}) {
  const links: Schemas["LinkIdentityRequest"][] = [];
  server.use(
    http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, login_providers_only: limited })),
    http.get(`${apiBase}/passkeys`, () => HttpResponse.json({ items: [phone] })),
    http.get(`${apiBase}/accounts/password`, () => HttpResponse.json({ has_password: hasPassword })),
    http.get(`${apiBase}/linked_identities`, () =>
      HttpResponse.json({
        items: [],
        providers: [{ id: corp, kind: "oidc", display_name: "Corp SSO", allow_link: true }],
      }),
    ),
    http.post(`${apiBase}/linked_identities`, async ({ request }) => {
      links.push((await request.json()) as Schemas["LinkIdentityRequest"]);
      return HttpResponse.json({
        start_url: `https://social.example/auth/external/${corp}/start?intent=x`,
        expires_at: "2026-10-20T00:10:00Z",
      });
    }),
  );
  return { links };
}

beforeEach(() => {
  server.use(signedInHandler);
  installPasskeys();
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  removePasskeys();
});

describe("sign-in limited to login providers", () => {
  test("the password page offers no new password but still removes the current one", async () => {
    limitServer(true);
    await renderApp({ path: "/settings/password" });
    expect(await screen.findByText(/a password cannot be set or changed here/)).toBeInTheDocument();
    expect(screen.queryByLabelText("New password")).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Change the password" })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Remove the password" })).toBeInTheDocument();
  });

  test("a user without a password gets no form to set one", async () => {
    limitServer(true, { hasPassword: false });
    await renderApp({ path: "/settings/password" });
    expect(await screen.findByText(/a password cannot be set or changed here/)).toBeInTheDocument();
    expect(screen.queryByLabelText("New password")).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Remove the password" })).not.toBeInTheDocument();
  });

  test("without the limit the password page keeps its forms", async () => {
    limitServer(false);
    await renderApp({ path: "/settings/password" });
    expect(await screen.findByLabelText("New password")).toBeInTheDocument();
    expect(screen.queryByText(/a password cannot be set or changed here/)).not.toBeInTheDocument();
  });

  test("the passkeys page adds no passkey but still lists and removes them", async () => {
    limitServer(true);
    await renderApp({ path: "/settings/passkeys" });
    expect(await screen.findByText(/a passkey cannot be added here/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add a passkey" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Remove.*Phone/u })).toBeInTheDocument();
  });

  test("without the limit the passkeys page offers to add one", async () => {
    limitServer(false);
    await renderApp({ path: "/settings/passkeys" });
    expect(await screen.findByRole("button", { name: "Add a passkey" })).toBeInTheDocument();
  });

  test("a user with only a password confirms with it and links a provider", async () => {
    const { links } = limitServer(true);
    const { user, leftTo } = await renderApp({ path: "/settings/linked-accounts" });
    expect(await screen.findByText(/you cannot sign in again once your sessions end/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Link an account" }));
    await user.click(await screen.findByRole("button", { name: "Link Corp SSO" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Link your account at Corp SSO" });
    await user.type(within(dialog).getByLabelText(/Current password/), "correct horse");
    await user.click(within(dialog).getByRole("button", { name: "Continue" }));
    await waitFor(() => {
      expect(leftTo.map(String)).toEqual([`https://social.example/auth/external/${corp}/start?intent=x`]);
    });
    expect(links.at(-1)).toEqual({
      provider_id: corp,
      password: "correct horse",
      return_to: "/settings/linked-accounts",
    });
  });
});
