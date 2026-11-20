// SPDX-License-Identifier: AGPL-3.0-only

// The fake's webhooks (FR-722, ADR-0060), as a holder of `manage_webhooks` keeps them under /admin/webhooks, checked
// as the server checks them: a URL of http or https of at most 2048 bytes without user info or a fragment, a
// description of at most 200 characters, at least one event and none twice, and no event whose permission the reader
// lacks among those it is to receive, its old ones too (`missing_permission`). The webhooks: `alerts`, enabled and healthy, with deliveries that
// succeeded, one that waits and one that keeps failing; `chat`, enabled and failing now; `archive`, which the server
// disabled after its deliveries kept failing; `old`, which an administrator disabled. A secret is `whsec_` and the
// base64 of 32 bytes, as the server makes it; a test adds a delivery that waits, and a redelivery puts one back to
// waiting.

import { page } from "./common";
import type { StaffServerContext } from "./staff-server";
import type { Handlers, Schema } from "./spec";

type Webhook = Schema<"AdminWebhook">;
type Delivery = Schema<"AdminWebhookDelivery">;
type Event = Schema<"WebhookEventType">;
type Permission = Schema<"Permission">;

/** What the stories ask of the fake's webhooks. */
export type FakeWebhookOptions = {
  /** The server starts with no webhook. */
  readonly none?: boolean;
};

/** The permission an event needs besides `manage_webhooks`, as the server's `WebhookEventType` says. */
function needed(event: Event): Permission | undefined {
  if (event.startsWith("report.")) return "manage_reports";
  if (event.startsWith("domain_moderation_action.")) return "manage_domains";
  if (
    event === "registration.pending" ||
    event.startsWith("account.") ||
    event.startsWith("moderation_action.") ||
    event === "appeal.created"
  )
    return "manage_accounts";
  if (event.startsWith("custom_emoji.")) return "manage_custom_emojis";
  if (event.startsWith("error_group.")) return "manage_errors";
  return undefined;
}

/** A secret as the server makes it: `whsec_` and the base64 of 32 bytes. */
function secret(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  return `whsec_${btoa(String.fromCharCode(...bytes))}`;
}

const hour = 3_600_000;

export function webhookHandlers(c: StaffServerContext): {
  handlers: Handlers;
  id: (key: string) => string;
} {
  const at = (ms: number) => new Date(ms).toISOString();
  const s = c.started;
  const keyed = new Map<string, string>();
  const webhooks: Webhook[] = [];
  const deliveries = new Map<string, Delivery[]>();
  let made = 0;
  const add = (key: string, w: Omit<Webhook, "id">, list: Omit<Delivery, "id" | "message_id">[]) => {
    const n = webhooks.length;
    const id = c.idAt(Date.parse(w.created_at), 80_000 + n);
    keyed.set(key, id);
    webhooks.push({ ...w, id });
    deliveries.set(
      id,
      list.map((d, i) => ({
        ...d,
        id: c.idAt(Date.parse(d.created_at), 81_000 + n * 100 + i),
        message_id: c.idAt(Date.parse(d.created_at), 82_000 + n * 100 + i),
      })),
    );
  };
  if (c.options?.webhooks?.none !== true) {
    add(
      "old",
      {
        url: "http://10.0.0.8:8080/hook",
        description: "The old status board",
        events: ["account.created"],
        state: "disabled",
        state_reason: "admin",
        state_changed_at: at(s - 120 * 24 * hour),
        created_at: at(s - 200 * 24 * hour),
        revised_at: at(s - 200 * 24 * hour),
      },
      [],
    );
    add(
      "archive",
      {
        url: "https://archive.example.com/collect",
        description: "Custom emoji changes for the archive",
        events: ["custom_emoji.created", "custom_emoji.updated", "custom_emoji.deleted"],
        state: "disabled",
        state_reason: "failing",
        state_changed_at: at(s - 3 * 24 * hour),
        failure: {
          consecutive_failures: 23,
          first_failed_at: at(s - 7 * 24 * hour),
        },
        created_at: at(s - 90 * 24 * hour),
        revised_at: at(s - 90 * 24 * hour),
      },
      [
        {
          event_type: "custom_emoji.updated",
          status: "failing",
          attempts: 8,
          last_attempt: {
            outcome: "connection_error",
            duration_ms: 40,
            attempted_at: at(s - 3 * 24 * hour),
          },
          created_at: at(s - 4 * 24 * hour),
        },
      ],
    );
    add(
      "alerts",
      {
        url: "https://alerts.example.net/hooks/mycroblog",
        description: "Reports and registrations to the moderators' chat",
        events: ["report.created", "report.resolved", "registration.pending", "appeal.created"],
        state: "enabled",
        state_reason: "admin",
        state_changed_at: at(s - 60 * 24 * hour),
        created_at: at(s - 60 * 24 * hour),
        revised_at: at(s - 20 * 24 * hour),
      },
      [
        {
          event_type: "report.created",
          status: "pending",
          attempts: 0,
          created_at: at(s - 0.1 * hour),
        },
        {
          event_type: "registration.pending",
          status: "failing",
          attempts: 3,
          last_attempt: {
            outcome: "http_error",
            status_code: 502,
            duration_ms: 812,
            attempted_at: at(s - 1 * hour),
          },
          created_at: at(s - 2 * hour),
        },
        {
          event_type: "report.resolved",
          status: "succeeded",
          attempts: 1,
          last_attempt: {
            outcome: "succeeded",
            status_code: 200,
            duration_ms: 143,
            attempted_at: at(s - 5 * hour),
          },
          created_at: at(s - 5 * hour),
        },
        {
          event_type: "webhook.test",
          status: "succeeded",
          attempts: 2,
          last_attempt: {
            outcome: "succeeded",
            status_code: 204,
            duration_ms: 98,
            attempted_at: at(s - 30 * hour),
          },
          created_at: at(s - 31 * hour),
        },
      ],
    );
    add(
      "chat",
      {
        url: "https://chat.example.org/api/incoming/a-long-path-of-the-chat-service-that-runs-past-a-phone",
        events: ["domain_moderation_action.created", "status.created"],
        state: "enabled",
        state_reason: "admin",
        state_changed_at: at(s - 10 * 24 * hour),
        failure: {
          consecutive_failures: 7,
          first_failed_at: at(s - 26 * hour),
        },
        created_at: at(s - 10 * 24 * hour),
        revised_at: at(s - 10 * 24 * hour),
      },
      [
        {
          event_type: "status.created",
          status: "failing",
          attempts: 7,
          last_attempt: {
            outcome: "timeout",
            duration_ms: 15_000,
            attempted_at: at(s - 0.5 * hour),
          },
          created_at: at(s - 26 * hour),
        },
      ],
    );
  }

  const allowed = () => !c.visitor && c.permissions().includes("manage_webhooks");
  const forbidden = () =>
    c.problem(403, "Forbidden", {
      type: "/api/v1/problems/missing-permission",
    });
  const refused = (reason: Schema<"WebhookProblemReason">, missing?: readonly Permission[]) =>
    c.problem(422, "The webhook breaks a rule", {
      type: "/api/v1/problems/invalid-webhook",
      reason,
      ...(missing === undefined ? {} : { missing_permissions: [...missing] }),
    });
  const found = (id: string) => webhooks.find((w) => w.id === id);
  /** What is wrong with a URL, a description or events, as the server checks them; none for valid ones. */
  const check = (url?: string, description?: string, events?: readonly Event[]) => {
    if (url !== undefined) {
      let parsed: URL | undefined;
      try {
        parsed = new URL(url);
      } catch {
        parsed = undefined;
      }
      if (
        parsed === undefined ||
        !["http:", "https:"].includes(parsed.protocol) ||
        parsed.username !== "" ||
        parsed.password !== "" ||
        url.includes("#") ||
        new TextEncoder().encode(url).length > 2048
      )
        return refused("url");
    }
    if (description !== undefined && (Array.from(description).length > 200 || /\p{Cc}/u.test(description)))
      return refused("description");
    if (events !== undefined) {
      if (events.length === 0 || new Set(events).size !== events.length) return refused("events");
      const mine = c.permissions();
      // As the server does, every event the webhook is to receive needs its permission, not only those that change.
      const missing = [
        ...new Set(
          events.flatMap((e) => {
            const p = needed(e);
            return p === undefined || mine.includes(p) ? [] : [p];
          }),
        ),
      ];
      if (missing.length > 0) return refused("missing_permission", missing);
    }
    return undefined;
  };
  const newDelivery = (id: string, event: Schema<"WebhookMessageType">): Delivery => {
    made += 1;
    const d: Delivery = {
      id: c.idAt(Date.now(), 83_000 + made),
      message_id: c.idAt(Date.now(), 84_000 + made),
      event_type: event,
      status: "pending",
      attempts: 0,
      created_at: at(Date.now()),
    };
    deliveries.set(id, [d, ...(deliveries.get(id) ?? [])]);
    return d;
  };

  const handlers: Handlers = {
    "GET /admin/webhooks": () =>
      allowed()
        ? {
            status: 200,
            body: { items: structuredClone(webhooks.toReversed()) },
          }
        : forbidden(),
    "POST /admin/webhooks": ({ body }) => {
      if (!allowed()) return forbidden();
      const wrong = check(body.url, body.description, body.events);
      if (wrong !== undefined) return wrong;
      made += 1;
      const now = at(Date.now());
      const w: Webhook = {
        id: c.idAt(Date.now(), 85_000 + made),
        url: body.url,
        ...(body.description === undefined || body.description === "" ? {} : { description: body.description }),
        events: [...body.events],
        state: "enabled",
        state_reason: "admin",
        state_changed_at: now,
        created_at: now,
        revised_at: now,
      };
      webhooks.push(w);
      deliveries.set(w.id, []);
      return {
        status: 201,
        body: { webhook: structuredClone(w), secret: secret() },
      };
    },
    "GET /admin/webhooks/{id}": ({ params }) => {
      if (!allowed()) return forbidden();
      const w = found(params.id);
      return w === undefined ? c.notFound() : { status: 200, body: structuredClone(w) };
    },
    "PATCH /admin/webhooks/{id}": ({ params, body }) => {
      if (!allowed()) return forbidden();
      const w = found(params.id);
      if (w === undefined) return c.notFound();
      const wrong = check(body.url, body.description, body.events ?? w.events);
      if (wrong !== undefined) return wrong;
      if (body.url !== undefined) w.url = body.url;
      if (body.description !== undefined) {
        if (body.description === "") delete w.description;
        else w.description = body.description;
      }
      if (body.events !== undefined) w.events = [...body.events];
      w.revised_at = at(Date.now());
      return { status: 200, body: structuredClone(w) };
    },
    "DELETE /admin/webhooks/{id}": ({ params }) => {
      if (!allowed()) return forbidden();
      const w = found(params.id);
      if (w === undefined) return c.notFound();
      webhooks.splice(webhooks.indexOf(w), 1);
      deliveries.delete(w.id);
      return { status: 204 };
    },
    "POST /admin/webhooks/{id}/enable": ({ params }) => {
      if (!allowed()) return forbidden();
      const w = found(params.id);
      if (w === undefined) return c.notFound();
      if (w.state !== "enabled") {
        w.state = "enabled";
        w.state_reason = "admin";
        w.state_changed_at = at(Date.now());
        delete w.failure;
      }
      return { status: 200, body: structuredClone(w) };
    },
    "POST /admin/webhooks/{id}/disable": ({ params }) => {
      if (!allowed()) return forbidden();
      const w = found(params.id);
      if (w === undefined) return c.notFound();
      if (w.state !== "disabled") {
        w.state = "disabled";
        w.state_reason = "admin";
        w.state_changed_at = at(Date.now());
      }
      return { status: 200, body: structuredClone(w) };
    },
    "POST /admin/webhooks/{id}/rotate_secret": ({ params }) => {
      if (!allowed()) return forbidden();
      return found(params.id) === undefined ? c.notFound() : { status: 200, body: { secret: secret() } };
    },
    "POST /admin/webhooks/{id}/test": ({ params }) => {
      if (!allowed()) return forbidden();
      if (found(params.id) === undefined) return c.notFound();
      return {
        status: 202,
        body: structuredClone(newDelivery(params.id, "webhook.test")),
      };
    },
    "GET /admin/webhooks/{id}/deliveries": ({ params, query }) => {
      if (!allowed()) return forbidden();
      if (found(params.id) === undefined) return c.notFound();
      return { status: 200, body: structuredClone(page(deliveries.get(params.id) ?? [], query).body) };
    },
    "POST /admin/webhooks/{id}/deliveries/{delivery_id}/redeliver": ({ params }) => {
      if (!allowed()) return forbidden();
      const d = deliveries.get(params.id)?.find((x) => x.id === params.delivery_id);
      if (d === undefined) return c.notFound();
      d.status = "pending";
      return { status: 202, body: structuredClone(d) };
    },
  };
  return { handlers, id: (key) => keyed.get(key) ?? "none" };
}
