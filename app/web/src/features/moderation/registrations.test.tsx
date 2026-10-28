// SPDX-License-Identifier: AGPL-3.0-only
// The registrations screen through the whole app (web-client/README.md T4): a moderator with manage_accounts reviews
// the sign-ups awaiting approval and approves or rejects them through their confirmation, with a note, with
// MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { registrationFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_accounts"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  server.events.removeAllListeners();
});

type Registration = Schemas["AdminRegistration"];

/** U+2066 LEFT-TO-RIGHT ISOLATE … U+2069 POP DIRECTIONAL ISOLATE: a handle inside a sentence. */
function isolated(handle: string): string {
  return `⁦${handle}⁩`;
}

const bob = registrationFixture();
// A reason in Arabic, which reads right to left whatever the page's direction.
const carol = registrationFixture({
  user_id: "0192f0c4-7a8e-7000-8000-0000000000b2",
  account_id: "0192f0c4-7a8e-7000-8000-0000000000c2",
  username: "carol",
  email: "",
  email_verified: false,
  reason: "أريد الانضمام",
  invite_id: "0192f0c4-7a8e-7000-8000-0000000000e1",
  submitted_at: "2026-10-16T10:00:00Z",
});
const gone = registrationFixture({
  user_id: "0192f0c4-7a8e-7000-8000-0000000000b3",
  submitted_at: "2026-10-16T11:00:00Z",
});
delete gone.username;
delete gone.account_id;
delete gone.reason;

type Decision = { userId: string; decision: string; body: Schemas["RegistrationDecisionRequest"] };

/**
 * Plays the registrations of the server: the pending ones, which decisions take off; records the decisions.
 * `decided` holds the decisions other moderators took first, which a decision keeps.
 */
function registrationsServer(
  initial: Registration[],
  decided: Readonly<Record<string, Schemas["RegistrationState"]>> = {},
) {
  let pending = initial;
  const decisions: Decision[] = [];
  const answer = (decision: "approve" | "reject") =>
    http.post(`${apiBase}/admin/registrations/:userId/${decision}`, async ({ request, params }) => {
      const userId = String(params["userId"]);
      decisions.push({ userId, decision, body: (await request.json()) as Schemas["RegistrationDecisionRequest"] });
      const registration = initial.find((item) => item.user_id === userId);
      if (registration === undefined) return problem(404, "about:blank", "Not Found");
      pending = pending.filter((item) => item.user_id !== userId);
      return HttpResponse.json({
        ...registration,
        state: decided[userId] ?? (decision === "approve" ? "approved" : "rejected"),
        since: "2026-10-17T00:00:00Z",
      });
    });
  server.use(
    http.get(`${apiBase}/admin/registrations`, () => HttpResponse.json({ items: pending })),
    answer("approve"),
    answer("reject"),
  );
  return { decisions };
}

describe("the list of registrations", () => {
  test("shows each registration awaiting approval with its applicant's reason in the reason's own direction", async () => {
    registrationsServer([bob, carol, gone]);
    await renderApp({ path: "/moderation/registrations" });

    expect(await screen.findByRole("heading", { level: 1, name: "Registrations" })).toBeInTheDocument();
    expect(
      within(screen.getByRole("table"))
        .getAllByRole("rowheader")
        .map((header) => header.textContent),
    ).toEqual(["@bobbob@example.com", "@carolNone", "A removed accountbob@example.com"]);
    const [first, second, third] = within(screen.getByRole("table")).getAllByRole("row").slice(1);
    if (first === undefined || second === undefined || third === undefined)
      throw new Error("a registration is missing");

    const bobRow = within(first);
    expect(bobRow.getByText(/^Oct 16, 2026/)).toBeInTheDocument();
    expect(bobRow.getByText("bob@example.com")).toBeInTheDocument();
    expect(bobRow.getByText("Confirmed")).toBeInTheDocument();
    expect(bobRow.getByText("None")).toBeInTheDocument();
    expect(bobRow.getByText("I would like to post about birds.")).toHaveAttribute("dir", "auto");
    expect(bobRow.getByRole("button", { name: `Approve ${isolated("@bob")}` })).toBeInTheDocument();
    expect(bobRow.getByRole("button", { name: `Reject ${isolated("@bob")}` })).toBeInTheDocument();

    const carolRow = within(second);
    expect(carolRow.getByText("أريد الانضمام")).toHaveAttribute("dir", "auto");
    expect(carolRow.getByText("Not confirmed")).toBeInTheDocument();
    expect(carolRow.getByText("Signed up with an invite")).toBeInTheDocument();

    expect(within(third).getByText("No reason given")).toBeInTheDocument();
    expect(within(third).getByRole("button", { name: "Approve A removed account" })).toBeInTheDocument();
  });

  test("says when no registration awaits approval", async () => {
    registrationsServer([]);
    await renderApp({ path: "/moderation/registrations" });

    expect(
      await screen.findByRole("heading", { level: 2, name: "No registrations await approval" }),
    ).toBeInTheDocument();
  });

  test("loads more registrations with the cursor of the page before", async () => {
    const cursors: (string | null)[] = [];
    server.use(
      http.get(`${apiBase}/admin/registrations`, ({ request }) => {
        const cursor = new URL(request.url).searchParams.get("cursor");
        cursors.push(cursor);
        return HttpResponse.json(cursor === null ? { items: [bob], next_cursor: "next" } : { items: [carol] });
      }),
    );
    const { user } = await renderApp({ path: "/moderation/registrations" });

    await user.click(
      within(await screen.findByRole("navigation", { name: "Pages" })).getByRole("button", { name: "Next" }),
    );

    expect(await screen.findByRole("rowheader", { name: /@carol/ })).toBeInTheDocument();
    expect(
      within(screen.getByRole("navigation", { name: "Pages" })).getByRole("button", { name: "Next" }),
    ).toBeDisabled();
    expect(cursors).toEqual([null, "next"]);
  });
});

describe("deciding a registration", () => {
  test("approves after the confirmation with a note: the registration leaves the list, focus moves on, and the result is a status", async () => {
    const api = registrationsServer([bob, carol]);
    const { user } = await renderApp({ path: "/moderation/registrations" });

    await user.click(await screen.findByRole("button", { name: `Approve ${isolated("@bob")}` }));
    const dialog = await screen.findByRole("alertdialog", { name: `Approve ${isolated("@bob")}?` });
    expect(dialog).toHaveAccessibleDescription(
      `${isolated("@bob")} can sign in and start posting. The decision cannot be changed afterwards.`,
    );
    const note = within(dialog).getByRole("textbox", { name: "Note for moderators" });
    await waitFor(() => {
      expect(note).toHaveFocus();
    });
    await user.type(note, "  Known from the birding club.  ");
    expect(api.decisions).toEqual([]);

    await user.click(within(dialog).getByRole("button", { name: "Approve" }));

    await waitFor(() => {
      expect(screen.queryByRole("rowheader", { name: /@bob/ })).not.toBeInTheDocument();
    });
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: `Approve ${isolated("@carol")}` })).toHaveFocus();
    });
    expect(screen.getAllByRole("status").map((status) => status.textContent)).toContain(
      `${isolated("@bob")} is approved and can sign in.`,
    );
    expect(api.decisions).toEqual([
      { userId: bob.user_id, decision: "approve", body: { note: "Known from the birding club." } },
    ]);
  });

  test("rejects without a note, and the last decision leaves an empty list with focus on the heading", async () => {
    const api = registrationsServer([carol]);
    const { user } = await renderApp({ path: "/moderation/registrations" });

    await user.click(await screen.findByRole("button", { name: `Reject ${isolated("@carol")}` }));
    const dialog = await screen.findByRole("alertdialog", { name: `Reject ${isolated("@carol")}?` });
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    expect(
      await screen.findByRole("heading", { level: 2, name: "No registrations await approval" }),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole("heading", { level: 1 })).toHaveFocus();
    });
    expect(screen.getAllByRole("status").map((status) => status.textContent)).toContain(
      `${isolated("@carol")} is rejected.`,
    );
    expect(api.decisions).toEqual([{ userId: carol.user_id, decision: "reject", body: {} }]);
  });

  test("cancel and Esc close the confirmation without deciding, and focus returns to its button", async () => {
    const api = registrationsServer([bob]);
    const { user } = await renderApp({ path: "/moderation/registrations" });
    const reject = await screen.findByRole("button", { name: `Reject ${isolated("@bob")}` });

    await user.click(reject);
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(reject).toHaveFocus();
    });

    await user.keyboard("{Enter}");
    expect(await screen.findByRole("alertdialog", { name: `Reject ${isolated("@bob")}?` })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(reject).toHaveFocus();
    });
    expect(screen.getByRole("rowheader", { name: /@bob/ })).toBeInTheDocument();
    expect(api.decisions).toEqual([]);
  });

  test("a note over the limit is refused by the form before anything is sent", async () => {
    const api = registrationsServer([bob]);
    const { user } = await renderApp({ path: "/moderation/registrations" });

    await user.click(await screen.findByRole("button", { name: `Approve ${isolated("@bob")}` }));
    const dialog = await screen.findByRole("alertdialog");
    const note = within(dialog).getByRole("textbox", { name: "Note for moderators" });
    await user.click(note);
    await user.paste("x".repeat(1001));
    await user.click(within(dialog).getByRole("button", { name: "Approve" }));

    expect(await within(dialog).findByText("Use at most 1,000 characters")).toBeInTheDocument();
    expect(note).toHaveAttribute("aria-invalid", "true");
    expect(api.decisions).toEqual([]);
  });

  test("a failure stays in the confirmation with the reason, and the registration stays", async () => {
    registrationsServer([bob]);
    server.use(
      http.post(`${apiBase}/admin/registrations/:userId/approve`, () =>
        problem(500, "about:blank", "Internal Server Error"),
      ),
    );
    const { user } = await renderApp({ path: "/moderation/registrations" });

    await user.click(await screen.findByRole("button", { name: `Approve ${isolated("@bob")}` }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Approve" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "Could not save the decision: Internal Server Error",
    );
    // The page behind the modal dialog is hidden from assistive technology while it is open.
    expect(screen.getByRole("rowheader", { name: /@bob/, hidden: true })).toBeInTheDocument();
  });

  test("a registration another moderator decided first leaves the list, and the status says which decision stands", async () => {
    registrationsServer([bob], { [bob.user_id]: "rejected" });
    const { user } = await renderApp({ path: "/moderation/registrations" });

    await user.click(await screen.findByRole("button", { name: `Approve ${isolated("@bob")}` }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Approve" }));

    expect(
      await screen.findByText(`${isolated("@bob")} was already rejected by another moderator. That decision stands.`),
    ).toBeInTheDocument();
    expect(screen.queryByRole("rowheader", { name: /@bob/ })).not.toBeInTheDocument();
  });
});

describe("permissions", () => {
  test("a moderator without manage_accounts is told why, and the server is asked for nothing", async () => {
    server.use(operatorHandler(["manage_domains"]));
    const requested: string[] = [];
    server.events.on("request:start", ({ request }) => {
      if (new URL(request.url).pathname.startsWith("/api/v1/admin/")) requested.push(request.url);
    });
    await renderApp({ path: "/moderation/registrations" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.getByText(/the permission manage_accounts/)).toBeInTheDocument();
    expect(requested).toEqual([]);
  });

  test("a refusal of the server for a missing permission is shown as such", async () => {
    server.use(
      http.get(`${apiBase}/admin/registrations`, () =>
        problem(403, "/api/v1/problems/missing-permission", "Forbidden"),
      ),
    );
    await renderApp({ path: "/moderation/registrations" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Registrations" })).toBeInTheDocument();
  });
});
