// SPDX-License-Identifier: AGPL-3.0-only
// The announcements screen through the whole app (web-client/README.md T4, FR-726, ADR-0125): an administrator with
// manage_announcements writes an announcement with a title shown for a chosen time, publishes a draft, edits its title
// in a dialog, and withdraws one after an alert dialog; a viewer without the permission sees why; with MSW playing the API.
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

/** Plays the announcements of the server: GET lists them, and each write changes them; records the writes. */
function announcementsServer(initial: readonly Announcement[]) {
  let announcements = [...initial];
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
    http.get(`${apiBase}/admin/announcements`, () => HttpResponse.json({ items: announcements })),
    http.post(`${apiBase}/admin/announcements`, async ({ request }) => {
      await record(request);
      const { publish, title, text, ...window } = (await request.json()) as Schemas["AdminAnnouncementCreateRequest"];
      const created = adminAnnouncementFixture("0199f0e0-0000-7000-8000-00000000c009", {
        title,
        text,
        content: `<p>${text}</p>`,
        state: publish ? "published" : "draft",
        ...window,
      });
      announcements = [created, ...announcements];
      return HttpResponse.json(created);
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
  return { writes };
}

const draft = adminAnnouncementFixture("0199f0e0-0000-7000-8000-00000000c001", {
  title: "Maintenance",
  text: "# Maintenance\n\nOn **Sunday**.",
});
const published = adminAnnouncementFixture("0199f0e0-0000-7000-8000-00000000c002", {
  title: "Welcome!",
  text: "Welcome to the server.",
  content: "<p>Welcome!</p>",
  state: "published",
  published_at: "2026-10-20T00:00:00Z",
  ends_at: "2026-10-27T00:00:00Z",
});

/** U+2068 FIRST STRONG ISOLATE … U+2069 POP DIRECTIONAL ISOLATE: an announcement inside a heading. */
function isolated(text: string): string {
  return `⁨${text}⁩`;
}

describe("the announcements screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_announcements"]));
  });

  test("lists every announcement with its state and the text as the users see it", async () => {
    announcementsServer([draft, published]);
    await renderApp({ path: "/admin/announcements" });

    expect(await screen.findByRole("heading", { level: 1, name: "Announcements" })).toBeInTheDocument();
    const list = screen.getByRole("region", { name: "All announcements" });
    const headings = within(list).getAllByRole("heading", { level: 3 });
    expect(headings.map((heading) => heading.textContent)).toEqual([
      `${isolated("Maintenance")} (Draft)`,
      `${isolated("Welcome!")} (Published)`,
    ]);
    expect(within(list).getByText("Sunday")).toBeInTheDocument();
    expect(within(list).getByRole("button", { name: "Publish: Maintenance" })).toBeInTheDocument();
    expect(within(list).queryByRole("button", { name: "Publish: Welcome!" })).not.toBeInTheDocument();
  });

  test("writes an announcement that shows for a week and publishes it at once", async () => {
    const announcements = announcementsServer([]);
    const { user } = await renderApp({ path: "/admin/announcements" });
    const form = await screen.findByRole("region", { name: "Write an announcement" });

    // Without a title and a text, the form says what is missing and sends nothing.
    await user.click(within(form).getByRole("button", { name: "Publish the announcement" }));
    expect(await within(form).findByText("Give the announcement a title.")).toBeInTheDocument();
    expect(within(form).getByText("Write the announcement.")).toBeInTheDocument();

    const title = within(form).getByRole("textbox", { name: "Title" });
    expect(title).toHaveAccessibleDescription(/100 characters left/u);
    await user.type(title, "Maintenance tonight");
    expect(title).toHaveAccessibleDescription(/81 characters left/u);
    await user.type(within(form).getByRole("textbox", { name: "Text" }), "The server is read-only from 02:00.");
    await user.click(within(form).getByRole("button", { name: "Publish the announcement" }));

    expect(
      await within(form).findByText("Published Maintenance tonight. The audit log records the change."),
    ).toBeInTheDocument();
    expect(announcements.writes).toHaveLength(1);
    const body = announcements.writes[0]?.body as Schemas["AdminAnnouncementCreateRequest"];
    expect(body.title).toBe("Maintenance tonight");
    expect(body.text).toBe("The server is read-only from 02:00.");
    expect(body.publish).toBe(true);
    expect(body.ends_at).toBeDefined();
    expect(Date.parse(body.ends_at ?? "") - Date.now()).toBeGreaterThan(6 * 86_400_000);
    expect(
      within(screen.getByRole("region", { name: "All announcements" })).getByRole("heading", {
        name: `${isolated("Maintenance tonight")} (Published)`,
      }),
    ).toBeInTheDocument();
  });

  test("keeps a new announcement as a draft", async () => {
    const announcements = announcementsServer([]);
    const { user } = await renderApp({ path: "/admin/announcements" });
    const form = await screen.findByRole("region", { name: "Write an announcement" });

    await user.type(within(form).getByRole("textbox", { name: "Title" }), "Draft");
    await user.type(within(form).getByRole("textbox", { name: "Text" }), "Draft");
    await user.click(within(form).getByRole("checkbox", { name: /Publish now/u }));
    await user.click(within(form).getByRole("button", { name: "Save as a draft" }));

    expect(
      await within(form).findByText("Saved the announcement as a draft. The audit log records the change."),
    ).toBeInTheDocument();
    expect(announcements.writes[0]?.body).toMatchObject({ title: "Draft", text: "Draft", publish: false });
  });

  test("says at once that a title is too long, and sends nothing", async () => {
    const announcements = announcementsServer([]);
    const { user } = await renderApp({ path: "/admin/announcements" });
    const form = await screen.findByRole("region", { name: "Write an announcement" });

    await user.click(within(form).getByRole("textbox", { name: "Title" }));
    await user.paste("x".repeat(101));
    expect(await within(form).findByText("Use at most 100 characters.")).toBeInTheDocument();
    await user.type(within(form).getByRole("textbox", { name: "Text" }), "Text");
    await user.click(within(form).getByRole("button", { name: "Publish the announcement" }));
    expect(announcements.writes).toHaveLength(0);
  });

  test("publishes a draft, edits it, and withdraws it after confirming", async () => {
    const announcements = announcementsServer([draft]);
    const { user } = await renderApp({ path: "/admin/announcements" });
    const list = await screen.findByRole("region", { name: "All announcements" });

    await user.click(within(list).getByRole("button", { name: "Publish: Maintenance" }));
    expect(
      await within(list).findByText("Published Maintenance. The audit log records the change."),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(
        within(list).getByRole("heading", { level: 3, name: `${isolated("Maintenance")} (Published)` }),
      ).toHaveFocus();
    });

    await user.click(within(list).getByRole("button", { name: "Edit: Maintenance" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit the announcement" });
    expect(
      within(dialog).getByText("Changing a published announcement does not notify anyone again."),
    ).toBeInTheDocument();
    const field = within(dialog).getByRole("textbox", { name: "Title" });
    expect(field).toHaveFocus();
    await user.clear(field);
    await user.type(field, "Maintenance moved to Monday");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(
      await within(list).findByText("Changed Maintenance moved to Monday. The audit log records the change."),
    ).toBeInTheDocument();

    await user.click(within(list).getByRole("button", { name: "Withdraw: Maintenance moved to Monday" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Withdraw Maintenance moved to Monday?" });
    await user.click(within(confirm).getByRole("button", { name: "Withdraw" }));
    expect(
      await within(list).findByText("Withdrew Maintenance moved to Monday. The audit log records the change."),
    ).toBeInTheDocument();
    expect(within(list).queryByRole("button", { name: /Withdraw/u })).not.toBeInTheDocument();

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
    expect(screen.queryByRole("region", { name: "Write an announcement" })).not.toBeInTheDocument();
  });
});
