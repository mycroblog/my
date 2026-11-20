// SPDX-License-Identifier: AGPL-3.0-only

// The audit log (#3261, FR-718), on the fake API: every kind of operation the API names reads as a sentence that says
// who did it (a person, the command line, the server, another server) and what, its actor and its target linked where
// the reader may open their screens and plain otherwise, with readable labels for roles, settings, sign-up blocks,
// events and the closure; a kind the app does not know yet reads as an operation; the log filtered by its actor, its
// target and its kind; a one-person server leaves out who did what; and who may read it. What the screen shows at rest
// is the stories' (domains.stories.tsx).

import type { FakeApi } from "@my/e2e/fake";
import { describe, expect, test } from "bun:test";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

type Entry = ReturnType<FakeApi["domains"]["audit"]>[number];
type Action = Entry["action"];

/** `text` without the marks that isolate a name or a number inside a sentence. */
const plain = (text: string) =>
  text.replaceAll(String.fromCodePoint(0x2068), "").replaceAll(String.fromCodePoint(0x2069), "");

/** The address of a target's screen an entry links to: its whole address, or how it starts (an ID follows). */
type Leads = { to: string } | { under: string };

/**
 * What an entry of each kind says after its actor, and where its target leads, as the fake's log holds it: its targets
 * are @quickdeals, spam.example, the role moderator @marco, the setting registration_mode, the sign-up block
 * `email_domain spam.example`, the event Book swap, and the rest of domains.ts. A Record of the API's kinds: a kind the
 * specification adds and this table lacks fails `web:typecheck`.
 */
const kinds: Record<Action, readonly [says: string, leads: Leads | null]> = {
  account_action_issued: ["took an action on @quickdeals", { under: "/moderation/accounts/" }],
  account_action_lifted: ["lifted an action on @quickdeals", { under: "/moderation/accounts/" }],
  domain_action_issued: ["took an action on the domain spam.example", { to: "/moderation/instances/spam.example" }],
  domain_action_lifted: ["lifted an action on the domain spam.example", { to: "/moderation/instances/spam.example" }],
  report_assigned: ["assigned the report on @quickdeals", { under: "/moderation/reports/" }],
  report_unassigned: ["unassigned the report on @quickdeals", { under: "/moderation/reports/" }],
  report_resolved: ["resolved the report on @quickdeals", { under: "/moderation/reports/" }],
  report_reopened: ["reopened the report on @quickdeals", { under: "/moderation/reports/" }],
  report_note_added: ["added a note to the report on @quickdeals", { under: "/moderation/reports/" }],
  report_forwarded: ["sent the report on @quickdeals to the account's server", { under: "/moderation/reports/" }],
  registration_approved: ["approved the registration of @newcomer", { to: "/moderation/registrations" }],
  registration_rejected: ["rejected the registration of @newcomer", { to: "/moderation/registrations" }],
  setting_changed: ["changed the setting Who can sign up", null],
  text_changed: ["changed the server text Short description", null],
  rule_added: ["added the rule Be kind to each other.", null],
  rule_revised: ["revised the rule Be kind to each other.", null],
  rule_moved: ["moved the rule Be kind to each other.", null],
  rule_retired: ["retired the rule Be kind to each other.", null],
  role_granted: ["granted the role Moderator to @marco", null],
  role_revoked: ["revoked the role Moderator from @marco", null],
  role_created: ["created the role Moderator", null],
  role_updated: ["changed the role Moderator", null],
  role_deleted: ["deleted the role Moderator", null],
  // The appeal is gone: its account is named, and nothing is linked.
  appeal_upheld: ["upheld the action that @quickdeals appealed", null],
  appeal_overturned: ["overturned the action that @quickdeals appealed", null],
  account_deletion_canceled: ["cancelled the deletion of @quickdeals", { under: "/moderation/accounts/" }],
  revealed_private_data: [
    "was shown the email address and IP addresses of @quickdeals",
    { under: "/moderation/accounts/" },
  ],
  signup_block_added: ["blocked sign-ups from the email domain spam.example", null],
  signup_block_lifted: ["lifted the sign-up block on the email domain spam.example", null],
  hook_decision_applied: [
    "applied the automated review's decision on @quickdeals",
    { under: "/moderation/hook-decisions/" },
  ],
  hook_decision_overridden: [
    "cancelled the automated review's decision on @quickdeals",
    { under: "/moderation/hook-decisions/" },
  ],
  hook_appeal_upheld: [
    "upheld the refusal of a post that @quickdeals appealed",
    { under: "/moderation/hook-decisions/" },
  ],
  two_factor_disabled: ["removed the two-factor authentication of @quickdeals", { under: "/moderation/accounts/" }],
  password_reset_link_issued: ["issued a password reset link for @quickdeals", { under: "/moderation/accounts/" }],
  email_suppression_lifted: ["resumed email to @quickdeals", { under: "/moderation/accounts/" }],
  account_moved: [
    "moved the limited account @quickdeals to another account of this server",
    { under: "/moderation/accounts/" },
  ],
  custom_emoji_created: ["added the custom emoji :sourdough:", null],
  custom_emoji_image_replaced: ["replaced the image of the custom emoji :sourdough:", null],
  custom_emoji_copied: ["copied the custom emoji :sourdough: from another server", null],
  custom_emoji_updated: ["changed the custom emoji :sourdough:", null],
  custom_emoji_enabled: ["turned on the custom emoji :sourdough:", null],
  custom_emoji_disabled: ["turned off the custom emoji :sourdough:", null],
  custom_emoji_deleted: ["deleted the custom emoji :sourdough:", null],
  webhook_created: ["added the webhook Moderation alerts (hooks.example)", null],
  webhook_updated: ["changed the webhook Moderation alerts (hooks.example)", null],
  webhook_deleted: ["deleted the webhook Moderation alerts (hooks.example)", null],
  webhook_enabled: ["turned on the webhook Moderation alerts (hooks.example)", null],
  webhook_disabled: ["turned off the webhook Moderation alerts (hooks.example)", null],
  webhook_secret_rotated: ["made a new secret for the webhook Moderation alerts (hooks.example)", null],
  announcement_created: ["wrote the announcement Maintenance on Saturday", null],
  announcement_updated: ["changed the announcement Maintenance on Saturday", null],
  announcement_published: ["published the announcement Maintenance on Saturday", null],
  announcement_withdrawn: ["withdrew the announcement Maintenance on Saturday", null],
  invite_created: ["created the invite Q7xK", { to: "/moderation/invites" }],
  invite_revoked: ["revoked the invite Q7xK", { to: "/moderation/invites" }],
  relay_added: ["added the relay https://relay.example/inbox", null],
  relay_followed: ["followed the relay https://relay.example/inbox", null],
  relay_unfollowed: ["unfollowed the relay https://relay.example/inbox", null],
  relay_removed: ["removed the relay https://relay.example/inbox", null],
  relay_accepted: ["accepted the follow of https://relay.example/inbox", null],
  relay_rejected: ["rejected the follow of https://relay.example/inbox", null],
  status_refetch_queued: [
    "asked to refetch the post https://photos.example/users/sora/statuses/1",
    { to: "https://photos.example/users/sora/statuses/1" },
  ],
  account_statuses_refetch_queued: ["asked to refetch the posts of @quickdeals", { under: "/moderation/accounts/" }],
  domain_statuses_refetch_queued: [
    "asked to refetch the posts of the server spam.example",
    { to: "/moderation/instances/spam.example" },
  ],
  billing_import_queued: ["started importing the bills of AWS", null],
  cost_item_created: ["added the cost Object storage", null],
  cost_item_relabeled: ["renamed or recategorized the cost Object storage", null],
  cost_item_price_revised: ["revised the amount of the cost Object storage", null],
  cost_item_end_changed: ["changed the end of the cost Object storage", null],
  cost_item_deleted: ["deleted the cost Object storage", null],
  supporter_program_enabled: ["turned on the supporter plans with Stripe", null],
  supporter_program_disabled: ["turned off the supporter plans with Stripe", null],
  supporter_tips_enabled: ["turned on the tips (Stripe)", null],
  supporter_tips_revised: ["changed the amounts of the tips (Stripe)", null],
  supporter_tips_disabled: ["turned off the tips (Stripe)", null],
  supporter_plan_created: ["created the plan Friend of the bakery", null],
  supporter_plan_revised: ["revised the plan Friend of the bakery", null],
  supporter_plan_retired: ["retired the plan Friend of the bakery", null],
  supporter_subscription_ended: ["ended the subscription Friend of the bakery @layla", null],
  custom_emoji_request_submitted: ["requested the custom emoji :baguette:", null],
  custom_emoji_request_withdrawn: ["withdrew the request for the custom emoji :baguette:", null],
  custom_emoji_request_approved: ["approved the request for the custom emoji :baguette:", null],
  custom_emoji_request_rejected: ["rejected the request for the custom emoji :baguette:", null],
  payment_reconcile_queued: ["started checking the payments with Stripe", null],
  oauth_client_disabled: ["turned off the app Toot Viewer (toot.example) and signed it out everywhere", null],
  oauth_client_enabled: ["turned on the app Toot Viewer (toot.example)", null],
  oauth_client_host_denied: ["denied the apps of apps.example", null],
  oauth_client_host_allowed: ["allowed the apps of apps.example again", null],
  service_closure_announced: ["announced the closure of the service, or changed its dates", null],
  service_closure_withdrawn: ["withdrew the closure of the service", null],
  service_closure_closed: ["closed the service", null],
  service_closure_delivery_completed: ["finished telling other servers of the closure", null],
  voice_room_ended: ["ended the voice chat room Bread talk", null],
  login_provider_created: ["added the login provider GitHub", null],
  login_provider_updated: ["changed the login provider GitHub", null],
  login_provider_deleted: ["deleted the login provider GitHub", null],
  login_provider_enabled: ["turned on the login provider GitHub", null],
  login_provider_disabled: ["turned off the login provider GitHub", null],
  login_provider_secret_rotated: ["replaced the client secret of the login provider GitHub", null],
  link_card_domain_fetch_stopped: ["stopped fetching link cards from tracker.example", null],
  link_card_domain_images_stopped: ["stopped fetching link card images from tracker.example", null],
  link_card_domain_rule_removed: ["resumed fetching link cards from tracker.example", null],
  instance_marked_available: ["resumed deliveries to spam.example", { to: "/moderation/instances/spam.example" }],
  delivery_revealed: [
    "was shown the hidden fields of a delivery to spam.example",
    { to: "/moderation/instances/spam.example" },
  ],
  account_deliveries_viewed: ["listed the deliveries of @quickdeals", { under: "/moderation/accounts/" }],
  domain_allow_added: ["allowed friend.example to federate", { to: "/moderation/domain-actions/allowed" }],
  domain_allow_removed: ["removed friend.example from the allowlist", { to: "/moderation/domain-actions/allowed" }],
  domain_list_subscription_created: [
    "subscribed to the blocklist Shared block list",
    { under: "/moderation/domain-actions/subscriptions/" },
  ],
  domain_list_subscription_updated: [
    "changed the settings of the blocklist Shared block list",
    { under: "/moderation/domain-actions/subscriptions/" },
  ],
  domain_list_subscription_deleted: [
    "stopped subscribing to the blocklist Shared block list",
    { under: "/moderation/domain-actions/subscriptions/" },
  ],
  domain_list_draft_accepted: [
    "accepted a draft of the blocklist Shared block list",
    { under: "/moderation/domain-actions/subscriptions/" },
  ],
  domain_list_draft_rejected: [
    "rejected a draft of the blocklist Shared block list",
    { under: "/moderation/domain-actions/subscriptions/" },
  ],
  domain_list_fetch_queued: [
    "asked to fetch the blocklist Shared block list now",
    { under: "/moderation/domain-actions/subscriptions/" },
  ],
  event_cancelled: ["cancelled the event Book swap", { under: "/events/" }],
};

/** Who an entry says did it: the person's handle, the command line, the server, or another server. */
const actorOf = (entry: Entry): string => {
  if (entry.actor_kind === "user") return `@${entry.actor_account?.username ?? "?"}`;
  if (entry.actor_kind === "cli") return "The command line";
  if (entry.actor_kind === "system") return "The server";
  return "Another server";
};

/** The entries of the audit log on the screen, each the list's own item. */
const entries = () => {
  const list = main().getByRole("list", { name: "Audit log" });
  return within(list)
    .getAllByRole("listitem")
    .filter((item) => item.parentElement === list);
};

/** What an entry's sentence says: its words after its time and before its Details. */
const sentenceOf = (item: Element) =>
  plain(words(item))
    .replace(/Details.*$/, "")
    .replace(/^(?:\d+[smhd] ago|[A-Z][a-z]{2} \d{1,2}(?:, \d{4})?)/, "");

/** The links of an entry's sentence, each as its name and where it leads, its Details left out. */
const linksOf = (item: HTMLElement) =>
  within(item)
    .queryAllByRole("link")
    .filter((link) => !["Their other operations", "Other operations on this"].includes(plain(words(link))))
    .map((link) => [plain(words(link)), link.getAttribute("href") ?? ""] as const);

/** A server of the fake read by `viewer` (marco), and its audit log as it holds it at the start, newest first. */
function atLog({ viewer = "marco", ...options }: Parameters<typeof fakeApp>[0] = {}) {
  const on = fakeApp({ viewer, ...options });
  return { on, log: on.fake.domains.audit().slice() };
}

/** Renders the log the fake gives `on`, once its entries are listed. */
async function drawn(on: ReturnType<typeof fakeApp>, at = "/moderation/audit-log") {
  const app = await renderApp(at, { on });
  await main().findByRole("list", { name: "Audit log" });
  await waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the log is on its way");
  });
  return app;
}

describe("every kind of operation reads as a sentence, its actor and its target linked where they lead", () => {
  const chunk = 40;

  test("the fake's log holds one entry of every kind the API names", () => {
    const { log } = atLog();
    const held: string[] = [...new Set(log.map((entry) => entry.action))];
    expect(held.toSorted()).toEqual(Object.keys(kinds).toSorted());
  });

  for (const from of [0, chunk, 2 * chunk])
    test(`entries ${String(from + 1)} to ${String(from + chunk)}`, async () => {
      const { on, log } = atLog();
      const shown = log.slice(from, from + chunk);
      // A page of the log as the screen reads it, these entries alone.
      on.fake.script("GET /admin/audit_log", { change: () => ({ items: shown }) });
      await drawn(on);

      const items = entries();
      expect(items).toHaveLength(shown.length);
      const wrong: string[] = [];
      shown.forEach((entry, i) => {
        const item = items[i];
        if (item === undefined) return;
        const [says, leads] = kinds[entry.action];
        const sentence = `${actorOf(entry)} ${says}`;
        if (sentenceOf(item) !== sentence) wrong.push(`${entry.action}: says "${sentenceOf(item)}", not "${sentence}"`);
        const expected: [string, (href: string) => boolean][] = [];
        if (entry.actor_kind === "user")
          expected.push([actorOf(entry), (href) => href === `/moderation/accounts/${entry.actor_account_id ?? ""}`]);
        if (leads !== null)
          expected.push([
            entry.target_label.replace(/^moderator /, ""),
            (href) => ("to" in leads ? href === leads.to : href.startsWith(leads.under)),
          ]);
        const links = linksOf(item);
        const fits =
          links.length === expected.length &&
          expected.every(([name, leading], n) => {
            const link = links[n];
            return link !== undefined && link[0] === name && leading(link[1]);
          });
        if (!fits) wrong.push(`${entry.action}: links ${JSON.stringify(links)}`);
      });
      expect(wrong).toEqual([]);
    });
});

test("an operation of a kind the app does not know reads as an operation on its target, by its actor", async () => {
  const { on } = atLog();
  on.fake.script("GET /admin/audit_log", {
    change: (page) => ({
      ...page,
      // A kind a newer server records, which no specification of this app names.
      items: page.items.map((entry, i) => (i === 0 ? { ...entry, action: "zz_new_kind" as Action } : entry)),
    }),
  });
  await drawn(on);

  const [first] = entries();
  if (first === undefined) throw new Error("test: no entry");
  expect(sentenceOf(first)).toBe("@marco did an operation on @quickdeals");
  expect(linksOf(first).map(([name]) => name)).toEqual(["@marco", "@quickdeals"]);
});

test("a domain is plain text in the log of a reader who may not open the domains' screens, and an account stays linked", async () => {
  const { on } = atLog();
  on.fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({
      ...account,
      permissions: account.permissions.filter((permission) => permission !== "manage_domains"),
    }),
  });
  await drawn(on, "/moderation/audit-log?target_kind=domain&action=domain_action_lifted");

  const items = entries();
  expect(items.map(sentenceOf)).toEqual(["@marco lifted an action on the domain spam.example"]);
  expect(linksOf(items[0] as HTMLElement).map(([name]) => name)).toEqual(["@marco"]);
});

// Filters.

/** Opens an entry's Details and follows its link `name`, and returns the request of the log it led to. */
async function follow(app: TestApp, item: HTMLElement, name: string) {
  const read = app.fake.script("GET /admin/audit_log");
  await app.user.click(within(item).getByRole("button", { name: "Details" }));
  await app.user.click(within(item).getByRole("link", { name }));
  return (await read.arrived()).url.searchParams;
}

test("an actor's other operations are the log of that actor alone", async () => {
  const { on } = atLog();
  const app = await drawn(on);
  const hana = entries().find((item) => sentenceOf(item).startsWith("@hana "));
  if (hana === undefined) throw new Error("test: no entry of hana's");

  const asked = await follow(app, hana, "Their other operations");

  expect(asked.get("actor_account_id")).toBe(app.fake.accountId("hana"));
  await waitFor(() => {
    const said = entries().map(sentenceOf);
    if (said.length === 0 || !said.every((sentence) => sentence.startsWith("@hana ")))
      throw new Error(`test: the log is not hana's alone: ${JSON.stringify(said.slice(0, 3))}`);
  });
});

test("a target's other operations are the log of that target alone", async () => {
  const { on } = atLog();
  const app = await drawn(on);
  const domain = entries().find((item) => sentenceOf(item).endsWith("on the domain spam.example"));
  if (domain === undefined) throw new Error("test: no entry on spam.example");

  const asked = await follow(app, domain, "Other operations on this");

  expect([asked.get("target_kind"), asked.get("target_id")]).toEqual(["domain", "spam.example"]);
  await waitFor(() => {
    const said = entries().map(sentenceOf);
    if (said.length === 0 || !said.every((sentence) => sentence.includes("spam.example")))
      throw new Error(`test: the log is not spam.example's alone: ${JSON.stringify(said)}`);
  });
});

test("the log of one kind of operation asks the server for that kind and lists it alone", async () => {
  const { on } = atLog();
  const read = on.fake.script("GET /admin/audit_log");
  await drawn(on, "/moderation/audit-log?target_kind=domain&action=domain_action_issued");

  const asked = (await read.arrived()).url.searchParams;
  expect([asked.get("action"), asked.get("target_kind")]).toEqual(["domain_action_issued", "domain"]);
  expect(entries().map(sentenceOf)).toEqual(["The command line took an action on the domain spam.example"]);
});

// A one-person server.

test("a one-person server's log offers no choice of who and no operations of an actor, and still says who did each", async () => {
  const { on } = atLog({ viewer: "hana", singleUser: true });
  await drawn(on);

  const filters = main().getByRole("search", { name: "Filter the audit log" });
  expect(within(filters).queryAllByRole("button", { name: "Who" })).toHaveLength(0);
  expect(within(filters).getAllByRole("button", { name: "On" })).toHaveLength(1);
  expect(main().queryAllByRole("link", { name: "Their other operations", hidden: true })).toHaveLength(0);
  expect(main().queryAllByRole("link", { name: "Other operations on this", hidden: true }).length).toBeGreaterThan(0);
  expect(sentenceOf(entries()[0] as HTMLElement)).toBe("@hana took an action on @quickdeals");
});

// Who may read it.

test("a reader without view_audit is not led to the audit log, and its address opens nothing", async () => {
  for (const path of ["/moderation", "/moderation/audit-log"]) {
    cleanup();
    const on = fakeApp({ viewer: "marco" });
    on.fake.script("GET /accounts/verify_credentials", {
      change: (account) => ({
        ...account,
        permissions: account.permissions.filter((permission) => permission !== "view_audit"),
      }),
    });
    await renderApp(path, { on });
    await main().findByRole("heading", { level: 1, name: "Moderation" });
    if (path === "/moderation") {
      await main().findByRole("link", { name: /^Domain actions\b/ });
      expect(main().queryAllByRole("link", { name: /^Audit log\b/ })).toHaveLength(0);
    } else
      await waitFor(() => {
        if (!words(screen.getByRole("main")).includes("This address leads to no screen open to you."))
          throw new Error("test: the audit log's address is not refused");
      });
  }
});

test("a domain action's page leads to its audit log only for a reader who may read it", async () => {
  const on = fakeApp({ viewer: "marco" });
  on.fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({
      ...account,
      permissions: account.permissions.filter((permission) => permission !== "view_audit"),
    }),
  });
  await renderApp(`/moderation/domain-actions/${on.fake.domains.id("action", "spam")}`, { on });

  await main().findByRole("link", { name: "Open the server" });
  expect(main().queryAllByRole("link", { name: "Audit log" })).toHaveLength(0);
});
