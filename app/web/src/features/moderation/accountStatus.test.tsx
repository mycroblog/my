// SPDX-License-Identifier: AGPL-3.0-only
// The account status page through the whole app (web-client/README.md T4): the moderation actions on the user's own
// account in effect and past, their posts, and appealing one through its dialog, with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import { preferencesFixture, type Schemas, viewerStatusFixture } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { ownActionFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  server.events.removeAllListeners();
});

type Action = Schemas["ModerationAction"];

const suspension = ownActionFixture();
// Two posts marked as sensitive, one of them deleted since; the explanation is in Arabic.
const sensitivePosts = ownActionFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000007a2",
  kind: "mark_statuses_sensitive",
  kind_label: "Posts marked as sensitive",
  reason_category: "other",
  reason_category_label: "Other",
  public_reason: "صور غير مناسبة",
  status_ids: ["0192f0c4-7a8e-7000-8000-0000000000a1", "0192f0c4-7a8e-7000-8000-0000000000a9"],
  state: "expired",
  issued_at: "2026-09-01T00:00:00Z",
  appealable: false,
  appeal_deadline: "2026-09-02T00:00:00Z",
});
delete sensitivePosts.expires_at;
// A lifted limit that was appealed and overturned.
const overturned = ownActionFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000007a3",
  kind: "limit",
  kind_label: "Limited",
  state: "lifted",
  issued_at: "2026-08-01T00:00:00Z",
  lifted_at: "2026-08-05T12:00:00Z",
  appealable: false,
  appeal: {
    id: "0192f0c4-7a8e-7000-8000-0000000007b2",
    text: "These were not spam.",
    state: "overturned",
    submitted_at: "2026-08-02T00:00:00Z",
    decided_at: "2026-08-05T12:00:00Z",
    decision_reason: "We agree; lifted.",
  },
});
delete overturned.expires_at;
delete overturned.public_reason;

/**
 * Plays the actions on the user's account: the list (`active=true` keeps those in effect) and appeals, which
 * it records. `refuse` answers every appeal with that problem type, 422.
 */
function actionsServer(initial: Action[], { refuse, afterRefusal }: { refuse?: string; afterRefusal?: Action[] } = {}) {
  let actions = initial;
  const appeals: { id: string; body: Schemas["AppealModerationActionRequest"] }[] = [];
  server.use(
    http.get(`${apiBase}/moderation/actions`, ({ request }) => {
      const active = new URL(request.url).searchParams.get("active") === "true";
      return HttpResponse.json({ items: active ? actions.filter((action) => action.state === "active") : actions });
    }),
    http.post(`${apiBase}/moderation/actions/:id/appeal`, async ({ request, params }) => {
      const id = String(params["id"]);
      const body = (await request.json()) as Schemas["AppealModerationActionRequest"];
      appeals.push({ id, body });
      if (refuse !== undefined) {
        if (afterRefusal !== undefined) actions = afterRefusal;
        return problem(422, refuse, "Unprocessable");
      }
      const action = actions.find((item) => item.id === id);
      if (action === undefined) return problem(404, "about:blank", "Not Found");
      const appealed: Action = {
        ...action,
        appealable: false,
        appeal: {
          id: "0192f0c4-7a8e-7000-8000-0000000007b9",
          text: body.text,
          state: "pending",
          submitted_at: "2026-10-17T00:00:00Z",
        },
      };
      actions = actions.map((item) => (item.id === id ? appealed : item));
      return HttpResponse.json(appealed);
    }),
    // The first post is the viewer's; the second one is deleted.
    http.get(`${apiBase}/statuses/:id`, ({ params }) =>
      params["id"] === "0192f0c4-7a8e-7000-8000-0000000000a1"
        ? HttpResponse.json(viewerStatusFixture({ content: "<p>A sunset</p>" }))
        : problem(404, "about:blank", "Not Found"),
    ),
  );
  return { appeals };
}

/** The `article` of the action whose heading is `kind`. */
async function findAction(kind: string): Promise<HTMLElement> {
  return screen.findByRole("article", { name: kind });
}

describe("the account status", () => {
  test("opens from the settings, and lists the actions in effect and the past ones with their facts", async () => {
    actionsServer([suspension, sensitivePosts, overturned]);
    const { user } = await renderApp({ path: "/settings" });

    await user.click(await screen.findByRole("link", { name: "Account status" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Account status" })).toBeInTheDocument();
    const inEffect = screen.getByRole("region", { name: "Actions in effect" });
    const past = screen.getByRole("region", { name: "Past actions" });
    expect(
      within(inEffect)
        .getAllByRole("heading", { level: 3 })
        .map((heading) => heading.textContent),
    ).toEqual(["Suspended"]);
    expect(
      within(past)
        .getAllByRole("heading", { level: 3 })
        .map((heading) => heading.textContent),
    ).toEqual(["Posts marked as sensitive", "Limited"]);

    const suspended = within(await findAction("Suspended"));
    expect(suspended.getByText("In effect")).toBeInTheDocument();
    expect(suspended.getByText("Rule violation")).toBeInTheDocument();
    expect(suspended.getByText("Oct 16, 2026, 9:30 AM")).toHaveAttribute("dateTime", "2026-10-16T09:30:00Z");
    expect(suspended.getByText("Nov 15, 2026, 9:30 AM")).toBeInTheDocument();
    expect(suspended.getByText("Repeated spam in replies.")).toHaveAttribute("dir", "auto");
    // How long it lasts and how long it can be appealed, in the words of the server's status page; the days count from now.
    expect(suspended.getByText(/^Ends in \d+ days?$/)).toBeInTheDocument();
    expect(
      suspended.getByText(/^You can appeal once, within \d+ days? \(until Apr 14, 2027, 9:30 AM\)\.$/),
    ).toBeInTheDocument();
    expect(suspended.getByRole("button", { name: "Appeal the action of Oct 16, 2026, 9:30 AM" })).toBeInTheDocument();

    // An action on posts shows the posts, and says which one is gone; its window to appeal has closed.
    const marked = within(await findAction("Posts marked as sensitive"));
    expect(marked.getByText("Ended")).toBeInTheDocument();
    expect(marked.getByText("صور غير مناسبة")).toHaveAttribute("dir", "auto");
    const posts = marked.getByRole("region", { name: "Posts" });
    expect(await within(posts).findByText("A sunset")).toBeInTheDocument();
    expect(await within(posts).findByText("This post is deleted or cannot be shown.")).toBeInTheDocument();
    expect(marked.queryByText("Ends")).not.toBeInTheDocument();
    expect(marked.getByText("The time to appeal this action ended on Sep 2, 2026, 12:00 AM.")).toBeInTheDocument();
    expect(marked.queryByRole("button", { name: /Appeal/ })).not.toBeInTheDocument();

    // An appealed action shows its appeal and the decision instead of the form.
    const limited = within(await findAction("Limited"));
    expect(limited.getAllByText("Lifted")).toHaveLength(2);
    // A lifted action that had no end has none to show.
    expect(limited.queryByText("Ends")).not.toBeInTheDocument();
    expect(limited.getByText("None given")).toBeInTheDocument();
    expect(limited.getByText("Action lifted")).toBeInTheDocument();
    expect(limited.getByText("These were not spam.")).toHaveAttribute("dir", "auto");
    expect(limited.getByText("We agree; lifted.")).toBeInTheDocument();
    expect(limited.queryByRole("button", { name: /Appeal/ })).not.toBeInTheDocument();
  });

  test("shows the dates in the user's locale and time zone", async () => {
    actionsServer([suspension]);
    server.use(
      http.get(`${apiBase}/preferences`, () =>
        HttpResponse.json({ ...preferencesFixture, locale: "ja", time_zone: "Asia/Tokyo" }),
      ),
    );
    await renderApp({ path: "/settings/account-status" });

    const format = new Intl.DateTimeFormat("ja", { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Tokyo" });
    const action = within(await screen.findByRole("article", { name: "Suspended" }));
    expect(await action.findByText(format.format(new Date(suspension.issued_at)))).toBeInTheDocument();
    expect(action.getByText(format.format(new Date("2026-11-15T09:30:00Z")))).toBeInTheDocument();
    expect(format.format(new Date(suspension.issued_at))).toContain("18:30");
    expect(
      action.getByRole("button", { name: `${format.format(new Date(suspension.issued_at))}の処分に異議を申し立てる` }),
    ).toBeInTheDocument();
  });

  test("says when no action is in effect and none is past", async () => {
    actionsServer([]);
    await renderApp({ path: "/settings/account-status" });

    expect(
      await screen.findByRole("heading", { level: 3, name: "No actions are in effect on your account" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 3, name: "No past actions" })).toBeInTheDocument();
  });

  test("loads more past actions with the cursor of the page before", async () => {
    const cursors: (string | null)[] = [];
    server.use(
      http.get(`${apiBase}/moderation/actions`, ({ request }) => {
        const params = new URL(request.url).searchParams;
        if (params.get("active") === "true") return HttpResponse.json({ items: [] });
        const cursor = params.get("cursor");
        cursors.push(cursor);
        return HttpResponse.json(
          cursor === null ? { items: [suspension, sensitivePosts], next_cursor: "next" } : { items: [overturned] },
        );
      }),
    );
    const { user } = await renderApp({ path: "/settings/account-status" });
    const past = await screen.findByRole("region", { name: "Past actions" });

    await user.click(within(past).getByRole("button", { name: "Show more" }));

    expect(await within(past).findByRole("heading", { level: 3, name: "Limited" })).toBeInTheDocument();
    // The action in effect is on the whole list too, but only under the actions in effect.
    expect(within(past).queryByRole("heading", { name: "Suspended" })).not.toBeInTheDocument();
    expect(within(past).queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
    expect(cursors).toEqual([null, "next"]);
  });
});

describe("appealing an action", () => {
  test("needs a reason, sends it trimmed, then shows the appeal under review, the result, and keeps focus on the appeal", async () => {
    const api = actionsServer([suspension]);
    const { user } = await renderApp({ path: "/settings/account-status" });

    await user.click(await screen.findByRole("button", { name: "Appeal the action of Oct 16, 2026, 9:30 AM" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Appeal this action?" });
    expect(dialog).toHaveAccessibleDescription(
      "Moderators review “Suspended” of Oct 16, 2026, 9:30 AM again and tell you their decision in a notification. You can appeal an action only once, and you cannot change the appeal after sending it.",
    );
    const text = within(dialog).getByRole("textbox", { name: "Why should the moderators reconsider it?" });
    await waitFor(() => {
      expect(text).toHaveFocus();
    });

    // White space only is no reason, and nothing is sent.
    await user.type(text, "   ");
    await user.click(within(dialog).getByRole("button", { name: "Send appeal" }));
    expect(
      await within(dialog).findByText("Enter why the moderators should reconsider the action"),
    ).toBeInTheDocument();
    expect(within(dialog).getByRole("textbox")).toBeInvalid();
    expect(api.appeals).toEqual([]);

    await user.type(within(dialog).getByRole("textbox"), "I did not post spam. ");
    await user.click(within(dialog).getByRole("button", { name: "Send appeal" }));

    await waitFor(() => {
      expect(dialog).not.toBeInTheDocument();
    });
    expect(api.appeals).toEqual([{ id: suspension.id, body: { text: "I did not post spam." } }]);
    expect(
      await screen.findByText(
        "You appealed “Suspended”. The moderators will tell you their decision in a notification.",
      ),
    ).toBeInTheDocument();
    const action = within(await findAction("Suspended"));
    expect(action.getByText("Under review")).toBeInTheDocument();
    expect(action.getByText("I did not post spam.")).toBeInTheDocument();
    expect(action.queryByRole("button", { name: /Appeal/ })).not.toBeInTheDocument();
    await waitFor(() => {
      expect(action.getByRole("heading", { level: 4, name: "Appeal" })).toHaveFocus();
    });
  });

  test("cancel and Esc close the dialog without appealing", async () => {
    const api = actionsServer([suspension]);
    const { user } = await renderApp({ path: "/settings/account-status" });
    const open = await screen.findByRole("button", { name: "Appeal the action of Oct 16, 2026, 9:30 AM" });

    await user.click(open);
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(open).toHaveFocus();
    });

    await user.keyboard("{Enter}");
    await screen.findByRole("alertdialog");
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(api.appeals).toEqual([]);
  });

  test("a refusal stays in the dialog, and when the window has closed the page reads the actions again and drops the form", async () => {
    const closed: Action = { ...suspension, appealable: false, appeal_deadline: "2026-10-16T10:00:00Z" };
    actionsServer([suspension], { refuse: "/api/v1/problems/appeal-window-closed", afterRefusal: [closed] });
    const { user } = await renderApp({ path: "/settings/account-status" });

    await user.click(await screen.findByRole("button", { name: "Appeal the action of Oct 16, 2026, 9:30 AM" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.type(within(dialog).getByRole("textbox"), "Please reconsider.");
    await user.click(within(dialog).getByRole("button", { name: "Send appeal" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Could not send the appeal: Unprocessable");
    expect(within(dialog).getByRole("textbox")).toHaveValue("Please reconsider.");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    const action = within(await findAction("Suspended"));
    expect(
      await action.findByText("The time to appeal this action ended on Oct 16, 2026, 10:00 AM."),
    ).toBeInTheDocument();
    expect(action.queryByRole("button", { name: /Appeal/ })).not.toBeInTheDocument();
  });
});
