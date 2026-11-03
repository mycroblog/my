// SPDX-License-Identifier: AGPL-3.0-only
// Deactivating the account through the whole app (web-client/README.md T4): the date before and after confirming,
// confirming with the password, a code or a linked provider, what a wrong one says, and the refusals; MSW plays the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

const due = "2026-12-03T09:00:00Z";
const corp = "019a0000-0000-7000-8000-0000000000c1";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

/** Plays what the page reads and records the confirmations of the deactivation. */
function deactivationServer(preview: Schemas["DeactivationPreview"], twoFactorEnabled = false) {
  const bodies: Schemas["DeactivateOwnAccountRequest"][] = [];
  server.use(
    http.get(`${apiBase}/exports`, () => HttpResponse.json({ items: [] })),
    http.get(`${apiBase}/accounts/deactivation`, () => HttpResponse.json(preview)),
    http.get(`${apiBase}/two_factor`, () => HttpResponse.json({ enabled: twoFactorEnabled })),
    http.post(`${apiBase}/accounts/deactivation`, async ({ request }) => {
      const body = (await request.json()) as Schemas["DeactivateOwnAccountRequest"];
      bodies.push(body);
      if (body.password === "wrong") {
        return problem(422, "/api/v1/problems/wrong-password", "The password is wrong.");
      }
      if (body.two_factor_code === "000000") {
        return problem(422, "/api/v1/problems/invalid-two-factor-code", "The code is wrong.");
      }
      return HttpResponse.json({ deletion_due_at: due } satisfies Schemas["Deactivation"]);
    }),
  );
  return bodies;
}

describe("/settings/account, deactivating", () => {
  test("states the date, confirms with the password, and ends on the page that states the date", async () => {
    const bodies = deactivationServer({ deletion_due_at: due });
    const { user, router } = await renderApp({ path: "/settings/account" });
    await user.click(await screen.findByRole("button", { name: "Deactivate account…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Deactivate your account?" });
    expect(within(dialog).getByText(/It will be deleted on .*2026/u)).toBeInTheDocument();

    await user.type(within(dialog).getByLabelText(/Current password/u), "wrong");
    await user.click(within(dialog).getByRole("button", { name: "Deactivate" }));
    expect(await within(dialog).findByText("The password is not correct.")).toBeInTheDocument();

    await user.clear(within(dialog).getByLabelText(/Current password/u));
    await user.type(within(dialog).getByLabelText(/Current password/u), "right");
    await user.click(within(dialog).getByRole("button", { name: "Deactivate" }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/deactivated");
    });
    expect(bodies.at(-1)).toEqual({ password: "right" });
    expect(await screen.findByRole("heading", { name: "Your account is deactivated" })).toBeInTheDocument();
    expect(screen.getByText(/It will be deleted on .*2026/u)).toBeInTheDocument();
    expect(within(screen.getByRole("main")).getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/login");
  });

  test("a code of the second factor confirms in place of the password", async () => {
    const bodies = deactivationServer({ deletion_due_at: due }, true);
    const { user, router } = await renderApp({ path: "/settings/account" });
    await user.click(await screen.findByRole("button", { name: "Deactivate account…" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Use an authentication code instead" }));

    await user.type(within(dialog).getByLabelText(/Authentication code/u), "000000");
    await user.click(within(dialog).getByRole("button", { name: "Deactivate" }));
    expect(await within(dialog).findByText("The code is not correct.")).toBeInTheDocument();

    await user.clear(within(dialog).getByLabelText(/Authentication code/u));
    await user.type(within(dialog).getByLabelText(/Authentication code/u), "123456");
    await user.click(within(dialog).getByRole("button", { name: "Deactivate" }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/deactivated");
    });
    expect(bodies.at(-1)).toEqual({ two_factor_code: "123456" });
  });

  test("cancelling sends nothing", async () => {
    const bodies = deactivationServer({ deletion_due_at: due });
    const { user } = await renderApp({ path: "/settings/account" });
    await user.click(await screen.findByRole("button", { name: "Deactivate account…" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Keep my account active" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(bodies).toEqual([]);
  });

  test("a paid subscription refuses it and leads to the subscription", async () => {
    deactivationServer({ deletion_due_at: due, refusal: "paid_subscription" });
    await renderApp({ path: "/settings/account" });
    expect(await screen.findByText(/Your supporter subscription is still running/u)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Manage your subscription" })).toHaveAttribute(
      "href",
      "/settings/supporter",
    );
    expect(screen.queryByRole("button", { name: "Deactivate account…" })).not.toBeInTheDocument();
  });

  test("a recent reactivation refuses it and says from when it can be done", async () => {
    deactivationServer({ deletion_due_at: due, refusal: "recently_reactivated", available_at: "2026-11-04T09:00:00Z" });
    await renderApp({ path: "/settings/account" });
    expect(await screen.findByText(/You can deactivate it again from .*2026/u)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deactivate account…" })).not.toBeInTheDocument();
  });
});

describe("/settings/account, deactivating with a linked provider", () => {
  /** A user with neither a password nor a passkey and a linked account at Corp SSO; returns the requests. */
  function providerUser() {
    const bodies = deactivationServer({ deletion_due_at: due });
    const confirmations: Schemas["ExternalConfirmationRequest"][] = [];
    server.use(
      http.get(`${apiBase}/accounts/password`, () => HttpResponse.json({ has_password: false })),
      http.get(`${apiBase}/passkeys`, () => HttpResponse.json({ items: [] })),
      http.get(`${apiBase}/linked_identities`, () =>
        HttpResponse.json({
          items: [
            {
              id: "019a0000-0000-7000-8000-0000000000d1",
              provider_id: corp,
              provider_kind: "oidc",
              provider_name: "Corp SSO",
              linked_at: "2026-10-01T09:00:00Z",
              available: true,
            },
          ],
          providers: [],
        } satisfies Schemas["LinkedIdentityList"]),
      ),
      http.post(`${apiBase}/linked_identities/confirmation`, async ({ request }) => {
        confirmations.push((await request.json()) as Schemas["ExternalConfirmationRequest"]);
        return HttpResponse.json({ start_url: `https://social.example/auth/external/${corp}/start?intent=d` });
      }),
    );
    return { bodies, confirmations };
  }
  const section = () => screen.findByRole("region", { name: "Deactivate your account" });
  const back = `#external_confirmation=tok-1&operation=account_deactivation&provider=${corp}`;

  test("signs in again at the provider first, and comes back to deactivate with the confirmation", async () => {
    const { bodies, confirmations } = providerUser();
    const view = await renderApp({ path: "/settings/account" });
    const before = await section();
    expect(within(before).queryByRole("button", { name: "Deactivate account…" })).not.toBeInTheDocument();
    await view.user.click(within(before).getByRole("button", { name: "Sign in again with Corp SSO" }));
    await waitFor(() => {
      expect(view.leftTo.map(String)).toEqual([`https://social.example/auth/external/${corp}/start?intent=d`]);
    });
    expect(confirmations).toEqual([
      { provider_id: corp, operation: "account_deactivation", return_to: "/settings/account" },
    ]);
    view.unmount();

    const { user, router } = await renderApp({ path: `/settings/account${back}` });
    const after = await section();
    expect(within(after).getByText("You signed in again with Corp SSO. Finish within 10 minutes.")).toBeInTheDocument();
    await waitFor(() => {
      expect(router.state.location.hash).toBe("");
    });
    await user.click(within(after).getByRole("button", { name: "Deactivate account…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Deactivate your account?" });
    expect(within(dialog).getByText(/The sign-in at your linked account confirms this/u)).toBeInTheDocument();
    expect(within(dialog).queryByLabelText(/password/iu)).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Deactivate" }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/deactivated");
    });
    expect(bodies).toEqual([{ external_confirmation: "tok-1" }]);
  });

  test("the confirmation of another operation does not open the dialog", async () => {
    providerUser();
    await renderApp({
      path: `/settings/account#external_confirmation=tok-1&operation=account_deletion&provider=${corp}`,
    });
    const before = await section();
    expect(within(before).queryByRole("button", { name: "Deactivate account…" })).not.toBeInTheDocument();
    expect(within(before).getByRole("button", { name: "Sign in again with Corp SSO" })).toBeInTheDocument();
  });

  test("says why the sign-in at the provider did not confirm", async () => {
    providerUser();
    await renderApp({
      path: `/settings/account#external_error=mismatch&operation=account_deactivation&provider=${corp}`,
    });
    expect(await within(await section()).findByRole("alert")).toHaveTextContent(
      "You signed in with an account that is not linked to yours. Try again with your linked account.",
    );
  });

  test("a confirmation that expired leads back to the provider", async () => {
    const { bodies } = providerUser();
    server.use(
      http.post(`${apiBase}/accounts/deactivation`, async ({ request }) => {
        bodies.push((await request.json()) as Schemas["DeactivateOwnAccountRequest"]);
        return problem(422, "/api/v1/problems/external-confirmation-expired", "The confirmation expired.");
      }),
    );
    const { user } = await renderApp({ path: `/settings/account${back}` });
    const after = await section();
    await user.click(within(after).getByRole("button", { name: "Deactivate account…" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Deactivate" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(await within(after).findByRole("alert")).toHaveTextContent(
      "The confirmation took too long or was already used. Try again.",
    );
    expect(within(after).getByRole("button", { name: "Sign in again with Corp SSO" })).toBeInTheDocument();
    expect(within(after).queryByRole("button", { name: "Deactivate account…" })).not.toBeInTheDocument();
    expect(bodies).toEqual([{ external_confirmation: "tok-1" }]);
  });
});
