// SPDX-License-Identifier: AGPL-3.0-only
// The drafts page (web-client/screens/drafts.md) through the whole app, with MSW for the server: the drafts are kept in
// memory, so a list fetched again after a change shows it as the server would.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { apiBase, oauthPaths, tokenResponse } from "../../test/msw/http";
import { draftFixture, type Schemas } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

/** A draft's answer with its ETag, as every draft operation but the list sends it. */
function draftResponse(draft: Schemas["Draft"], etag: string, status = 200) {
  return HttpResponse.json(draft, { status, headers: { ETag: etag } });
}

function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

const scheduled = draftFixture(
  "d1",
  { text: "Good morning, everyone" },
  { scheduled_at: "2026-11-11T09:00:00Z", updated_at: "2026-11-09T10:00:00Z" },
);
const failed = draftFixture(
  "d2",
  { text: "A reply that lost its post", in_reply_to_id: "s1" },
  {
    last_schedule: {
      publish_at: "2026-11-09T08:00:00Z",
      outcome: "failed",
      reason: "reply_unavailable",
      occurred_at: "2026-11-09T08:00:05Z",
    },
  },
);

/** The server's drafts in memory; returns what was written. */
function draftsServer(initial: Schemas["Draft"][]) {
  const drafts = new Map(initial.map((d) => [d.id, d]));
  const writes: string[] = [];
  server.use(
    http.get(`${apiBase}/drafts`, ({ request }) => {
      const wantScheduled = new URL(request.url).searchParams.get("scheduled") === "true";
      return HttpResponse.json({
        items: [...drafts.values()].filter((d) => (d.scheduled_at !== undefined) === wantScheduled),
      } satisfies Schemas["DraftPage"]);
    }),
    http.get(`${apiBase}/drafts/:id`, ({ params }) => {
      const draft = drafts.get(String(params["id"]));
      return draft === undefined
        ? HttpResponse.json({ type: "about:blank", title: "Not found", status: 404 }, { status: 404 })
        : draftResponse(draft, '"v1"');
    }),
    http.put(`${apiBase}/drafts/:id`, async ({ request, params }) => {
      const id = String(params["id"]);
      writes.push(`PUT ${id} ${request.headers.get("If-Match") ?? ""}`);
      if (request.headers.get("If-Match") !== '"v1"')
        return HttpResponse.json({ type: "about:blank", title: "Changed", status: 412 }, { status: 412 });
      const draft = draftFixture(id, (await request.json()) as Schemas["DraftRequest"]);
      drafts.set(id, draft);
      return draftResponse(draft, '"v2"');
    }),
    http.post(`${apiBase}/drafts`, async ({ request }) => {
      writes.push("POST");
      const draft = draftFixture("d9", (await request.json()) as Schemas["DraftRequest"]);
      drafts.set(draft.id, draft);
      return draftResponse(draft, '"v1"', 201);
    }),
    http.put(`${apiBase}/drafts/:id/schedule`, async ({ request, params }) => {
      const id = String(params["id"]);
      const { publish_at: at } = (await request.json()) as Schemas["DraftScheduleRequest"];
      writes.push(`SCHEDULE ${id} ${at}`);
      const draft = { ...(drafts.get(id) as Schemas["Draft"]), scheduled_at: at };
      drafts.set(id, draft);
      return draftResponse(draft, '"v1"');
    }),
    http.delete(`${apiBase}/drafts/:id/schedule`, ({ params }) => {
      const id = String(params["id"]);
      writes.push(`UNSCHEDULE ${id}`);
      const { scheduled_at: _, ...draft } = drafts.get(id) as Schemas["Draft"];
      drafts.set(id, draft);
      return draftResponse(draft, '"v1"');
    }),
    http.delete(`${apiBase}/drafts/:id`, ({ params }) => {
      const id = String(params["id"]);
      writes.push(`DELETE ${id}`);
      drafts.delete(id);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return writes;
}

function list(name: "Scheduled posts" | "Drafts") {
  return screen.getByRole("list", { name });
}

beforeEach(() => {
  // Only Date is faked: timers, MSW and user-event run as usual.
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-11-10T00:00:00Z") });
  sessionStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("the drafts page", () => {
  test("lists the scheduled posts, then the drafts, with why a schedule failed", async () => {
    signedIn();
    draftsServer([scheduled, failed]);
    await renderApp({ path: "/drafts" });

    expect(await screen.findByRole("heading", { level: 1, name: "Drafts and scheduled posts" })).toBeInTheDocument();
    expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual([
      "Scheduled posts",
      "Drafts",
    ]);
    const [scheduledRow] = within(list("Scheduled posts")).getAllByRole("listitem") as [HTMLElement];
    expect(scheduledRow).toHaveTextContent("Good morning, everyone");
    // In the user's time zone (UTC), once the preferences have said which.
    await waitFor(() => {
      expect(scheduledRow).toHaveTextContent("To be published Nov 11, 2026, 9:00 AM");
    });
    const [draftRow] = within(list("Drafts")).getAllByRole("listitem") as [HTMLElement];
    expect(draftRow).toHaveTextContent("Reply");
    expect(draftRow).toHaveTextContent("The post it replies to is no longer available.");
    expect(within(draftRow).queryByRole("button", { name: /^Cancel schedule/ })).not.toBeInTheDocument();
  });

  test("says when there is nothing", async () => {
    signedIn();
    draftsServer([]);
    await renderApp({ path: "/drafts" });

    expect(await screen.findByRole("heading", { level: 3, name: "No scheduled posts" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 3, name: "No drafts" })).toBeInTheDocument();
  });

  test("opens a draft in the form, asking first when the form holds something else", async () => {
    signedIn();
    const writes = draftsServer([failed]);
    const { user } = await renderApp({ path: "/drafts" });
    await user.click((await screen.findAllByRole("button", { name: "Write a post" }))[0] as HTMLElement);
    let sheet = await screen.findByRole("dialog", { name: "New post" });
    await user.type(within(sheet).getByRole("textbox", { name: "Your post" }), "Something else");
    await user.click(within(sheet).getByRole("button", { name: "Close" }));

    await user.click(await screen.findByRole("button", { name: "Edit: A reply that lost its post" }));
    const question = await screen.findByRole("alertdialog", { name: "Replace what you are writing?" });
    await user.click(within(question).getByRole("button", { name: "Replace" }));

    sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveValue("A reply that lost its post");
    expect(within(sheet).getByText("You are editing a saved draft.")).toBeInTheDocument();
    await user.click(within(sheet).getByRole("button", { name: "Save draft" }));

    await waitFor(() => {
      expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveValue("");
    });
    expect(writes).toEqual(['PUT d2 "v1"']);
  });

  test("keeps a draft that another device saved since, and saves it again as a new one", async () => {
    signedIn();
    const writes = draftsServer([failed]);
    server.use(http.get(`${apiBase}/drafts/d2`, () => draftResponse(failed, '"v0"')));
    const { user } = await renderApp({ path: "/drafts" });

    await user.click(await screen.findByRole("button", { name: "Edit: A reply that lost its post" }));
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    await user.click(within(sheet).getByRole("button", { name: "Save draft" }));

    expect(
      await within(sheet).findByText(
        "This draft was changed on another device. Save again to keep this as a new draft.",
      ),
    ).toBeInTheDocument();
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveValue("A reply that lost its post");
    await user.click(within(sheet).getByRole("button", { name: "Save draft" }));

    await waitFor(() => {
      expect(writes).toEqual(['PUT d2 "v0"', "POST"]);
    });
  });

  test("cancels a schedule, and the draft moves to the drafts", async () => {
    signedIn();
    const writes = draftsServer([scheduled]);
    const { user } = await renderApp({ path: "/drafts" });

    await user.click(await screen.findByRole("button", { name: "Cancel schedule: Good morning, everyone" }));

    await waitFor(() => {
      expect(within(list("Drafts")).getByRole("listitem")).toHaveTextContent("Good morning, everyone");
    });
    expect(screen.getByRole("heading", { level: 3, name: "No scheduled posts" })).toBeInTheDocument();
    expect(writes).toEqual(["UNSCHEDULE d1"]);
  });

  test("moves a schedule to another time", async () => {
    signedIn();
    const writes = draftsServer([scheduled]);
    const { user } = await renderApp({ path: "/drafts" });

    await user.click(await screen.findByRole("button", { name: "Change time: Good morning, everyone" }));
    const popover = await screen.findByRole("dialog", { name: "Schedule the post" });
    // The fields start at the time of the schedule, in the user's time zone (UTC here).
    expect(within(popover).getByLabelText(/^Date/)).toHaveValue("2026-11-11");
    expect(within(popover).getByLabelText(/^Time/)).toHaveValue("09:00");
    await user.clear(within(popover).getByLabelText(/^Time/));
    await user.type(within(popover).getByLabelText(/^Time/), "18:15");
    await user.click(within(popover).getByRole("button", { name: "Schedule" }));

    await waitFor(() => {
      expect(within(list("Scheduled posts")).getByRole("listitem")).toHaveTextContent(
        "To be published Nov 11, 2026, 6:15 PM",
      );
    });
    expect(writes).toEqual(["SCHEDULE d1 2026-11-11T18:15:00.000Z"]);
  });

  test("deletes a draft once confirmed", async () => {
    signedIn();
    const writes = draftsServer([failed]);
    const { user } = await renderApp({ path: "/drafts" });

    await user.click(await screen.findByRole("button", { name: "Delete: A reply that lost its post" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete this draft?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByRole("heading", { level: 3, name: "No drafts" })).toBeInTheDocument();
    expect(writes).toEqual(["DELETE d2"]);
  });
});
