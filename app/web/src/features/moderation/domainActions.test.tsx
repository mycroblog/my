// SPDX-License-Identifier: AGPL-3.0-only
// The domain actions screen through the whole app (web-client/README.md T4): an operator with manage_domains lists the
// actions, opens an action's page and lifts it there, and takes new ones on the page of the form (a suspension only
// after its confirmation), with MSW playing the admin API. The server keeps domains in IDNA A-labels; the screen
// shows them with their U-labels.
import { screen, waitFor, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { domainActionFixture, newDomainAction } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_domains"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type DomainAction = Schemas["AdminDomainAction"];
type NewAction = Schemas["CreateDomainActionRequest"];

/** U+2066 LEFT-TO-RIGHT ISOLATE … U+2069 POP DIRECTIONAL ISOLATE: a domain inside a sentence. */
function isolated(domain: string): string {
  return `⁦${domain}⁩`;
}

const idn = domainActionFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000000a2",
  domain: "xn--r8jz45g.jp",
  kind: "suspend",
  public_comment: "Spam from every account.",
  private_note: "Reported 12 times.",
  obfuscate: true,
  issued_at: "2026-10-12T08:00:00Z",
  expires_at: "2026-11-11T08:00:00Z",
});
const limit = domainActionFixture();
const lifted = domainActionFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000000a0",
  kind: "reject_media",
  state: "lifted",
  lifted_at: "2026-10-11T00:00:00Z",
});

/** Plays the domain actions of the server: a list that new actions join and lifts change; records the requests. */
function domainActionsServer(initial: DomainAction[]) {
  let actions = initial;
  const lists: string[] = [];
  const created: NewAction[] = [];
  const liftedIds: string[] = [];
  server.use(
    http.get(`${apiBase}/admin/domain_actions`, ({ request }) => {
      const query = new URL(request.url).searchParams;
      lists.push(query.toString());
      const domain = query.get("domain");
      const items = actions.filter(
        (action) =>
          (query.get("active") !== "true" || action.state === "active") &&
          (domain === null || action.domain === domain),
      );
      return HttpResponse.json({ items });
    }),
    http.get(`${apiBase}/admin/domain_actions/:id`, ({ params }) => {
      const action = actions.find((item) => item.id === String(params["id"]));
      return action === undefined ? problem(404, "about:blank", "Not Found") : HttpResponse.json(action);
    }),
    http.post(`${apiBase}/admin/domain_actions`, async ({ request }) => {
      const body = (await request.json()) as NewAction;
      created.push(body);
      // The server records U-labels as A-labels.
      const action = newDomainAction({ ...body, domain: body.domain === "例え.jp" ? "xn--r8jz45g.jp" : body.domain });
      actions = [action, ...actions];
      return HttpResponse.json(action);
    }),
    http.post(`${apiBase}/admin/domain_actions/:id/lift`, ({ params }) => {
      const id = String(params["id"]);
      liftedIds.push(id);
      const action = actions.find((item) => item.id === id);
      if (action === undefined) return problem(404, "about:blank", "Not Found");
      const next: DomainAction = { ...action, state: "lifted", lifted_at: "2026-10-17T00:00:00Z" };
      actions = actions.map((item) => (item.id === id ? next : item));
      return HttpResponse.json(next);
    }),
  );
  return { lists, created, liftedIds };
}

/** The rows of the table of actions, without its header row. */
function rows() {
  return within(screen.getByRole("table")).queryAllByRole("row").slice(1);
}

/** Shows the actions under another filter of the toolbar. */
async function showFilter(user: UserEvent, name: string) {
  await user.click(screen.getByRole("button", { name: /Show domain actions/ }));
  await user.click(await screen.findByRole("option", { name }));
}

describe("the list of domain actions", () => {
  test("shows the actions in effect with both forms of a domain and no comments, and all of them under the other filter", async () => {
    const api = domainActionsServer([idn, limit, lifted]);
    const { user, router } = await renderApp({ path: "/moderation/domain-actions" });

    expect(await screen.findByRole("heading", { level: 1, name: "Domain actions" })).toBeInTheDocument();
    const pages = screen.getByRole("navigation", { name: "Domain action pages" });
    expect(within(pages).getByRole("link", { name: "Actions" })).toHaveAttribute("aria-current", "page");
    expect(within(pages).getByRole("link", { name: "Mastodon's CSV" })).toHaveAttribute(
      "href",
      "/moderation/domain-actions/import",
    );
    expect(screen.getByRole("link", { name: "New action" })).toHaveAttribute("href", "/moderation/domain-actions/new");
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(
      within(await screen.findByRole("table"))
        .getAllByRole("rowheader")
        .map((header) => header.textContent),
    ).toEqual(["例え.jp (xn--r8jz45g.jp)", "spam.example"]);
    expect(within(screen.getByRole("table")).getAllByRole("columnheader")).toHaveLength(4);
    const [row] = rows();
    if (row === undefined) throw new Error("no action is listed");
    const details = within(row);
    expect(details.getByRole("link", { name: "例え.jp (xn--r8jz45g.jp)" })).toHaveAttribute(
      "href",
      `/moderation/domain-actions/${idn.id}`,
    );
    expect(details.getByText("Suspend")).toBeInTheDocument();
    expect(details.getByText("In effect")).toBeInTheDocument();
    expect(details.getByText(/^Nov 11, 2026/)).toBeInTheDocument();
    expect(details.queryByText("Spam from every account.")).not.toBeInTheDocument();
    expect(details.queryByText("Reported 12 times.")).not.toBeInTheDocument();
    expect(details.queryByRole("button")).not.toBeInTheDocument();

    await showFilter(user, "All");
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ filter: "all" });
    });
    const all = within(screen.getByRole("table"));
    expect(await all.findByText("Lifted")).toBeInTheDocument();
    expect(rows()).toHaveLength(3);
    expect(api.lists).toEqual(["limit=40&active=true", "limit=40"]);
  });

  test("says when no action is in effect", async () => {
    domainActionsServer([]);
    await renderApp({ path: "/moderation/domain-actions" });

    expect(await screen.findByRole("heading", { level: 2, name: "No domain actions in effect" })).toBeInTheDocument();
  });
});

describe("a domain action's page", () => {
  test("shows the action with its comments, and lifts it: focus moves to the state's heading, and the result stays", async () => {
    const api = domainActionsServer([idn, limit]);
    const { user } = await renderApp({ path: `/moderation/domain-actions/${idn.id}` });

    expect(
      await screen.findByRole("heading", { level: 1, name: `Suspend for ${isolated("例え.jp (xn--r8jz45g.jp)")}` }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to domain actions" })).toHaveAttribute(
      "href",
      "/moderation/domain-actions",
    );
    const details = within(screen.getByRole("region", { name: "Details" }));
    expect(details.getByText("Spam from every account.")).toBeInTheDocument();
    expect(details.getByText("Reported 12 times.")).toBeInTheDocument();
    expect(details.getByText("Partly hidden")).toBeInTheDocument();
    expect(details.getByRole("link", { name: "Every action on this domain" })).toHaveAttribute(
      "href",
      "/moderation/domain-actions?filter=all&domain=xn--r8jz45g.jp",
    );
    const state = within(screen.getByRole("region", { name: "State" }));
    expect(state.getByText(/^Oct 12, 2026/)).toBeInTheDocument();
    expect(state.getByText(/^Nov 11, 2026/)).toBeInTheDocument();

    await user.click(state.getByRole("button", { name: "Lift" }));

    await waitFor(() => {
      expect(state.queryByRole("button", { name: "Lift" })).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(state.getByRole("heading", { level: 2, name: "State" })).toHaveFocus();
    });
    expect(state.getByText("Lifted")).toBeInTheDocument();
    expect(state.getByRole("status")).toHaveTextContent(
      `Suspend for ${isolated("例え.jp (xn--r8jz45g.jp)")} is lifted.`,
    );
    expect(api.liftedIds).toEqual([idn.id]);
  });

  // #471: a lift writes the lift into every cached list, so no filter shows the action as it was.
  test("after a lift, the lists show the action lifted at once, however slowly they come", async () => {
    domainActionsServer([idn, limit]);
    const { user, router } = await renderApp({ path: "/moderation/domain-actions?filter=all" });
    expect(
      await within(await screen.findByRole("table")).findByRole("rowheader", { name: "spam.example" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("link", { name: "spam.example" }));
    const state = within(await screen.findByRole("region", { name: "State" }));
    await user.click(state.getByRole("button", { name: "Lift" }));
    await waitFor(() => {
      expect(state.queryByRole("button", { name: "Lift" })).not.toBeInTheDocument();
    });
    // From now on every list takes a while; the handler falls through to domainActionsServer's.
    server.use(
      http.get(`${apiBase}/admin/domain_actions`, async () => {
        await delay(300);
      }),
    );

    await router.navigate({ to: "/moderation/domain-actions", search: { filter: "all" } });
    expect(await screen.findByRole("rowheader", { name: "spam.example" })).toBeInTheDocument();
    const row = rows().find((item) => within(item).queryByRole("rowheader", { name: "spam.example" }) !== null);
    if (row === undefined) throw new Error("the lifted action is not listed");
    expect(within(row).getByText("Lifted")).toBeInTheDocument();
  });

  test("an action that cannot be lifted stays, and the failure says why", async () => {
    domainActionsServer([limit]);
    server.use(
      http.post(`${apiBase}/admin/domain_actions/:id/lift`, () => problem(503, "about:blank", "Service Unavailable")),
    );
    const { user } = await renderApp({ path: `/moderation/domain-actions/${limit.id}` });

    await user.click(await screen.findByRole("button", { name: "Lift" }));

    expect(await screen.findByText("Could not lift the action: Service Unavailable")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Lift" })).toBeInTheDocument();
  });

  test("a lifted action has no lift", async () => {
    domainActionsServer([lifted]);
    await renderApp({ path: `/moderation/domain-actions/${lifted.id}` });

    const state = within(await screen.findByRole("region", { name: "State" }));
    expect(state.getByText("Lifted")).toBeInTheDocument();
    expect(state.queryByRole("button")).not.toBeInTheDocument();
  });
});

describe("a new domain action", () => {
  test("is taken on a domain in non-Latin letters, recorded in A-labels, and listed in both forms", async () => {
    const api = domainActionsServer([]);
    const { user, router } = await renderApp({ path: "/moderation/domain-actions/new" });

    expect(await screen.findByRole("heading", { level: 1, name: "New action" })).toBeInTheDocument();
    await user.type(await screen.findByRole("textbox", { name: "Domain" }), "例え.jp");
    await user.click(screen.getByRole("radio", { name: "Limit" }));
    await user.click(screen.getByRole("button", { name: /Duration/ }));
    await user.click(await screen.findByRole("option", { name: "7 days" }));
    await user.type(screen.getByRole("textbox", { name: "Public comment" }), "Unmoderated spam.");
    await user.type(screen.getByRole("textbox", { name: "Private note" }), "See report 12.");
    await user.click(screen.getByRole("checkbox", { name: "Hide part of the domain in the public list" }));
    await user.click(screen.getByRole("button", { name: "Take action" }));

    expect(
      await screen.findByText(`Limit is in effect for ${isolated("例え.jp (xn--r8jz45g.jp)")}.`),
    ).toBeInTheDocument();
    expect(api.created).toEqual([
      {
        domain: "例え.jp",
        kind: "limit",
        obfuscate: true,
        public_comment: "Unmoderated spam.",
        private_note: "See report 12.",
        expires_in: 7 * 86_400,
      },
    ]);
    // The form is ready for the next action on the same domain.
    expect(screen.getByRole("textbox", { name: "Domain" })).toHaveValue("例え.jp");
    expect(screen.getByRole("radio", { name: "Limit" })).not.toBeChecked();
    expect(screen.getByRole("textbox", { name: "Public comment" })).toHaveValue("");

    await router.navigate({ to: "/moderation/domain-actions" });
    expect(
      await within(await screen.findByRole("table")).findByRole("rowheader", { name: "例え.jp (xn--r8jz45g.jp)" }),
    ).toBeInTheDocument();
  });

  // #471: a list shown before, cached as the route loads it again, must not show the actions as they were before the
  // new one while a slow server answers the list again.
  test("is in the lists at once, however slowly they come", async () => {
    const api = domainActionsServer([limit, lifted]);
    const { user, router } = await renderApp({ path: "/moderation/domain-actions?filter=all" });
    expect(await within(await screen.findByRole("table")).findByText("Lifted")).toBeInTheDocument();
    await user.click(screen.getByRole("link", { name: "New action" }));
    await user.type(await screen.findByRole("textbox", { name: "Domain" }), "new.example");
    await user.click(screen.getByRole("radio", { name: "Limit" }));
    // From now on every list takes a while; the handler falls through to domainActionsServer's.
    server.use(
      http.get(`${apiBase}/admin/domain_actions`, async () => {
        await delay(300);
      }),
    );
    await user.click(screen.getByRole("button", { name: "Take action" }));
    expect(await screen.findByText(`Limit is in effect for ${isolated("new.example")}.`)).toBeInTheDocument();

    await router.navigate({ to: "/moderation/domain-actions", search: { filter: "all" } });
    expect(await screen.findByRole("rowheader", { name: "new.example" })).toBeInTheDocument();
    expect(api.lists[0]).toEqual("limit=40");
  });

  test("needs a domain and a kind before anything is sent", async () => {
    const api = domainActionsServer([]);
    const { user } = await renderApp({ path: "/moderation/domain-actions/new" });

    await user.click(await screen.findByRole("button", { name: "Take action" }));

    expect(await screen.findByText("Enter a domain")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Domain" })).toBeInvalid();
    expect(screen.getByText("Choose an action")).toBeInTheDocument();
    expect(api.created).toEqual([]);
  });

  test("a suspension asks first; cancelling sends nothing and confirming takes it", async () => {
    const api = domainActionsServer([]);
    const { user } = await renderApp({ path: "/moderation/domain-actions/new" });

    await user.type(await screen.findByRole("textbox", { name: "Domain" }), "spam.example");
    await user.click(screen.getByRole("radio", { name: "Suspend" }));
    const submit = screen.getByRole("button", { name: "Take action" });
    await user.click(submit);

    const dialog = await screen.findByRole("alertdialog", { name: `Suspend ${isolated("spam.example")}?` });
    expect(dialog).toHaveAccessibleDescription(/lifting the suspension does not bring them back/);
    await waitFor(() => {
      expect(within(dialog).getByRole("button", { name: "Cancel" })).toHaveFocus();
    });
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(submit).toHaveFocus();
    });
    expect(api.created).toEqual([]);

    await user.click(submit);
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Suspend" }));

    expect(await screen.findByText(`Suspend is in effect for ${isolated("spam.example")}.`)).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(api.created).toEqual([{ domain: "spam.example", kind: "suspend", obfuscate: false }]);
  });

  test("a suspension that fails says why in its confirmation, which stays open", async () => {
    domainActionsServer([]);
    server.use(http.post(`${apiBase}/admin/domain_actions`, () => problem(503, "about:blank", "Service Unavailable")));
    const { user } = await renderApp({ path: "/moderation/domain-actions/new" });

    await user.type(await screen.findByRole("textbox", { name: "Domain" }), "spam.example");
    await user.click(screen.getByRole("radio", { name: "Suspend" }));
    await user.click(screen.getByRole("button", { name: "Take action" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Suspend" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "Could not take the action: Service Unavailable",
    );
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });

  test("an action the server refuses says why under the form, and keeps what was entered", async () => {
    domainActionsServer([]);
    server.use(
      http.post(`${apiBase}/admin/domain_actions`, () =>
        HttpResponse.json(
          { type: "about:blank", title: "Unprocessable Content", status: 422, detail: "domain: not a valid domain" },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/moderation/domain-actions/new" });

    await user.type(await screen.findByRole("textbox", { name: "Domain" }), "not a domain");
    await user.click(screen.getByRole("radio", { name: "Reject reports" }));
    await user.click(screen.getByRole("button", { name: "Take action" }));

    expect(await screen.findByText("Could not take the action: domain: not a valid domain")).toHaveAttribute(
      "role",
      "alert",
    );
    expect(screen.getByRole("textbox", { name: "Domain" })).toHaveValue("not a domain");
    expect(screen.getByRole("radio", { name: "Reject reports" })).toBeChecked();
  });
});

describe("the way from a remote server", () => {
  test("a server's page links to its domain actions, which are listed, and the new action is filled in with the domain", async () => {
    const api = domainActionsServer([
      limit,
      domainActionFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000a3", domain: "other.example" }),
    ]);
    const { user, router } = await renderApp({ path: "/moderation/instances/spam.example" });

    const link = await within(await screen.findByRole("main")).findByRole("link", { name: "Domain actions" });
    expect(link).toHaveAccessibleDescription("See the moderation actions on this server's domain, or take a new one.");
    await user.click(link);

    expect(await screen.findByRole("heading", { level: 1, name: "Domain actions" })).toBeInTheDocument();
    expect(router.state.location.search).toEqual({ domain: "spam.example" });
    expect(screen.getByText(`Showing the actions on ${isolated("spam.example")}.`)).toBeInTheDocument();
    expect(
      within(screen.getByRole("table"))
        .getAllByRole("rowheader")
        .map((header) => header.textContent),
    ).toEqual(["spam.example"]);

    await user.click(screen.getByRole("link", { name: "New action" }));
    expect(await screen.findByRole("textbox", { name: "Domain" })).toHaveValue("spam.example");
    expect(router.state.location.pathname).toBe("/moderation/domain-actions/new");

    await router.navigate({ to: "/moderation/domain-actions", search: { domain: "spam.example" } });
    await user.click(await screen.findByRole("link", { name: "Show every domain" }));
    // The list of one domain stays on screen, or the pending page replaces it, while the list of every domain loads.
    expect(screen.getByRole("table")).toContainElement(await screen.findByRole("rowheader", { name: "other.example" }));
    expect(api.lists).toEqual(["limit=40&active=true&domain=spam.example", "limit=40&active=true"]);
  });
});

describe("permissions", () => {
  test("the screen is in the moderation menu of a user with manage_domains", async () => {
    await renderApp({ path: "/moderation" });

    const menu = await screen.findByRole("group", { name: "Federation" });
    expect(within(menu).getByRole("link", { name: "Domain actions" })).toHaveAttribute(
      "href",
      "/moderation/domain-actions",
    );
  });

  test.each(["/moderation/domain-actions", `/moderation/domain-actions/${limit.id}`, "/moderation/domain-actions/new"])(
    "a user without manage_domains is told why on %s, and nothing is asked of the server",
    async (path) => {
      server.use(operatorHandler(["manage_jobs"]));
      const api = domainActionsServer([limit]);
      await renderApp({ path });

      expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
      expect(api.lists).toEqual([]);
    },
  );
});
