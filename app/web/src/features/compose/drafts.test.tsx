// SPDX-License-Identifier: AGPL-3.0-only
// Saving the form's draft on the server and scheduling it (web-client/screens/compose.md §4, ADR-0156) through the
// whole app, with MSW for the server.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { apiBase, oauthPaths, tokenResponse } from "../../test/msw/http";
import { draftFixture, preferencesFixture, type Schemas } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { draftResponse, savedDraftId } from "./test/handlers";

function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

type Request = { method: string; path: string; body: unknown; ifMatch: string | null };

/** Records every request to /drafts and below, then lets the default handlers answer. */
function recordDrafts() {
  const sent: Request[] = [];
  server.events.on("request:start", ({ request }) => {
    const url = new URL(request.url);
    if (!url.pathname.startsWith("/api/v1/drafts") || request.method === "GET") return;
    void request
      .clone()
      .text()
      .then((text) => {
        sent.push({
          method: request.method,
          path: url.pathname.replace("/api/v1", ""),
          body: text === "" ? null : (JSON.parse(text) as unknown),
          ifMatch: request.headers.get("If-Match"),
        });
      });
  });
  return sent;
}

/** Records the text of every POST /statuses, then lets the default handler answer. */
function recordStatuses() {
  const texts: string[] = [];
  server.events.on("request:start", ({ request }) => {
    if (new URL(request.url).pathname !== "/api/v1/statuses" || request.method !== "POST") return;
    void request
      .clone()
      .json()
      .then((body) => {
        texts.push((body as { text: string }).text);
      });
  });
  return texts;
}

async function findToast(text: string) {
  return within(await screen.findByRole("region", { name: "Messages" })).findByText(text);
}

async function openSheet(user: Awaited<ReturnType<typeof renderApp>>["user"]) {
  await user.click((await screen.findAllByRole("button", { name: "Write a post" }))[0] as HTMLElement);
  return screen.findByRole("dialog", { name: "New post" });
}

beforeEach(() => {
  // Only Date is faked: timers, MSW and user-event run as usual. 09:00 in Tokyo.
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-11-10T00:00:00Z") });
  sessionStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
  server.events.removeAllListeners();
});

describe("a draft saved on the server", () => {
  test("is saved with what the form holds, and the form empties", async () => {
    signedIn();
    const sent = recordDrafts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);

    await user.type(within(sheet).getByRole("textbox", { name: "Your post" }), "To finish later");
    await user.click(within(sheet).getByRole("button", { name: "Save draft" }));

    expect(await findToast("Draft saved")).toBeInTheDocument();
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveValue("");
    expect(sent).toEqual([
      {
        method: "POST",
        path: "/drafts",
        body: { text: "To finish later", visibility: "public", language: "en" },
        ifMatch: null,
      },
    ]);
  });

  test("says why an empty draft is not saved", async () => {
    signedIn();
    const sent = recordDrafts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);

    await user.click(within(sheet).getByRole("button", { name: "Save draft" }));

    expect(await within(sheet).findByText("There is nothing to save yet.")).toBeInTheDocument();
    expect(sent).toEqual([]);
    // The reason goes once there is something to save.
    await user.type(within(sheet).getByRole("textbox", { name: "Your post" }), "Now");
    expect(within(sheet).queryByText("There is nothing to save yet.")).not.toBeInTheDocument();
  });
});

describe("a scheduled post", () => {
  /** The user's time zone is Tokyo's, whatever the browser's. */
  function inTokyo() {
    server.use(
      http.get(`${apiBase}/preferences`, () =>
        HttpResponse.json({ ...preferencesFixture, time_zone: "Asia/Tokyo" } satisfies Schemas["Preferences"]),
      ),
    );
  }

  test("is picked in the user's time zone and refuses a past time", async () => {
    signedIn();
    inTokyo();
    const sent = recordDrafts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    await user.type(within(sheet).getByRole("textbox", { name: "Your post" }), "Good evening");

    await user.click(within(sheet).getByRole("button", { name: "Schedule the post" }));
    const popover = await screen.findByRole("dialog", { name: "Schedule the post" });
    expect(
      within(popover).getByText("The date and time are in your time zone, Japan Standard Time."),
    ).toBeInTheDocument();
    const date = within(popover).getByLabelText(/^Date/);
    const time = within(popover).getByLabelText(/^Time/);
    // The next full hour in Tokyo.
    expect(date).toHaveValue("2026-11-10");
    expect(time).toHaveValue("10:00");

    await user.clear(time);
    await user.type(time, "08:00");
    await user.click(within(popover).getByRole("button", { name: "Schedule" }));
    expect(await within(popover).findByText("Choose a time at least 5 minutes from now.")).toBeInTheDocument();
    expect(sent).toEqual([]);

    await user.clear(time);
    await user.type(time, "21:30");
    await user.click(within(popover).getByRole("button", { name: "Schedule" }));

    expect(await findToast("Scheduled for Nov 10, 2026, 9:30 PM")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Schedule the post" })).not.toBeInTheDocument();
    });
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveValue("");
    expect(sent).toEqual([
      {
        method: "POST",
        path: "/drafts",
        body: { text: "Good evening", visibility: "public", language: "en" },
        ifMatch: null,
      },
      {
        method: "PUT",
        path: `/drafts/${savedDraftId}/schedule`,
        body: { publish_at: "2026-11-10T12:30:00.000Z" },
        ifMatch: null,
      },
    ]);
  });

  test("keeps the saved draft when the server refuses the time, and the next try replaces it", async () => {
    signedIn();
    const sent = recordDrafts();
    server.use(
      http.put(`${apiBase}/drafts/:id/schedule`, () =>
        HttpResponse.json(
          { type: "about:blank", title: "You have 25 posts scheduled for that day already.", status: 422 },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    await user.type(within(sheet).getByRole("textbox", { name: "Your post" }), "Busy day");

    await user.click(within(sheet).getByRole("button", { name: "Schedule the post" }));
    const popover = await screen.findByRole("dialog", { name: "Schedule the post" });
    await user.click(within(popover).getByRole("button", { name: "Schedule" }));

    expect(
      await within(popover).findByText(/^The post could not be scheduled: You have 25 posts scheduled/),
    ).toBeInTheDocument();
    // The popover stays open over the form, which is hidden from assistive technology meanwhile.
    expect(within(sheet).getByRole("textbox", { name: "Your post", hidden: true })).toHaveValue("Busy day");
    expect(within(sheet).getByText("You are editing a saved draft.")).toBeInTheDocument();

    await user.click(within(popover).getByRole("button", { name: "Schedule" }));
    await waitFor(() => {
      expect(sent.map((r) => `${r.method} ${r.path} ${r.ifMatch ?? ""}`)).toEqual([
        "POST /drafts ",
        `PUT /drafts/${savedDraftId}/schedule `,
        `PUT /drafts/${savedDraftId} "v1"`,
        `PUT /drafts/${savedDraftId}/schedule `,
      ]);
    });
  });
});

describe("a draft opened from the server", () => {
  /** The drafts page lists one draft, which GET /drafts/d1 reads with the ETag "v1". */
  function oneDraft() {
    const draft = draftFixture("d1", { text: "Written on my phone" });
    server.use(
      http.get(`${apiBase}/drafts`, ({ request }) =>
        HttpResponse.json({ items: new URL(request.url).searchParams.get("scheduled") === "true" ? [] : [draft] }),
      ),
      http.get(`${apiBase}/drafts/d1`, () => draftResponse(draft, '"v1"')),
    );
  }

  test("is published by the server with what the form holds, never as a new post", async () => {
    signedIn();
    const sent = recordDrafts();
    const statuses: unknown[] = [];
    server.events.on("request:start", ({ request }) => {
      if (new URL(request.url).pathname === "/api/v1/statuses") statuses.push(request.method);
    });
    oneDraft();
    const { user } = await renderApp({ path: "/drafts" });

    await user.click(await screen.findByRole("button", { name: "Edit: Written on my phone" }));
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    const text = within(sheet).getByRole("textbox", { name: "Your post" });
    expect(text).toHaveValue("Written on my phone");
    await user.type(text, ", finished on my laptop");
    await user.click(within(sheet).getByRole("button", { name: "Post" }));

    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent).toEqual([
      {
        method: "PUT",
        path: "/drafts/d1",
        body: { text: "Written on my phone, finished on my laptop", visibility: "public", language: "en" },
        ifMatch: '"v1"',
      },
      { method: "POST", path: "/drafts/d1/publish", body: null, ifMatch: null },
    ]);
    expect(statuses).toEqual([]);
  });

  test("is not published when another device saved it since, and posting again posts it as a new post", async () => {
    signedIn();
    const sent = recordDrafts();
    oneDraft();
    server.use(
      http.put(`${apiBase}/drafts/d1`, () =>
        HttpResponse.json(
          { type: "about:blank", title: "Precondition Failed", status: 412 },
          { status: 412, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const posted = recordStatuses();
    const { user } = await renderApp({ path: "/drafts" });

    await user.click(await screen.findByRole("button", { name: "Edit: Written on my phone" }));
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    await user.click(within(sheet).getByRole("button", { name: "Post" }));

    expect(
      await within(sheet).findByText(
        "This draft was changed on another device, so nothing was posted. Post again to publish this version as a new post.",
      ),
    ).toBeInTheDocument();
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveValue("Written on my phone");
    expect(within(sheet).queryByText("You are editing a saved draft.")).not.toBeInTheDocument();
    await user.click(within(sheet).getByRole("button", { name: "Post" }));

    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent.map((r) => `${r.method} ${r.path}`)).toEqual(["PUT /drafts/d1"]);
    expect(posted).toEqual(["Written on my phone"]);
  });
});
