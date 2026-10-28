// SPDX-License-Identifier: AGPL-3.0-only
// The login providers screen through the whole app (web-client/README.md T4, ADR-0106): an administrator with
// manage_login_providers sets up a provider of each kind and copies its callback URL, sees a refused field and an
// incomplete condition, changes a provider in a dialog without its secret, gives it a new secret, and disables and
// deletes it after a warning with the number of linked users; with MSW playing the API.
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

describe("the login providers screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_login_providers"]));
  });

  test("tells a viewer without the permission why it is empty", async () => {
    server.use(operatorHandler(["manage_settings"]));
    await renderApp({ path: "/admin/login-providers" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
  });

  test("lists the providers with their kind, state, linked users and callback URL", async () => {
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
    expect(within(sso).getByText("OpenID Connect")).toBeInTheDocument();
    expect(within(sso).getByText("Enabled")).toBeInTheDocument();
    expect(within(sso).getByText("Sign-up allowed")).toBeInTheDocument();
    expect(within(sso).getByText("3 users")).toBeInTheDocument();
    expect(within(sso).getByText(`https://my.example/auth/external/${providerId}/callback`)).toBeInTheDocument();
    expect(within(sso).getByRole("button", { name: `Copy the callback URL of ${name}` })).toBeInTheDocument();
    const github = within(list).getByRole("row", { name: /^GitHub/u });
    expect(within(github).getByText("Disabled")).toBeInTheDocument();
    expect(within(github).getByRole("button", { name: "Enable ⁨GitHub⁩" })).toBeInTheDocument();
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
    const { user } = await renderApp({ path: "/admin/login-providers" });
    const form = await screen.findByRole("region", { name: "Set up a login provider" });

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

    expect(
      await within(form).findByText(`https://my.example/auth/external/${providerId}/callback`),
    ).toBeInTheDocument();
    expect(within(form).getByRole("status")).toHaveTextContent("is set up.");
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
    expect(within(form).getByRole("textbox", { name: "Display name" })).toHaveValue("");
    expect(within(form).getByLabelText(/^Client secret/u)).toHaveValue("");
  });

  test("sends only what a GitHub provider takes", async () => {
    let sent: Schemas["CreateLoginProviderRequest"] | undefined;
    server.use(
      http.post(`${apiBase}/admin/login_providers`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateLoginProviderRequest"];
        return HttpResponse.json(loginProviderFixture(providerId, { kind: "github", display_name: "GitHub" }));
      }),
    );
    const { user } = await renderApp({ path: "/admin/login-providers" });
    const form = await screen.findByRole("region", { name: "Set up a login provider" });

    await user.click(within(form).getByRole("radio", { name: "GitHub" }));
    expect(within(form).queryByRole("textbox", { name: "Issuer" })).not.toBeInTheDocument();
    expect(within(form).queryByRole("group", { name: "Conditions" })).not.toBeInTheDocument();
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "GitHub");
    await user.type(within(form).getByRole("textbox", { name: "Client ID" }), "Iv1.abc");
    await user.type(within(form).getByLabelText(/^Client secret/u), "s3cret");
    await user.type(within(form).getByRole("textbox", { name: "Organizations" }), "my-org, other-org");
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    expect(await within(form).findByRole("status")).toHaveTextContent("is set up.");
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
    const { user } = await renderApp({ path: "/admin/login-providers" });
    const form = await screen.findByRole("region", { name: "Set up a login provider" });

    await user.click(within(form).getByRole("radio", { name: "Discord" }));
    expect(within(form).queryByRole("textbox", { name: "Issuer" })).not.toBeInTheDocument();
    expect(within(form).queryByRole("textbox", { name: "Organizations" })).not.toBeInTheDocument();
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "Discord");
    await user.type(within(form).getByRole("textbox", { name: "Client ID" }), "1234");
    await user.type(within(form).getByLabelText(/^Client secret/u), "s3cret");
    await user.type(within(form).getByRole("textbox", { name: "Discord servers" }), "1234567890, 987654321");
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    expect(await within(form).findByRole("status")).toHaveTextContent("is set up.");
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
    const { user } = await renderApp({ path: "/admin/login-providers" });
    const form = await screen.findByRole("region", { name: "Set up a login provider" });
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
    const { user } = await renderApp({ path: "/admin/login-providers" });
    const form = await screen.findByRole("region", { name: "Set up a login provider" });
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

  test("keeps the secret of a Discord provider out of its edit form", async () => {
    server.use(
      http.get(`${apiBase}/admin/login_providers`, () =>
        HttpResponse.json({
          items: [loginProviderFixture("p3", { kind: "discord", display_name: "Discord", guilds: ["42"] })],
        }),
      ),
    );
    const { user } = await renderApp({ path: "/admin/login-providers" });
    const list = await screen.findByRole("region", { name: "Login providers of this server" });
    await user.click(within(list).getByRole("button", { name: /^Edit/u }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("textbox", { name: "Discord servers" })).toHaveValue("42");
    expect(within(dialog).queryByLabelText(/^Client secret/u)).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("textbox", { name: "Issuer" })).not.toBeInTheDocument();
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
    const { user } = await renderApp({ path: "/admin/login-providers" });
    const form = await screen.findByRole("region", { name: "Set up a login provider" });

    await user.click(within(form).getByRole("radio", { name: "Mastodon" }));
    expect(within(form).getByRole("textbox", { name: "Server" })).toBeRequired();
    expect(within(form).getByRole("textbox", { name: "Client ID" })).not.toBeRequired();
    expect(within(form).getByLabelText(/^Client secret/u)).not.toBeRequired();
    expect(within(form).queryByRole("textbox", { name: "Extra scopes" })).not.toBeInTheDocument();
    expect(within(form).getByText("Every user of the server can sign in with it.")).toBeInTheDocument();
    await user.type(within(form).getByRole("textbox", { name: "Display name" }), "Social");
    await user.type(within(form).getByRole("textbox", { name: "Server" }), "social.example");
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    expect(await within(form).findByRole("status")).toHaveTextContent("is set up.");
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
    const { user } = await renderApp({ path: "/admin/login-providers" });
    const form = await screen.findByRole("region", { name: "Set up a login provider" });
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
    const { user } = await renderApp({ path: "/admin/login-providers" });
    const form = await screen.findByRole("region", { name: "Set up a login provider" });
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
    const { user } = await renderApp({ path: "/admin/login-providers" });
    const form = await screen.findByRole("region", { name: "Set up a login provider" });
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

  test("changes a provider in a dialog without its secret", async () => {
    let change: Schemas["UpdateLoginProviderRequest"] | undefined;
    server.use(
      http.get(`${apiBase}/admin/login_providers`, () =>
        HttpResponse.json({ items: [loginProviderFixture(providerId, change)] }),
      ),
      http.patch(`${apiBase}/admin/login_providers/:id`, async ({ request }) => {
        change = (await request.json()) as Schemas["UpdateLoginProviderRequest"];
        return HttpResponse.json(loginProviderFixture(providerId, change));
      }),
    );
    const { user } = await renderApp({ path: "/admin/login-providers" });
    await user.click(await screen.findByRole("button", { name: `Edit ${name}` }));
    const dialog = await screen.findByRole("dialog", { name: `Edit ${name}` });

    expect(within(dialog).queryByLabelText(/^Client secret/u)).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("radio")).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("checkbox", { name: "Users may link their accounts" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(change).toEqual({
      display_name: "Example SSO",
      issuer: "https://sso.example",
      client_id: "my-client",
      scopes: [],
      allow_signup: false,
      allow_link: false,
      claims: [],
    });
    const list = screen.getByRole("region", { name: "Login providers of this server" });
    expect(within(list).getByRole("status")).toHaveTextContent("is saved.");
  });

  test("gives a provider a new client secret in a dialog", async () => {
    let rotated: Schemas["RotateLoginProviderSecretRequest"] | undefined;
    server.use(
      http.get(`${apiBase}/admin/login_providers`, () =>
        HttpResponse.json({ items: [loginProviderFixture(providerId)] }),
      ),
      http.post(`${apiBase}/admin/login_providers/:id/rotate_secret`, async ({ request }) => {
        rotated = (await request.json()) as Schemas["RotateLoginProviderSecretRequest"];
        return HttpResponse.json(loginProviderFixture(providerId));
      }),
    );
    const { user } = await renderApp({ path: "/admin/login-providers" });
    await user.click(await screen.findByRole("button", { name: `New secret for ${name}` }));
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
      http.get(`${apiBase}/admin/login_providers`, () =>
        HttpResponse.json({ items: [loginProviderFixture(providerId, { state, linked_users: 2 })] }),
      ),
      http.post(`${apiBase}/admin/login_providers/:id/disable`, () => {
        state = "disabled";
        return HttpResponse.json(loginProviderFixture(providerId, { state, linked_users: 2 }));
      }),
      http.post(`${apiBase}/admin/login_providers/:id/enable`, () => {
        state = "enabled";
        return HttpResponse.json(loginProviderFixture(providerId, { state, linked_users: 2 }));
      }),
    );
    const { user } = await renderApp({ path: "/admin/login-providers" });
    await user.click(await screen.findByRole("button", { name: `Disable ${name}` }));
    const dialog = await screen.findByRole("alertdialog", { name: `Disable ${name}?` });
    expect(dialog).toHaveAccessibleDescription(/2 linked users can no longer sign in with it/u);
    await user.click(within(dialog).getByRole("button", { name: "Disable" }));

    const list = screen.getByRole("region", { name: "Login providers of this server" });
    await user.click(await within(list).findByRole("button", { name: `Enable ${name}` }));
    expect(await within(list).findByRole("button", { name: `Disable ${name}` })).toBeInTheDocument();
    expect(within(list).getByRole("status")).toHaveTextContent("is enabled.");
  });

  test("deletes a provider after a warning", async () => {
    let deleted = "";
    server.use(
      http.get(`${apiBase}/admin/login_providers`, () =>
        HttpResponse.json({ items: deleted === "" ? [loginProviderFixture(providerId, { linked_users: 1 })] : [] }),
      ),
      http.delete(`${apiBase}/admin/login_providers/:id`, ({ params }) => {
        deleted = String(params["id"]);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/login-providers" });
    await user.click(await screen.findByRole("button", { name: `Delete ${name}` }));
    const dialog = await screen.findByRole("alertdialog", { name: `Delete ${name}?` });
    expect(dialog).toHaveAccessibleDescription(/1 linked user can no longer sign in with it/u);
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByRole("heading", { level: 3, name: "No login providers yet" })).toBeInTheDocument();
    expect(deleted).toBe(providerId);
  });
});
