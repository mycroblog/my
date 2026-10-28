// SPDX-License-Identifier: AGPL-3.0-only
// The account search and an account's admin page through the whole app (web-client/README.md T4): a moderator with
// manage_accounts searches the local and remote accounts, opens one, shows its email and IP addresses only by
// pressing "show", and takes and lifts moderation actions on it; an administrator also removes two-factor
// authentication and issues a password reset link, shown once. MSW plays the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { accountFixture, otherAccountFixture, remoteAccountFixture, type Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import {
  accountActionFixture,
  adminAccountDetailFixture,
  adminAccountFixture,
  newAccountAction,
  reportFixture,
  signupBlockFixture,
} from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_accounts", "manage_reports", "view_audit"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Detail = Schemas["AdminAccountDetail"];

/** A name inside a sentence, as the screens isolate it (U+2068 … U+2069). */
function isolated(name: string): string {
  return `⁨${name}⁩`;
}

/** The requests to the account screens' operations the app sent, as "METHOD path?query". */
function recordAccountRequests() {
  const requests: string[] = [];
  server.events.on("request:start", ({ request }) => {
    const url = new URL(request.url);
    const path = url.pathname.slice(apiBase.length);
    if (path.startsWith("/admin/account")) requests.push(`${request.method} ${path}${url.search}`);
  });
  return requests;
}

function region(name: string) {
  return screen.getByRole("region", { name });
}

/** The rows of the table in `container`, without its header row. */
function rows(container: HTMLElement) {
  return within(container).queryAllByRole("row").slice(1);
}

const dave = accountFixture(4, "Dave", "my.example");
const bob = adminAccountFixture(otherAccountFixture, {
  state: "limited",
  restrictions: ["visibility_ceiling_private", "public_discovery"],
});
const carol = adminAccountFixture(remoteAccountFixture, {
  state: "suspended",
  restrictions: ["profile_view", "federation"],
});

describe("the account search", () => {
  test("lists the accounts newest first with their state and server, and never their email or IP addresses", async () => {
    server.use(http.get(`${apiBase}/admin/accounts`, () => HttpResponse.json({ items: [bob, carol] })));
    await renderApp({ path: "/moderation/accounts" });

    expect(await screen.findByRole("heading", { level: 1, name: "Accounts" })).toBeInTheDocument();
    const results = region("Results");
    expect(await within(results).findByRole("link", { name: /^Bob\s?@bob$/ })).toHaveAttribute(
      "href",
      `/moderation/accounts/${otherAccountFixture.id}`,
    );
    const first = within(rows(results)[0] as HTMLElement);
    expect(first.getByText("Limited")).toBeInTheDocument();
    expect(first.getByText("This server")).toBeInTheDocument();
    expect(first.getByText("Approved")).toBeInTheDocument();
    expect(
      first.getByText("Posts are followers-only and Posts stay off the public timelines and search"),
    ).toBeInTheDocument();
    const second = within(rows(results)[1] as HTMLElement);
    expect(second.getByRole("link", { name: /^Carol\s?@carol@remote\.example$/ })).toBeInTheDocument();
    expect(second.getByText("Suspended")).toBeInTheDocument();
    expect(second.getByText("remote.example")).toHaveAttribute("dir", "ltr");
    expect(second.queryByText("Registration")).not.toBeInTheDocument();
  });

  test("searches by keyboard with the conditions in the URL: a handle, a domain in its A-labels, the origin, the state and an IP address", async () => {
    const requests = recordAccountRequests();
    server.use(
      http.get(`${apiBase}/admin/accounts`, ({ request }) => {
        const query = new URL(request.url).searchParams;
        return HttpResponse.json({ items: query.has("ip") ? [bob] : [bob, carol] });
      }),
    );
    const { user, router } = await renderApp({ path: "/moderation/accounts" });

    const search = await screen.findByRole("region", { name: "Search" });
    await user.type(within(search).getByRole("textbox", { name: "Username" }), "@Carol@例え.jp");
    await user.click(within(search).getByRole("button", { name: /State/ }));
    await user.click(await screen.findByRole("option", { name: "Suspended" }));
    await user.click(within(search).getByRole("button", { name: /Where/ }));
    await user.click(await screen.findByRole("option", { name: "Other servers" }));
    await user.click(within(search).getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(router.state.location.search).toEqual({
        username: "Carol",
        domain: "xn--r8jz45g.jp",
        origin: "remote",
        state: "suspended",
      });
    });
    // The form starts from the new search.
    expect(await within(region("Search")).findByRole("textbox", { name: "Server" })).toHaveValue("xn--r8jz45g.jp");

    const form = region("Search");
    await user.clear(within(form).getByRole("textbox", { name: "Username" }));
    await user.clear(within(form).getByRole("textbox", { name: "Server" }));
    await user.click(within(form).getByRole("button", { name: /Email or IP address/ }));
    await user.click(await screen.findByRole("option", { name: "IP address" }));
    const ip = within(form).getByRole("textbox", { name: "IP address" });
    // Required once chosen: the browser's validation stops an empty search.
    await user.click(within(form).getByRole("button", { name: "Search" }));
    expect(ip).toBeInvalid();
    await user.type(ip, " 192.0.2.7 ");
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(router.state.location.search).toEqual({ origin: "remote", state: "suspended", ip: "192.0.2.7" });
    });
    await waitFor(() => {
      expect(rows(region("Results"))).toHaveLength(1);
    });
    expect(requests).toEqual([
      "GET /admin/accounts?limit=40",
      "GET /admin/accounts?username=Carol&domain=xn--r8jz45g.jp&origin=remote&state=suspended&limit=40",
      "GET /admin/accounts?origin=remote&state=suspended&ip=192.0.2.7&limit=40",
    ]);
  });

  test("a search the server refuses says why in the results, and tries again", async () => {
    let refuse = true;
    server.use(
      http.get(`${apiBase}/admin/accounts`, () =>
        refuse ? problem(400, "about:blank", "ip is not an IP address.") : HttpResponse.json({ items: [bob] }),
      ),
    );
    const { user } = await renderApp({ path: "/moderation/accounts?ip=nope" });

    const results = await screen.findByRole("region", { name: "Results" });
    expect(
      await within(results).findByRole("heading", { level: 3, name: "Could not search the accounts" }),
    ).toBeInTheDocument();
    expect(within(results).getByText("ip is not an IP address.")).toBeInTheDocument();
    // The form keeps the search that failed.
    expect(within(region("Search")).getByRole("textbox", { name: "IP address" })).toHaveValue("nope");
    refuse = false;
    await user.click(within(results).getByRole("button", { name: "Try again" }));
    expect(await within(results).findByRole("link", { name: /^Bob\s?@bob$/ })).toBeInTheDocument();
  });

  test("a page that found nothing yet offers to search further", async () => {
    server.use(
      http.get(`${apiBase}/admin/accounts`, ({ request }) => {
        const cursor = new URL(request.url).searchParams.get("cursor");
        return HttpResponse.json(cursor === null ? { items: [], next_cursor: "c1" } : { items: [carol] });
      }),
    );
    const { user } = await renderApp({ path: "/moderation/accounts?state=suspended" });

    const results = await screen.findByRole("region", { name: "Results" });
    expect(
      await within(results).findByRole("heading", { level: 3, name: "No accounts match so far" }),
    ).toBeInTheDocument();
    await user.click(within(results).getByRole("button", { name: "Search further" }));
    expect(await within(results).findByRole("link", { name: /^Carol\s?@carol@remote\.example$/ })).toBeInTheDocument();
  });

  test("an empty search says so", async () => {
    await renderApp({ path: "/moderation/accounts?username=nobody" });

    expect(await screen.findByRole("heading", { level: 3, name: "No accounts match" })).toBeInTheDocument();
  });

  test("is in the moderation menu of a viewer with manage_accounts", async () => {
    await renderApp({ path: "/moderation" });

    expect(
      await within(await screen.findByRole("navigation", { name: "Admin and moderation" })).findByRole("link", {
        name: "Accounts",
      }),
    ).toHaveAttribute("href", "/moderation/accounts");
  });

  test("a viewer without manage_accounts sees why, and nothing is asked of the server", async () => {
    const requests = recordAccountRequests();
    server.use(operatorHandler(["manage_reports"]));
    await renderApp({ path: "/moderation/accounts" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(requests).toEqual([]);
  });
});

/** Plays one account of the server: its details, its actions, new actions and lifts; records the requests. */
function accountServer(initial: Detail, actions: Schemas["AdminAccountAction"][] = []) {
  let detail = initial;
  let list = actions;
  const created: Schemas["CreateAccountActionRequest"][] = [];
  const lifts: { id: string; body: unknown }[] = [];
  server.use(
    http.get(`${apiBase}/admin/accounts/:id`, ({ params }) =>
      params["id"] === detail.id ? HttpResponse.json(detail) : problem(404, "about:blank", "Not Found"),
    ),
    http.get(`${apiBase}/admin/accounts/:id/actions`, () => HttpResponse.json({ items: list })),
    // The search finds the account as it is now.
    http.get(`${apiBase}/admin/accounts`, () =>
      HttpResponse.json({
        items: [adminAccountFixture(detail.account, { state: detail.state, restrictions: detail.restrictions })],
      }),
    ),
    http.post(`${apiBase}/admin/accounts/:id/actions`, async ({ request, params }) => {
      const body = (await request.json()) as Schemas["CreateAccountActionRequest"];
      created.push(body);
      const action = newAccountAction(String(params["id"]), body);
      list = [action, ...list];
      detail = { ...detail, state: body.kind === "suspend" ? "suspended" : detail.state };
      return HttpResponse.json(action);
    }),
    http.post(`${apiBase}/admin/account_actions/:id/lift`, async ({ request, params }) => {
      const id = String(params["id"]);
      const body = (await request.json()) as Schemas["LiftAccountActionRequest"];
      lifts.push({ id, body });
      const found = list.find((action) => action.id === id);
      if (found === undefined) return problem(404, "about:blank", "Not Found");
      const lifted: Schemas["AdminAccountAction"] = {
        ...found,
        state: "lifted",
        lifted_at: "2026-10-17T00:00:00Z",
        ...(body.reason === undefined ? {} : { lift_reason: body.reason }),
      };
      list = list.map((action) => (action.id === id ? lifted : action));
      detail = { ...detail, state: "active", restrictions: [] };
      return HttpResponse.json(lifted);
    }),
  );
  return { created, lifts };
}

/** From now on every search takes a while; the handler falls through to accountServer's. */
function slowSearches() {
  server.use(
    http.get(`${apiBase}/admin/accounts`, async () => {
      await delay(300);
    }),
  );
}

const bobDetail = adminAccountDetailFixture(otherAccountFixture, {
  state: "limited",
  restrictions: ["visibility_ceiling_private", "public_discovery"],
  roles: ["moderator"],
  registration: {
    submitted_at: "2026-10-01T00:00:00Z",
    state_changed_at: "2026-10-01T01:00:00Z",
    reason: "I like birds.",
  },
  reports: [reportFixture({ id: "0192f0c4-7a8e-7000-8000-0000000c0007", comment: "Spam again." })],
});
const limit = accountActionFixture({ public_reason: "Spam links." });
const emailHash = "5d41402abc4b2a76b9719d911017c592ae0b4f1c7ad8e2c6f5a1b0e9d8c7b6a5";
const ban = accountActionFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000d00a0",
  kind: "ban",
  issued_at: "2026-09-01T00:00:00Z",
  state: "active",
});

describe("an account's admin page", () => {
  test("shows the account, its restrictions, its actions and its reports, without asking for its email or IP addresses", async () => {
    const requests = recordAccountRequests();
    accountServer(bobDetail, [limit]);
    await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Account" })).toBeInTheDocument();
    const summary = region("Account");
    expect(within(summary).getByText("Limited")).toBeInTheDocument();
    expect(within(summary).getByText("This server")).toBeInTheDocument();
    expect(within(summary).getByText("Verified")).toBeInTheDocument();
    expect(within(summary).getByText("moderator")).toHaveAttribute("dir", "ltr");
    expect(within(summary).getByText("I like birds.")).toHaveAttribute("dir", "auto");
    expect(within(summary).getByRole("link", { name: "Open the profile" })).toHaveAttribute("href", "/@bob");
    expect(
      within(summary).getByRole("link", { name: "Show operations on the account in the audit log" }),
    ).toHaveAttribute("href", `/moderation/audit-log?target_kind=account&target=${bobDetail.id}`);
    expect(
      within(within(region("Account")).getByRole("list", { name: "Restrictions" }))
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["Posts are followers-only", "Posts stay off the public timelines and search"]);
    const history = region("Actions on the account");
    expect(await within(history).findByRole("heading", { level: 3, name: "Limit" })).toBeInTheDocument();
    expect(within(history).getByText("Spam links.")).toHaveAttribute("dir", "auto");
    const reports = region("Reports on the account");
    expect(within(reports).getByRole("link", { name: /^Report of / })).toHaveAttribute(
      "href",
      "/moderation/reports/0192f0c4-7a8e-7000-8000-0000000c0007",
    );

    const privateData = region("Email and IP addresses");
    expect(within(privateData).getByRole("button", { name: "Show email and IP addresses" })).toBeInTheDocument();
    expect(screen.queryByText("bob@example.com")).not.toBeInTheDocument();
    expect(requests.filter((request) => request.includes("/reveal"))).toEqual([]);
  });

  test("shows the invite the account signed up with and who created it", async () => {
    const invited = adminAccountDetailFixture(otherAccountFixture, {
      registration: {
        submitted_at: "2026-10-01T00:00:00Z",
        state_changed_at: "2026-10-01T00:00:00Z",
        invite_id: "0192f0c4-7a8e-7000-8000-0000000006a1",
        invite: {
          id: "0192f0c4-7a8e-7000-8000-0000000006a1",
          creator_kind: "member",
          created_by: dave,
          created_by_deleted: false,
        },
      },
    });
    accountServer(invited);
    const { unmount } = await renderApp({ path: `/moderation/accounts/${invited.id}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Account" })).toBeInTheDocument();
    const summary = region("Account");
    expect(within(summary).getByText("Member invite")).toBeInTheDocument();
    expect(within(summary).getByRole("link", { name: "@dave" })).toHaveAttribute(
      "href",
      `/moderation/accounts/${dave.id}`,
    );
    unmount();

    const byCli = adminAccountDetailFixture(otherAccountFixture, {
      registration: {
        submitted_at: "2026-10-01T00:00:00Z",
        state_changed_at: "2026-10-01T00:00:00Z",
        invite_id: "0192f0c4-7a8e-7000-8000-0000000006a2",
        invite: { id: "0192f0c4-7a8e-7000-8000-0000000006a2", creator_kind: "staff", created_by_deleted: false },
      },
    });
    accountServer(byCli);
    await renderApp({ path: `/moderation/accounts/${byCli.id}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Account" })).toBeInTheDocument();
    expect(within(region("Account")).getByText("Moderator invite")).toBeInTheDocument();
    expect(within(region("Account")).getByText("Command line")).toBeInTheDocument();
  });

  test("shows the email and IP addresses only when asked, once, with the other accounts of the same addresses", async () => {
    const requests = recordAccountRequests();
    accountServer(bobDetail);
    server.use(
      http.post(`${apiBase}/admin/accounts/:id/reveal`, () =>
        HttpResponse.json({
          email: "bob@birds.example",
          email_verified: false,
          registration_ip: "2001:db8::7",
          registered_at: "2026-10-01T00:00:00Z",
          sign_ins: [
            { ip: "192.0.2.7", user_agent: "Mozilla/5.0 (X11; Linux x86_64)", signed_in_at: "2026-10-16T08:00:00Z" },
            { signed_in_at: "2026-10-15T08:00:00Z" },
          ],
          same_ip_accounts: [{ ip: "192.0.2.7", accounts: [adminAccountFixture(dave, { state: "suspended" })] }],
        }),
      ),
    );
    const { user } = await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    const privateData = await screen.findByRole("region", { name: "Email and IP addresses" });
    await user.click(within(privateData).getByRole("button", { name: "Show email and IP addresses" }));

    expect(await within(privateData).findByText("bob@birds.example")).toHaveAttribute("dir", "ltr");
    await waitFor(() => {
      expect(within(privateData).getByRole("heading", { level: 2 })).toHaveFocus();
    });
    expect(within(privateData).getByRole("status")).toHaveTextContent(
      "Email and IP addresses shown. This was recorded in the audit log.",
    );
    expect(within(privateData).getByText("Not verified")).toBeInTheDocument();
    expect(within(privateData).getByText("2001:db8::7")).toBeInTheDocument();
    expect(
      within(privateData).getByRole("link", { name: `Search the accounts with email at ⁦birds.example⁩` }),
    ).toHaveAttribute("href", "/moderation/accounts?email_domain=birds.example");
    expect(within(privateData).getByText("Mozilla/5.0 (X11; Linux x86_64)")).toHaveAttribute("dir", "ltr");
    expect(within(privateData).getAllByText("Unknown")).toHaveLength(2);
    expect(within(privateData).getByRole("link", { name: /^Dave\s?@dave$/ })).toHaveAttribute(
      "href",
      `/moderation/accounts/${dave.id}`,
    );
    expect(within(privateData).getByRole("link", { name: "Search the accounts of ⁦192.0.2.7⁩" })).toHaveAttribute(
      "href",
      "/moderation/accounts?ip=192.0.2.7",
    );
    expect(within(privateData).queryByRole("button", { name: "Show email and IP addresses" })).not.toBeInTheDocument();
    expect(requests.filter((request) => request.includes("/reveal"))).toEqual([
      `POST /admin/accounts/${bobDetail.id}/reveal`,
    ]);
  });

  test("a failure to show the email and IP addresses is a toast, and the button stays", async () => {
    accountServer(bobDetail);
    server.use(
      http.post(`${apiBase}/admin/accounts/:id/reveal`, () => problem(503, "about:blank", "Service Unavailable")),
    );
    const { user } = await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    const privateData = await screen.findByRole("region", { name: "Email and IP addresses" });
    await user.click(within(privateData).getByRole("button", { name: "Show email and IP addresses" }));

    expect(
      await screen.findByText("Could not show the email and IP addresses: Service Unavailable"),
    ).toBeInTheDocument();
    expect(within(privateData).getByRole("button", { name: "Show email and IP addresses" })).toBeInTheDocument();
  });

  test("a remote account has no email or IP addresses, and takes only the kinds of action a remote account takes", async () => {
    const carolDetail = adminAccountDetailFixture(remoteAccountFixture);
    const api = accountServer(carolDetail);
    const { user } = await renderApp({ path: `/moderation/accounts/${carolDetail.id}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Account" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Email and IP addresses" })).not.toBeInTheDocument();
    const action = region("Take action");
    expect(within(action).getAllByRole("radio")).toHaveLength(3);
    expect(within(action).getByRole("radio", { name: "Mark media sensitive" })).toBeInTheDocument();
    expect(within(action).getByRole("radio", { name: "Limit" })).toBeInTheDocument();
    expect(within(action).getByRole("radio", { name: "Suspend" })).toBeInTheDocument();
    expect(within(action).queryByRole("radio", { name: "Warn" })).not.toBeInTheDocument();
    expect(within(action).queryByRole("checkbox", { name: "Tell the account" })).not.toBeInTheDocument();

    await user.click(within(action).getByRole("radio", { name: "Suspend" }));
    await user.click(within(action).getByRole("button", { name: "Take action" }));

    expect(await within(action).findByText("Action taken (Suspend).")).toBeInTheDocument();
    expect(api.created).toEqual([{ kind: "suspend", reason_category: "other", notify_target: false }]);
    // The account is read again: its state and its actions.
    expect(await within(region("Account")).findByText("Suspended")).toBeInTheDocument();
    expect(
      await within(region("Actions on the account")).findByRole("heading", { level: 3, name: "Suspend" }),
    ).toBeInTheDocument();
  });

  test("a local account takes every kind but those on posts, told by default", async () => {
    const api = accountServer(bobDetail);
    const { user } = await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    const action = await screen.findByRole("region", { name: "Take action" });
    expect(within(action).getAllByRole("radio")).toHaveLength(8);
    expect(within(action).queryByRole("radio", { name: "Delete posts" })).not.toBeInTheDocument();
    await user.click(within(action).getByRole("radio", { name: "Restrict posting" }));
    await user.click(within(action).getByRole("button", { name: /Duration/ }));
    await user.click(await screen.findByRole("option", { name: "7 days" }));
    await user.type(within(action).getByRole("textbox", { name: "Private note" }), "Second time.");
    await user.click(within(action).getByRole("button", { name: "Take action" }));

    expect(await within(action).findByText("Action taken (Restrict posting).")).toBeInTheDocument();
    expect(api.created).toEqual([
      {
        kind: "restrict_posting",
        reason_category: "other",
        private_note: "Second time.",
        notify_target: true,
        expires_in: 7 * 86_400,
      },
    ]);
  });

  test("a ban of a local account can also block sign-ups with its email address, which the list of sign-up blocks shows at once; only a ban offers it", async () => {
    const api = accountServer(bobDetail);
    // The server adds the block with the ban.
    let blocks: Schemas["AdminSignupBlock"][] = [];
    server.use(
      http.get(`${apiBase}/admin/signup_blocks`, () => HttpResponse.json({ items: blocks })),
      http.post(`${apiBase}/admin/accounts/:id/actions`, async ({ request }) => {
        const body = (await request.clone().json()) as Schemas["CreateAccountActionRequest"];
        if (body.block_email === true)
          blocks = [signupBlockFixture({ kind: "email_hash", value: emailHash, moderation_action_id: ban.id })];
        // Falls through to accountServer's handler, which takes the action.
      }),
    );
    // The list is read once before the ban, and kept fresh for a while (staleTime).
    const { user, router } = await renderApp({ path: "/moderation/signup-blocks" });
    expect(await screen.findByRole("heading", { level: 1, name: "Sign-up blocks" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 3, name: "No sign-up blocks in effect" })).toBeInTheDocument();
    await router.navigate({ to: "/moderation/accounts/$id", params: { id: bobDetail.id } });

    const action = await screen.findByRole("region", { name: "Take action" });
    await user.click(within(action).getByRole("radio", { name: "Suspend" }));
    expect(
      within(action).queryByRole("checkbox", { name: "Block sign-ups with this email address" }),
    ).not.toBeInTheDocument();
    await user.click(within(action).getByRole("radio", { name: "Ban" }));
    const blockEmail = within(action).getByRole("checkbox", { name: "Block sign-ups with this email address" });
    expect(blockEmail).not.toBeChecked();
    expect(blockEmail).toHaveAccessibleDescription(
      /^New sign-ups with the email address of the account's user are rejected too\./,
    );
    await user.click(blockEmail);
    await user.click(within(action).getByRole("button", { name: "Take action" }));
    const confirmation = await screen.findByRole("alertdialog", { name: `Ban ${isolated("@bob")}?` });
    await user.click(within(confirmation).getByRole("button", { name: "Ban" }));

    expect(await within(action).findByText("Action taken (Ban).")).toBeInTheDocument();
    expect(api.created).toEqual([{ kind: "ban", reason_category: "other", notify_target: true, block_email: true }]);

    await router.navigate({ to: "/moderation/signup-blocks" });
    expect(
      await within(await screen.findByRole("table")).findByRole("rowheader", { name: emailHash }),
    ).toBeInTheDocument();
  });

  test("a ban of a local account without an email address offers no block of it", async () => {
    const { email_verified: _, ...withoutEmail } = bobDetail;
    accountServer(withoutEmail);
    const { user } = await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    const action = await screen.findByRole("region", { name: "Take action" });
    await user.click(within(action).getByRole("radio", { name: "Ban" }));
    expect(
      within(action).queryByRole("checkbox", { name: "Block sign-ups with this email address" }),
    ).not.toBeInTheDocument();
  });

  test("lifts an action after its confirmation with an optional reason, and keeps focus on its row; a ban has no lift", async () => {
    const api = accountServer(bobDetail, [limit, ban]);
    const { user } = await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    const history = await screen.findByRole("region", { name: "Actions on the account" });
    expect(await within(history).findByRole("heading", { level: 3, name: "Ban" })).toBeInTheDocument();
    expect(within(history).getAllByRole("button", { name: /^Lift / })).toHaveLength(1);
    await user.click(within(history).getByRole("button", { name: /^Lift Limit, taken / }));

    const dialog = await screen.findByRole("alertdialog", { name: `Lift Limit on ${isolated("@bob")}?` });
    const reason = within(dialog).getByRole("textbox", { name: "Reason for lifting" });
    await waitFor(() => {
      expect(reason).toHaveFocus();
    });
    await user.type(reason, "  Appealed by email.  ");
    await user.click(within(dialog).getByRole("button", { name: "Lift" }));

    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(api.lifts).toEqual([{ id: limit.id, body: { reason: "Appealed by email." } }]);
    expect(await within(history).findByText("Appealed by email.")).toBeInTheDocument();
    expect(within(history).getByRole("status")).toHaveTextContent("Limit is lifted.");
    expect(within(history).queryAllByRole("button", { name: /^Lift / })).toHaveLength(0);
    await waitFor(() => {
      expect(within(history).getByRole("heading", { level: 3, name: "Limit" })).toHaveFocus();
    });
    // The account is read again: nothing is taken away now.
    expect(
      await within(region("Account")).findByText("The account's actions take nothing away now."),
    ).toBeInTheDocument();
  });

  test("a lift the server refuses stays in the confirmation to try again; cancel keeps the action", async () => {
    accountServer(bobDetail, [limit]);
    server.use(
      http.post(`${apiBase}/admin/account_actions/:id/lift`, () => problem(503, "about:blank", "Service Unavailable")),
    );
    const { user } = await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    const history = await screen.findByRole("region", { name: "Actions on the account" });
    const lift = await within(history).findByRole("button", { name: /^Lift Limit, taken / });
    await user.click(lift);
    const dialog = await screen.findByRole("alertdialog");
    // Without a reason, the request asks for nothing more.
    await user.click(within(dialog).getByRole("button", { name: "Lift" }));
    expect(await within(dialog).findByText("Could not lift the action: Service Unavailable")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(lift).toHaveFocus();
    });
  });

  // #471: the search, shown from its cache as the route loads it, must not show the account as it was before
  // the action or the lift while a slow server answers the search again.
  test("back on the search after an action, the account shows its new state at once, however slowly the search comes", async () => {
    const carolDetail = adminAccountDetailFixture(remoteAccountFixture);
    const api = accountServer(carolDetail);
    const { user } = await renderApp({ path: "/moderation/accounts" });
    await user.click(
      await within(await screen.findByRole("region", { name: "Results" })).findByRole("link", { name: /^Carol/ }),
    );
    const action = await screen.findByRole("region", { name: "Take action" });
    await user.click(within(action).getByRole("radio", { name: "Suspend" }));
    slowSearches();
    await user.click(within(action).getByRole("button", { name: "Take action" }));
    expect(await within(action).findByText("Action taken (Suspend).")).toBeInTheDocument();
    expect(api.created).toHaveLength(1);

    await user.click(screen.getByRole("link", { name: "Back to the accounts" }));
    const results = await screen.findByRole("region", { name: "Results" });
    expect(await within(results).findByRole("link", { name: /^Carol/ })).toBeInTheDocument();
    const [row] = rows(results);
    if (row === undefined) throw new Error("Carol is not listed");
    expect(within(row).getByText("Suspended")).toBeInTheDocument();
  });

  test("back on the search after a lift, the account shows its new state at once, however slowly the search comes", async () => {
    const api = accountServer(bobDetail, [limit]);
    const { user } = await renderApp({ path: "/moderation/accounts" });
    await user.click(
      await within(await screen.findByRole("region", { name: "Results" })).findByRole("link", { name: /^Bob/ }),
    );
    const history = await screen.findByRole("region", { name: "Actions on the account" });
    await user.click(await within(history).findByRole("button", { name: /^Lift Limit, taken / }));
    slowSearches();
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Lift" }));
    expect(await within(history).findByRole("status")).toHaveTextContent("Limit is lifted.");
    expect(api.lifts).toHaveLength(1);

    await user.click(screen.getByRole("link", { name: "Back to the accounts" }));
    const results = await screen.findByRole("region", { name: "Results" });
    expect(await within(results).findByRole("link", { name: /^Bob/ })).toBeInTheDocument();
    const [row] = rows(results);
    if (row === undefined) throw new Error("Bob is not listed");
    expect(within(row).getByText("Active")).toBeInTheDocument();
    expect(within(row).queryByText("Limited")).not.toBeInTheDocument();
  });

  test("removes the two-factor authentication of a local account after its confirmation, and keeps focus in its region", async () => {
    const requests = recordAccountRequests();
    let detail: Detail = { ...bobDetail, two_factor_enabled: true };
    server.use(
      operatorHandler(["manage_accounts", "manage_reports", "view_audit", "manage_login_recovery"]),
      http.get(`${apiBase}/admin/accounts/:id`, () => HttpResponse.json(detail)),
      http.post(`${apiBase}/admin/accounts/:id/two_factor/disable`, () => {
        detail = { ...detail, two_factor_enabled: false };
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    const twoFactor = await screen.findByRole("region", { name: "Two-factor authentication" });
    expect(
      within(twoFactor).getByText("On: signing in asks for a code from an authenticator app."),
    ).toBeInTheDocument();
    await user.click(within(twoFactor).getByRole("button", { name: "Remove two-factor authentication" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: `Remove the two-factor authentication of ${isolated("@bob")}?`,
    });
    await user.click(within(dialog).getByRole("button", { name: "Remove two-factor authentication" }));

    expect(await within(twoFactor).findByRole("status")).toHaveTextContent(
      `Removed the two-factor authentication of ${isolated("@bob")}.`,
    );
    expect(within(twoFactor).getByText("Off.")).toBeInTheDocument();
    expect(
      within(twoFactor).queryByRole("button", { name: "Remove two-factor authentication" }),
    ).not.toBeInTheDocument();
    await waitFor(() => {
      expect(within(twoFactor).getByRole("heading", { level: 2 })).toHaveFocus();
    });
    expect(requests.filter((request) => request.includes("two_factor"))).toEqual([
      `POST /admin/accounts/${bobDetail.id}/two_factor/disable`,
    ]);
  });

  test("shows the email suppression of a local account and lifts it after its confirmation, keeping focus in its region", async () => {
    const requests = recordAccountRequests();
    let detail: Detail = { ...bobDetail, email_suppression: { kind: "hard_bounce", since: "2026-10-10T09:30:00Z" } };
    server.use(
      operatorHandler(["manage_accounts", "manage_reports", "view_audit"]),
      http.get(`${apiBase}/admin/accounts/:id`, () => HttpResponse.json(detail)),
      http.post(`${apiBase}/admin/accounts/:id/email_suppression/lift`, () => {
        const { email_suppression: _, ...lifted } = detail;
        detail = lifted;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    const region = await screen.findByRole("region", { name: "Email delivery" });
    expect(within(region).getByText(/an email to it bounced/)).toBeInTheDocument();
    expect(within(region).getByText(/^Since /)).toBeInTheDocument();
    await user.click(within(region).getByRole("button", { name: "Lift the suppression" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: `Lift the email suppression of ${isolated("@bob")}?`,
    });
    await user.click(within(dialog).getByRole("button", { name: "Lift the suppression" }));

    expect(await within(region).findByRole("status")).toHaveTextContent(
      `Lifted the email suppression of ${isolated("@bob")}.`,
    );
    expect(within(region).queryByRole("button", { name: "Lift the suppression" })).not.toBeInTheDocument();
    expect(within(region).queryByText(/an email to it bounced/)).not.toBeInTheDocument();
    await waitFor(() => {
      expect(within(region).getByRole("heading", { level: 2 })).toHaveFocus();
    });
    expect(requests.filter((request) => request.includes("email_suppression"))).toEqual([
      `POST /admin/accounts/${bobDetail.id}/email_suppression/lift`,
    ]);
  });

  test("an account without an email suppression shows no such region", async () => {
    accountServer(bobDetail);
    await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    expect(await screen.findByRole("region", { name: "Two-factor authentication" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Email delivery" })).not.toBeInTheDocument();
  });

  test("a moderator sees the two-factor authentication but cannot remove it", async () => {
    accountServer({ ...bobDetail, two_factor_enabled: true });
    await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    const twoFactor = await screen.findByRole("region", { name: "Two-factor authentication" });
    expect(
      within(twoFactor).getByText("On: signing in asks for a code from an authenticator app."),
    ).toBeInTheDocument();
    expect(
      within(twoFactor).queryByRole("button", { name: "Remove two-factor authentication" }),
    ).not.toBeInTheDocument();
  });

  test("a remote account has no two-factor authentication region", async () => {
    accountServer(adminAccountDetailFixture(remoteAccountFixture));
    await renderApp({ path: `/moderation/accounts/${remoteAccountFixture.id}` });

    expect(await screen.findByRole("region", { name: "Actions on the account" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Two-factor authentication" })).not.toBeInTheDocument();
  });

  test("issues a password reset link after its confirmation and shows it once, to copy", async () => {
    const requests = recordAccountRequests();
    const url = "https://my.example/auth/reset_password?token=rst_secret";
    accountServer({ ...bobDetail, two_factor_enabled: false });
    server.use(
      operatorHandler(["manage_accounts", "manage_reports", "view_audit", "manage_login_recovery"]),
      http.post(`${apiBase}/admin/accounts/:id/password_reset_link`, () =>
        HttpResponse.json({ url, expires_at: "2026-10-17T01:00:00Z" }, { status: 201 }),
      ),
    );
    const { user } = await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    const reset = await screen.findByRole("region", { name: "Password reset" });
    expect(within(reset).queryByText(url)).not.toBeInTheDocument();
    await user.click(within(reset).getByRole("button", { name: "Issue password reset link" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: `Issue a password reset link for ${isolated("@bob")}?`,
    });
    // Cancelling issues nothing.
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(requests.filter((request) => request.includes("password_reset_link"))).toEqual([]);

    await user.click(within(reset).getByRole("button", { name: "Issue password reset link" }));
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Issue password reset link" }),
    );

    expect(await within(reset).findByRole("status")).toHaveTextContent(
      `Issued a password reset link for ${isolated("@bob")}.`,
    );
    expect(within(reset).getByLabelText("Password reset link")).toHaveTextContent(url);
    expect(within(reset).getByText(/^This link is not shown again\./)).toBeInTheDocument();
    await user.click(within(reset).getByRole("button", { name: "Copy link" }));
    // user-event's clipboard stands in for the browser's.
    expect(await navigator.clipboard.readText()).toBe(url);
    expect(requests.filter((request) => request.includes("password_reset_link"))).toEqual([
      `POST /admin/accounts/${bobDetail.id}/password_reset_link`,
    ]);
  });

  test("a password reset link the server refuses stays in the confirmation to try again", async () => {
    accountServer({ ...bobDetail, two_factor_enabled: false });
    server.use(
      operatorHandler(["manage_accounts", "manage_reports", "view_audit", "manage_login_recovery"]),
      http.post(`${apiBase}/admin/accounts/:id/password_reset_link`, () =>
        problem(503, "about:blank", "Service Unavailable"),
      ),
    );
    const { user } = await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    const reset = await screen.findByRole("region", { name: "Password reset" });
    await user.click(within(reset).getByRole("button", { name: "Issue password reset link" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Issue password reset link" }));

    expect(await within(dialog).findByText("Could not issue the link: Service Unavailable")).toBeInTheDocument();
    expect(within(reset).queryByLabelText("Password reset link")).not.toBeInTheDocument();
  });

  test("a moderator without manage_login_recovery cannot issue a password reset link", async () => {
    const requests = recordAccountRequests();
    accountServer({ ...bobDetail, two_factor_enabled: false });
    await renderApp({ path: `/moderation/accounts/${bobDetail.id}` });

    expect(await screen.findByRole("region", { name: "Two-factor authentication" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Password reset" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Issue password reset link" })).not.toBeInTheDocument();
    expect(requests.filter((request) => request.includes("password_reset_link"))).toEqual([]);
  });

  test("an unknown account says it is not found", async () => {
    accountServer(bobDetail);
    await renderApp({ path: "/moderation/accounts/0192f0c4-7a8e-7000-8000-00000000ffff" });

    expect(await screen.findByRole("heading", { level: 1, name: "Account" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 2, name: /not found/i })).toBeInTheDocument();
  });
});
