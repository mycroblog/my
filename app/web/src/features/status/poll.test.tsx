// SPDX-License-Identifier: AGPL-3.0-only
// The poll of a post through the whole app (web-client/README.md T4, FR-112): the real router and QueryClient, MSW for
// the client API. Time is fixed so that the end of a poll is known.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { apiBase, oauthPaths, tokenResponse } from "../../test/msw/http";
import { remoteStatusFixture, type Schemas, statusFixture } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";
import { pollFixture, votedPoll } from "./test/handlers";

const postId = "0192f0c4-7a8e-7000-8000-0000000000e0";

function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

/** Answers GET /statuses/{id} with `post` with `poll`, and records the votes it receives. */
function servePoll(poll: Schemas["Poll"], post: Status = { ...statusFixture(postId), content: "<p>Which color?</p>" }) {
  const votes: Schemas["VoteInPollRequest"][] = [];
  let current = poll;
  server.use(
    http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json({ ...post, poll: current })),
    http.post(`${apiBase}/statuses/:id/poll/votes`, async ({ request }) => {
      const body = (await request.json()) as Schemas["VoteInPollRequest"];
      votes.push(body);
      current = votedPoll(current, body.choices);
      return HttpResponse.json({ ...post, poll: current });
    }),
  );
  return { votes, path: `/@${post.account.acct}/status/${post.id}` };
}

/** Finds a visible toast by its text (the announcer's live regions say it too). */
async function findToast(text: string) {
  return within(await screen.findByRole("region", { name: "Messages" })).findByText(text);
}

beforeEach(() => {
  // Only Date is faked: timers, MSW and user-event run as usual.
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-10-16T12:00:00Z") });
  sessionStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("the poll of a post", () => {
  test("is voted in with one option, then shows each option's share with the viewer's vote", async () => {
    signedIn();
    const { votes, path } = servePoll(pollFixture);
    const user = userEvent.setup();
    await renderApp({ path });

    const poll = await screen.findByRole("region", { name: "Poll" });
    // The form shows once the viewer is known.
    const options = await within(poll).findByRole("radiogroup", { name: "Poll options" });
    expect(options).toHaveAccessibleDescription("Choose one option.");
    expect(within(poll).getByText("4 people")).toBeInTheDocument();
    expect(within(poll).getByText("Ends in 1 day")).toBeInTheDocument();
    const vote = within(poll).getByRole("button", { name: "Vote" });
    expect(vote).toBeDisabled();

    await user.click(within(options).getByRole("radio", { name: "Blue" }));
    await user.click(vote);

    await findToast("Vote sent");
    expect(votes).toEqual([{ poll_id: pollFixture.id, choices: [1] }]);
    const results = within(poll).getByRole("list", { name: "Poll results" });
    const rows = within(results).getAllByRole("listitem");
    expect(rows.map((row) => row.textContent)).toEqual(["20%Red", "80%BlueYour vote", "0%Green"]);
    expect(within(poll).queryByRole("button", { name: "Vote" })).not.toBeInTheDocument();
    expect(within(poll).getByText("5 people")).toBeInTheDocument();
  });

  test("of several choices is voted in with checkboxes", async () => {
    signedIn();
    const { votes, path } = servePoll({ ...pollFixture, multiple: true });
    const user = userEvent.setup();
    await renderApp({ path });

    const poll = await screen.findByRole("region", { name: "Poll" });
    const group = await within(poll).findByRole("group", { name: "Poll options" });
    expect(group).toHaveAccessibleDescription("Choose one or more options.");
    await user.click(within(group).getByRole("checkbox", { name: "Green" }));
    await user.click(within(group).getByRole("checkbox", { name: "Red" }));
    await user.click(within(poll).getByRole("button", { name: "Vote" }));

    await findToast("Vote sent");
    expect(votes).toEqual([{ poll_id: pollFixture.id, choices: [0, 2] }]);
  });

  test("that has ended shows its results and takes no vote", async () => {
    signedIn();
    const { path } = servePoll({ ...pollFixture, expires_at: "2026-10-16T11:00:00Z", expired: true });
    await renderApp({ path });

    const poll = await screen.findByRole("region", { name: "Poll" });
    expect(within(poll).getByRole("list", { name: "Poll results" })).toBeInTheDocument();
    expect(within(poll).getByText("Closed")).toBeInTheDocument();
    expect(within(poll).queryByRole("button", { name: "Vote" })).not.toBeInTheDocument();
  });

  test("that hides its votes until it ends shows the options without shares", async () => {
    const { voted: _voted, own_votes: _ownVotes, ...anonymous } = pollFixture;
    const { path } = servePoll({ ...anonymous, options: anonymous.options.map(({ title }) => ({ title })) });
    await renderApp({ path });

    const poll = await screen.findByRole("region", { name: "Poll" });
    const rows = within(within(poll).getByRole("list", { name: "Poll results" })).getAllByRole("listitem");
    expect(rows.map((row) => row.textContent)).toEqual(["Red", "Blue", "Green"]);
    expect(within(poll).getByText("The votes of each option are shown when the poll ends.")).toBeInTheDocument();
    expect(within(poll).getByText("4 people")).toBeInTheDocument();
  });

  test("shows its results to a visitor who is not signed in", async () => {
    const { voted: _voted, own_votes: _ownVotes, ...anonymous } = pollFixture;
    const { path } = servePoll(anonymous);
    await renderApp({ path });

    const poll = await screen.findByRole("region", { name: "Poll" });
    expect(within(poll).getByRole("list", { name: "Poll results" })).toBeInTheDocument();
    expect(within(poll).queryByRole("radiogroup")).not.toBeInTheDocument();
  });

  test("of another server says whose counts it shows", async () => {
    signedIn();
    const { path } = servePoll(
      { ...pollFixture, voted: true, own_votes: [0] },
      { ...remoteStatusFixture, id: postId, spoiler_text: "", media_attachments: [] },
    );
    await renderApp({ path });

    const poll = await screen.findByRole("region", { name: "Poll" });
    expect(within(poll).getByText("Counts as remote.example last reported them")).toBeInTheDocument();
    expect(within(poll).getAllByRole("listitem")[0]).toHaveTextContent("Your vote");
  });

  test("tells why a vote was refused and reads the post again", async () => {
    signedIn();
    let reads = 0;
    const post: Status = { ...statusFixture(postId), content: "<p>Which color?</p>", poll: pollFixture };
    server.use(
      http.get(`${apiBase}/statuses/:id`, () => {
        reads++;
        return HttpResponse.json(post);
      }),
      http.post(`${apiBase}/statuses/:id/poll/votes`, () => {
        const body: Schemas["Problem"] = {
          type: "about:blank",
          title: "Unprocessable Content",
          status: 422,
          detail: "This poll has ended.",
        };
        return HttpResponse.json(body, { status: 422, headers: { "Content-Type": "application/problem+json" } });
      }),
    );
    const user = userEvent.setup();
    await renderApp({ path: `/@bob/status/${postId}` });

    const poll = await screen.findByRole("region", { name: "Poll" });
    await user.click(await within(poll).findByRole("radio", { name: "Red" }));
    await user.click(within(poll).getByRole("button", { name: "Vote" }));

    await findToast("Could not vote: This poll has ended.");
    expect(reads).toBeGreaterThan(1);
  });
});
