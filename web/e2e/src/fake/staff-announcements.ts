// SPDX-License-Identifier: AGPL-3.0-only

// The fake's announcements as their administrators write them (FR-726, ADR-0125), under /admin/announcements: one in
// each state the list filters by (a draft, one published to start in a few days, one showing now, one whose window
// ended, one withdrawn), newest first, and the writes, checked as the server checks them (a title of 1 to 100
// graphemes on one line, a text of 1 to 5,000 characters, a window that ends after it starts, nothing changed once
// withdrawn, only a draft deleted). What the readers see of them (`shown`) is only what the administration published while the fake ran, so
// that the seed's announcements stay the readers' in every other story and test: those published and in their window,
// newest first.

import { day, idAt, page } from "./common";
import { serverHtml } from "./html";
import type { StaffServerContext } from "./staff-server";
import type { Answer, Handlers, Schema } from "./spec";

type Announcement = Schema<"AdminAnnouncement">;

/** What the stories ask of the fake's announcements: none at the start (`none`), or the fake's own by default. */
export type FakeAnnouncementOptions = { readonly start?: "none" };

/** An announcement of the fake as it starts, by key: its title, its text, its state and its window in days from now. */
type Plan = {
  readonly key: string;
  readonly title: string;
  readonly text: string;
  readonly state: Announcement["state"];
  readonly startsIn?: number;
  readonly endsIn?: number;
  /** How many days ago it was written. */
  readonly ago: number;
};

const plans: readonly Plan[] = [
  {
    key: "draft",
    title: "Moderation report for the autumn",
    text: "Here is what the moderators handled this season.\n\n42 reports, 3 suspended accounts.",
    state: "draft",
    ago: 1,
  },
  {
    key: "scheduled",
    title: "Planned maintenance on Saturday night",
    text: "The server will be down for about 30 minutes while the database is upgraded.",
    state: "published",
    startsIn: 3,
    endsIn: 7,
    ago: 2,
  },
  {
    key: "showing",
    title: "Welcome to the new web client",
    text: "The web client was rebuilt. Tell us what you think under #feedback.",
    state: "published",
    ago: 5,
  },
  {
    key: "ended",
    title: "Photo contest: send your entries by Friday",
    text: "The theme is autumn leaves. One photo a person.",
    state: "published",
    startsIn: -30,
    endsIn: -20,
    ago: 31,
  },
  {
    key: "withdrawn",
    title: "Server move tonight",
    text: "Posted by mistake: the move is next month.",
    state: "withdrawn",
    ago: 40,
  },
];

/** How many user-perceived characters `s` holds, as the server counts a title. */
const graphemes = (s: string) => [...new Intl.Segmenter("en", { granularity: "grapheme" }).segment(s)].length;

const iso = (ms: number) => new Date(ms).toISOString();

export function announcementHandlers(c: StaffServerContext): {
  handlers: Handlers;
  id: (key: string) => string;
  shown: () => Schema<"Announcement">[];
} {
  const keyed = new Map<string, string>();
  const list: Announcement[] = (c.options?.announcements?.start === "none" ? [] : plans).map((p, i) => {
    const at = c.started - p.ago * day;
    const id = idAt(at, 22_000 + i);
    keyed.set(p.key, id);
    return {
      id,
      title: p.title,
      text: p.text,
      content: serverHtml(p.text, c.host),
      state: p.state,
      ...(p.startsIn === undefined ? {} : { starts_at: iso(c.started + p.startsIn * day) }),
      ...(p.endsIn === undefined ? {} : { ends_at: iso(c.started + p.endsIn * day) }),
      ...(p.state === "draft" ? {} : { published_at: iso(at) }),
      created_at: iso(at),
      revised_at: iso(at),
    };
  });
  /** The IDs of the announcements the administration published while the fake ran, which the readers see. */
  const published = new Set<string>();
  let made = 0;

  const allowed = () => !c.visitor && c.permissions().includes("manage_announcements");
  const forbidden = () => c.problem(403, "Forbidden", { type: "/api/v1/problems/missing-permission" });
  const found = (id: string) => list.find((a) => a.id === id);
  const standing = (a: Announcement) => {
    if (a.state !== "published") return a.state;
    if (a.ends_at !== undefined && Date.parse(a.ends_at) <= Date.now()) return "ended";
    if (a.starts_at !== undefined && Date.parse(a.starts_at) > Date.now()) return "scheduled";
    return "showing";
  };
  /** What is wrong with an announcement's input, as the server says it; undefined for none. */
  const refusal = (input: Schema<"AdminAnnouncementInput">) => {
    const title = input.title.trim();
    if (title === "" || graphemes(title) > 100 || /[\n\r]/.test(title))
      return c.problem(422, "Unprocessable Content", {
        detail: "Write a title of 1 to 100 characters on one line.",
        field: "title",
      });
    if (input.text.trim() === "" || Array.from(input.text).length > 5000)
      return c.problem(422, "Unprocessable Content", {
        detail: "Write a text of 1 to 5,000 characters.",
        field: "text",
      });
    if (
      input.starts_at !== undefined &&
      input.ends_at !== undefined &&
      Date.parse(input.ends_at) <= Date.parse(input.starts_at)
    )
      return c.problem(422, "Unprocessable Content", { detail: "End it after it starts.", field: "ends_at" });
    return undefined;
  };
  /** `a` with the input's title, text and window, as the server keeps them. */
  const revised = (a: Announcement, input: Schema<"AdminAnnouncementInput">): Announcement => {
    const { starts_at: _starts, ends_at: _ends, ...rest } = a;
    return {
      ...rest,
      title: input.title.trim(),
      text: input.text,
      content: serverHtml(input.text, c.host),
      ...(input.starts_at === undefined ? {} : { starts_at: iso(Date.parse(input.starts_at)) }),
      ...(input.ends_at === undefined ? {} : { ends_at: iso(Date.parse(input.ends_at)) }),
      revised_at: iso(Date.now()),
    };
  };
  /** Answers with what `change` makes of the announcement `id`, kept, or with the refusal it returns. */
  const changed = (
    id: string,
    change: (a: Announcement) => Announcement | Answer<"POST /admin/announcements/{id}/publish">,
  ): Answer<"POST /admin/announcements/{id}/publish"> => {
    if (!allowed()) return forbidden();
    const a = found(id);
    if (a === undefined) return c.notFound();
    const next = change(a);
    if ("problem" in next) return next;
    if (!("id" in next)) return next;
    list.splice(list.indexOf(a), 1, next);
    return { status: 200, body: structuredClone(next) };
  };
  const withdrawnAlready = () => c.problem(409, "Conflict", { detail: "The announcement is withdrawn." });

  const handlers: Handlers = {
    "GET /admin/announcements": ({ query }) => {
      if (!allowed()) return forbidden();
      const state = query.get("state");
      if (state !== null && !["draft", "scheduled", "showing", "ended", "withdrawn"].includes(state))
        return c.problem(400, "Bad Request", { detail: "state is not a state of an announcement.", field: "state" });
      return {
        status: 200,
        body: page(
          list.filter((a) => state === null || standing(a) === state),
          query,
        ).body,
      };
    },
    "POST /admin/announcements": ({ body }) => {
      if (!allowed()) return forbidden();
      const refused = refusal(body);
      if (refused !== undefined) return refused;
      made += 1;
      const at = Date.now();
      const publish = body.publish;
      const a = revised(
        {
          id: idAt(at, 23_000 + made),
          title: "",
          text: "",
          content: "",
          state: publish ? "published" : "draft",
          created_at: iso(at),
          revised_at: iso(at),
          ...(publish ? { published_at: iso(at) } : {}),
        },
        body,
      );
      list.unshift(a);
      if (publish) published.add(a.id);
      return { status: 201, body: structuredClone(a) };
    },
    "GET /admin/announcements/{id}": ({ params }) => {
      if (!allowed()) return forbidden();
      const a = found(params.id);
      return a === undefined ? c.notFound() : { status: 200, body: structuredClone(a) };
    },
    "PUT /admin/announcements/{id}": ({ params, body }) =>
      changed(params.id, (a) => (a.state === "withdrawn" ? withdrawnAlready() : (refusal(body) ?? revised(a, body)))),
    "POST /admin/announcements/{id}/publish": ({ params }) =>
      changed(params.id, (a) => {
        if (a.state === "withdrawn") return withdrawnAlready();
        if (a.state === "published") return a;
        published.add(a.id);
        return { ...a, state: "published", published_at: iso(Date.now()) };
      }),
    "POST /admin/announcements/{id}/withdraw": ({ params }) =>
      changed(params.id, (a) => ({ ...a, state: "withdrawn" })),
    "DELETE /admin/announcements/{id}": ({ params }) => {
      if (!allowed()) return forbidden();
      const a = found(params.id);
      if (a === undefined) return c.notFound();
      if (a.state !== "draft") return c.problem(409, "Conflict");
      list.splice(list.indexOf(a), 1);
      return { status: 204 };
    },
  };

  return {
    handlers,
    id: (key) => keyed.get(key) ?? "none",
    shown: () =>
      list
        .filter((a) => published.has(a.id) && standing(a) === "showing")
        .map((a) => ({
          id: a.id,
          title: a.title,
          content: a.content,
          published_at: a.published_at ?? a.created_at,
          ...(a.ends_at === undefined ? {} : { ends_at: a.ends_at }),
          revised_at: a.revised_at,
          read: false,
          ended: false,
        })),
  };
}
