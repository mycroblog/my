// SPDX-License-Identifier: AGPL-3.0-only
// The development page through the whole app (web-client/README.md T4): registering an app and seeing its secret once,
// changing, deleting, giving a new secret, and issuing tokens to oneself, with MSW playing the server.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

const bot: Schemas["OAuthApp"] = {
  id: "019a0000-0000-7000-8000-0000000000b1",
  client_id: "my_ci_bot",
  name: "Morning bot",
  redirect_uris: ["http://127.0.0.1/callback"],
  scope: "read write",
  client_type: "confidential",
  disabled: false,
  created_at: "2026-10-01T09:00:00Z",
};

const script: Schemas["OAuthApp"] = { ...bot, id: "019a0000-0000-7000-8000-0000000000b2", client_id: "my_ci_script", name: "Script", client_type: "public" };

/** The server's apps of the viewer, which the operations change, with the requests they received. */
function appServer(initial: Schemas["OAuthApp"][]) {
  let items = initial;
  const created: Schemas["CreateOAuthAppRequest"][] = [];
  const updated: Schemas["UpdateOAuthAppRequest"][] = [];
  const issued: Schemas["IssueOAuthAppTokenRequest"][] = [];
  const rotated: string[] = [];
  server.use(
    http.get(`${apiBase}/oauth_apps`, () => HttpResponse.json({ items })),
    http.post(`${apiBase}/oauth_apps`, async ({ request }) => {
      const body = (await request.json()) as Schemas["CreateOAuthAppRequest"];
      created.push(body);
      const app: Schemas["OAuthApp"] = {
        id: "019a0000-0000-7000-8000-0000000000b3",
        client_id: "my_ci_new",
        name: body.name,
        redirect_uris: body.redirect_uris,
        scope: body.scope,
        client_type: body.client_type,
        disabled: false,
        created_at: "2026-10-20T00:00:00Z",
      };
      items = [...items, app];
      const answer: Schemas["OAuthAppCreated"] = body.client_type === "confidential" ? { app, client_secret: "my_cs_first" } : { app };
      return HttpResponse.json(answer, { status: 201 });
    }),
    http.patch(`${apiBase}/oauth_apps/:id`, async ({ params, request }) => {
      const body = (await request.json()) as Schemas["UpdateOAuthAppRequest"];
      updated.push(body);
      if (body.scope === "admin") return problem(422, "/api/v1/problems/validation", "The scope admin is not available to apps.");
      items = items.map((app) => (app.id === params["id"] ? { ...app, ...(body.name !== undefined && { name: body.name }), ...(body.scope !== undefined && { scope: body.scope }) } : app));
      return HttpResponse.json(items.find((app) => app.id === params["id"]));
    }),
    http.delete(`${apiBase}/oauth_apps/:id`, ({ params }) => {
      items = items.filter((app) => app.id !== params["id"]);
      return new HttpResponse(null, { status: 204 });
    }),
    http.post(`${apiBase}/oauth_apps/:id/secret`, ({ params }) => {
      rotated.push(String(params["id"]));
      return HttpResponse.json({ client_secret: "my_cs_second" });
    }),
    http.post(`${apiBase}/oauth_apps/:id/tokens`, async ({ request }) => {
      const body = (await request.json()) as Schemas["IssueOAuthAppTokenRequest"];
      issued.push(body);
      if (body.scope === "follow") return problem(422, "/api/v1/problems/validation", "The scope follow is beyond the app's.");
      return HttpResponse.json({ access_token: "at-1", token_type: "Bearer", expires_in: 3600, refresh_token: "rt-1", scope: body.scope ?? "read write" });
    }),
  );
  return { created, updated, issued, rotated };
}

describe("development settings", () => {
  test("are in the settings menu", async () => {
    await renderApp({ path: "/settings" });
    expect(await screen.findByRole("link", { name: "Development" })).toHaveAttribute("href", "/settings/apps");
  });

  test("registers a confidential app and shows its secret once, with a copy button", async () => {
    const api = appServer([]);
    const { user } = await renderApp({ path: "/settings/apps" });

    expect(await screen.findByRole("heading", { level: 1, name: "Development" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 3, name: "No apps yet" })).toBeInTheDocument();

    const form = screen.getByRole("region", { name: "Register an app" });
    await user.type(within(form).getByRole("textbox", { name: "Name" }), " Morning bot ");
    await user.type(within(form).getByRole("textbox", { name: /^Redirect URIs/ }), "http://127.0.0.1/callback{Enter}https://bot.example/cb");
    await user.click(within(form).getByRole("radio", { name: /^Confidential/ }));
    await user.click(within(form).getByRole("button", { name: "Register app" }));

    const notice = await screen.findByRole("region", { name: "Client secret of Morning bot" });
    expect(within(notice).getByText("my_cs_first")).toBeInTheDocument();
    expect(within(notice).getByText("my_ci_new")).toBeInTheDocument();
    expect(within(notice).getByText(/only time the secret is shown/)).toBeInTheDocument();
    expect(notice).toHaveFocus();
    expect(api.created).toEqual([
      {
        name: "Morning bot",
        redirect_uris: ["http://127.0.0.1/callback", "https://bot.example/cb"],
        scope: "read write",
        client_type: "confidential",
      },
    ]);
    expect(await screen.findByText("The app “Morning bot” was registered.")).toBeInTheDocument();
    const list = screen.getByRole("region", { name: "Your apps" });
    expect(await within(list).findByText("Morning bot")).toHaveAttribute("dir", "auto");

    await user.click(within(notice).getByRole("button", { name: "Copy: Client secret" }));
    expect(await navigator.clipboard.readText()).toBe("my_cs_first");
    expect(await screen.findByText("Copied to the clipboard.")).toBeInTheDocument();
  });

  test("registers a public app, which has no secret, and says why a refusal happened", async () => {
    const api = appServer([]);
    const { user } = await renderApp({ path: "/settings/apps" });
    const form = await screen.findByRole("region", { name: "Register an app" });

    server.use(http.post(`${apiBase}/oauth_apps`, () => problem(409, "/api/v1/problems/too-many-oauth-apps", "You have as many apps as allowed.")));
    await user.type(within(form).getByRole("textbox", { name: "Name" }), "Script");
    await user.type(within(form).getByRole("textbox", { name: /^Redirect URIs/ }), "http://127.0.0.1/cb");
    await user.click(within(form).getByRole("button", { name: "Register app" }));
    expect(await within(form).findByRole("alert")).toHaveTextContent("The app was not registered: You have as many apps as allowed.");
    expect(screen.queryByRole("region", { name: /^Client secret/ })).not.toBeInTheDocument();
    expect(api.created).toEqual([]);
  });

  test("shows a public app's client ID without a secret", async () => {
    appServer([]);
    const { user } = await renderApp({ path: "/settings/apps" });
    const form = await screen.findByRole("region", { name: "Register an app" });
    await user.type(within(form).getByRole("textbox", { name: "Name" }), "Script");
    await user.type(within(form).getByRole("textbox", { name: /^Redirect URIs/ }), "http://127.0.0.1/cb");
    await user.click(within(form).getByRole("button", { name: "Register app" }));
    const notice = await screen.findByRole("region", { name: "Client secret of Script" });
    expect(within(notice).getByText("my_ci_new")).toBeInTheDocument();
    expect(within(notice).getByText(/has no secret/)).toBeInTheDocument();
    expect(within(notice).queryByRole("button", { name: "Copy: Client secret" })).not.toBeInTheDocument();
  });

  test("changes an app in a dialog and keeps the dialog open on a refusal", async () => {
    const api = appServer([bot]);
    const { user } = await renderApp({ path: "/settings/apps" });
    await user.click(await screen.findByRole("button", { name: "Edit: Morning bot" }));

    const dialog = await screen.findByRole("dialog", { name: "Edit Morning bot" });
    const scope = within(dialog).getByRole("textbox", { name: /^Scopes/ });
    expect(scope).toHaveValue("read write");
    await user.clear(scope);
    await user.type(scope, "admin");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("The app was not saved: The scope admin is not available to apps.");

    await user.clear(scope);
    await user.type(scope, "read");
    await user.clear(within(dialog).getByRole("textbox", { name: "Name" }));
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "Evening bot");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Evening bot")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(api.updated.at(-1)).toEqual({ name: "Evening bot", website: "", redirect_uris: ["http://127.0.0.1/callback"], scope: "read" });
  });

  test("gives a confidential app a new secret after a confirmation, and has no such button for a public app", async () => {
    const api = appServer([bot, script]);
    const { user } = await renderApp({ path: "/settings/apps" });
    expect(await screen.findByRole("button", { name: "New secret: Morning bot" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "New secret: Script" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "New secret: Morning bot" }));
    const dialog = await screen.findByRole("alertdialog");
    expect(dialog).toHaveTextContent("The old secret stops working at once");
    await user.click(within(dialog).getByRole("button", { name: "New secret" }));

    const notice = await screen.findByRole("region", { name: "Client secret of Morning bot" });
    expect(within(notice).getByText("my_cs_second")).toBeInTheDocument();
    expect(api.rotated).toEqual([bot.id]);
  });

  test("deletes an app after a confirmation", async () => {
    appServer([bot, script]);
    const { user } = await renderApp({ path: "/settings/apps" });
    await user.click(await screen.findByRole("button", { name: "Delete: Morning bot" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete Morning bot?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    const list = screen.getByRole("region", { name: "Your apps" });
    await within(list).findByText("Script");
    expect(within(list).queryByText("Morning bot")).not.toBeInTheDocument();
  });

  test("keeps an app when deleting fails", async () => {
    appServer([bot]);
    server.use(http.delete(`${apiBase}/oauth_apps/:id`, () => problem(404, "/api/v1/problems/not-found", "No such app.")));
    const { user } = await renderApp({ path: "/settings/apps" });
    await user.click(await screen.findByRole("button", { name: "Delete: Morning bot" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("The app was not deleted: No such app.");
    expect(screen.getByRole("region", { name: "Your apps", hidden: true })).toHaveTextContent("Morning bot");
  });

  test("issues tokens to oneself and shows them once", async () => {
    const api = appServer([bot]);
    const { user } = await renderApp({ path: "/settings/apps" });
    await user.click(await screen.findByRole("button", { name: "Issue token: Morning bot" }));
    const dialog = await screen.findByRole("dialog", { name: "Issue a token of Morning bot" });

    const scope = within(dialog).getByRole("textbox", { name: /^Scopes/ });
    await user.clear(scope);
    await user.type(scope, "follow");
    await user.click(within(dialog).getByRole("button", { name: "Issue token" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("The token was not issued: The scope follow is beyond the app's.");

    await user.clear(scope);
    await user.type(scope, "read");
    await user.click(within(dialog).getByRole("button", { name: "Issue token" }));
    expect(await within(dialog).findByText("at-1")).toBeInTheDocument();
    expect(within(dialog).getByText("rt-1")).toBeInTheDocument();
    expect(within(dialog).getByText(/shown only now/)).toBeInTheDocument();
    expect(within(dialog).getByText(/lasts 60 minutes/)).toBeInTheDocument();
    expect(api.issued).toEqual([{ scope: "follow" }, { scope: "read" }]);

    await user.click(within(dialog).getByRole("button", { name: "Copy: Access token" }));
    expect(await navigator.clipboard.readText()).toBe("at-1");

    await user.click(within(dialog).getByRole("button", { name: "Done" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    // Opening it again asks for the scopes again; the tokens are gone.
    await user.click(screen.getByRole("button", { name: "Issue token: Morning bot" }));
    const again = await screen.findByRole("dialog", { name: "Issue a token of Morning bot" });
    expect(within(again).queryByText("at-1")).not.toBeInTheDocument();
  });
});
