// SPDX-License-Identifier: AGPL-3.0-only
// The login providers screen through the whole app (web-client/README.md T4, ADR-0106): an administrator with
// manage_login_providers sees the providers in the list, sets up a provider of each kind on the form's page and lands on
// its page with its callback URL, sees a refused field and an incomplete condition, and on the page of a provider
// changes it without its secret, gives it a new secret, and disables and deletes it after a warning with the number of
// linked users; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { loginProviderFixture } from "./test/handlers";

const providerId = "0199f0e0-0000-7000-8000-0000000000c9";
const name = "⁨Example SSO⁩";
const newPath = "/admin/login-providers/new";
const providerPath = `/admin/login-providers/${providerId}`;

/** The form of a new provider, once its page has loaded. */
async function newForm(): Promise<HTMLElement> {
  const main = await screen.findByRole("main");
  await within(main).findByRole("radiogroup", { name: "Kind" });
  return main;
}

/** Answers GET /admin/login_providers/{id} with what `provider` gives at each request. */
function providerHandler(provider: () => Schemas["AdminLoginProvider"]) {
  return http.get(`${apiBase}/admin/login_providers/:id`, () => HttpResponse.json(provider()));
}

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

function problem(reason: string, detail: string) {
  return HttpResponse.json(
    {
      type: "/api/v1/problems/invalid-login-provider",
      title: "The login provider cannot be saved.",
      status: 422,
      detail,
      reason,
    },
    { status: 422, headers: { "Content-Type": "application/problem+json" } },
  );
}

/** What the server says about the limit of sign-in to login providers: by default, the setting is off. */
function impactHandler(impact: Partial<Schemas["LoginProvidersOnlyImpact"]>, seen: (string | null)[] = []) {
  return http.get(`${apiBase}/admin/settings/login_providers_only/impact`, ({ request }) => {
    seen.push(new URL(request.url).searchParams.get("provider_id"));
    return HttpResponse.json({
      setting_on: false,
      enabled_providers: 0,
      users_without_provider: 0,
      includes_viewer: false,
      starts_limit: false,
      ...impact,
    } satisfies Schemas["LoginProvidersOnlyImpact"]);
  });
}

describe("the login providers screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_login_providers"]), impactHandler({}));
  });

  test("tells a viewer without the permission why it is empty", async () => {
    server.use(operatorHandler(["manage_settings"]));
    await renderApp({ path: "/admin/login-providers" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
  });

  test("lists the providers with their kind and state, linking to their page", async () => {
    server.use(
      http.get(`${apiBase}/admin/login_providers`, () =>
        HttpResponse.json({
          items: [
            loginProviderFixture(providerId, { linked_users: 3, allow_signup: true }),
            loginProviderFixture("p2", {
              kind: "github",
              display_name: "GitHub",
              state: "disabled",
              organizations: ["my-org"],
            }),
          ],
        }),
      ),
    );
    await renderApp({ path: "/admin/login-providers" });
    const list = await screen.findByRole("region", { name: "Login providers of this server" });

    const sso = within(list).getByRole("row", { name: /^Example SSO/u });
    expect(within(sso).getByRole("link", { name: "Example SSO" })).toHaveAttribute("href", providerPath);
    expect(within(sso).getByText("OpenID Connect")).toBeInTheDocument();
    expect(within(sso).getByText("Enabled")).toBeInTheDocument();
    expect(within(sso).getByText("Sign-up allowed")).toBeInTheDocument();
    const github = within(list).getByRole("row", { name: /^GitHub/u });
    expect(within(github).getByText("Disabled")).toBeInTheDocument();
    // Every action is on the provider's page.
    expect(within(list).queryByRole("button")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a provider" })).toHaveAttribute("href", newPath);
  });

  test("shows a provider's facts and callback URL on its page", async () => {
    server.use(providerHandler(() => loginProviderFixture(providerId, { linked_users: 3, allow_signup: true })));
    await renderApp({ path: providerPath });
    const provider = await screen.findByRole("region", { name: "Example SSO" });

    expect(within(provider).getByText("OpenID Connect")).toBeInTheDocument();
    expect(within(provider).getByText("Sign-up allowed")).toBeInTheDocument();
    expect(within(provider).getByText("3 users")).toBeInTheDocument();
    expect(within(provider).getByText(`https://my.example/auth/external/${providerId}/callback`)).toBeInTheDocument();
    expect(within(provider).getByRole("button", { name: `Copy the callback URL of ${name}` })).toBeInTheDocument();
  });

  test("sets up an OpenID Connect provider with a condition and shows its callback URL", async () => {
    let sent: Schemas["CreateLoginProviderRequest"] | undefined;
    server.use(
      http.get(`${apiBase}/admin/login_providers`, () =>
        HttpResponse.json({ items: sent === undefined ? [] : [loginProviderFixture(providerId)] }),
      ),
      http.post(`${apiBase}/admin/login_providers`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateLoginProviderRequest"];
        return HttpResponse.json(loginProviderFixture(providerId));
      }),
    );
    const { user } = await renderApp({ path: newPath });
    const form = await newForm();

    expect(within(form).getByRole("radio", { name: "OpenID Connect" })).toBeChecked();
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "Example SSO");
    await user.type(within(form).getByRole("textbox", { name: "Issuer" }), "https://sso.example");
    await user.type(within(form).getByRole("textbox", { name: "Client ID" }), "my-client");
    const secret = within(form).getByLabelText(/^Client secret/u);
    expect(secret).toHaveAttribute("type", "password");
    await user.type(secret, "s3cret");
    await user.type(within(form).getByRole("textbox", { name: "Extra scopes" }), "email  profile");
    await user.click(within(form).getByRole("checkbox", { name: "New users may sign up" }));
    await user.click(within(form).getByRole("button", { name: "Add a condition" }));
    expect(within(form).getByRole("textbox", { name: "Claim of condition 1" })).toHaveFocus();
    await user.type(within(form).getByRole("textbox", { name: "Claim of condition 1" }), "hd");
    await user.type(
      within(form).getByRole("textbox", { name: "Allowed values of condition 1" }),
      "example.com, example.org",
    );
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    // Setting it up opens its page, with the callback URL to register.
    expect(await screen.findByRole("heading", { level: 1, name: "Login provider" })).toBeInTheDocument();
    expect(await screen.findByText(`https://my.example/auth/external/${providerId}/callback`)).toBeInTheDocument();
    expect(sent).toEqual({
      kind: "oidc",
      display_name: "Example SSO",
      issuer: "https://sso.example",
      client_id: "my-client",
      client_secret: "s3cret",
      scopes: ["email", "profile"],
      allow_signup: true,
      allow_link: true,
      claims: [{ claim: "hd", values: ["example.com", "example.org"] }],
    });
    // The form's page is left behind, and the secret with it.
    expect(within(form).queryByRole("radiogroup", { name: "Kind" })).not.toBeInTheDocument();
    expect(screen.queryByDisplayValue("s3cret")).not.toBeInTheDocument();
  });

  test("sends only what a GitHub provider takes", async () => {
    let sent: Schemas["CreateLoginProviderRequest"] | undefined;
    server.use(
      http.post(`${apiBase}/admin/login_providers`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateLoginProviderRequest"];
        return HttpResponse.json(loginProviderFixture(providerId, { kind: "github", display_name: "GitHub" }));
      }),
    );
    const { user } = await renderApp({ path: newPath });
    const form = await newForm();

    await user.click(within(form).getByRole("radio", { name: "GitHub" }));
    expect(within(form).queryByRole("textbox", { name: "Issuer" })).not.toBeInTheDocument();
    expect(within(form).queryByRole("group", { name: "Conditions" })).not.toBeInTheDocument();
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "GitHub");
    await user.type(within(form).getByRole("textbox", { name: "Client ID" }), "Iv1.abc");
    await user.type(within(form).getByLabelText(/^Client secret/u), "s3cret");
    await user.type(within(form).getByRole("textbox", { name: "Organizations" }), "my-org, other-org");
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Login provider" })).toBeInTheDocument();
    expect(sent).toEqual({
      kind: "github",
      display_name: "GitHub",
      client_id: "Iv1.abc",
      client_secret: "s3cret",
      scopes: [],
      allow_signup: false,
      allow_link: true,
      organizations: ["my-org", "other-org"],
    });
  });

  test("sends the servers of a Discord provider and no issuer", async () => {
    let sent: Schemas["CreateLoginProviderRequest"] | undefined;
    server.use(
      http.post(`${apiBase}/admin/login_providers`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateLoginProviderRequest"];
        return HttpResponse.json(loginProviderFixture(providerId, { kind: "discord", display_name: "Discord" }));
      }),
    );
    const { user } = await renderApp({ path: newPath });
    const form = await newForm();

    await user.click(within(form).getByRole("radio", { name: "Discord" }));
    expect(within(form).queryByRole("textbox", { name: "Issuer" })).not.toBeInTheDocument();
    expect(within(form).queryByRole("textbox", { name: "Organizations" })).not.toBeInTheDocument();
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "Discord");
    await user.type(within(form).getByRole("textbox", { name: "Client ID" }), "1234");
    await user.type(within(form).getByLabelText(/^Client secret/u), "s3cret");
    await user.type(within(form).getByRole("textbox", { name: "Discord servers" }), "1234567890, 987654321");
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Login provider" })).toBeInTheDocument();
    expect(sent).toEqual({
      kind: "discord",
      display_name: "Discord",
      client_id: "1234",
      client_secret: "s3cret",
      scopes: [],
      allow_signup: false,
      allow_link: true,
      guilds: ["1234567890", "987654321"],
    });
  });

  test("refuses a Discord server ID that is not a number before sending", async () => {
    let called = false;
    server.use(
      http.post(`${apiBase}/admin/login_providers`, () => {
        called = true;
        return HttpResponse.json(loginProviderFixture(providerId, { kind: "discord" }));
      }),
    );
    const { user } = await renderApp({ path: newPath });
    const form = await newForm();
    await user.click(within(form).getByRole("radio", { name: "Discord" }));
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "Discord");
    await user.type(within(form).getByRole("textbox", { name: "Client ID" }), "1234");
    await user.type(within(form).getByLabelText(/^Client secret/u), "s3cret");
    const guilds = within(form).getByRole("textbox", { name: "Discord servers" });
    await user.type(guilds, "12345, my-server");
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    await waitFor(() => {
      expect(guilds).toBeInvalid();
    });
    expect(guilds).toHaveAccessibleDescription(/A Discord server ID is a number of up to 20 digits\./u);
    expect(guilds).toHaveValue("12345, my-server");
    expect(called).toBe(false);
  });

  test("shows the server's refusal of the servers on their field", async () => {
    server.use(
      http.post(`${apiBase}/admin/login_providers`, () => problem("conditions", "The server IDs are not valid.")),
    );
    const { user } = await renderApp({ path: newPath });
    const form = await newForm();
    await user.click(within(form).getByRole("radio", { name: "Discord" }));
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "Discord");
    await user.type(within(form).getByRole("textbox", { name: "Client ID" }), "1234");
    await user.type(within(form).getByLabelText(/^Client secret/u), "s3cret");
    await user.type(within(form).getByRole("textbox", { name: "Discord servers" }), "123");
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    const guilds = within(form).getByRole("textbox", { name: "Discord servers" });
    await waitFor(() => {
      expect(guilds).toBeInvalid();
    });
    expect(guilds).toHaveAccessibleDescription(/The server IDs are not valid\./u);
  });

  test("keeps the secret of a Discord provider out of its settings", async () => {
    server.use(
      providerHandler(() => loginProviderFixture("p3", { kind: "discord", display_name: "Discord", guilds: ["42"] })),
    );
    await renderApp({ path: "/admin/login-providers/p3" });
    const settings = await screen.findByRole("region", { name: "Settings" });
    expect(within(settings).getByRole("textbox", { name: "Discord servers" })).toHaveValue("42");
    expect(within(settings).queryByLabelText(/^Client secret/u)).not.toBeInTheDocument();
    expect(within(settings).queryByRole("textbox", { name: "Issuer" })).not.toBeInTheDocument();
  });

  test("asks a Mastodon provider for its server, and lets the server register its client", async () => {
    let sent: Schemas["CreateLoginProviderRequest"] | undefined;
    server.use(
      http.post(`${apiBase}/admin/login_providers`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateLoginProviderRequest"];
        return HttpResponse.json(
          loginProviderFixture(providerId, {
            kind: "mastodon",
            display_name: "Social",
            issuer: "https://social.example",
          }),
        );
      }),
    );
    const { user } = await renderApp({ path: newPath });
    const form = await newForm();

    await user.click(within(form).getByRole("radio", { name: "Mastodon" }));
    expect(within(form).getByRole("textbox", { name: "Server" })).toBeRequired();
    expect(within(form).getByRole("textbox", { name: "Client ID" })).not.toBeRequired();
    expect(within(form).getByLabelText(/^Client secret/u)).not.toBeRequired();
    expect(within(form).queryByRole("textbox", { name: "Extra scopes" })).not.toBeInTheDocument();
    expect(within(form).getByText("Every user of the server can sign in with it.")).toBeInTheDocument();
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "Social");
    await user.type(within(form).getByRole("textbox", { name: "Server" }), "social.example");
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Login provider" })).toBeInTheDocument();
    expect(sent).toEqual({
      kind: "mastodon",
      display_name: "Social",
      issuer: "social.example",
      allow_signup: false,
      allow_link: true,
    });
  });

  test("shows the server's refusal on its field and keeps the draft", async () => {
    server.use(
      http.post(`${apiBase}/admin/login_providers`, () => problem("issuer", "The issuer must be an https URL.")),
    );
    const { user } = await renderApp({ path: newPath });
    const form = await newForm();
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "Example SSO");
    await user.type(within(form).getByRole("textbox", { name: "Issuer" }), "http://sso.example");
    await user.type(within(form).getByRole("textbox", { name: "Client ID" }), "my-client");
    await user.type(within(form).getByLabelText(/^Client secret/u), "s3cret");
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    const issuer = within(form).getByRole("textbox", { name: "Issuer" });
    await waitFor(() => {
      expect(issuer).toBeInvalid();
    });
    expect(issuer).toHaveAccessibleDescription(/The issuer must be an https URL\./u);
    expect(issuer).toHaveValue("http://sso.example");
  });

  test("shows a refused secret on the secret field", async () => {
    server.use(
      http.post(`${apiBase}/admin/login_providers`, () => problem("client_secret", "The client secret is too long.")),
    );
    const { user } = await renderApp({ path: newPath });
    const form = await newForm();
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "Example SSO");
    await user.type(within(form).getByRole("textbox", { name: "Issuer" }), "https://sso.example");
    await user.type(within(form).getByRole("textbox", { name: "Client ID" }), "my-client");
    await user.type(within(form).getByLabelText(/^Client secret/u), "s3cret");
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    const secret = within(form).getByLabelText(/^Client secret/u);
    await waitFor(() => {
      expect(secret).toBeInvalid();
    });
    expect(secret).toHaveAccessibleDescription(/The client secret is too long\./u);
  });

  test("refuses a condition without its values before sending", async () => {
    let posted = false;
    server.use(
      http.post(`${apiBase}/admin/login_providers`, () => {
        posted = true;
        return HttpResponse.json(loginProviderFixture(providerId));
      }),
    );
    const { user } = await renderApp({ path: newPath });
    const form = await newForm();
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "Example SSO");
    await user.type(within(form).getByRole("textbox", { name: "Issuer" }), "https://sso.example");
    await user.type(within(form).getByRole("textbox", { name: "Client ID" }), "my-client");
    await user.type(within(form).getByLabelText(/^Client secret/u), "s3cret");
    await user.click(within(form).getByRole("button", { name: "Add a condition" }));
    await user.type(within(form).getByRole("textbox", { name: "Claim of condition 1" }), "hd");
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    const conditions = within(form).getByRole("group", { name: "Conditions" });
    expect(await within(conditions).findByRole("alert")).toHaveTextContent(
      "Each condition needs a claim and at least one value.",
    );
    expect(posted).toBe(false);

    await user.click(within(conditions).getByRole("button", { name: "Remove condition 1" }));
    expect(within(conditions).getByRole("button", { name: "Add a condition" })).toHaveFocus();
  });

  test("changes a provider on its page without its secret, and says so next to the button", async () => {
    let change: Schemas["UpdateLoginProviderRequest"] | undefined;
    server.use(
      providerHandler(() => loginProviderFixture(providerId, change)),
      http.patch(`${apiBase}/admin/login_providers/:id`, async ({ request }) => {
        change = (await request.json()) as Schemas["UpdateLoginProviderRequest"];
        return HttpResponse.json(loginProviderFixture(providerId, change));
      }),
    );
    const { user } = await renderApp({ path: providerPath });
    const settings = await screen.findByRole("region", { name: "Settings" });

    expect(within(settings).queryByLabelText(/^Client secret/u)).not.toBeInTheDocument();
    expect(within(settings).queryByRole("radio")).not.toBeInTheDocument();
    await user.click(within(settings).getByRole("checkbox", { name: "Users may link their accounts" }));
    await user.click(within(settings).getByRole("button", { name: "Save" }));

    expect(await within(settings).findByRole("status")).toHaveTextContent("is saved.");
    expect(change).toEqual({
      display_name: "Example SSO",
      issuer: "https://sso.example",
      client_id: "my-client",
      scopes: [],
      allow_signup: false,
      allow_link: false,
      claims: [],
    });
  });

  test("gives a provider a new client secret in a dialog", async () => {
    let rotated: Schemas["RotateLoginProviderSecretRequest"] | undefined;
    server.use(
      providerHandler(() => loginProviderFixture(providerId)),
      http.post(`${apiBase}/admin/login_providers/:id/rotate_secret`, async ({ request }) => {
        rotated = (await request.json()) as Schemas["RotateLoginProviderSecretRequest"];
        return HttpResponse.json(loginProviderFixture(providerId));
      }),
    );
    const { user } = await renderApp({ path: providerPath });
    const actions = await screen.findByRole("region", { name: "Secret and state" });
    await user.click(within(actions).getByRole("button", { name: "New secret" }));
    const dialog = await screen.findByRole("dialog", { name: `New client secret for ${name}` });
    const secret = within(dialog).getByLabelText(/^Client secret/u);
    expect(secret).toHaveFocus();
    await user.type(secret, "n3w");
    await user.click(within(dialog).getByRole("button", { name: "Save the secret" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(rotated).toEqual({ client_secret: "n3w" });
    expect(screen.queryByText("n3w")).not.toBeInTheDocument();
  });

  test("disables a provider after a warning with its linked users, and enables it", async () => {
    let state: Schemas["AdminLoginProviderState"] = "enabled";
    server.use(
      providerHandler(() => loginProviderFixture(providerId, { state, linked_users: 2 })),
      http.post(`${apiBase}/admin/login_providers/:id/disable`, () => {
        state = "disabled";
        return HttpResponse.json(loginProviderFixture(providerId, { state, linked_users: 2 }));
      }),
      http.post(`${apiBase}/admin/login_providers/:id/enable`, () => {
        state = "enabled";
        return HttpResponse.json(loginProviderFixture(providerId, { state, linked_users: 2 }));
      }),
    );
    const { user } = await renderApp({ path: providerPath });
    await user.click(
      within(await screen.findByRole("region", { name: "Secret and state" })).getByRole("button", {
        name: "Disable",
      }),
    );
    const dialog = await screen.findByRole("alertdialog", { name: `Disable ${name}?` });
    expect(dialog).toHaveAccessibleDescription(/2 linked users can no longer sign in with it/u);
    await user.click(within(dialog).getByRole("button", { name: "Disable" }));

    // The page is hidden from assistive technology until the dialog has closed.
    const actions = await screen.findByRole("region", { name: "Secret and state" });
    await user.click(await within(actions).findByRole("button", { name: "Enable" }));
    expect(await within(actions).findByRole("button", { name: "Disable" })).toBeInTheDocument();
    expect(within(actions).getByRole("status")).toHaveTextContent("is enabled.");
  });

  describe("when the action would start the limit of sign-in to login providers", () => {
    const disabledList = () =>
      providerHandler(() => loginProviderFixture(providerId, { state: enabledState, linked_users: 1 }));
    let enabledState: Schemas["AdminLoginProviderState"] = "disabled";
    function enableServer() {
      enabledState = "disabled";
      const enables: string[] = [];
      server.use(
        disabledList(),
        http.post(`${apiBase}/admin/login_providers/:id/enable`, ({ params }) => {
          enables.push(String(params["id"]));
          enabledState = "enabled";
          return HttpResponse.json(loginProviderFixture(providerId, { state: "enabled", linked_users: 1 }));
        }),
      );
      return enables;
    }
    const title = "Start limiting sign-in to login providers?";

    test("warns before enabling, with the count for this provider, and enables once confirmed", async () => {
      const enables = enableServer();
      const seen: (string | null)[] = [];
      server.use(impactHandler({ setting_on: true, starts_limit: true, users_without_provider: 3 }, seen));
      const { user } = await renderApp({ path: providerPath });
      await user.click(await screen.findByRole("button", { name: "Enable" }));

      const dialog = await screen.findByRole("alertdialog", { name: title });
      expect(seen).toEqual([providerId]);
      expect(dialog).toHaveAccessibleDescription(/3 active users have no link to this provider/u);
      expect(dialog).toHaveAccessibleDescription(/my instance set login_providers_only false/u);
      expect(dialog).not.toHaveAccessibleDescription(/You are one of them/u);
      // The least destructive choice has the focus; cancelling enables nothing.
      expect(within(dialog).getByRole("button", { name: "Cancel" })).toHaveFocus();
      await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
      await waitFor(() => {
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
      });
      expect(enables).toEqual([]);

      await user.click(screen.getByRole("button", { name: "Enable" }));
      const again = await screen.findByRole("alertdialog", { name: title });
      await user.click(within(again).getByRole("button", { name: "Enable and start the limit" }));
      await waitFor(() => {
        expect(enables).toEqual([providerId]);
      });
    });

    test("says plainly when the count includes the administrator", async () => {
      enableServer();
      server.use(
        impactHandler({
          setting_on: true,
          starts_limit: true,
          users_without_provider: 1,
          includes_viewer: true,
        }),
      );
      const { user } = await renderApp({ path: providerPath });
      await user.click(await screen.findByRole("button", { name: "Enable" }));
      const dialog = await screen.findByRole("alertdialog", { name: title });
      expect(dialog).toHaveAccessibleDescription(/1 active user has no link to this provider/u);
      expect(dialog).toHaveAccessibleDescription(/You are one of them/u);
    });

    test("enables with no dialog when the setting is off or the limit is in force", async () => {
      const enables = enableServer();
      // Neither case starts anything: the server says so with starts_limit.
      server.use(
        impactHandler({
          setting_on: true,
          enabled_providers: 1,
          users_without_provider: 3,
          includes_viewer: true,
        }),
      );
      const { user } = await renderApp({ path: providerPath });
      await user.click(await screen.findByRole("button", { name: "Enable" }));
      await waitFor(() => {
        expect(enables).toEqual([providerId]);
      });
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });

    test("warns when the server cannot say, rather than start the limit unannounced", async () => {
      const enables = enableServer();
      server.use(
        http.get(`${apiBase}/admin/settings/login_providers_only/impact`, () =>
          HttpResponse.json(
            { title: "Failed", status: 500 },
            { status: 500, headers: { "Content-Type": "application/problem+json" } },
          ),
        ),
      );
      const { user } = await renderApp({ path: providerPath });
      await user.click(await screen.findByRole("button", { name: "Enable" }));
      const dialog = await screen.findByRole("alertdialog", { name: title });
      expect(dialog).toHaveAccessibleDescription(/could not say whether this starts the limit/u);
      expect(enables).toEqual([]);
      await user.click(within(dialog).getByRole("button", { name: "Enable" }));
      await waitFor(() => {
        expect(enables).toEqual([providerId]);
      });
    });

    /** Fills in the form of a GitHub provider. */
    async function fillGitHub(user: Awaited<ReturnType<typeof renderApp>>["user"]) {
      const form = await newForm();
      await user.click(within(form).getByRole("radio", { name: "GitHub" }));
      await user.type(within(form).getByRole("textbox", { name: "Display name" }), "GitHub");
      await user.type(within(form).getByRole("textbox", { name: "Client ID" }), "my-client");
      await user.type(within(form).getByLabelText(/^Client secret/u), "s3cret");
      await user.click(within(form).getByRole("button", { name: "Set up" }));
      return form;
    }

    function createServer() {
      const creates: unknown[] = [];
      server.use(
        http.get(`${apiBase}/admin/login_providers`, () => HttpResponse.json({ items: [] })),
        http.post(`${apiBase}/admin/login_providers`, async ({ request }) => {
          creates.push(await request.json());
          return HttpResponse.json(loginProviderFixture(providerId));
        }),
      );
      return creates;
    }

    test("warns before setting up the provider that starts the limit, and keeps the draft on cancel", async () => {
      const creates = createServer();
      const seen: (string | null)[] = [];
      server.use(
        impactHandler(
          {
            setting_on: true,
            starts_limit: true,
            users_without_provider: 4,
            includes_viewer: true,
          },
          seen,
        ),
      );
      const { user } = await renderApp({ path: newPath });
      await fillGitHub(user);

      const dialog = await screen.findByRole("alertdialog", { name: title });
      expect(seen).toEqual([null]);
      expect(dialog).toHaveAccessibleDescription(/4 active users have no link to this provider/u);
      expect(dialog).toHaveAccessibleDescription(/You are one of them/u);
      await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
      await waitFor(() => {
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
      });
      expect(creates).toEqual([]);
      const form = screen.getByRole("main");
      expect(within(form).getByRole("textbox", { name: "Display name" })).toHaveValue("GitHub");

      await user.click(within(form).getByRole("button", { name: "Set up" }));
      await user.click(
        within(await screen.findByRole("alertdialog", { name: title })).getByRole("button", {
          name: "Set up and start the limit",
        }),
      );
      await waitFor(() => {
        expect(creates).toHaveLength(1);
      });
    });

    test("sets up a provider with no dialog when that starts nothing", async () => {
      const creates = createServer();
      server.use(impactHandler({ setting_on: true, enabled_providers: 2 }));
      const { user } = await renderApp({ path: newPath });
      await fillGitHub(user);
      await waitFor(() => {
        expect(creates).toHaveLength(1);
      });
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
  });

  test("deletes a provider after a warning, and goes back to the list", async () => {
    let deleted = "";
    server.use(
      providerHandler(() => loginProviderFixture(providerId, { linked_users: 1 })),
      http.get(`${apiBase}/admin/login_providers`, () =>
        HttpResponse.json({
          items: deleted === "" ? [loginProviderFixture(providerId, { linked_users: 1 })] : [],
        }),
      ),
      http.delete(`${apiBase}/admin/login_providers/:id`, ({ params }) => {
        deleted = String(params["id"]);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user, router } = await renderApp({ path: providerPath });
    await user.click(
      within(await screen.findByRole("region", { name: "Secret and state" })).getByRole("button", {
        name: "Delete",
      }),
    );
    const dialog = await screen.findByRole("alertdialog", { name: `Delete ${name}?` });
    expect(dialog).toHaveAccessibleDescription(/1 linked user can no longer sign in with it/u);
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByRole("heading", { level: 3, name: "No login providers yet" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/admin/login-providers");
    expect(deleted).toBe(providerId);
  });
});
