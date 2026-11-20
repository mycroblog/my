// SPDX-License-Identifier: AGPL-3.0-only

// The fake's custom emoji as their administrators and their requesters keep them (FR-111, FR-915, ADR-0051,
// ADR-0070): under /admin/custom_emojis the scenario's local emoji with categories and aliases, three more of this
// server's (mio's `mio_heart`, one hidden from the picker, one disabled) and four of other servers' (one disabled, one
// whose image cannot be shown); under /admin/custom_emoji_requests the requests of mio, who holds `artist`
// (staff-roles.ts): two pending, one approved (`mio_heart`), one rejected with a reason, one withdrawn; and one of
// kai's, pending, whom no role lets create emoji any more, which can only be rejected. A reader holding
// `create_custom_emojis` reads their own emoji and requests under /custom_emojis/mine and /custom_emoji_requests, with
// a monthly limit of 5 unless the options say otherwise. The checks are the server's: a shortcode's rules, one a local
// emoji has or a pending request reserves, the limit, a request no longer pending. An image uploaded for an emoji
// shows as the fake's sparkle.

import { page } from "./common";
import type { StaffServerContext } from "./staff-server";
import type { Handlers, Refusal, Schema } from "./spec";

type Emoji = Schema<"AdminCustomEmoji">;
type Request = Schema<"CustomEmojiRequest">;
type Reason = Schema<"CustomEmojiProblemReason">;

/** What the stories ask of the fake's custom emoji. */
export type FakeEmojiOptions = {
  /**
   * How many requests to create an emoji the reader may submit in a month: 5 by default, `none` for no limit; the
   * requests of this month the seed gives them count against it.
   */
  readonly limit?: number | "none";
  /** There is no emoji and no request at all. */
  readonly none?: boolean;
};

const hour = 3_600_000;
const day = 24 * hour;

/** A request of the fake's, with who made it, by username. */
type Held = Request & { readonly requester: string };

/** An image of an emoji, or none. */
type Image = { url?: string; static_url?: string };

export function emojiHandlers(c: StaffServerContext): {
  handlers: Handlers;
  id: (key: string) => string;
  requestId: (key: string) => string;
} {
  const none = c.options?.emoji?.none === true;
  const limitOption = c.options?.emoji?.limit;
  const limit = limitOption === "none" ? undefined : (limitOption ?? 5);
  const at = (ms: number) => new Date(ms).toISOString();
  const imageOf = (shortcode: string): Image => {
    const e = c.localEmojis.find((x) => x.shortcode === shortcode) ?? c.localEmojis[0];
    return e === undefined ? {} : { url: e.url, static_url: e.static_url };
  };
  const keyed = new Map<string, string>();
  /** Who made each local emoji, by its ID. */
  const creators = new Map<string, string>();
  let made = 0;
  const emoji = (plan: Omit<Emoji, "id" | "created_by_deleted" | "updated_at"> & { by?: string }): Emoji => {
    made += 1;
    const id = c.idAt(Date.parse(plan.created_at), 73_000 + made);
    keyed.set(plan.shortcode, id);
    const { by, ...rest } = plan;
    if (by !== undefined) creators.set(id, by);
    return {
      ...rest,
      id,
      updated_at: plan.created_at,
      created_by_deleted: false,
      ...(by === undefined || !c.known(by) ? {} : { created_by: c.accountOf(by) }),
    };
  };
  const remote = (domain: string, shortcode: string, ago: number, more: Partial<Emoji> = {}) =>
    emoji({
      shortcode,
      local: false,
      domain,
      uri: `https://${domain}/emojis/${shortcode}`,
      state: "enabled",
      visible_in_picker: true,
      aliases: [],
      created_at: at(c.started - ago * day),
      ...more,
    });
  const s = c.started;
  const emojis: Emoji[] = none
    ? []
    : [
        emoji({
          shortcode: "mio_heart",
          local: true,
          ...imageOf("sparkle"),
          state: "enabled",
          visible_in_picker: true,
          aliases: [],
          created_at: at(s - 3 * day),
          by: "mio",
        }),
        ...c.localEmojis.map((e, i) =>
          emoji({
            shortcode: e.shortcode,
            local: true,
            url: e.url,
            static_url: e.static_url,
            state: "enabled",
            visible_in_picker: true,
            ...(e.shortcode === "wave"
              ? { category: "Greetings" }
              : e.shortcode === "teacup"
                ? { category: "Food" }
                : {}),
            aliases: e.shortcode === "wave" ? ["hello", "hi"] : e.shortcode === "teacup" ? ["tea"] : [],
            created_at: at(s - (200 - i) * day),
            by: "hana",
          }),
        ),
        emoji({
          shortcode: "staff_only",
          local: true,
          ...imageOf("teacup"),
          state: "enabled",
          visible_in_picker: false,
          category: "Staff",
          aliases: [],
          created_at: at(s - 120 * day),
          by: "hana",
        }),
        emoji({
          shortcode: "old_logo",
          local: true,
          ...imageOf("sparkle"),
          state: "disabled",
          visible_in_picker: true,
          aliases: [],
          created_at: at(s - 400 * day),
        }),
        remote("remote.example", "blobfox", 10, imageOf("wave")),
        remote("social.example.org", "ablobcat", 20, imageOf("teacup")),
        remote("deals.example", "spam_logo", 30, { ...imageOf("sparkle"), state: "disabled" }),
        remote("rejected-media.example", "hidden_media", 40),
      ];
  emojis.sort((a, b) => b.created_at.localeCompare(a.created_at));

  let requested = 0;
  const request = (key: string, requester: string, plan: Omit<Request, "id" | "kind">): Held => {
    requested += 1;
    const id = c.idAt(Date.parse(plan.created_at), 74_000 + requested);
    keyed.set(`request:${key}`, id);
    return { kind: "create", ...plan, id, requester };
  };
  const requests: Held[] = none
    ? []
    : [
        request("party", "mio", {
          state: "pending",
          shortcode: "party_cat",
          ...imageOf("sparkle"),
          created_at: at(s - 2 * hour),
        }),
        request("sakura", "mio", {
          state: "pending",
          shortcode: "sakura",
          ...imageOf("teacup"),
          created_at: at(s - 5 * hour),
        }),
        request("kai", "kai", {
          state: "pending",
          shortcode: "kai_wave",
          ...imageOf("wave"),
          created_at: at(s - 26 * hour),
        }),
        request("heart", "mio", {
          state: "approved",
          shortcode: "mio_heart",
          emoji_id: keyed.get("mio_heart") ?? "",
          ...imageOf("sparkle"),
          created_at: at(s - 4 * day),
          decided_at: at(s - 3 * day),
        }),
        request("logo", "mio", {
          state: "rejected",
          shortcode: "brand_logo",
          ...imageOf("sparkle"),
          reason: "Logos of other services are not allowed. Please draw your own.",
          created_at: at(s - 6 * day),
          decided_at: at(s - 5 * day),
        }),
        request("sketch", "mio", {
          state: "withdrawn",
          shortcode: "rough_sketch",
          created_at: at(s - 8 * day),
          decided_at: at(s - 8 * day),
        }),
      ];
  requests.sort((a, b) => b.created_at.localeCompare(a.created_at));

  /** Whether the person of `username` may create emoji now: not kai, whose role was revoked since. */
  const mayCreate = (username: string) => username !== "kai";

  const manages = () => !c.visitor && c.permissions().includes("manage_custom_emojis");
  const creates = () => !c.visitor && c.permissions().includes("create_custom_emojis");
  const forbidden = () => c.problem(403, "Forbidden", { type: "/api/v1/problems/missing-permission" });
  const refused = (reason: Reason): Refusal =>
    c.problem(422, "The custom emoji breaks a rule", { type: "/api/v1/problems/invalid-custom-emoji", reason });
  const conflict = (reason: Schema<"CustomEmojiRequestConflictReason">): Refusal =>
    c.problem(409, "The request cannot be decided", { type: "/api/v1/problems/custom-emoji-request-conflict", reason });
  const shortcodeRefusal = (shortcode: string): Refusal | undefined => {
    if (!/^[A-Za-z0-9_]{2,30}$/.test(shortcode)) return refused("invalid_shortcode");
    const taken =
      emojis.some((e) => e.local && e.shortcode === shortcode) ||
      requests.some((r) => r.state === "pending" && r.kind === "create" && r.shortcode === shortcode);
    return taken ? refused("shortcode_taken") : undefined;
  };
  const settingsRefusal = (body: { category?: string; aliases?: string[] }): Refusal | undefined => {
    if (body.category !== undefined && Array.from(body.category).length > 64) return refused("invalid_category");
    if (
      body.aliases !== undefined &&
      (body.aliases.length > 10 || body.aliases.some((a) => !/^[A-Za-z0-9_]{2,30}$/.test(a)))
    )
      return refused("invalid_alias");
    return undefined;
  };
  /** The image of an upload, as the fake shows every one. */
  const uploaded = (): Image => imageOf("sparkle");
  const imageOfRequest = (r: Held): Image => ({
    ...(r.url === undefined ? {} : { url: r.url }),
    ...(r.static_url === undefined ? {} : { static_url: r.static_url }),
  });
  const asAdmin = (r: Held): Schema<"AdminCustomEmojiRequest"> => {
    const { requester, ...rest } = r;
    return { ...structuredClone(rest), requester: c.accountOf(requester) };
  };
  const asOwn = (r: Held): Request => {
    const { requester: _requester, ...rest } = r;
    return structuredClone(rest);
  };
  const asOwnEmoji = (e: Emoji): Schema<"OwnCustomEmoji"> => ({
    id: e.id,
    shortcode: e.shortcode,
    ...(e.url === undefined ? {} : { url: e.url }),
    ...(e.static_url === undefined ? {} : { static_url: e.static_url }),
    state: e.state,
    visible_in_picker: e.visible_in_picker,
    ...(e.category === undefined ? {} : { category: e.category }),
    aliases: [...e.aliases],
    created_at: e.created_at,
    updated_at: e.updated_at,
  });
  const found = (id: string) => emojis.find((e) => e.id === id);
  const thisMonth = () => {
    const now = new Date();
    return Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1);
  };
  const used = () =>
    requests.filter(
      (r) =>
        r.requester === c.viewer &&
        r.kind === "create" &&
        (r.state === "approved" || r.state === "pending") &&
        Date.parse(r.created_at) >= thisMonth(),
    );
  const changeSettings = (e: Emoji, body: { category?: string; aliases?: string[]; visible_in_picker?: boolean }) => {
    if (body.category !== undefined) {
      const category = body.category.trim();
      if (category === "") delete e.category;
      else e.category = category;
    }
    if (body.aliases !== undefined) e.aliases = [...body.aliases];
    if (body.visible_in_picker !== undefined) e.visible_in_picker = body.visible_in_picker;
  };
  const make = (shortcode: string, image: Image, by: string, more: Partial<Emoji> = {}): Emoji => {
    made += 1;
    const now = Date.now();
    const e: Emoji = {
      id: c.idAt(now, 73_000 + made),
      shortcode,
      local: true,
      ...image,
      state: "enabled",
      visible_in_picker: true,
      aliases: [],
      created_by: c.accountOf(by),
      created_by_deleted: false,
      created_at: at(now),
      updated_at: at(now),
      ...more,
    };
    creators.set(e.id, by);
    emojis.unshift(e);
    return e;
  };
  /** The emoji of `id`, changed by `change` as the reader may; a refusal where `change` gives one. */
  const changed = (id: string, change: (e: Emoji) => Refusal | undefined) => {
    if (!manages()) return forbidden();
    const e = found(id);
    if (e === undefined) return c.notFound();
    return change(e) ?? { status: 200 as const, body: structuredClone(e) };
  };

  const handlers: Handlers = {
    "GET /admin/custom_emojis": ({ query }) => {
      if (!manages()) return forbidden();
      const localOnly = query.get("local");
      const domain = query.get("domain");
      const shortcode = query.get("shortcode")?.toLowerCase();
      const kept = emojis.filter(
        (e) =>
          (localOnly === null || e.local === (localOnly === "true")) &&
          (domain === null || e.domain === domain) &&
          (shortcode === undefined || e.shortcode.toLowerCase().includes(shortcode)),
      );
      return { status: 200, body: page(structuredClone(kept), query).body };
    },
    "POST /admin/custom_emojis": ({ body }) => {
      if (!manages()) return forbidden();
      const refusal = shortcodeRefusal(body.shortcode) ?? settingsRefusal(body);
      if (refusal !== undefined) return refusal;
      const e = make(body.shortcode, uploaded(), c.viewer, {
        visible_in_picker: body.visible_in_picker,
        aliases: [...(body.aliases ?? [])],
        ...(body.category === undefined || body.category.trim() === "" ? {} : { category: body.category.trim() }),
      });
      return { status: 201, body: structuredClone(e) };
    },
    "GET /admin/custom_emojis/{id}": ({ params }) => changed(params.id, () => undefined),
    "PATCH /admin/custom_emojis/{id}": ({ params, body }) =>
      changed(params.id, (e) => {
        if (!e.local) return refused("remote_emoji");
        const refusal = settingsRefusal(body);
        if (refusal !== undefined) return refusal;
        changeSettings(e, body);
        return undefined;
      }),
    "DELETE /admin/custom_emojis/{id}": ({ params }) => {
      if (!manages()) return forbidden();
      const e = found(params.id);
      if (e === undefined) return c.notFound();
      if (!e.local) return refused("remote_emoji");
      emojis.splice(emojis.indexOf(e), 1);
      return { status: 204 };
    },
    "PUT /admin/custom_emojis/{id}/image": ({ params }) =>
      changed(params.id, (e) => {
        if (!e.local) return refused("remote_emoji");
        Object.assign(e, uploaded(), { updated_at: at(Date.now()) });
        return undefined;
      }),
    "POST /admin/custom_emojis/{id}/enable": ({ params }) =>
      changed(params.id, (e) => {
        e.state = "enabled";
        return undefined;
      }),
    "POST /admin/custom_emojis/{id}/disable": ({ params }) =>
      changed(params.id, (e) => {
        e.state = "disabled";
        return undefined;
      }),
    "POST /admin/custom_emojis/{id}/copy": ({ params, body }) => {
      if (!manages()) return forbidden();
      const e = found(params.id);
      if (e === undefined) return c.notFound();
      if (e.local) return refused("local_emoji");
      if (e.url === undefined) return refused("fetch_failed");
      const shortcode = body?.shortcode ?? e.shortcode;
      const refusal = shortcodeRefusal(shortcode);
      if (refusal !== undefined) return refusal;
      const image: Image = { url: e.url, ...(e.static_url === undefined ? {} : { static_url: e.static_url }) };
      return { status: 201, body: structuredClone(make(shortcode, image, c.viewer)) };
    },
    "GET /admin/custom_emoji_requests": ({ query }) => {
      if (!manages()) return forbidden();
      const state = query.get("state") ?? "pending";
      return { status: 200, body: page(requests.filter((r) => r.state === state).map(asAdmin), query).body };
    },
    "POST /admin/custom_emoji_requests/{id}/approve": ({ params }) => {
      if (!manages()) return forbidden();
      const r = requests.find((x) => x.id === params.id);
      if (r === undefined) return c.notFound();
      if (r.state !== "pending") return conflict("not_pending");
      if (!mayCreate(r.requester)) return conflict("requester_cannot_create");
      if (emojis.some((e) => e.local && e.shortcode === r.shortcode)) return conflict("shortcode_taken");
      const e = make(r.shortcode, imageOfRequest(r), r.requester);
      Object.assign(r, { state: "approved", emoji_id: e.id, decided_at: at(Date.now()) });
      return { status: 200, body: asAdmin(r) };
    },
    "POST /admin/custom_emoji_requests/{id}/reject": ({ params, body }) => {
      if (!manages()) return forbidden();
      const r = requests.find((x) => x.id === params.id);
      if (r === undefined) return c.notFound();
      if ((body?.reason ?? "").length > 500) return refused("invalid_reason");
      if (r.state !== "pending") return conflict("not_pending");
      Object.assign(r, {
        state: "rejected",
        decided_at: at(Date.now()),
        ...(body?.reason === undefined || body.reason === "" ? {} : { reason: body.reason }),
      });
      return { status: 200, body: asAdmin(r) };
    },
    "GET /custom_emoji_requests": ({ query }) => {
      if (!creates()) return forbidden();
      return { status: 200, body: page(requests.filter((r) => r.requester === c.viewer).map(asOwn), query).body };
    },
    "POST /custom_emoji_requests": ({ body }) => {
      if (!creates()) return forbidden();
      if (body.kind !== "create")
        return c.problem(422, "Unprocessable Content", { detail: "Name the emoji.", field: "emoji_id" });
      if (body.shortcode === undefined)
        return c.problem(422, "Unprocessable Content", { detail: "Name the emoji.", field: "shortcode" });
      const refusal = shortcodeRefusal(body.shortcode);
      if (refusal !== undefined) return refusal;
      if (limit !== undefined && used().length >= limit) return refused("limit_reached");
      const r = request(`new-${String(requested)}`, c.viewer, {
        state: "pending",
        shortcode: body.shortcode,
        ...uploaded(),
        created_at: at(Date.now()),
      });
      requests.unshift(r);
      return { status: 201, body: asOwn(r) };
    },
    "POST /custom_emoji_requests/{id}/withdraw": ({ params }) => {
      if (!creates()) return forbidden();
      const r = requests.find((x) => x.id === params.id && x.requester === c.viewer);
      if (r === undefined) return c.notFound();
      if (r.state !== "pending") return conflict("not_pending");
      Object.assign(r, { state: "withdrawn", decided_at: at(Date.now()) });
      return { status: 200, body: asOwn(r) };
    },
    "GET /custom_emojis/mine": ({ query }) => {
      if (!creates()) return forbidden();
      const mine = emojis.filter((e) => creators.get(e.id) === c.viewer).map(asOwnEmoji);
      const now = new Date();
      return {
        status: 200,
        body: {
          ...page(mine, query).body,
          used: used().length,
          pending: used().filter((r) => r.state === "pending").length,
          ...(limit === undefined ? {} : { limit }),
          resets_at: at(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + 1, 1)),
        },
      };
    },
    "PATCH /custom_emojis/mine/{id}": ({ params, body }) => {
      if (!creates()) return forbidden();
      const e = found(params.id);
      if (e === undefined || creators.get(e.id) !== c.viewer) return c.notFound();
      const refusal = settingsRefusal(body);
      if (refusal !== undefined) return refusal;
      changeSettings(e, body);
      return { status: 200, body: asOwnEmoji(e) };
    },
    "DELETE /custom_emojis/mine/{id}": ({ params }) => {
      if (!creates()) return forbidden();
      const e = found(params.id);
      if (e === undefined || creators.get(e.id) !== c.viewer) return c.notFound();
      emojis.splice(emojis.indexOf(e), 1);
      return { status: 204 };
    },
  };
  return {
    handlers,
    id: (key) => keyed.get(key) ?? "none",
    requestId: (key) => keyed.get(`request:${key}`) ?? "none",
  };
}
