// SPDX-License-Identifier: AGPL-3.0-only
// The linked accounts page through the whole app (web-client/README.md T4): the list, unlinking (and the last way to
// sign in), linking after the password, the outcome that the provider's round trip brings back in the fragment, and
// a user with neither a password nor a passkey who confirms the deletion of the account with a linked provider, and
// what every operation confirmed that way does when the confirmation expired, with MSW playing the server.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import { accountFixture, type Schemas } from "../../test/fixtures";
import { apiBase, oauthPaths, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { sectionHeadings } from "../../test/sections";
import { installPasskeys, removePasskeys } from "../../test/webauthn";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  removePasskeys();
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
    // docs/architecture/web-client/screens/settings-linked-accounts.md §3
    expect(sectionHeadings()).toEqual(["Your linked accounts"]);
    expect(within(list).getByText("Corp SSO")).toHaveAttribute("dir", "auto");
    expect(within(list).getByText("1 linked account")).toBeInTheDocument();
    // Only the providers without a link are offered, in the panel that the one button opens.
    await user.click(within(list).getByRole("button", { name: "Link an account" }));
    const add = within(list).getByRole("region", { name: "Link an account" });
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
    // Above the list, where the new row is.
    const list = await screen.findByRole("region", { name: "Your linked accounts" });
    expect(await within(list).findByRole("status")).toHaveTextContent("Linked your account at Corp SSO.");
    await waitFor(() => {
      expect(router.state.location.hash).toBe("");
    });
  });

  test("says why a link failed", async () => {
    linkServer([]);
    await renderApp({ path: `/settings/linked-accounts#external_error=linked_elsewhere&provider=${lab}` });
    const list = await screen.findByRole("region", { name: "Your linked accounts" });
    expect(await within(list).findByRole("alert")).toHaveTextContent(
      "That account is linked to another user of this server.",
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

  test("a confirmation that expired leads back to the provider", async () => {
    linkServer([corpLink]);
    server.use(
      http.get(`${apiBase}/accounts/password`, () => HttpResponse.json({ has_password: false })),
      http.post(`${apiBase}/accounts/delete`, () =>
        problem(422, "/api/v1/problems/external-confirmation-expired", "The confirmation expired."),
      ),
    );
    const { user } = await renderApp({
      path: `/settings/account#external_confirmation=tok-1&operation=account_deletion&provider=${corp}`,
    });
    const back = await screen.findByRole("region", { name: "Delete your account" });
    await user.click(within(back).getByRole("button", { name: "Delete account…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete your account?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete account" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(await within(back).findByRole("alert")).toHaveTextContent(
      "The confirmation took too long or was already used. Try again.",
    );
    expect(within(back).getByRole("button", { name: "Sign in again with Corp SSO" })).toBeInTheDocument();
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

describe("a confirmation with the provider that expired leads back to the provider", () => {
  const expired = () => problem(422, "/api/v1/problems/external-confirmation-expired", "The server's own words.");
  const back = (operation: string) => `#external_confirmation=tok-1&operation=${operation}&provider=${corp}`;
  const registrationOptions = {
    options: {
      rp: { name: "social.example", id: "social.example" },
      user: { name: "alice@social.example", displayName: "alice@social.example", id: "dTE" },
      challenge: "AQID",
      pubKeyCredParams: [{ type: "public-key", alg: -7 }],
      authenticatorSelection: { residentKey: "required", userVerification: "required" },
      attestation: "none",
    },
    registration_token: "reg-1",
    expires_at: "2026-10-20T00:05:00Z",
  };

  /** A user with a linked account at Corp SSO, no passkey, and perhaps a password. */
  function providerUser({ hasPassword = false } = {}) {
    linkServer([corpLink]);
    server.use(
      http.get(`${apiBase}/accounts/password`, () => HttpResponse.json({ has_password: hasPassword })),
      http.get(`${apiBase}/passkeys`, () => HttpResponse.json({ items: [] })),
    );
  }

  /** The dialog is gone, and `section` says why in the app's own words and offers the provider. */
  async function expectLedBack(section: HTMLElement) {
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(await within(section).findByRole("alert")).toHaveTextContent(
      "The confirmation took too long or was already used. Try again.",
    );
    expect(within(section).getByRole("button", { name: "Sign in again with Corp SSO" })).toBeInTheDocument();
    expect(screen.queryByText(/The server's own words/u)).not.toBeInTheDocument();
  }

  test("when setting a password", async () => {
    providerUser();
    const bodies: Schemas["ChangePasswordRequest"][] = [];
    server.use(
      http.post(`${apiBase}/accounts/password`, async ({ request }) => {
        bodies.push((await request.json()) as Schemas["ChangePasswordRequest"]);
        return expired();
      }),
    );
    const { user } = await renderApp({ path: `/settings/password${back("password_change")}` });
    const section = await screen.findByRole("region", { name: "Set a password" });
    await user.type(within(section).getByLabelText(/^New password/u), "correct horse battery");
    await user.click(within(section).getByRole("button", { name: "Set password" }));
    await expectLedBack(section);
    expect(within(section).queryByRole("button", { name: "Set password" })).not.toBeInTheDocument();
    expect(bodies).toEqual([{ external_confirmation: "tok-1", new_password: "correct horse battery" }]);
  });

  test("when removing the password", async () => {
    providerUser({ hasPassword: true });
    const bodies: Schemas["RemovePasswordRequest"][] = [];
    server.use(
      http.post(`${apiBase}/accounts/password/remove`, async ({ request }) => {
        bodies.push((await request.json()) as Schemas["RemovePasswordRequest"]);
        return expired();
      }),
    );
    const { user } = await renderApp({ path: `/settings/password${back("password_removal")}` });
    const section = await screen.findByRole("region", { name: "Remove the password" });
    await user.click(within(section).getByRole("button", { name: "Remove password…" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Remove password" }));
    await expectLedBack(section);
    expect(within(section).queryByRole("button", { name: "Remove password…" })).not.toBeInTheDocument();
    expect(bodies).toEqual([{ external_confirmation: "tok-1" }]);
  });

  test("when adding a passkey", async () => {
    installPasskeys();
    providerUser();
    const bodies: Schemas["PasskeyRegistrationRequest"][] = [];
    server.use(
      http.post(`${apiBase}/passkeys/registration`, async ({ request }) => {
        bodies.push((await request.json()) as Schemas["PasskeyRegistrationRequest"]);
        return expired();
      }),
    );
    const { user } = await renderApp({ path: `/settings/passkeys${back("passkey_registration")}` });
    const form = await screen.findByRole("region", { name: "Add a passkey" });
    await user.type(within(form).getByRole("textbox", { name: "Name" }), "Laptop");
    await user.click(within(form).getByRole("button", { name: "Add passkey" }));
    await expectLedBack(form);
    expect(within(form).queryByRole("button", { name: "Add passkey" })).not.toBeInTheDocument();
    expect(bodies).toEqual([{ name: "Laptop", external_confirmation: "tok-1" }]);
  });

  test("when adding a passkey again after dismissing the browser's prompt, which spent the confirmation", async () => {
    const credentials = installPasskeys();
    credentials.create.mockRejectedValueOnce(new DOMException("dismissed", "NotAllowedError"));
    providerUser();
    let registrations = 0;
    server.use(
      http.post(`${apiBase}/passkeys/registration`, () => {
        registrations++;
        return registrations > 1 ? expired() : HttpResponse.json(registrationOptions);
      }),
    );
    const { user } = await renderApp({ path: `/settings/passkeys${back("passkey_registration")}` });
    const form = await screen.findByRole("region", { name: "Add a passkey" });
    await user.type(within(form).getByRole("textbox", { name: "Name" }), "Laptop");
    await user.click(within(form).getByRole("button", { name: "Add passkey" }));
    expect(await within(form).findByRole("alert")).toBeInTheDocument();
    expect(registrations).toBe(1);
    await user.click(within(form).getByRole("button", { name: "Add passkey" }));
    await waitFor(() => {
      expect(registrations).toBe(2);
    });
    await expectLedBack(form);
  });

  test("when linking another account", async () => {
    providerUser();
    const bodies: Schemas["LinkIdentityRequest"][] = [];
    server.use(
      http.post(`${apiBase}/linked_identities`, async ({ request }) => {
        bodies.push((await request.json()) as Schemas["LinkIdentityRequest"]);
        return expired();
      }),
    );
    const { user, leftTo } = await renderApp({ path: `/settings/linked-accounts${back("link")}` });
    const panel = await screen.findByRole("region", { name: "Link an account" });
    await user.click(within(panel).getByRole("button", { name: "Link Lab ID" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Continue" }));
    await expectLedBack(panel);
    expect(within(panel).queryByRole("button", { name: "Link Lab ID" })).not.toBeInTheDocument();
    expect(bodies).toEqual([
      { provider_id: lab, external_confirmation: "tok-1", return_to: "/settings/linked-accounts" },
    ]);
    expect(leftTo).toEqual([]);
  });

  test("when moving the account", async () => {
    providerUser();
    const bodies: Schemas["MoveOwnAccountRequest"][] = [];
    const carol = accountFixture(7, "Carol", "new.example");
    server.use(
      http.get(`${apiBase}/accounts/lookup`, () => HttpResponse.json(carol)),
      http.post(`${apiBase}/accounts/move`, async ({ request }) => {
        bodies.push((await request.json()) as Schemas["MoveOwnAccountRequest"]);
        return expired();
      }),
    );
    const { user } = await renderApp({ path: `/settings/move${back("account_move")}` });
    const section = await screen.findByRole("region", { name: "Choose the new account" });
    await user.type(within(section).getByRole("textbox", { name: /New account/u }), "@carol@new.example");
    await user.click(within(section).getByRole("button", { name: "Continue" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Move account" }));
    await expectLedBack(section);
    expect(within(section).queryByRole("textbox", { name: /New account/u })).not.toBeInTheDocument();
    expect(bodies).toEqual([{ target_account_id: carol.id, external_confirmation: "tok-1" }]);
  });
});
