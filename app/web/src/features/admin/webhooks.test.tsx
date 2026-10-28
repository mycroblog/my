// SPDX-License-Identifier: AGPL-3.0-only
// The webhooks screens through the whole app (web-client/README.md T4, ADR-0060): an administrator with manage_webhooks sets
// up a webhook and sees its secret once, sees why the server refused one, disables, rotates and deletes it, and sends
// a delivery again from its page; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
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

  test("sets up a webhook and shows its secret once", async () => {
    let sent: Schemas["CreateWebhookRequest"] | undefined;
    let created = false;
    server.use(
      http.get(`${apiBase}/admin/webhooks`, () =>
        HttpResponse.json({ items: created ? [webhookFixture(webhookId, sent)] : [] }),
      ),
      http.post(`${apiBase}/admin/webhooks`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateWebhookRequest"];
        created = true;
        return HttpResponse.json({ webhook: webhookFixture(webhookId, sent), secret: webhookSecret });
      }),
    );
    const { user } = await renderApp({ path: "/admin/webhooks" });
    const form = await screen.findByRole("region", { name: "Set up a webhook" });

    await user.type(within(form).getByRole("textbox", { name: "URL" }), "https://hooks.example/in");
    await user.type(within(form).getByRole("textbox", { name: "Description" }), "Chat");
    const events = within(form).getByRole("group", { name: "Events" });
    await user.click(within(events).getByRole("checkbox", { name: "A report is resolved" }));
    await user.click(within(events).getByRole("checkbox", { name: "A report is filed" }));
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    expect(await within(form).findByText(webhookSecret)).toBeInTheDocument();
    expect(within(form).getByText(/it is not shown again/)).toBeInTheDocument();
    expect(within(form).getByRole("status")).toHaveTextContent("is set up.");
    expect(sent).toEqual({
      url: "https://hooks.example/in",
      description: "Chat",
      events: ["report.created", "report.resolved"],
    });
    expect(within(form).getByRole("textbox", { name: "URL" })).toHaveValue("");
    const list = screen.getByRole("region", { name: "Webhooks of this server" });
    expect(await within(list).findByRole("rowheader", { name: /^Chat/u })).toBeInTheDocument();
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
    const { user } = await renderApp({ path: "/admin/webhooks" });
    const form = await screen.findByRole("region", { name: "Set up a webhook" });
    await user.type(within(form).getByRole("textbox", { name: "URL" }), "https://hooks.example/in");
    await user.click(within(form).getByRole("checkbox", { name: "An account is created" }));
    await user.click(within(form).getByRole("button", { name: "Set up" }));

    const events = within(form).getByRole("group", { name: "Events" });
    expect(await within(events).findByRole("alert")).toHaveTextContent(
      /^You need the permissions of every event you choose\./,
    );
    expect(within(events).getByText("Missing permissions: ⁦manage_accounts⁩")).toBeInTheDocument();
    expect(within(form).getByRole("textbox", { name: "URL" })).toHaveValue("https://hooks.example/in");
  });

  test("shows a webhook that the server disabled, and enables it", async () => {
    const failing = webhookFixture(webhookId, {
      state: "disabled",
      state_reason: "failing",
      failure: { consecutive_failures: 20, first_failed_at: "2026-10-17T00:00:00Z" },
    });
    let enabled = false;
    server.use(
      http.get(`${apiBase}/admin/webhooks`, () =>
        HttpResponse.json({ items: [enabled ? webhookFixture(webhookId) : failing] }),
      ),
      http.post(`${apiBase}/admin/webhooks/:id/enable`, () => {
        enabled = true;
        return HttpResponse.json(webhookFixture(webhookId));
      }),
    );
    const { user } = await renderApp({ path: "/admin/webhooks" });
    const list = await screen.findByRole("region", { name: "Webhooks of this server" });

    expect(await within(list).findByText("(the deliveries kept failing)")).toBeInTheDocument();
    expect(within(list).getByText("20 failed attempts")).toBeInTheDocument();
    await user.click(within(list).getByRole("button", { name: "Enable ⁨Chat⁩" }));

    expect(await within(list).findByRole("button", { name: "Disable ⁨Chat⁩" })).toBeInTheDocument();
    expect(within(list).getByRole("status")).toHaveTextContent("is enabled.");
  });

  test("gives a webhook a new secret after asking", async () => {
    server.use(http.get(`${apiBase}/admin/webhooks`, () => HttpResponse.json({ items: [webhookFixture(webhookId)] })));
    const { user } = await renderApp({ path: "/admin/webhooks" });
    await user.click(await screen.findByRole("button", { name: "New secret for ⁨Chat⁩" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Make a new secret for ⁨Chat⁩?" });
    await user.click(within(dialog).getByRole("button", { name: "New secret" }));

    const list = screen.getByRole("region", { name: "Webhooks of this server" });
    expect(await within(list).findByText(webhookSecret)).toBeInTheDocument();
  });

  test("changes a webhook in a dialog", async () => {
    let change: Schemas["UpdateWebhookRequest"] | undefined;
    server.use(
      http.get(`${apiBase}/admin/webhooks`, () => HttpResponse.json({ items: [webhookFixture(webhookId, change)] })),
      http.patch(`${apiBase}/admin/webhooks/:id`, async ({ request }) => {
        change = (await request.json()) as Schemas["UpdateWebhookRequest"];
        return HttpResponse.json(webhookFixture(webhookId, change));
      }),
    );
    const { user } = await renderApp({ path: "/admin/webhooks" });
    await user.click(await screen.findByRole("button", { name: "Edit ⁨Chat⁩" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit ⁨Chat⁩" });
    await user.click(within(dialog).getByRole("checkbox", { name: "A public post is published" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(change).toEqual({
      url: "https://hooks.example/in",
      description: "Chat",
      events: ["report.created", "status.created"],
    });
  });

  test("deletes a webhook after asking", async () => {
    let deleted = "";
    server.use(
      http.get(`${apiBase}/admin/webhooks`, () =>
        HttpResponse.json({ items: deleted === "" ? [webhookFixture(webhookId)] : [] }),
      ),
      http.delete(`${apiBase}/admin/webhooks/:id`, ({ params }) => {
        deleted = String(params["id"]);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/webhooks" });
    await user.click(await screen.findByRole("button", { name: "Delete ⁨Chat⁩" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete ⁨Chat⁩?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByRole("heading", { level: 3, name: "No webhooks yet" })).toBeInTheDocument();
    expect(deleted).toBe(webhookId);
  });
});

describe("the page of a webhook", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_webhooks"]));
  });

  test("lists the deliveries and sends one again", async () => {
    const failing = webhookDeliveryFixture("2", {
      status: "failing",
      attempts: 3,
      last_attempt: { outcome: "http_error", status_code: 503, duration_ms: 120, attempted_at: "2026-10-20T02:00:00Z" },
    });
    let redelivered = "";
    server.use(
      http.get(`${apiBase}/admin/webhooks/:id`, () => HttpResponse.json(webhookFixture(webhookId))),
      http.get(`${apiBase}/admin/webhooks/:id/deliveries`, () => HttpResponse.json({ items: [failing] })),
      http.post(`${apiBase}/admin/webhooks/:id/deliveries/:delivery_id/redeliver`, ({ params }) => {
        redelivered = String(params["delivery_id"]);
        return HttpResponse.json(failing);
      }),
    );
    const { user } = await renderApp({ path: `/admin/webhooks/${webhookId}` });
    const deliveries = await screen.findByRole("region", { name: "Deliveries" });

    expect(within(deliveries).getByRole("rowheader", { name: /^A report is filed/u })).toBeInTheDocument();
    expect(within(deliveries).getByText("Failing")).toBeInTheDocument();
    expect(within(deliveries).getByText("HTTP 503")).toBeInTheDocument();
    await user.click(
      within(deliveries).getByRole("button", { name: `Send A report is filed ${failing.message_id} again` }),
    );

    expect(await within(deliveries).findByRole("status")).toHaveTextContent("The delivery is on its way again.");
    expect(redelivered).toBe("2");
  });
});
