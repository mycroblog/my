// SPDX-License-Identifier: AGPL-3.0-only
// Reporting an account from a post's menu and from a profile's menu, through the whole app (web-client/README.md T4):
// the real router and QueryClient, MSW for the client API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import {
  otherAccountFixture,
  remoteAccountFixture,
  type Schemas,
  statusFixture,
  timelineStatusFixture,
  viewerStatusFixture,
} from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { instanceWithRules, rulesFixture } from "./test/handlers";

const postId = "0192f0c4-7a8e-7000-8000-0000000000c1";
const post: Schemas["Status"] = { ...statusFixture(postId), content: "<p>The post</p>" };

/** Serves `post` and its empty thread. */
function servePost(...statuses: Schemas["Status"][]) {
  server.use(
    http.get(`${apiBase}/statuses/:id/context`, () => HttpResponse.json({ ancestors: [], descendants: [] })),
    http.get(`${apiBase}/statuses/:id`, ({ params }) => {
      const found = [post, ...statuses].find((s) => s.id === params["id"]);
      return found === undefined ? problem(404, "/api/v1/problems/not-found", "Not Found") : HttpResponse.json(found);
    }),
  );
}

/** Records the bodies of POST /reports, answered by the default handler. */
function recordReports() {
  const bodies: Schemas["CreateReportRequest"][] = [];
  server.events.on("request:start", ({ request }) => {
    if (request.method === "POST" && new URL(request.url).pathname === `${apiBase}/reports`) {
      void request
        .clone()
        .json()
        .then((body: Schemas["CreateReportRequest"]) => bodies.push(body));
    }
  });
  return bodies;
}

/** Opens the report dialog from the menu of the post the page is about. */
async function openFromPost(user: Awaited<ReturnType<typeof renderApp>>["user"]) {
  const article = await screen.findByRole("article", { description: "The post" });
  await user.click(await within(article).findByRole("button", { name: "More actions" }));
  await user.click(await screen.findByRole("menuitem", { name: "Report" }));
  return screen.findByRole("dialog", { name: "Report \u2068Bob\u2069" });
}

/** Waits until the heading of a step has the focus. */
async function expectStep(name: string) {
  const heading = await screen.findByRole("heading", { name });
  await waitFor(() => {
    expect(heading).toHaveFocus();
  });
}

describe("reporting from a post", () => {
  test("goes through every step with the keyboard only, the focus on each step's heading", async () => {
    server.use(signedInHandler, instanceWithRules);
    servePost();
    const bodies = recordReports();
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const article = await screen.findByRole("article", { description: "The post" });
    const more = await within(article).findByRole("button", { name: "More actions" });

    // The menu button pattern: Enter opens the menu on its first item.
    more.focus();
    await user.keyboard("{Enter}");
    await screen.findByRole("menu", { name: "More actions" });
    await user.keyboard("{ArrowDown}{Enter}");
    const dialog = await screen.findByRole("dialog", { name: "Report \u2068Bob\u2069" });
    await expectStep("Why are you reporting this account?");
    expect(within(dialog).getByText("Step 1 of 3")).toBeInTheDocument();

    // Tab enters the radio group, the arrow keys choose (spam → illegal content → rule violation).
    await user.tab();
    expect(within(dialog).getByRole("radio", { name: "Spam" })).toHaveFocus();
    await user.keyboard("{ArrowDown}{ArrowDown}");
    expect(within(dialog).getByRole("radio", { name: "Breaks the server rules" })).toBeChecked();
    expect(within(dialog).getByText("Step 1 of 4")).toBeInTheDocument();
    await user.tab();
    expect(within(dialog).getByRole("button", { name: "Next" })).toHaveFocus();
    await user.keyboard("{Enter}");

    await expectStep("Which rules does it break?");
    const rules = within(dialog).getByRole("group", { name: "Which rules does it break?" });
    expect(within(rules).getByRole("checkbox", { name: "No advertising" })).toHaveAccessibleDescription(
      rulesFixture[1]?.hint ?? "",
    );
    await user.tab();
    await user.tab();
    expect(within(rules).getByRole("checkbox", { name: "No advertising" })).toHaveFocus();
    await user.keyboard(" ");
    await user.tab({ shift: false });
    await user.tab();
    expect(within(dialog).getByRole("button", { name: "Next" })).toHaveFocus();
    await user.keyboard("{Enter}");

    await expectStep("Which posts are you reporting?");
    const posts = within(dialog).getByRole("group", { name: "Which posts are you reporting?" });
    // The post the report started from is chosen, the account's latest posts follow it.
    const checkboxes = await within(posts).findAllByRole("checkbox");
    expect(checkboxes).toHaveLength(2);
    expect(checkboxes[0]).toBeChecked();
    expect(checkboxes[0]).toHaveAccessibleDescription("The post");
    expect(checkboxes[1]).not.toBeChecked();
    expect(checkboxes[1]).toHaveAccessibleDescription("Hello from the local timeline!");
    await user.tab();
    await user.tab();
    expect(checkboxes[1]).toHaveFocus();
    await user.keyboard(" ");
    await user.tab();
    await user.tab();
    await user.keyboard("{Enter}");

    await expectStep("Anything else the moderators should know?");
    // A local account's report cannot be forwarded anywhere.
    expect(within(dialog).queryByRole("checkbox")).not.toBeInTheDocument();
    await user.tab();
    expect(within(dialog).getByRole("textbox", { name: "Comment" })).toHaveFocus();
    await user.keyboard("Sells things in every reply");
    await user.tab();
    await user.tab();
    expect(within(dialog).getByRole("button", { name: "Send report" })).toHaveFocus();
    await user.keyboard("{Enter}");

    await expectStep("Report sent");
    expect(bodies).toEqual([
      {
        account_id: otherAccountFixture.id,
        category: "violation",
        rule_ids: [rulesFixture[1]?.id],
        status_ids: [postId, timelineStatusFixture.id],
        comment: "Sells things in every reply",
        forward: false,
      },
    ]);
    await user.tab();
    expect(within(dialog).getByRole("button", { name: "Done" })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await waitFor(() => {
      expect(more).toHaveFocus();
    });
  });

  test("asks for a reason and, for a rule violation, a rule before going on", async () => {
    server.use(signedInHandler, instanceWithRules);
    servePost();
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const dialog = await openFromPost(user);

    await user.click(within(dialog).getByRole("button", { name: "Next" }));
    expect(await within(dialog).findByText("Choose a reason.")).toBeInTheDocument();
    expect(within(dialog).getByRole("heading", { name: "Why are you reporting this account?" })).toBeInTheDocument();

    await user.click(within(dialog).getByRole("radio", { name: "Breaks the server rules" }));
    await user.click(within(dialog).getByRole("button", { name: "Next" }));
    await expectStep("Which rules does it break?");
    await user.click(within(dialog).getByRole("button", { name: "Next" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Choose at least one rule.");
    expect(within(dialog).getByRole("group", { name: "Which rules does it break?" })).toHaveAccessibleDescription(
      "Choose every rule that applies. Choose at least one rule.",
    );
    await user.click(within(dialog).getByRole("checkbox", { name: "Be kind to each other" }));
    expect(within(dialog).queryByRole("alert")).not.toBeInTheDocument();

    // Back keeps the answers.
    await user.click(within(dialog).getByRole("button", { name: "Back" }));
    await expectStep("Why are you reporting this account?");
    expect(within(dialog).getByRole("radio", { name: "Breaks the server rules" })).toBeChecked();
  });

  test("offers no rule violation while the server has no rules, and skips the rules", async () => {
    server.use(signedInHandler);
    servePost();
    const bodies = recordReports();
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const dialog = await openFromPost(user);

    expect(
      within(dialog)
        .getAllByRole("radio")
        .map((radio) => radio.getAttribute("value")),
    ).toEqual(["spam", "legal", "other"]);
    await user.click(within(dialog).getByRole("radio", { name: "Spam" }));
    await user.click(within(dialog).getByRole("button", { name: "Next" }));
    await expectStep("Which posts are you reporting?");
    expect(within(dialog).getByText("Step 2 of 3")).toBeInTheDocument();
    // The post can be left out too.
    await user.click(await within(dialog).findByRole("checkbox", { name: /^Post of/, checked: true }));
    await user.click(within(dialog).getByRole("button", { name: "Next" }));
    await user.click(await within(dialog).findByRole("button", { name: "Send report" }));

    await expectStep("Report sent");
    expect(bodies).toEqual([{ account_id: otherAccountFixture.id, category: "spam", forward: false }]);
  });

  test("keeps the dialog and the answers when the server refuses the report", async () => {
    server.use(
      signedInHandler,
      http.post(`${apiBase}/reports`, () =>
        problem(429, "/api/v1/problems/rate-limited", "Too many reports in the last hour."),
      ),
    );
    servePost();
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const dialog = await openFromPost(user);

    await user.click(within(dialog).getByRole("radio", { name: "Something else" }));
    await user.click(within(dialog).getByRole("button", { name: "Next" }));
    await user.click(await within(dialog).findByRole("button", { name: "Next" }));
    await user.type(await within(dialog).findByRole("textbox", { name: "Comment" }), "Impersonates me");
    await user.click(within(dialog).getByRole("button", { name: "Send report" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "Could not send the report: Too many reports in the last hour.",
    );
    expect(within(dialog).getByRole("textbox", { name: "Comment" })).toHaveValue("Impersonates me");
    expect(
      within(dialog).getByRole("heading", { name: "Anything else the moderators should know?" }),
    ).toBeInTheDocument();
  });

  test("tells why the posts did not load and loads them again", async () => {
    let fails = true;
    server.use(
      signedInHandler,
      http.get(`${apiBase}/accounts/:id/statuses`, () =>
        fails
          ? problem(503, "about:blank", "The server is busy.")
          : HttpResponse.json({ items: [timelineStatusFixture] }),
      ),
    );
    servePost();
    const { user } = await renderApp({ path: `/@bob/status/${postId}` });
    const dialog = await openFromPost(user);
    await user.click(within(dialog).getByRole("radio", { name: "Spam" }));
    await user.click(within(dialog).getByRole("button", { name: "Next" }));

    expect(await within(dialog).findByRole("heading", { name: "Could not load the posts" })).toBeInTheDocument();
    expect(within(dialog).getByText("The server is busy.")).toBeInTheDocument();
    fails = false;
    await user.click(within(dialog).getByRole("button", { name: "Try again" }));
    expect(await within(dialog).findAllByRole("checkbox")).toHaveLength(2);
  });

  test("is not offered on the viewer's own posts", async () => {
    const own = viewerStatusFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000e1", content: "<p>My post</p>" });
    server.use(signedInHandler);
    servePost(own);
    const { user } = await renderApp({ path: `/@alice/status/${own.id}` });

    await user.click(
      await within(await screen.findByRole("article", { description: "My post" })).findByRole("button", {
        name: "More actions",
      }),
    );
    expect(await screen.findByRole("menuitem", { name: "Copy link" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Report" })).not.toBeInTheDocument();
  });
});

describe("reporting from a profile", () => {
  test("chooses no post at first, and forwards the report of another server's account when asked", async () => {
    server.use(signedInHandler);
    const bodies = recordReports();
    const { user } = await renderApp({ path: "/@carol@remote.example" });

    await user.click(await screen.findByRole("button", { name: "More actions for \u2068Carol\u2069" }));
    await user.click(await screen.findByRole("menuitem", { name: "Report" }));
    const dialog = await screen.findByRole("dialog", { name: "Report \u2068Carol\u2069" });
    await expectStep("Why are you reporting this account?");
    await user.click(within(dialog).getByRole("radio", { name: "Illegal content" }));
    await user.click(within(dialog).getByRole("button", { name: "Next" }));

    await expectStep("Which posts are you reporting?");
    const checkbox = await within(dialog).findByRole("checkbox");
    expect(checkbox).not.toBeChecked();
    // A post behind a content warning shows the warning, not the text.
    expect(checkbox).toHaveAccessibleDescription("Content warning: Cats 2 attachments");
    await user.click(checkbox);
    await user.click(within(dialog).getByRole("button", { name: "Next" }));

    await expectStep("Anything else the moderators should know?");
    const forward = within(dialog).getByRole("checkbox", {
      name: "Also send the report to \u2066remote.example\u2069",
    });
    expect(forward).not.toBeChecked();
    expect(forward).toHaveAccessibleDescription(
      "The moderators of \u2066remote.example\u2069 get a copy without your name.",
    );
    await user.click(forward);
    await user.click(within(dialog).getByRole("button", { name: "Send report" }));

    await expectStep("Report sent");
    expect(
      within(dialog).getByText("A copy without your name goes to \u2066remote.example\u2069."),
    ).toBeInTheDocument();
    expect(bodies).toEqual([
      {
        account_id: remoteAccountFixture.id,
        category: "legal",
        status_ids: ["0192f0c4-7a8e-7000-8000-0000000003c1"],
        forward: true,
      },
    ]);
  });
});
