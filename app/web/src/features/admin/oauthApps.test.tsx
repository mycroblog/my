// SPDX-License-Identifier: AGPL-3.0-only
// The OAuth apps screen through the whole app (web-client/README.md T4, ADR-0090 §1): an administrator with
// manage_oauth_apps sees the apps and keeps them to a registration route, disables one after asking and enables it
// again, denies the host of an app's metadata document, and lifts the denial of a host; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { hostDenialFixture, oauthClientFixture } from "./test/handlers";

const clientId = "0199f0e0-0000-7000-8000-0000000000e1";
const documentClientId = "0199f0e0-0000-7000-8000-0000000000e2";
const firstPartyId = "0199f0e0-0000-7000-8000-0000000000e3";
const name = "⁦Tootle⁩";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

describe("the OAuth apps screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_oauth_apps"]));
  });

  test("tells a viewer without the permission why it is empty", async () => {
    server.use(operatorHandler(["manage_webhooks"]));
    await renderApp({ path: "/admin/oauth-apps" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
  });

  test("lists the apps with their route, and offers no disabling of the server's own", async () => {
    server.use(
      http.get(`${apiBase}/admin/oauth_clients`, () =>
        HttpResponse.json({
          items: [
            oauthClientFixture(clientId, { owner_id: "0192f0c4-7a8e-7000-8000-000000000002" }),
            oauthClientFixture(firstPartyId, { name: "my web", client_id: "web", registration: "first_party" }),
          ],
        } satisfies Schemas["AdminOAuthClientPage"]),
      ),
    );
    await renderApp({ path: "/admin/oauth-apps" });
    const list = await screen.findByRole("region", { name: "Apps" });

    const [appRow, ownRow] = within(list).getAllByRole("row").slice(1);
    if (appRow === undefined || ownRow === undefined) throw new Error("two apps expected");
    expect(within(appRow).getByRole("rowheader", { name: /^Tootle/u })).toBeInTheDocument();
    expect(within(appRow).getByText("A user")).toBeInTheDocument();
    expect(within(appRow).getByText("read write")).toBeInTheDocument();
    expect(within(appRow).getByRole("button", { name: `Disable ${name}` })).toBeInTheDocument();
    expect(within(ownRow).getByText("This server")).toBeInTheDocument();
    expect(within(ownRow).queryByRole("button")).not.toBeInTheDocument();
    expect(within(ownRow).getByText("Cannot be disabled")).toBeInTheDocument();
  });

  test("keeps the list to one registration route", async () => {
    const asked: (string | null)[] = [];
    server.use(
      http.get(`${apiBase}/admin/oauth_clients`, ({ request }) => {
        const registration = new URL(request.url).searchParams.get("registration");
        asked.push(registration);
        return HttpResponse.json({
          items:
            registration === "dynamic" ? [oauthClientFixture(clientId, { registration: "dynamic", name: "Dyn" })] : [],
        } satisfies Schemas["AdminOAuthClientPage"]);
      }),
    );
    const { user } = await renderApp({ path: "/admin/oauth-apps" });
    await screen.findByRole("heading", { level: 3, name: "No apps" });

    await user.click(screen.getByRole("button", { name: /Registered by/u }));
    await user.click(await screen.findByRole("option", { name: "Dynamic registration" }));

    expect(await screen.findByRole("rowheader", { name: /^Dyn/u })).toBeInTheDocument();
    expect(asked).toEqual([null, "dynamic"]);
  });

  test("disables an app after asking, and enables it again", async () => {
    let disabled = false;
    const disabledIds: string[] = [];
    server.use(
      http.get(`${apiBase}/admin/oauth_clients`, () =>
        HttpResponse.json({
          items: [oauthClientFixture(clientId, { disabled })],
        } satisfies Schemas["AdminOAuthClientPage"]),
      ),
      http.post(`${apiBase}/admin/oauth_clients/:id/disable`, ({ params }) => {
        disabled = true;
        disabledIds.push(String(params["id"]));
        return HttpResponse.json(oauthClientFixture(clientId, { disabled }));
      }),
      http.post(`${apiBase}/admin/oauth_clients/:id/enable`, () => {
        disabled = false;
        return HttpResponse.json(oauthClientFixture(clientId, { disabled }));
      }),
    );
    const { user } = await renderApp({ path: "/admin/oauth-apps" });
    await user.click(await screen.findByRole("button", { name: `Disable ${name}` }));
    const dialog = await screen.findByRole("alertdialog", { name: `Disable ${name}?` });
    await user.click(within(dialog).getByRole("button", { name: "Disable" }));

    const list = screen.getByRole("region", { name: "Apps" });
    const enable = await within(list).findByRole("button", { name: `Enable ${name}` });
    expect(within(list).getByText("Disabled")).toBeInTheDocument();
    await waitFor(() => {
      expect(within(list).getByRole("status")).toHaveTextContent(`${name} is disabled, and its tokens are revoked.`);
    });
    expect(disabledIds).toEqual([clientId]);
    await user.click(enable);

    expect(await within(list).findByRole("button", { name: `Disable ${name}` })).toBeInTheDocument();
    expect(within(list).getByRole("status")).toHaveTextContent(`${name} is enabled.`);
  });

  test("shows why the server refused to disable an app", async () => {
    server.use(
      http.get(`${apiBase}/admin/oauth_clients`, () =>
        HttpResponse.json({ items: [oauthClientFixture(clientId)] } satisfies Schemas["AdminOAuthClientPage"]),
      ),
      http.post(`${apiBase}/admin/oauth_clients/:id/disable`, () =>
        HttpResponse.json(
          { type: "about:blank", title: "A first-party client cannot be disabled.", status: 422 },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/admin/oauth-apps" });
    await user.click(await screen.findByRole("button", { name: `Disable ${name}` }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Disable" }));

    const list = screen.getByRole("region", { name: "Apps" });
    expect(await within(list).findByRole("alert")).toHaveTextContent(
      "Could not change the app: A first-party client cannot be disabled.",
    );
  });

  test("denies the host of an app's metadata document", async () => {
    const denied: unknown[] = [];
    server.use(
      http.get(`${apiBase}/admin/oauth_clients`, () =>
        HttpResponse.json({
          items: [
            oauthClientFixture(documentClientId, {
              registration: "metadata_document",
              client_id: "https://App.Example/client.json",
              name: "Doc",
            }),
          ],
        } satisfies Schemas["AdminOAuthClientPage"]),
      ),
      http.post(`${apiBase}/admin/oauth_client_host_denials`, async ({ request }) => {
        denied.push(await request.json());
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/oauth-apps" });
    await user.click(await screen.findByRole("button", { name: "Deny the host app.example" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Deny app.example?" });
    await user.click(within(dialog).getByRole("button", { name: "Deny" }));

    const list = screen.getByRole("region", { name: "Apps" });
    await waitFor(() => {
      expect(within(list).getByRole("status")).toHaveTextContent(
        "app.example is denied, and the tokens of its apps are revoked.",
      );
    });
    expect(denied).toEqual([{ host: "app.example" }]);
  });
});

describe("the denied hosts", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_oauth_apps"]));
  });

  test("denies a host by its name", async () => {
    const denied: unknown[] = [];
    server.use(
      http.post(`${apiBase}/admin/oauth_client_host_denials`, async ({ request }) => {
        denied.push(await request.json());
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/oauth-apps?tab=hosts" });
    await screen.findByRole("heading", { level: 3, name: "No denied hosts" });
    await user.click(screen.getByRole("button", { name: "Deny a host" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Deny a host" });
    await user.type(within(dialog).getByRole("textbox", { name: "Host name" }), "Spam.example");
    await user.click(within(dialog).getByRole("button", { name: "Deny" }));

    await waitFor(() => {
      expect(denied).toEqual([{ host: "Spam.example" }]);
    });
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
  });

  test("lists the denied hosts and lifts a denial", async () => {
    let lifted = "";
    server.use(
      http.get(`${apiBase}/admin/oauth_client_host_denials`, () =>
        HttpResponse.json({
          items: lifted === "" ? [hostDenialFixture("0199f0e0-0000-7000-8000-0000000000f1", "spam.example")] : [],
        } satisfies Schemas["AdminOAuthClientHostDenialPage"]),
      ),
      http.delete(`${apiBase}/admin/oauth_client_host_denials/:host`, ({ params }) => {
        lifted = String(params["host"]);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/oauth-apps?tab=hosts" });
    expect(await screen.findByRole("rowheader", { name: "spam.example" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Allow the host spam.example" }));

    expect(await screen.findByRole("heading", { level: 3, name: "No denied hosts" })).toBeInTheDocument();
    expect(lifted).toBe("spam.example");
  });
});
