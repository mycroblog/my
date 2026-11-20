// SPDX-License-Identifier: AGPL-3.0-only

// The moderation of domains (#3261, FR-702, FR-704, FR-706, FR-716, FR-717; ADR-0159), on the fake API, as what follows
// a press: an action imposed with its kind and expiry and lifted; a suspend asked first, with the follows it ends as the
// server counts them for the domain submitted, confirmed by typing the domain once the count came or failed; Mastodon's
// CSV previewed before anything is applied, its suspends' follows summed up to a limit, imported, and exported; the
// allowlist; a block-list subscription's drafts accepted and rejected, a fetch asked, a failing one marked once; a
// remote server marked available, its hidden fields of a delivery shown on demand, its posts fetched again; the domain
// search refused in the search's own words; and who may open these screens. What each screen shows at rest is the
// stories' (domains.stories.tsx, domain-suspend.stories.tsx).

import { scenario } from "@my/e2e/fake";
import { expect, test } from "bun:test";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { chooseFiles } from "./test/files";
import { fakeApp } from "./test/server";

/**
 * `text` without the marks that isolate a name or a number inside a sentence, and the space that keeps a count with
 * its noun ("2 rows") as a plain one.
 */
const plain = (text: string) =>
  text
    .replaceAll(String.fromCodePoint(0x2068), "")
    .replaceAll(String.fromCodePoint(0x2069), "")
    .replaceAll(String.fromCodePoint(0xa0), " ");

/** What `element` says, the isolating marks taken out. */
const said = (element: Element) => plain(words(element));

/** The app at `path` (`{action:spam}` and `{list:shared}` name the fake's domain action and subscription), read by marco. */
async function at(path: string, options: Parameters<typeof fakeApp>[0] = {}) {
  const on = fakeApp({ viewer: "marco", ...options });
  const resolved = path.replace(/\{(action|list):(\w+)\}/g, (_, kind: "action" | "list", key: string) =>
    on.fake.domains.id(kind, key),
  );
  const app = await renderApp(resolved, { on });
  await main().findByRole("heading", { level: 1 });
  return app;
}

/** A reader without `permission`, as `verify_credentials` says it. */
function without(on: ReturnType<typeof fakeApp>, ...permissions: string[]) {
  on.fake.script("GET /accounts/verify_credentials", {
    change: (account) => ({
      ...account,
      permissions: account.permissions.filter((permission) => !permissions.includes(permission)),
    }),
  });
}

/** Waits until the screen says `sentence`. */
const says = (sentence: string) =>
  waitFor(() => {
    if (!said(screen.getByRole("main")).includes(sentence))
      throw new Error(`test: the screen does not say ${JSON.stringify(sentence)}`);
  });

/** The confirmation named `name` once it is open. */
const confirmationElement = (name: string) =>
  waitFor(() => {
    const [found] = screen.queryAllByRole("alertdialog", { name: (n) => plain(n) === name });
    if (found === undefined) throw new Error(`test: no confirmation ${JSON.stringify(name)}`);
    return found;
  });

/** Waits until no confirmation is open. */
const closed = () =>
  waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: the confirmation is still open");
  });

/** Whether the button is offered for a press: neither disabled nor marked so. */
const pressable = (button: HTMLElement) =>
  !(button as HTMLButtonElement).disabled && button.getAttribute("aria-disabled") !== "true";

/** The button of the screen whose name, the isolating marks taken out, is `name`. */
const button = (name: string) => main().getByRole("button", { name: (n) => plain(n) === name });

/** Writes `text` into the field `label` of the screen. */
async function write(app: TestApp, label: string, text: string) {
  await app.user.click(main().getByRole("textbox", { name: label }));
  await app.user.paste(text);
}

/** Chooses `option` of the choice `label` of the screen. */
const choose = (app: TestApp, label: string, option: string) =>
  app.user.selectOptions(main().getByRole("combobox", { name: label }), option);

/** Waits until the screen's reads settled, and returns how many requests the script saw by then. */
const seenAtRest = async (app: TestApp, script: { seen: readonly unknown[] }) => {
  await waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });
  return script.seen.length;
};

/** What the facts labelled `label` say, a term and its value joined with no space. */
const facts = (label: string) => said(main().getByLabelText(label));

/** The rows of the table `name` of the screen, each as what it says. */
const rows = (name: string) =>
  within(main().getByRole("table", { name }))
    .getAllByRole("row")
    .slice(1)
    .map((row) => said(row));

const day = 24 * 60 * 60;

// The form of a new action.

test("a suspend asks first, reads the follows it ends for the domain submitted, and offers Suspend only once the domain is typed and the count came", async () => {
  const app = await at("/moderation/domain-actions/new");
  const impact = app.fake.script("GET /admin/domain_actions/impact", { hold: "response" });
  const taken = app.fake.script("POST /admin/domain_actions");

  await write(app, "Domain", "photos.example");
  // Nothing is counted while the domain is written: only the domain submitted is.
  expect(await seenAtRest(app, impact)).toBe(0);
  await app.user.click(button("Suspend…"));

  expect((await impact.arrived()).url.searchParams.get("domain")).toBe("photos.example");
  const element = await confirmationElement("Suspend photos.example?");
  const asking = within(element);
  const confirm = asking.getByRole("button", { name: "Suspend" });
  expect(pressable(confirm)).toBe(false);
  await app.user.type(asking.getByRole("textbox"), "photos.example");
  // The domain is typed, and the count has not come yet.
  await impact.served();
  expect(pressable(confirm)).toBe(false);
  impact.release();

  await waitFor(() => {
    if (!said(element).includes("1 follow between people here and its accounts ends for good, for 1 person here."))
      throw new Error("test: the confirmation does not say the follows the suspend ends");
  });
  expect(pressable(confirm)).toBe(true);
  expect(taken.seen).toHaveLength(0);
});

test("a suspend's confirmation offers Suspend only once the domain is typed whole", async () => {
  const app = await at("/moderation/domain-actions/new");

  await write(app, "Domain", "photos.example");
  await app.user.click(button("Suspend…"));
  const element = await confirmationElement("Suspend photos.example?");
  const asking = within(element);
  const confirm = asking.getByRole("button", { name: "Suspend" });
  await waitFor(() => {
    if (!said(element).includes("1 follow between people here")) throw new Error("test: the count has not come");
  });

  expect(pressable(confirm)).toBe(false);
  await app.user.type(asking.getByRole("textbox"), "photos.exampl");
  expect(pressable(confirm)).toBe(false);
  await app.user.type(asking.getByRole("textbox"), "e");
  expect(pressable(confirm)).toBe(true);
});

test("a suspend whose follows could not be counted says every follow ends uncounted, and is still confirmed by typing the domain", async () => {
  const on = fakeApp({ viewer: "marco" });
  on.fake.script("GET /admin/domain_actions/impact", { fail: 500 });
  const app = await renderApp("/moderation/domain-actions/new", { on });
  await main().findByRole("heading", { level: 2, name: "New action" });
  const taken = app.fake.script("POST /admin/domain_actions");

  await write(app, "Domain", "photos.example");
  await app.user.click(button("Suspend…"));
  const element = await confirmationElement("Suspend photos.example?");
  await waitFor(() => {
    if (
      !said(element).includes(
        "Every follow between people here and its accounts ends for good; the follows could not be counted.",
      )
    )
      throw new Error("test: the confirmation does not say the follows are uncounted");
  });
  const asking = within(element);
  await app.user.type(asking.getByRole("textbox"), "photos.example");
  const confirm = asking.getByRole("button", { name: "Suspend" });
  expect(pressable(confirm)).toBe(true);
  await app.user.click(confirm);

  const { body } = await taken.arrived();
  expect([body.domain, body.kind]).toEqual(["photos.example", "suspend"]);
});

test("a suspend confirmed goes to the server with its reasons, its part hidden and an expiry of ten years, and its page says it is in effect", async () => {
  const app = await at("/moderation/domain-actions/new");
  const taken = app.fake.script("POST /admin/domain_actions");

  await write(app, "Domain", "photos.example");
  await choose(app, "Lasts", "10 years");
  await write(app, "Public reason", "Spam from every account.");
  await write(app, "Moderators' note", "Reported by three people.");
  await app.user.click(main().getByRole("checkbox", { name: "Hide part of the domain where it is published" }));
  await app.user.click(button("Suspend…"));
  const element = await confirmationElement("Suspend photos.example?");
  await app.user.type(within(element).getByRole("textbox"), "photos.example");
  const confirm = within(element).getByRole("button", { name: "Suspend" });
  await waitFor(() => {
    if (!pressable(confirm)) throw new Error("test: Suspend is not offered");
  });
  await app.user.click(confirm);

  const { body } = await taken.arrived();
  expect(body).toEqual({
    domain: "photos.example",
    kind: "suspend",
    expires_in: 3650 * day,
    public_comment: "Spam from every account.",
    private_note: "Reported by three people.",
    obfuscate: true,
  });
  await main().findByRole("heading", { level: 1, name: "photos.example" });
  await waitFor(() => {
    if (!/State\s*In effect/.test(facts("About this action"))) throw new Error("test: the action is not in effect");
  });
  expect(facts("About this action")).toMatch(/Action\s*Suspend/);
});

test("a suspend asked and cancelled sends nothing", async () => {
  const app = await at("/moderation/domain-actions/new");
  const taken = app.fake.script("POST /admin/domain_actions");

  await write(app, "Domain", "photos.example");
  await app.user.click(button("Suspend…"));
  const asking = within(await confirmationElement("Suspend photos.example?"));
  await app.user.type(asking.getByRole("textbox"), "photos.example");
  await app.user.click(asking.getByRole("button", { name: "Cancel" }));

  await closed();
  expect(await seenAtRest(app, taken)).toBe(0);
});

for (const [kind, option, act, lasts, expires] of [
  ["limit", "Limit", "Limit", "30 days", 30 * day],
  ["reject_media", "Reject media", "Reject media", "Until lifted", undefined],
  ["reject_reports", "Reject reports", "Reject reports", "1 year", 365 * day],
  ["force_sensitive", "Media always marked as sensitive", "Mark media sensitive", "7 days", 7 * day],
] as const)
  test(`"${option}" is taken at once by a button that names the act, with its expiry, and asks nothing`, async () => {
    const app = await at("/moderation/domain-actions/new");
    const taken = app.fake.script("POST /admin/domain_actions");
    const impact = app.fake.script("GET /admin/domain_actions/impact");

    await write(app, "Domain", "bread.example");
    await choose(app, "Action", option);
    await choose(app, "Lasts", lasts);
    expect(main().queryAllByRole("button", { name: "Suspend…" })).toHaveLength(0);
    await app.user.click(button(act));

    const { body } = await taken.arrived();
    expect([body.domain, body.kind, body.expires_in]).toEqual(["bread.example", kind, expires]);
    expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
    await main().findByRole("heading", { level: 1, name: "bread.example" });
    expect(facts("About this action")).toMatch(new RegExp(`Action\\s*${option}`));
    expect(impact.seen).toHaveLength(0);
  });

test("an action in effect is lifted, the server told, and is no longer offered to be lifted", async () => {
  const app = await at("/moderation/domain-actions/{action:spam}");
  const id = app.fake.domains.id("action", "spam");
  const lifted = app.fake.script("POST /admin/domain_actions/{id}/lift");

  await app.user.click(button("Lift"));

  expect((await lifted.arrived()).url.pathname).toEndWith(`/admin/domain_actions/${id}/lift`);
  await says("The action is lifted.");
  expect(facts("About this action")).toMatch(/State\s*Lifted/);
  expect(main().queryAllByRole("button", { name: "Lift" })).toHaveLength(0);
});

// Mastodon's CSV.

/** A file of Mastodon's domain blocks with a header, one row a line. */
const csv = (...lines: string[]) =>
  new File(
    [["#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate", ...lines].join("\n")],
    "blocks.csv",
    {
      type: "text/csv",
    },
  );

/** A file of bare domains, one a line, each suspended. */
const domainsFile = (domains: readonly string[]) => new File([domains.join("\n")], "domains.csv", { type: "text/csv" });

/**
 * The import screen with `file` chosen, once its preview is shown; the reads of what a suspend ends counted from before
 * the file is chosen, or failed (`impact: "fails"`).
 */
async function previewed(
  file: File,
  { impact: failing, ...options }: Parameters<typeof fakeApp>[0] & { impact?: "fails" } = {},
) {
  const app = await at("/moderation/domain-actions/import", options);
  const preview = app.fake.script("POST /admin/domain_actions/import/preview");
  const imported = app.fake.script("POST /admin/domain_actions/import");
  const impact = app.fake.script("GET /admin/domain_actions/impact", failing === "fails" ? { fail: 500 } : {});
  await chooseFiles(app, main().getByRole("button", { name: "Choose a file" }), file);
  await main().findByRole("table", { name: "Rows to import" });
  return { app, preview, imported, impact };
}

/** Holds each of `texts` to the pattern at its place, as many texts as patterns. */
function matching(texts: readonly string[], patterns: readonly RegExp[]) {
  expect(texts).toHaveLength(patterns.length);
  patterns.forEach((pattern, i) => {
    expect(texts[i] ?? "").toMatch(pattern);
  });
}

/** Waits until `element` says `sentence`, and says what it said instead. */
const saying = (element: Element, sentence: string) =>
  waitFor(() => {
    if (!said(element).includes(sentence)) throw new Error(`test: it says ${JSON.stringify(said(element))}`);
  });

/** Opens the confirmation of the import of `count` rows. */
async function importing(app: TestApp, count: number) {
  await app.user.click(button(`Import ${String(count)} rows…`));
  return confirmationElement(`Import ${String(count)} rows?`);
}

test("the import shows what each row would do before anything is applied", async () => {
  const { app, preview, imported } = await previewed(
    csv(
      "bad.example,suspend,false,false,Spam,false",
      "loud.example,silence,true,false,,false",
      "spam.example,suspend,false,false,,false",
      "demo.test,suspend,false,false,,false",
      "not a domain,suspend,false,false,,false",
      "bad.example,suspend,false,false,,false",
      "quiet.example,noop,false,false,,false",
    ),
  );

  expect((await preview.arrived()).body.csv).toStartWith("#domain,#severity");
  matching(rows("Rows to import"), [
    /^bad\.example.*Line 2.*Suspend$/,
    /^loud\.example.*Line 3.*Limit and Reject media$/,
  ]);
  matching(rows("Rows that take no action"), [
    /^spam\.example.*Line 4.*An action covers it already$/,
    /^demo\.test.*Line 5.*This server's own domain$/,
    /^not a domain.*Line 6.*Not a valid row$/,
    /^bad\.example.*Line 7.*An earlier row has this domain$/,
    /^quiet\.example.*Line 8.*Asks for no action$/,
  ]);
  // Nothing is applied: no import was asked, and the server holds no action on the rows' domains.
  expect(await seenAtRest(app, imported)).toBe(0);
  const held = await app.data.api.GET("/admin/domain_actions", { params: { query: { domain: "bad.example" } } });
  expect(held.data?.items).toEqual([]);
});

test("the rows chosen are imported with the moderators' note after a confirmation that sums the follows their suspends end", async () => {
  const { app, imported } = await previewed(
    csv(
      "bad.example,suspend,false,false,Spam,false",
      "photos.example,suspend,false,false,,false",
      "loud.example,silence,false,false,,false",
    ),
  );

  await app.user.click(main().getByRole("checkbox", { name: "Select loud.example" }));
  await write(app, "Moderators' note for every action", "From the neighbours' list.");
  const element = await importing(app, 2);
  await saying(element, "1 follow between people here and their accounts ends for good, for 1 person here.");
  expect(said(element)).toContain("2 of them suspend domains");
  expect(imported.seen).toHaveLength(0);
  await app.user.click(within(element).getByRole("button", { name: "Import" }));

  const { body } = await imported.arrived();
  expect(body.blocks.map((block) => [block.domain, block.severity])).toEqual([
    ["bad.example", "suspend"],
    ["photos.example", "suspend"],
  ]);
  expect(body.private_note).toBe("From the neighbours' list.");
  await closed();
  const held = await app.data.api.GET("/admin/domain_actions", { params: { query: { domain: "bad.example" } } });
  expect(held.data?.items.map((action) => [action.kind, action.state])).toEqual([["suspend", "active"]]);
});

test("the import's confirmation adds up the follows each suspended domain ends, and the people who lose one", async () => {
  // photos.example's sora follows yuki; marco follows bread.example's kobo; far.example's kiri follows hana.
  const seed = {
    ...scenario,
    follows: [
      ...scenario.follows,
      { follower: "marco", followee: "kobo@bread.example" },
      { follower: "kiri@far.example", followee: "hana" },
    ],
  };
  const { app, impact } = await previewed(domainsFile(["photos.example", "bread.example", "far.example"]), { seed });

  const element = await importing(app, 3);
  await saying(element, "3 follows between people here and their accounts end for good, for 3 people here.");
  expect(impact.seen.map((seen) => seen.url.searchParams.get("domain")).toSorted()).toEqual([
    "bread.example",
    "far.example",
    "photos.example",
  ]);
});

test("the import counts the follows of twenty suspends, and calls those of more uncounted while still offering the import", async () => {
  const twenty = [...Array.from({ length: 19 }, (_, i) => `d${String(i)}.example`), "photos.example"];
  const counted = await previewed(domainsFile(twenty));
  const element = await importing(counted.app, 20);
  await saying(element, "1 follow between people here and their accounts ends for good, for 1 person here.");

  cleanup();
  const more = await previewed(domainsFile(["d20.example", ...twenty]));
  const asking = await importing(more.app, 21);
  await saying(
    asking,
    "Every follow between people here and their accounts ends for good; the follows could not be counted.",
  );
  expect(pressable(within(asking).getByRole("button", { name: "Import" }))).toBe(true);
});

test("the import's suspends whose follows could not be counted are called uncounted, and the import still offered", async () => {
  const { app } = await previewed(domainsFile(["bad.example", "photos.example"]), { impact: "fails" });

  const element = await importing(app, 2);
  await saying(element, "the follows could not be counted.");
  expect(pressable(within(element).getByRole("button", { name: "Import" }))).toBe(true);
});

test("Save as CSV asks the server for the export and says the file it saved", async () => {
  const app = await at("/moderation/domain-actions/import");
  const exported = app.fake.script("GET /admin/domain_actions/export");

  await app.user.click(button("Save as CSV"));

  await exported.arrived();
  await says("Saved domain_blocks.csv.");
});

// The allowlist.

test("a domain is allowed with the moderators' note, and is listed", async () => {
  const app = await at("/moderation/domain-actions/allowed");
  const allowed = app.fake.script("POST /admin/domain_allows");

  await write(app, "Domain", "pals.example");
  await write(app, "Moderators' note", "Our friends' server.");
  await app.user.click(button("Allow"));

  const { body } = await allowed.arrived();
  expect([body.domain, body.comment]).toEqual(["pals.example", "Our friends' server."]);
  await says("pals.example is allowed.");
  await waitFor(() => {
    if (!rows("Allowed domains").some((row) => row.startsWith("pals.exampleOur friends' server.")))
      throw new Error("test: the allowlist does not list the domain");
  });
});

test("a domain is removed from the allowlist after a confirmation that says it is cut off", async () => {
  const app = await at("/moderation/domain-actions/allowed");
  const removed = app.fake.script("DELETE /admin/domain_allows/{domain}");

  await app.user.click(main().getByRole("checkbox", { name: "Select photos.example" }));
  await app.user.click(button("Remove"));
  const element = await confirmationElement("Remove photos.example from the allowlist?");
  expect(said(element)).toContain("it is cut off at once, and every follow with its accounts ends for good.");
  expect(removed.seen).toHaveLength(0);
  await app.user.click(within(element).getByRole("button", { name: "Remove" }));

  expect((await removed.arrived()).url.pathname).toEndWith("/admin/domain_allows/photos.example");
  await waitFor(() => {
    if (rows("Allowed domains").some((row) => row.startsWith("photos.example")))
      throw new Error("test: the allowlist still lists the domain");
  });
});

// Block-list subscriptions.

test("a subscription's draft is accepted after a confirmation that says what its suspend ends, and leaves the drafts", async () => {
  const app = await at("/moderation/domain-actions/subscriptions/{list:shared}");
  const id = app.fake.domains.id("list", "shared");
  const accepted = app.fake.script("POST /admin/domain_list_subscriptions/{id}/drafts/accept");

  await app.user.click(main().getByRole("checkbox", { name: "Select bad.example" }));
  await app.user.click(button("Accept"));
  const element = await confirmationElement("Accept 1 draft?");
  expect(said(element)).toContain("1 of them suspends a domain, which ends every follow with its accounts for good.");
  expect(accepted.seen).toHaveLength(0);
  await app.user.click(within(element).getByRole("button", { name: "Accept" }));

  const { url, body } = await accepted.arrived();
  expect(url.pathname).toEndWith(`/admin/domain_list_subscriptions/${id}/drafts/accept`);
  expect(body?.keys).toEqual(["issue:suspend:bad.example"]);
  await waitFor(() => {
    if (rows("Drafts").some((row) => row.startsWith("bad.example"))) throw new Error("test: the draft is still listed");
  });
  const held = await app.data.api.GET("/admin/domain_actions", { params: { query: { domain: "bad.example" } } });
  expect(held.data?.items.map((action) => action.kind)).toEqual(["suspend"]);
});

test("a subscription's draft is rejected at once, the server told, and leaves the drafts", async () => {
  const app = await at("/moderation/domain-actions/subscriptions/{list:shared}");
  const rejected = app.fake.script("POST /admin/domain_list_subscriptions/{id}/drafts/reject");

  await app.user.click(main().getByRole("checkbox", { name: "Select loud.example" }));
  await app.user.click(button("Reject"));

  expect((await rejected.arrived()).body?.keys).toEqual(["issue:limit:loud.example"]);
  await says("1 draft rejected.");
  expect(rows("Drafts").filter((row) => row.startsWith("loud.example"))).toEqual([]);
});

test("a subscription is fetched now on asking, the server told", async () => {
  const app = await at("/moderation/domain-actions/subscriptions/{list:shared}");
  const id = app.fake.domains.id("list", "shared");
  const fetched = app.fake.script("POST /admin/domain_list_subscriptions/{id}/fetch");

  await app.user.click(button("Fetch now"));

  expect((await fetched.arrived()).url.pathname).toEndWith(`/admin/domain_list_subscriptions/${id}/fetch`);
  await says("The list will be fetched in a moment.");
});

test("the subscriptions mark the one whose fetches keep failing once, and the other not at all", async () => {
  await at("/moderation/domain-actions/subscriptions");
  const listed = await waitFor(() => {
    const found = rows("Blocklist subscriptions");
    if (found.length < 2) throw new Error("test: the subscriptions are not listed");
    return found;
  });

  const failing = listed.filter((row) => row.startsWith("Neighbours' published blocks"));
  const shared = listed.filter((row) => row.startsWith("Shared block list"));
  expect(failing.map((row) => row.split("failures in a row").length - 1)).toEqual([1]);
  expect(failing[0]).toContain("3 failures in a row");
  expect(shared.map((row) => row.includes("failure"))).toEqual([false]);
});

// Remote servers.

test("a server is marked available, the server told, and its deliveries are not read again", async () => {
  const app = await at("/moderation/instances/dead.example");
  await main().findByRole("list", { name: "Deliveries" });
  const marked = app.fake.script("POST /admin/instances/{domain}/mark_available");
  const deliveries = app.fake.script("GET /admin/instances/{domain}/deliveries");

  await app.user.click(button("Mark available"));

  expect((await marked.arrived()).url.pathname).toEndWith("/admin/instances/dead.example/mark_available");
  await waitFor(() => {
    if (!/State\s*Available/.test(said(screen.getByRole("main")))) throw new Error("test: the server is not available");
  });
  expect(said(screen.getByRole("main"))).toMatch(/Failed days\s*No failure/);
  expect(main().queryAllByRole("button", { name: "Mark available" })).toHaveLength(0);
  expect(await seenAtRest(app, deliveries)).toBe(0);
});

test("a delivery's hidden fields are asked for only once shown, and shown", async () => {
  const app = await at("/moderation/instances/dead.example");
  const list = await main().findByRole("list", { name: "Deliveries" });
  const revealed = app.fake.script("POST /admin/instances/{domain}/deliveries/{id}/reveal");
  const read = await app.data.api.GET("/admin/instances/{domain}/deliveries", {
    params: { path: { domain: "dead.example" } },
  });
  const masked = read.data?.items.find((delivery) => delivery.mask !== undefined);
  if (masked === undefined) throw new Error("test: the fake has no masked delivery");

  expect(await seenAtRest(app, revealed)).toBe(0);
  const item = within(list)
    .getAllByRole("listitem")
    .find((li) => said(li).includes("the sender, the activity and the inbox are hidden"));
  if (item === undefined) throw new Error("test: no delivery says its fields are hidden");
  await app.user.click(within(item).getByRole("button", { name: "Details" }));
  expect(revealed.seen).toHaveLength(0);
  await app.user.click(await within(item).findByRole("button", { name: "Show", description: "Sender" }));

  expect((await revealed.arrived()).url.pathname).toEndWith(
    `/admin/instances/dead.example/deliveries/${masked.id}/reveal`,
  );
  await waitFor(() => {
    if (within(list).queryAllByRole("button", { name: "Show", description: "Sender" }).length > 0)
      throw new Error("test: the sender is still hidden");
  });
  expect(revealed.seen).toHaveLength(1);
  expect(
    within(main().getByRole("list", { name: "Deliveries" }))
      .getAllByRole("listitem")
      .filter((li) => /hidden.*Sender\s*@marco/.test(said(li))),
  ).not.toHaveLength(0);
});

test("a server's posts are fetched again on asking, the server told", async () => {
  const app = await at("/moderation/instances/dead.example");
  const refetched = app.fake.script("POST /admin/instances/{domain}/refetch_statuses");

  await app.user.click(button("Refetch posts"));

  expect((await refetched.arrived()).url.pathname).toEndWith("/admin/instances/dead.example/refetch_statuses");
  await says("The posts of its accounts will be refetched over the next few minutes.");
});

// The domain search.

test("a domain the server refuses to search for is said under the search's field, and the actions stay", async () => {
  const app = await at("/moderation/domain-actions");
  await app.user.type(main().getByRole("searchbox", { name: "Domain" }), "bad domain");

  const field = await waitFor(
    () => {
      const found = main().getByRole("searchbox", { name: "Domain" });
      if (found.getAttribute("aria-invalid") !== "true")
        throw new Error(`test: the search's field is not marked, at ${app.path()}`);
      return found;
      // The list is read again some time after the last key, and the field is marked once the server refused it.
    },
    { timeout: 3000 },
  );
  expect(
    main().getByRole("searchbox", {
      name: "Domain",
      description: "Enter a domain, such as spam.example, without https:// or a path.",
    }),
  ).toBe(field);
  expect(main().queryAllByRole("alert")).toHaveLength(0);
});

test("the domain search keeps one entry of the history, whatever is typed", async () => {
  const app = await at("/moderation");
  await app.navigate("/moderation/domain-actions");
  await main().findByRole("table", { name: "Domain actions" });

  await app.user.type(main().getByRole("searchbox", { name: "Domain" }), "spam.example");
  await waitFor(() => {
    if (!app.path().includes("domain=spam.example")) throw new Error(`test: the address is ${app.path()}`);
  });
  await app.back();

  // Back leaves the search whole: to the list before it, or to the screen before the list, never to a part typed.
  expect(["/moderation", "/moderation/domain-actions"]).toContain(app.path());
});

// Who may open the screens.

test("a reader without manage_domains is not led to the domains' screens, and their addresses open nothing", async () => {
  for (const path of ["/moderation", "/moderation/domain-actions", "/moderation/instances/dead.example"]) {
    cleanup();
    const on = fakeApp({ viewer: "marco" });
    without(on, "manage_domains");
    await renderApp(path, { on });
    await main().findByRole("heading", { level: 1, name: "Moderation" });
    if (path === "/moderation") {
      await main().findByRole("link", { name: /^Audit log\b/ });
      expect(main().queryAllByRole("link", { name: /^(Domain actions|Remote servers)\b/ })).toHaveLength(0);
    } else await says("This address leads to no screen open to you.");
  }
});
