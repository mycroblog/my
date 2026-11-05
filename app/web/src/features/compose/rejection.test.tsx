// SPDX-License-Identifier: AGPL-3.0-only
// A post that the server's automated review refuses, through the whole app (web-client/README.md T4): the form keeps the
// draft, shows the server's fixed text of the refusal and nothing else, and lets the author appeal with the text they
// tried to post and a comment, with MSW playing the server.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, oauthPaths, problem, tokenResponse } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

const decisionId = "0192f0c4-7a8e-7000-8000-0000000002c1";

/** Resumes a session with the token the default handlers accept. */
function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

/** The 422 of a refused post, with the decision to appeal until `deadline`. */
function rejected(deadline: string) {
  return HttpResponse.json(
    {
      type: "/api/v1/problems/status-rejected",
      title: "This post was not published.",
      status: 422,
      detail: "The server's automated review did not accept it. You can appeal the decision.",
      hook_decision_id: decisionId,
      appeal_deadline: deadline,
    } satisfies Schemas["StatusRejectedProblem"],
    { status: 422, headers: { "Content-Type": "application/problem+json" } },
  );
}

/** Refuses every post; records the appeals and answers them with `answer`, or the default handler's. */
function refuseAndRecordAppeals(deadline: string, answer?: () => Response) {
  const appeals: { id: string; body: Schemas["AppealHookDecisionRequest"] }[] = [];
  server.use(
    http.post(`${apiBase}/statuses`, () => rejected(deadline)),
    http.post(`${apiBase}/hook_decisions/:id/appeal`, async ({ request, params }) => {
      appeals.push({
        id: String(params["id"]),
        body: (await request.clone().json()) as Schemas["AppealHookDecisionRequest"],
      });
      return answer?.();
    }),
  );
  return appeals;
}

type User = Awaited<ReturnType<typeof renderApp>>["user"];

/** Opens the sheet, writes `text` and posts it; returns the sheet. */
async function post(user: User, text: string) {
  await user.click((await screen.findAllByRole("button", { name: "Write a post" }))[0] as HTMLElement);
  const sheet = await screen.findByRole("dialog", { name: "New post" });
  await user.type(await within(sheet).findByRole("textbox", { name: "Your post" }), text);
  await user.click(within(sheet).getByRole("button", { name: "Post" }));
  return sheet;
}

const future = "2099-01-01T12:00:00Z";

beforeEach(() => {
  sessionStorage.clear();
  document.documentElement.lang = "en";
});

describe("a post the automated review refuses", () => {
  test("keeps the draft and shows only the server's fixed text, with the appeal", async () => {
    signedIn();
    refuseAndRecordAppeals(future);
    const { user } = await renderApp();
    const sheet = await post(user, "Bake sale on Saturday");

    expect(await within(sheet).findByText("This post was not published.")).toBeVisible();
    expect(
      within(sheet).getByText("The server's automated review did not accept it. You can appeal the decision."),
    ).toBeVisible();
    expect(within(sheet).getByText(/^You can appeal until Jan 1, 2099/u)).toBeVisible();
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveValue("Bake sale on Saturday");
    // Not the error of a failed attempt, nor anything waiting to be sent again.
    expect(within(sheet).queryByText(/Could not send the post/u)).not.toBeInTheDocument();
    expect(within(sheet).queryByRole("list", { name: "Posts being sent" })).not.toBeInTheDocument();
    expect(within(sheet).getByRole("button", { name: "Appeal" })).toBeVisible();
  });

  test("sends the appeal with the text the author tried to post and their comment, and says so", async () => {
    signedIn();
    const appeals = refuseAndRecordAppeals(future);
    const { user } = await renderApp();
    const sheet = await post(user, "Bake sale on Saturday");
    await user.click(await within(sheet).findByRole("button", { name: "Appeal" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Appeal this decision?" });
    const text = within(dialog).getByRole("textbox", { name: "Text of the post" });
    expect(text).toHaveValue("Bake sale on Saturday");
    expect(text).toHaveFocus();
    await user.type(text, " at noon");
    await user.type(
      within(dialog).getByRole("textbox", { name: "Comment for the moderators" }),
      "A school event, not an advert.",
    );
    await user.click(within(dialog).getByRole("button", { name: "Send appeal" }));

    expect(
      await within(sheet).findByText(
        "Your appeal was sent. A moderator will review it; if they cancel the decision, you can post again.",
      ),
    ).toBeVisible();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(within(sheet).queryByRole("button", { name: "Appeal" })).not.toBeInTheDocument();
    expect(appeals).toEqual([
      {
        id: decisionId,
        body: { status_text: "Bake sale on Saturday at noon", comment: "A school event, not an advert." },
      },
    ]);
    // The draft stays, for when a moderator lets the author post again.
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveValue("Bake sale on Saturday");
  });

  test("an appeal sent already gives way to the reason, and other failures stay in the dialog", async () => {
    signedIn();
    const answers = [
      () => problem(503, "about:blank", "The server is busy"),
      () => problem(422, "/api/v1/problems/already-appealed", "Already appealed"),
    ];
    refuseAndRecordAppeals(future, () => (answers.shift() as () => Response)());
    const { user } = await renderApp();
    const sheet = await post(user, "Bake sale");
    await user.click(await within(sheet).findByRole("button", { name: "Appeal" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Appeal this decision?" });

    await user.click(within(dialog).getByRole("button", { name: "Send appeal" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Could not send the appeal: The server is busy");

    await user.click(within(dialog).getByRole("button", { name: "Send appeal" }));
    expect(
      await within(sheet).findByText("You have already appealed this decision. A moderator will review it."),
    ).toBeVisible();
    expect(within(sheet).queryByRole("button", { name: "Appeal" })).not.toBeInTheDocument();
  });

  test("past its deadline, says the time to appeal ended", async () => {
    signedIn();
    refuseAndRecordAppeals("2020-01-01T12:00:00Z");
    const { user } = await renderApp();
    const sheet = await post(user, "Bake sale");

    expect(await within(sheet).findByText(/^The time to appeal ended on Jan 1, 2020/u)).toBeVisible();
    expect(within(sheet).queryByRole("button", { name: "Appeal" })).not.toBeInTheDocument();
  });

  test("gives way to the next post", async () => {
    signedIn();
    let refuse = true;
    server.use(http.post(`${apiBase}/statuses`, () => (refuse ? rejected(future) : undefined)));
    const { user } = await renderApp();
    const sheet = await post(user, "Bake sale");
    expect(await within(sheet).findByText("This post was not published.")).toBeVisible();

    refuse = false;
    await user.click(within(sheet).getByRole("button", { name: "Post" }));

    expect(
      await within(await screen.findByRole("region", { name: "Messages" })).findByText("Posted."),
    ).toBeInTheDocument();
    expect(screen.queryByText("This post was not published.")).not.toBeInTheDocument();
  });
});
