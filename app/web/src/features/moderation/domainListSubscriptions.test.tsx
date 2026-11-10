// SPDX-License-Identifier: AGPL-3.0-only
// The blocklist subscriptions through the whole app (web-client/README.md T4, ADR-0159 §5): an operator with
// manage_domains lists the subscriptions, subscribes to a list and lands on its page, accepts and rejects its drafts
// one by one and together (a suspension only after its confirmation), fetches it now, changes its settings, and stops
// subscribing, with MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { domainListDraftFixture, domainListFetchFixture, domainListSubscriptionFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_domains"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Subscription = Schemas["AdminDomainListSubscription"];
type Draft = Schemas["AdminDomainListDraft"];

const subscription = domainListSubscriptionFixture();
const failing = domainListSubscriptionFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000000e2",
  title: "Mastodon server's blocks",
  format: "mastodon_json",
  mode: "automatic",
  consecutive_failures: 3,
  last_fetch: domainListFetchFixture({
    id: "0192f0c4-7a8e-7000-8000-0000000000f2",
    outcome: "failed",
    http_status: 404,
    error_code: "http_status",
  }),
});
const limitDraft = domainListDraftFixture();
const suspendDraft = domainListDraftFixture({
  key: "issue:suspend:spam.example",
  domain: "spam.example",
  kind: "suspend",
  severity: "suspend",
  public_comment: "Spam from every account.",
});
const liftDraft = domainListDraftFixture({
  key: "lift:limit:fixed.example",
  change: "lift",
  domain: "fixed.example",
  action_id: "0192f0c4-7a8e-7000-8000-0000000000a9",
});

/** Plays the subscriptions of the server; records what was created, changed, decided, fetched, and deleted. */
function subscriptionsServer(initial: Subscription[], initialDrafts: Draft[] = []) {
  let subscriptions = initial;
  let drafts = initialDrafts;
  const created: Schemas["CreateDomainListSubscriptionRequest"][] = [];
  const patches: Schemas["UpdateDomainListSubscriptionRequest"][] = [];
  const decisions: { decision: string; keys: string[] }[] = [];
  const fetches: string[] = [];
  const deleted: string[] = [];
  const find = (id: unknown) => subscriptions.find((item) => item.id === String(id));
  const decide = (decision: string) =>
    http.post(`${apiBase}/admin/domain_list_subscriptions/:id/drafts/${decision}`, async ({ request }) => {
      const { keys = [] } = (await request.json()) as Schemas["DomainListDraftsRequest"];
      decisions.push({ decision, keys });
      const done = drafts.filter((draft) => keys.includes(draft.key));
      drafts = drafts.filter((draft) => !keys.includes(draft.key));
      return HttpResponse.json({ done, missing: keys.filter((key) => !done.some((draft) => draft.key === key)) });
    });
  server.use(
    http.get(`${apiBase}/admin/domain_list_subscriptions`, () => HttpResponse.json({ items: subscriptions })),
    http.post(`${apiBase}/admin/domain_list_subscriptions`, async ({ request }) => {
      const body = (await request.json()) as Schemas["CreateDomainListSubscriptionRequest"];
      created.push(body);
      const { last_fetch: _, ...rest } = domainListSubscriptionFixture({
        id: "0192f0c4-7a8e-7000-8000-0000000000e9",
        title: body.title,
        url: body.url,
        format: body.format,
      });
      subscriptions = [rest, ...subscriptions];
      return HttpResponse.json(rest, { status: 201 });
    }),
    http.get(`${apiBase}/admin/domain_list_subscriptions/:id`, ({ params }) => {
      const found = find(params["id"]);
      return found === undefined ? new HttpResponse(null, { status: 404 }) : HttpResponse.json(found);
    }),
    http.patch(`${apiBase}/admin/domain_list_subscriptions/:id`, async ({ request, params }) => {
      const body = (await request.json()) as Schemas["UpdateDomainListSubscriptionRequest"];
      patches.push(body);
      const found = find(params["id"]);
      if (found === undefined) return new HttpResponse(null, { status: 404 });
      const next = { ...found, ...body, updated_at: "2026-10-17T00:00:00Z" };
      subscriptions = subscriptions.map((item) => (item.id === next.id ? next : item));
      return HttpResponse.json(next);
    }),
    http.delete(`${apiBase}/admin/domain_list_subscriptions/:id`, ({ request, params }) => {
      deleted.push(`${String(params["id"])}${new URL(request.url).search}`);
      subscriptions = subscriptions.filter((item) => item.id !== String(params["id"]));
      return new HttpResponse(null, { status: 204 });
    }),
    http.get(`${apiBase}/admin/domain_list_subscriptions/:id/fetches`, ({ params }) => {
      const found = find(params["id"]);
      return HttpResponse.json({ items: found?.last_fetch === undefined ? [] : [found.last_fetch] });
    }),
    http.post(`${apiBase}/admin/domain_list_subscriptions/:id/fetch`, ({ params }) => {
      fetches.push(String(params["id"]));
      return new HttpResponse(null, { status: 202 });
    }),
    http.get(`${apiBase}/admin/domain_list_subscriptions/:id/drafts`, () => HttpResponse.json({ items: drafts })),
    decide("accept"),
    decide("reject"),
  );
  return { created, patches, decisions, fetches, deleted };
}

/** The rows of a table, without its header row. */
function rowsOf(table: HTMLElement) {
  return within(table).queryAllByRole("row").slice(1);
}

describe("the list of subscriptions", () => {
  test("shows each subscription's mode and last fetch, with the failures in a row", async () => {
    subscriptionsServer([subscription, failing]);
    await renderApp({ path: "/moderation/domain-actions/subscriptions" });

    const pages = await screen.findByRole("navigation", { name: "Domain action pages" });
    expect(within(pages).getByRole("link", { name: "Blocklist subscriptions" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Add subscription" })).toHaveAttribute(
      "href",
      "/moderation/domain-actions/subscriptions/new",
    );
    const rows = rowsOf(await screen.findByRole("table"));
    const [first] = rows;
    if (first === undefined) throw new Error("no row");
    expect(within(first).getByRole("link", { name: "Shared blocklist" })).toHaveAttribute(
      "href",
      `/moderation/domain-actions/subscriptions/${subscription.id}`,
    );
    expect(rows[0]).toHaveTextContent("Drafts");
    expect(rows[0]).toHaveTextContent("Applied");
    expect(rows[1]).toHaveTextContent("Automatic");
    expect(rows[1]).toHaveTextContent("Failed: the server answered with an error (HTTP 404)");
    expect(rows[1]).toHaveTextContent("Failed 3 times in a row");
  });

  test("shows an empty state without subscriptions", async () => {
    subscriptionsServer([]);
    await renderApp({ path: "/moderation/domain-actions/subscriptions" });
    expect(await screen.findByRole("heading", { name: "No blocklist subscriptions" })).toBeInTheDocument();
  });
});

describe("a new subscription", () => {
  test("subscribes with the defaults and opens the subscription's page", async () => {
    const api = subscriptionsServer([]);
    const { user, router } = await renderApp({ path: "/moderation/domain-actions/subscriptions/new" });
    expect(await screen.findByRole("heading", { level: 1, name: "Add subscription" })).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Name" }), "Friends' list");
    await user.type(screen.getByRole("textbox", { name: "URL" }), "https://lists.example/friends.txt");
    await user.click(screen.getByRole("radio", { name: "One domain per line" }));
    await user.click(screen.getByRole("button", { name: "Subscribe" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Friends' list" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe(
      "/moderation/domain-actions/subscriptions/0192f0c4-7a8e-7000-8000-0000000000e9",
    );
    expect(api.created).toEqual([
      {
        title: "Friends' list",
        url: "https://lists.example/friends.txt",
        format: "plain_text",
        mode: "draft",
        suspend_as: "suspend",
        silence_as: "limit",
        apply_reject_media: true,
        apply_reject_reports: true,
        public_comment_from_list: false,
      },
    ]);
    expect(screen.getByText("Not fetched yet")).toBeInTheDocument();
  });
});

describe("a subscription's page", () => {
  const path = `/moderation/domain-actions/subscriptions/${subscription.id}`;

  test("accepts one draft without asking, and keeps focus in the table", async () => {
    const api = subscriptionsServer([subscription], [liftDraft, limitDraft]);
    const { user } = await renderApp({ path });
    const drafts = await screen.findByRole("region", { name: "Drafts" });
    expect(within(drafts).getByText("2 drafts are waiting.")).toBeInTheDocument();
    const table = within(drafts).getByRole("table");
    expect(rowsOf(table).map((row) => within(row).getByRole("rowheader").textContent)).toEqual([
      "fixed.example",
      "noisy.example",
    ]);
    expect(rowsOf(table)[0]).toHaveTextContent("Lift: Limit");
    await user.click(within(drafts).getByRole("button", { name: "Accept the draft of ⁦fixed.example⁩" }));

    expect(await within(drafts).findByText("1 draft accepted.")).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(api.decisions).toEqual([{ decision: "accept", keys: ["lift:limit:fixed.example"] }]);
    await waitFor(() => {
      expect(within(drafts).getByRole("button", { name: "Accept the draft of ⁦noisy.example⁩" })).toHaveFocus();
    });
  });

  test("accepting drafts that suspend asks first, and rejecting does not", async () => {
    const api = subscriptionsServer([subscription], [limitDraft, suspendDraft]);
    const { user } = await renderApp({ path });
    const drafts = await screen.findByRole("region", { name: "Drafts" });
    expect(within(drafts).getByText("Spam from every account.")).toHaveAttribute("dir", "auto");
    await user.click(within(drafts).getByRole("button", { name: "Accept all" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Suspend 1 domain?" });
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(api.decisions).toEqual([]);

    await user.click(within(drafts).getByRole("button", { name: "Reject the draft of ⁦spam.example⁩" }));
    expect(await within(drafts).findByText("1 draft rejected.")).toBeInTheDocument();
    await user.click(within(drafts).getByRole("button", { name: "Accept all" }));
    expect(await within(drafts).findByText("1 draft accepted.")).toBeInTheDocument();
    expect(api.decisions).toEqual([
      { decision: "reject", keys: ["issue:suspend:spam.example"] },
      { decision: "accept", keys: ["issue:limit:noisy.example"] },
    ]);
    expect(within(drafts).getByRole("heading", { name: "No drafts are waiting" })).toBeInTheDocument();
    await waitFor(() => {
      expect(within(drafts).getByRole("heading", { level: 2, name: "Drafts" })).toHaveFocus();
    });
  });

  test("accepts suspensions once confirmed", async () => {
    const api = subscriptionsServer([subscription], [suspendDraft]);
    const { user } = await renderApp({ path });
    const drafts = await screen.findByRole("region", { name: "Drafts" });
    await user.click(within(drafts).getByRole("button", { name: "Accept the draft of ⁦spam.example⁩" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Suspend 1 domain?" });
    await user.click(within(dialog).getByRole("button", { name: "Accept" }));
    expect(await within(drafts).findByText("1 draft accepted.")).toBeInTheDocument();
    expect(api.decisions).toEqual([{ decision: "accept", keys: ["issue:suspend:spam.example"] }]);
  });

  test("queues a fetch now and shows the fetches", async () => {
    const api = subscriptionsServer([failing]);
    const { user } = await renderApp({ path: `/moderation/domain-actions/subscriptions/${failing.id}` });
    const fetches = await screen.findByRole("region", { name: "Fetches" });
    expect(within(fetches).getByText(failing.url)).toHaveAttribute("dir", "ltr");
    expect(rowsOf(within(fetches).getByRole("table"))).toHaveLength(1);
    await user.click(within(fetches).getByRole("button", { name: "Fetch now" }));
    expect(
      await within(fetches).findByText("The fetch is queued. Its outcome shows here once it has run."),
    ).toBeInTheDocument();
    expect(api.fetches).toEqual([failing.id]);
  });

  test("saves only the settings that changed", async () => {
    const api = subscriptionsServer([subscription]);
    const { user } = await renderApp({ path });
    const settings = await screen.findByRole("region", { name: "Settings" });
    await user.click(within(settings).getByRole("button", { name: "Save settings" }));
    expect(await within(settings).findByText("Nothing to save.")).toBeInTheDocument();

    await user.click(within(settings).getByRole("radio", { name: "Automatic" }));
    await user.click(within(settings).getByRole("checkbox", { name: "Copy the list's public comments" }));
    await user.click(within(settings).getByRole("button", { name: "Save settings" }));
    expect(await within(settings).findByText("Settings saved. A fetch applies them.")).toBeInTheDocument();
    expect(api.patches).toEqual([{ mode: "automatic", public_comment_from_list: true }]);
  });

  test("stops subscribing after its alert dialog, lifting the actions when asked, and goes back to the list", async () => {
    const api = subscriptionsServer([subscription]);
    const { user, router } = await renderApp({ path });
    const section = await screen.findByRole("region", { name: "Stop subscribing" });
    await user.click(within(section).getByRole("button", { name: "Stop subscribing" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Stop subscribing to ⁨Shared blocklist⁩?" });
    await user.click(within(dialog).getByRole("checkbox", { name: "Lift the actions this subscription issued" }));
    await user.click(within(dialog).getByRole("button", { name: "Stop subscribing" }));

    expect(
      await screen.findByText("Stopped subscribing to ⁨Shared blocklist⁩ and lifted its actions."),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/moderation/domain-actions/subscriptions");
    });
    expect(api.deleted).toEqual([`${subscription.id}?lift_actions=true`]);
  });
});
