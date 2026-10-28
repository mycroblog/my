// SPDX-License-Identifier: AGPL-3.0-only
// A profile of an account of another server through the whole app (web-client/README.md T4, ADR-0044): the notice
// that only the posts this server received are shown, with the link to the profile on the account's server,
// above the posts and in the empty state; and, when the first page started fetching the account's recent
// posts, the line that says so, announced, until the posts are read again once a few seconds later.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { apiBase, problem } from "../../test/msw/http";
import { remoteAccountFixture, remoteStatusFixture, statusPage } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { newTabName } from "../../test/links";
import { renderApp } from "../../test/render";
import { remoteFetchRereadDelay } from "../timeline";

const notice = "Only the posts this server has received are shown.";
const fetching = "Fetching recent posts from the original server.";
/** The link's name: the host inside the sentence is isolated, as names are (web-client/README.md Y10). */
const original = { name: newTabName("See on ⁨remote.example⁩") };
const path = "/@carol@remote.example";

/** A post of Carol that her server sent when this server fetched her recent posts. */
const fetchedStatus = {
  ...remoteStatusFixture,
  id: "0192f0c4-7a8e-7000-8000-0000000003c2",
  uri: "https://remote.example/users/carol/statuses/2",
  url: "https://remote.example/@carol/2",
  content: "<p>An older post, fetched from my server.</p>",
  spoiler_text: "",
  media_attachments: [],
};

/** The page's main region, where the profile is. */
function main() {
  return screen.getByRole("main");
}

/** The announcer's polite live region; the app's offline bar is another status region, empty while online. */
function politeRegion(): HTMLElement {
  const region = screen.getAllByRole("status").find((element) => element.hasAttribute("data-live-announcer"));
  if (region === undefined) throw new Error("the announcer is not rendered");
  return region;
}

/** Whether a live region says anything assertive (an error). */
function alerted(): boolean {
  return screen.getAllByRole("alert").some((element) => element.textContent !== "");
}

/**
 * Carol's posts as her server's fetch goes: the first request's first page starts the fetch, and every later
 * request waits for the test to open the gate and then answers `reread`, as the server would once it
 * stored the fetched posts. Returns the number of requests and the gate.
 */
function fetchServer(reread: () => Response) {
  const state = { requests: 0, open: () => {} };
  const gate = new Promise<void>((resolve) => {
    state.open = resolve;
  });
  server.use(
    http.get(`${apiBase}/accounts/${remoteAccountFixture.id}/statuses`, async ({ request }) => {
      // Carol pinned nothing that this server knows of.
      if (new URL(request.url).searchParams.get("pinned") === "true") return HttpResponse.json(statusPage([]));
      state.requests += 1;
      if (state.requests === 1)
        return HttpResponse.json({ ...statusPage([remoteStatusFixture]), remote_fetch_started: true });
      await gate;
      return reread();
    }),
  );
  return state;
}

/** Waits past the delay of the reread (real timers: the delay is a few seconds). */
const pastDelay = { timeout: remoteFetchRereadDelay + 3000 };

describe("a profile of another server", () => {
  test("says above its posts that only the posts this server received are shown, with a link to its server", async () => {
    await renderApp({ path });

    const feed = await screen.findByRole("feed", { name: "Posts" });
    expect(within(main()).getByText(notice)).toBeInTheDocument();
    // The link leaves the app for the account's profile on its server.
    expect(within(main()).getByRole("link", original)).toHaveAttribute("href", "https://remote.example/@carol");
    // Above the posts, outside the feed.
    expect(within(feed).queryByText(notice)).not.toBeInTheDocument();
    expect(
      within(main()).getByText(notice).compareDocumentPosition(feed) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(within(main()).queryByText(fetching)).not.toBeInTheDocument();
  });

  test("says the same, with the link, when it has no posts", async () => {
    server.use(http.get(`${apiBase}/accounts/:id/statuses`, () => HttpResponse.json(statusPage([]))));
    await renderApp({ path });

    expect(await screen.findByRole("heading", { level: 3, name: "No posts yet" })).toBeInTheDocument();
    expect(within(main()).getByText(notice)).toBeInTheDocument();
    expect(within(main()).getByRole("link", original)).toHaveAttribute("href", "https://remote.example/@carol");
  });
});

describe("a profile of another server whose recent posts are being fetched", () => {
  test("says so, announced, and reads the posts again once a few seconds later", async () => {
    const state = fetchServer(() => HttpResponse.json(statusPage([fetchedStatus, remoteStatusFixture])));
    await renderApp({ path });

    expect(await within(await screen.findByRole("main")).findByText(fetching)).toBeInTheDocument();
    expect(within(politeRegion()).getByText(fetching)).toBeInTheDocument();
    // The posts are read again once the delay is over, and the line stays until they are in.
    await waitFor(() => {
      expect(state.requests).toBe(2);
    }, pastDelay);
    expect(within(main()).getByText(fetching)).toBeInTheDocument();

    state.open();
    expect(
      await screen.findByRole("article", { description: "An older post, fetched from my server." }),
    ).toBeInTheDocument();
    expect(within(main()).queryByText(fetching)).not.toBeInTheDocument();
    expect(within(main()).getByText(notice)).toBeInTheDocument();
    // Once: no further read, however long the profile stays open.
    await new Promise((resolve) => setTimeout(resolve, remoteFetchRereadDelay + 500));
    expect(state.requests).toBe(2);
  }, 20_000);

  test("tells nothing when reading again fails, and keeps the posts, the notice and the link", async () => {
    const state = fetchServer(() => problem(500, "about:blank", "Internal Server Error"));
    state.open();
    await renderApp({ path });

    expect(await within(await screen.findByRole("main")).findByText(fetching)).toBeInTheDocument();
    await waitFor(() => {
      expect(within(main()).queryByText(fetching)).not.toBeInTheDocument();
    }, pastDelay);
    expect(state.requests).toBe(2);
    expect(within(screen.getByRole("feed", { name: "Posts" })).getByRole("article")).toBeInTheDocument();
    expect(within(main()).getByText(notice)).toBeInTheDocument();
    expect(within(main()).getByRole("link", original)).toBeInTheDocument();
    expect(alerted()).toBe(false);
  }, 20_000);
});
