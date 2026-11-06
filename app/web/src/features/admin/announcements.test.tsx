// SPDX-License-Identifier: AGPL-3.0-only
// The announcements screen through the whole app (web-client/README.md T4, FR-726, ADR-0125): an administrator with
// manage_announcements pages through the announcements and keeps one state in the URL, writes an announcement with a
// title shown for a chosen time on its own page, and on the page of one publishes a draft, edits its title, and
// withdraws it after an alert dialog; a viewer without the permission sees why; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { adminAnnouncementFixture } from "./test/handlers";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Announcement = Schemas["AdminAnnouncement"];

const createdId = "0199f0e0-0000-7000-8000-00000000c009";

/**
 * Plays the announcements of the server: GET lists them a page at a time (newest first, by `limit` and `cursor`, the
 * ID of the last one) or returns one, and each write changes them; records the list requests and the writes.
 */
function announcementsServer(initial: readonly Announcement[]) {
  let announcements = [...initial];
  const lists: string[] = [];
  const writes: { method: string; path: string; body: unknown }[] = [];
  const record = async (request: Request) => {
    const text = await request.clone().text();
    writes.push({
      method: request.method,
      path: new URL(request.url).pathname.replace("/api/v1", ""),
      body: text === "" ? undefined : JSON.parse(text),
    });
  };
  const change = (id: unknown, edit: Partial<Announcement>) => {
    announcements = announcements.map((item) => (item.id === id ? { ...item, ...edit } : item));
    return HttpResponse.json(announcements.find((item) => item.id === id));
  };
  server.use(
    http.get(`${apiBase}/admin/announcements`, ({ request }) => {
      const url = new URL(request.url);
      lists.push(`${url.pathname.replace("/api/v1", "")}${url.search}`);
      const state = url.searchParams.get("state");
      const cursor = url.searchParams.get("cursor");
      const limit = Number(url.searchParams.get("limit") ?? "20");
      const kept = announcements.filter((item) => state === null || item.state === state);
      const start = cursor === null ? 0 : kept.findIndex((item) => item.id === cursor) + 1;
      const items = kept.slice(start, start + limit);
      const last = items.at(-1);
      return HttpResponse.json({
        items,
        ...(start + limit < kept.length && last !== undefined ? { next_cursor: last.id } : {}),
      } satisfies Schemas["AdminAnnouncementPage"]);
    }),
    http.get(`${apiBase}/admin/announcements/:id`, ({ params }) => {
      const found = announcements.find((item) => item.id === params["id"]);
      return found === undefined
        ? HttpResponse.json({ type: "about:blank", title: "Not Found", status: 404 }, { status: 404 })
        : HttpResponse.json(found);
    }),
    http.post(`${apiBase}/admin/announcements`, async ({ request }) => {
      await record(request);
      const { publish, title, text, ...window } = (await request.json()) as Schemas["AdminAnnouncementCreateRequest"];
      const created = adminAnnouncementFixture(createdId, {
        title,
        text,
        content: `<p>${text}</p>`,
        state: publish ? "published" : "draft",
        ...(publish ? { published_at: new Date().toISOString() } : {}),
        ...window,
      });
      announcements = [created, ...announcements];
      return HttpResponse.json(created, { status: 201 });
    }),
    http.put(`${apiBase}/admin/announcements/:id`, async ({ request, params }) => {
      await record(request);
      const body = (await request.json()) as Schemas["AdminAnnouncementInput"];
      return change(params["id"], { title: body.title, text: body.text, content: `<p>${body.text}</p>` });
    }),
    http.post(`${apiBase}/admin/announcements/:id/publish`, async ({ request, params }) => {
      await record(request);
      return change(params["id"], { state: "published", published_at: "2026-10-21T01:00:00Z" });
    }),
    http.post(`${apiBase}/admin/announcements/:id/withdraw`, async ({ request, params }) => {
      await record(request);
      return change(params["id"], { state: "withdrawn" });
    }),
  );
  return { lists, writes };
}

const draft = adminAnnouncementFixture("0199f0e0-0000-7000-8000-00000000c001", {
  title: "Maintenance",
  text: "# Maintenance\n\nOn **Sunday**.",
  content: "<h1>Maintenance</h1><p>On <strong>Sunday</strong>.</p>",
});
const ended = adminAnnouncementFixture("0199f0e0-0000-7000-8000-00000000c002", {
  title: "Welcome!",
  text: "Welcome to the server.",
  content: "<p>Welcome to the server.</p>",
  state: "published",
  published_at: "2026-10-20T00:00:00Z",
  ends_at: "2026-10-27T00:00:00Z",
});

/** The rows of the table of announcements, without its header row. */
function rows(): HTMLElement[] {
  return within(screen.getByRole("table")).queryAllByRole("row").slice(1);
}

function row(index: number) {
  const found = rows()[index];
  if (found === undefined) throw new Error(`no row ${String(index)}`);
  return within(found);
}

/** The pagination under the table. */
function pagination() {
  return screen.getByRole("navigation", { name: "Pages" });
}

describe("the list of announcements", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_announcements"]));
  });

  test("lists the announcements by title, state and when they show, without their text", async () => {
    announcementsServer([draft, ended]);
    await renderApp({ path: "/admin/announcements" });

    expect(await screen.findByRole("heading", { level: 1, name: "Announcements" })).toBeInTheDocument();
    expect(await screen.findByRole("rowheader", { name: "Maintenance" })).toBeInTheDocument();
    expect(rows().map((item) => within(item).getByRole("rowheader").textContent)).toEqual(["Maintenance", "Welcome!"]);
    expect(row(0).getByRole("link", { name: "Maintenance" })).toHaveAttribute(
      "href",
      `/admin/announcements/${draft.id}`,
    );
    expect(row(0).getByText("Draft")).toBeInTheDocument();
    expect(row(0).getByText("From when it is published")).toBeInTheDocument();
    expect(row(1).getByText("Ended")).toBeInTheDocument();
    expect(row(1).getByText("Until Oct 27, 2026, 12:00 AM")).toBeInTheDocument();
    // The text is on the announcement's page only.
    expect(screen.queryByText("Sunday")).not.toBeInTheDocument();
    expect(screen.queryByText("Welcome to the server.")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Write an announcement" })).toHaveAttribute(
      "href",
      "/admin/announcements/new",
    );
  });

  test("pages by previous and next, a page of 40 at a time", async () => {
    const many = Array.from({ length: 45 }, (_, index) =>
      adminAnnouncementFixture(`0199f0e0-0000-7000-8000-${String(100 - index).padStart(12, "0")}`, {
        title: `Notice ${String(index + 1)}`,
      }),
    );
    const announcements = announcementsServer(many);
    const { user } = await renderApp({ path: "/admin/announcements" });

    expect(await screen.findByRole("rowheader", { name: "Notice 1" })).toBeInTheDocument();
    expect(rows()).toHaveLength(40);
    expect(within(pagination()).getByRole("button", { name: "Previous" })).toBeDisabled();
    await user.click(within(pagination()).getByRole("button", { name: "Next" }));
    expect(await screen.findByRole("rowheader", { name: "Notice 41" })).toBeInTheDocument();
    expect(rows()).toHaveLength(5);
    expect(within(pagination()).getByRole("button", { name: "Next" })).toBeDisabled();
    await user.click(within(pagination()).getByRole("button", { name: "Previous" }));
    expect(await screen.findByRole("rowheader", { name: "Notice 1" })).toBeInTheDocument();
    expect(announcements.lists).toEqual([
      "/admin/announcements?limit=40",
      `/admin/announcements?limit=40&cursor=${many[39]?.id ?? ""}`,
    ]);
  });

  test("keeps the state of the list in the URL", async () => {
    const announcements = announcementsServer([draft, ended]);
    const { user, router } = await renderApp({ path: "/admin/announcements" });

    await screen.findByRole("rowheader", { name: "Maintenance" });
    await user.click(screen.getByRole("button", { name: /Announcements by state/u }));
    await user.click(await screen.findByRole("option", { name: "Drafts" }));

    await waitFor(() => {
      expect(router.state.location.search).toEqual({ state: "draft" });
    });
    await waitFor(() => {
      expect(rows()).toHaveLength(1);
    });
    expect(announcements.lists).toContain("/admin/announcements?limit=40&state=draft");
  });

  test("says when a list has no announcements", async () => {
    announcementsServer([]);
    await renderApp({ path: "/admin/announcements?state=withdrawn" });

    expect(await screen.findByRole("heading", { name: "No announcements in this list" })).toBeInTheDocument();
  });
});

describe("the page that writes an announcement", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_announcements"]));
  });

  test("writes an announcement that shows for a week, publishes it at once and opens its page", async () => {
    const announcements = announcementsServer([]);
    const { user, router } = await renderApp({ path: "/admin/announcements/new" });

    expect(await screen.findByRole("heading", { level: 1, name: "Write an announcement" })).toBeInTheDocument();
    // Without a title and a text, the form says what is missing and sends nothing.
    await user.click(screen.getByRole("button", { name: "Publish the announcement" }));
    expect(await screen.findByText("Give the announcement a title.")).toBeInTheDocument();
    expect(screen.getByText("Write the announcement.")).toBeInTheDocument();

    const title = screen.getByRole("textbox", { name: "Title" });
    expect(title).toHaveAccessibleDescription(/100 characters left/u);
    await user.type(title, "Maintenance tonight");
    expect(title).toHaveAccessibleDescription(/81 characters left/u);
    await user.type(screen.getByRole("textbox", { name: "Text" }), "The server is read-only from 02:00.");
    await user.click(screen.getByRole("button", { name: "Publish the announcement" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Announcement" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe(`/admin/announcements/${createdId}`);
    expect(announcements.writes).toHaveLength(1);
    const body = announcements.writes[0]?.body as Schemas["AdminAnnouncementCreateRequest"];
    expect(body.title).toBe("Maintenance tonight");
    expect(body.text).toBe("The server is read-only from 02:00.");
    expect(body.publish).toBe(true);
    expect(body.ends_at).toBeDefined();
    expect(Date.parse(body.ends_at ?? "") - Date.now()).toBeGreaterThan(6 * 86_400_000);
    expect(within(await screen.findByRole("region", { name: "State" })).getByText("Showing")).toBeInTheDocument();
  });

  test("keeps a new announcement as a draft", async () => {
    const announcements = announcementsServer([]);
    const { user } = await renderApp({ path: "/admin/announcements/new" });

    await user.type(await screen.findByRole("textbox", { name: "Title" }), "Draft");
    await user.type(screen.getByRole("textbox", { name: "Text" }), "Draft");
    await user.click(screen.getByRole("checkbox", { name: /Publish now/u }));
    await user.click(screen.getByRole("button", { name: "Save as a draft" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Announcement" })).toBeInTheDocument();
    expect(announcements.writes[0]?.body).toMatchObject({ title: "Draft", text: "Draft", publish: false });
    expect(await screen.findByRole("button", { name: "Publish" })).toBeInTheDocument();
  });

  test("says at once that a title is too long, and sends nothing", async () => {
    const announcements = announcementsServer([]);
    const { user } = await renderApp({ path: "/admin/announcements/new" });

    await user.click(await screen.findByRole("textbox", { name: "Title" }));
    await user.paste("x".repeat(101));
    expect(await screen.findByText("Use at most 100 characters.")).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Text" }), "Text");
    await user.click(screen.getByRole("button", { name: "Publish the announcement" }));
    expect(announcements.writes).toHaveLength(0);
  });
});

describe("the page of an announcement", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_announcements"]));
  });

  test("shows the announcement as the users see it, with its facts", async () => {
    announcementsServer([ended]);
    await renderApp({ path: `/admin/announcements/${ended.id}` });

    const preview = await screen.findByRole("region", { name: "As everyone sees it" });
    expect(within(preview).getByRole("heading", { level: 3, name: "Welcome!" })).toBeInTheDocument();
    expect(within(preview).getByText("Welcome to the server.")).toBeInTheDocument();
    const state = screen.getByRole("region", { name: "State" });
    expect(within(state).getByText("Ended")).toBeInTheDocument();
    expect(within(state).queryByRole("button", { name: "Publish" })).not.toBeInTheDocument();
  });

  test("publishes a draft, edits it, and withdraws it after confirming", async () => {
    const announcements = announcementsServer([draft]);
    const { user } = await renderApp({ path: `/admin/announcements/${draft.id}` });
    const state = await screen.findByRole("region", { name: "State" });

    await user.click(within(state).getByRole("button", { name: "Publish" }));
    expect(
      await within(state).findByText("Published Maintenance. The audit log records the change."),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(within(state).getByRole("heading", { level: 2, name: "State" })).toHaveFocus();
    });
    expect(within(state).queryByRole("button", { name: "Publish" })).not.toBeInTheDocument();

    const edit = screen.getByRole("region", { name: "Edit the announcement" });
    expect(edit).toHaveAccessibleDescription("Changing a published announcement does not notify anyone again.");
    const field = within(edit).getByRole("textbox", { name: "Title" });
    await user.clear(field);
    await user.type(field, "Maintenance moved to Monday");
    await user.click(within(edit).getByRole("button", { name: "Save" }));
    expect(
      await within(edit).findByText("Changed Maintenance moved to Monday. The audit log records the change."),
    ).toBeInTheDocument();

    const withdraw = screen.getByRole("region", { name: "Withdraw the announcement" });
    await user.click(within(withdraw).getByRole("button", { name: "Withdraw" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Withdraw Maintenance moved to Monday?" });
    await user.click(within(confirm).getByRole("button", { name: "Withdraw" }));
    expect(
      await within(state).findByText("Withdrew Maintenance moved to Monday. The audit log records the change."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Withdraw the announcement" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Edit the announcement" })).not.toBeInTheDocument();
    expect(
      within(state).getByText("A withdrawn announcement can no longer be published or changed."),
    ).toBeInTheDocument();

    expect(announcements.writes[1]?.body).toMatchObject({ title: "Maintenance moved to Monday", text: draft.text });
    expect(announcements.writes.map((write) => `${write.method} ${write.path}`)).toEqual([
      `POST /admin/announcements/${draft.id}/publish`,
      `PUT /admin/announcements/${draft.id}`,
      `POST /admin/announcements/${draft.id}/withdraw`,
    ]);
  });
});

describe("the announcements screen without the permission", () => {
  test("says why it shows nothing", async () => {
    server.use(signedInHandler, operatorHandler(["manage_rules"]));
    await renderApp({ path: "/admin/announcements" });

    expect(await screen.findByRole("heading", { level: 1, name: "Announcements" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Write an announcement" })).not.toBeInTheDocument();
  });
});
