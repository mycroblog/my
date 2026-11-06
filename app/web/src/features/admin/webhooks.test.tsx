// SPDX-License-Identifier: AGPL-3.0-only
// The webhooks screens through the whole app (web-client/README.md T4, ADR-0060): an administrator with manage_webhooks sees
// the webhooks in the list, sets one up on the form's page and sees its secret once, sees why the server refused one,
// and on the page of a webhook enables, changes, rotates and deletes it and sends a delivery again; with MSW playing the
// API.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { webhookDeliveryFixture, webhookFixture, webhookSecret } from "./test/handlers";

const webhookId = "0199f0e0-0000-7000-8000-0000000000b9";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

describe("the webhooks screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_webhooks", "manage_reports"]));
  });

  test("tells a viewer without the permission why it is empty", async () => {
    server.use(operatorHandler(["manage_settings"]));
    await renderApp({ path: "/admin/webhooks" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
  });

  test("lists the webhooks with why the server disabled one, linking to their page", async () => {
    server.use(
      http.get(`${apiBase}/admin/webhooks`, () =>
        HttpResponse.json({
          items: [
            webhookFixture(webhookId, {
              state: "disabled",
              state_reason: "failing",
              failure: { consecutive_failures: 20, first_failed_at: "2026-10-17T00:00:00Z" },
            }),
          ],
        }),
      ),
    );
    await renderApp({ path: "/admin/webhooks" });
    const list = await screen.findByRole("region", { name: "Webhooks of this server" });

    expect(await within(list).findByRole("link", { name: "Chat" })).toHaveAttribute(
      "href",
      `/admin/webhooks/${webhookId}`,
    );
    expect(within(list).getByText("(the deliveries kept failing)")).toBeInTheDocument();
    expect(within(list).getByText("20 failed attempts")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a webhook" })).toHaveAttribute("href", "/admin/webhooks/new");
  });

  test("sets up a webhook and shows its secret once, with the link to its page", async () => {
    let sent: Schemas["CreateWebhookRequest"] | undefined;
    server.use(
      http.post(`${apiBase}/admin/webhooks`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateWebhookRequest"];
        return HttpResponse.json({
          webhook: webhookFixture(webhookId, sent),
          secret: webhookSecret,
        });
      }),
    );
    const { user } = await renderApp({ path: "/admin/webhooks/new" });

    await user.type(await screen.findByRole("textbox", { name: "URL" }), "https://hooks.example/in");
    await user.type(screen.getByRole("textbox", { name: "Description" }), "Chat");
    const events = screen.getByRole("group", { name: "Events" });
    await user.click(within(events).getByRole("checkbox", { name: "A report is resolved" }));
    await user.click(within(events).getByRole("checkbox", { name: "A report is filed" }));
    await user.click(screen.getByRole("button", { name: "Set up" }));

    expect(await screen.findByText(webhookSecret)).toBeInTheDocument();
    expect(screen.getByText(/it is not shown again/)).toBeInTheDocument();
    expect(screen.getByText(/is set up\./u)).toBeInTheDocument();
    expect(sent).toEqual({
      url: "https://hooks.example/in",
      description: "Chat",
      events: ["report.created", "report.resolved"],
    });
    // The secret takes the place of the form.
    expect(screen.queryByRole("textbox", { name: "URL" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open the webhook" })).toHaveAttribute(
      "href",
      `/admin/webhooks/${webhookId}`,
    );
  });

  test("shows the events the user lacks the permission of on the events", async () => {
    server.use(
      http.post(`${apiBase}/admin/webhooks`, () =>
        HttpResponse.json(
          {
            type: "/api/v1/problems/invalid-webhook",
            title: "The webhook cannot be saved.",
            status: 422,
            detail: "You need the permissions of every event you choose.",
            reason: "missing_permission",
            missing_permissions: ["manage_accounts"],
          },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/admin/webhooks/new" });
    await user.type(await screen.findByRole("textbox", { name: "URL" }), "https://hooks.example/in");
    await user.click(screen.getByRole("checkbox", { name: "An account is created" }));
    await user.click(screen.getByRole("button", { name: "Set up" }));

    const events = screen.getByRole("group", { name: "Events" });
    expect(await within(events).findByRole("alert")).toHaveTextContent(
      /^You need the permissions of every event you choose\./,
    );
    expect(within(events).getByText("Missing permissions: ⁦manage_accounts⁩")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "URL" })).toHaveValue("https://hooks.example/in");
  });
});

describe("the page of a webhook", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_webhooks", "manage_reports"]));
  });

  test("shows a webhook that the server disabled, and enables it", async () => {
    const failing = webhookFixture(webhookId, {
      state: "disabled",
      state_reason: "failing",
      failure: { consecutive_failures: 20, first_failed_at: "2026-10-17T00:00:00Z" },
    });
    let enabled = false;
    server.use(
      http.get(`${apiBase}/admin/webhooks/:id`, () => HttpResponse.json(enabled ? webhookFixture(webhookId) : failing)),
      http.post(`${apiBase}/admin/webhooks/:id/enable`, () => {
        enabled = true;
        return HttpResponse.json(webhookFixture(webhookId));
      }),
    );
    const { user } = await renderApp({ path: `/admin/webhooks/${webhookId}` });
    const details = await screen.findByRole("region", { name: "Chat" });

    expect(within(details).getByText("(the deliveries kept failing)")).toBeInTheDocument();
    expect(within(details).getByText("20 failed attempts")).toBeInTheDocument();
    const actions = screen.getByRole("region", { name: "State, test and secret" });
    await user.click(within(actions).getByRole("button", { name: "Enable" }));

    expect(await within(actions).findByRole("button", { name: "Disable" })).toBeInTheDocument();
    expect(within(actions).getByRole("status")).toHaveTextContent("is enabled.");
  });

  test("gives a webhook a new secret after asking", async () => {
    const { user } = await renderApp({ path: `/admin/webhooks/${webhookId}` });
    const actions = await screen.findByRole("region", { name: "State, test and secret" });
    await user.click(within(actions).getByRole("button", { name: "New secret" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Make a new secret for ⁨Chat⁩?" });
    await user.click(within(dialog).getByRole("button", { name: "New secret" }));

    expect(await within(actions).findByText(webhookSecret)).toBeInTheDocument();
  });

  test("changes a webhook in its settings, and says so next to the button", async () => {
    let change: Schemas["UpdateWebhookRequest"] | undefined;
    server.use(
      http.get(`${apiBase}/admin/webhooks/:id`, () => HttpResponse.json(webhookFixture(webhookId, change))),
      http.patch(`${apiBase}/admin/webhooks/:id`, async ({ request }) => {
        change = (await request.json()) as Schemas["UpdateWebhookRequest"];
        return HttpResponse.json(webhookFixture(webhookId, change));
      }),
    );
    const { user } = await renderApp({ path: `/admin/webhooks/${webhookId}` });
    const settings = await screen.findByRole("region", { name: "Settings" });
    await user.click(within(settings).getByRole("checkbox", { name: "A public post is published" }));
    await user.click(within(settings).getByRole("button", { name: "Save" }));

    expect(await within(settings).findByRole("status")).toHaveTextContent("is saved.");
    expect(change).toEqual({
      url: "https://hooks.example/in",
      description: "Chat",
      events: ["report.created", "status.created"],
    });
  });

  test("deletes a webhook after asking, and goes back to the list", async () => {
    let deleted = "";
    server.use(
      http.delete(`${apiBase}/admin/webhooks/:id`, ({ params }) => {
        deleted = String(params["id"]);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user, router } = await renderApp({ path: `/admin/webhooks/${webhookId}` });
    const actions = await screen.findByRole("region", { name: "State, test and secret" });
    await user.click(within(actions).getByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete ⁨Chat⁩?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByRole("heading", { level: 3, name: "No webhooks yet" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/admin/webhooks");
    expect(deleted).toBe(webhookId);
  });

  test("lists the deliveries and sends one again", async () => {
    const failing = webhookDeliveryFixture("2", {
      status: "failing",
      attempts: 3,
      last_attempt: {
        outcome: "http_error",
        status_code: 503,
        duration_ms: 120,
        attempted_at: "2026-10-20T02:00:00Z",
      },
    });
    let redelivered = "";
    server.use(
      http.get(`${apiBase}/admin/webhooks/:id`, () => HttpResponse.json(webhookFixture(webhookId))),
      http.get(`${apiBase}/admin/webhooks/:id/deliveries`, () => HttpResponse.json({ items: [failing] })),
      http.post(`${apiBase}/admin/webhooks/:id/deliveries/:delivery_id/redeliver`, ({ params }) => {
        redelivered = String(params["delivery_id"]);
        return HttpResponse.json(failing, { status: 202 });
      }),
    );
    const { user } = await renderApp({ path: `/admin/webhooks/${webhookId}` });
    const deliveries = await screen.findByRole("region", { name: "Deliveries" });

    expect(within(deliveries).getByRole("rowheader", { name: /^A report is filed/u })).toBeInTheDocument();
    expect(within(deliveries).getByText("Failing")).toBeInTheDocument();
    expect(within(deliveries).getByText("HTTP 503")).toBeInTheDocument();
    await user.click(
      within(deliveries).getByRole("button", {
        name: `Send again: A report is filed ${failing.message_id}`,
      }),
    );

    expect(await within(deliveries).findByRole("status")).toHaveTextContent("The delivery is on its way again.");
    expect(redelivered).toBe("2");
  });
});
