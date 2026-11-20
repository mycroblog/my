// SPDX-License-Identifier: AGPL-3.0-only

// The server's operations in the app (#3265, ADR-0004, ADR-0062, ADR-0103 §8, ADR-0058 §1), on the fake API, as what
// follows a press and what the server's state makes of each screen: the queue's figures by state, each a door to its
// jobs, and the notice of the jobs that failed for good leading to them; the list of jobs by state and by type; a
// job's facts, each that is a code copied whole, and the runs that did not succeed with their outcomes and errors, a
// section that a job without one leaves out; Run again, offered only for a job that failed for good and sent only once
// its question is answered, and a refusal that reads the job again; the open error groups and every group, each row
// headed by its message; a group's facts copied whole, its stack as code, its hours twelve at a time; Resolve, Mute
// and Unmute, each read again into the group and the lists; the link card crawler's rules forty at a time, a new rule
// refused for a domain that has one and leading to it, a rule changed, and a rule removed after a question; a code
// read left to right on a screen in a language written right to left; and each screen open only to its permission.
// What each screen shows at rest is the stories' (operations.stories.tsx); the UX measurements are
// operations.dom.test.tsx's, and Run again and Resolve on the real server the E2E spec's (operations.spec.ts).

import { expect, test } from "bun:test";
import { fakeOrigin, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

/** Waits until the app has no read on its way. */
const settled = (app: TestApp) =>
  waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the screen's reads are on their way");
  });

type On = ReturnType<typeof fakeApp>;

/** The app as hana, the scenario's administrator, at the address `at` gives for the fake, once its screen is read. */
async function atAdmin(at: (fake: On["fake"]) => string, options: FakeOptions = {}, before?: (on: On) => void) {
  const on = fakeApp({ viewer: "hana", ...options });
  before?.(on);
  const app = await renderApp(at(on.fake), { on });
  await main().findByRole("heading", { level: 1 });
  await settled(app);
  return app;
}

const button = (name: string) => main().getByRole("button", { name });

/** The question of `name` once it is open. */
const asking = async (name: string) => within(await screen.findByRole("alertdialog", { name }));

/** Waits until no question is open. */
const answered = () =>
  waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: a question is still open");
  });

/** The rows of the table `name` but its head, each as its words. */
const rowsOf = (name: string) => within(main().getByRole("table", { name })).getAllByRole("row").slice(1).map(words);

/** The heads of the rows of the table `name`, each as its words. */
const headsOf = (name: string) => within(main().getByRole("table", { name })).getAllByRole("rowheader").map(words);

/** The cells of each row of the table `name` but its head, each as its words, its head left out. */
const cellsOf = (name: string) =>
  within(main().getByRole("table", { name }))
    .getAllByRole("row")
    .slice(1)
    .map((row) => within(row).getAllByRole("cell").map(words));

/** What each term of the facts `name` says, by term. */
const facts = (name: string) => {
  const list = main().getByLabelText(name);
  const terms = [...list.querySelectorAll("dt")];
  return Object.fromEntries(terms.map((term) => [words(term), words(term.nextElementSibling ?? term)]));
};

/** Waits until the facts `name` say `value` of `term`. */
const factSays = (name: string, term: string, value: string) =>
  waitFor(() => {
    const said = facts(name)[term];
    if (said !== value) throw new Error(`test: ${term} says ${String(said)}`);
  });

/** Waits until a status of the screen says `sentence`. */
const says = (sentence: string) =>
  waitFor(() => {
    const said = screen.queryAllByRole("status").map(words);
    if (!said.includes(sentence)) throw new Error(`test: the statuses say ${said.join(" / ")}`);
  });

/** What the clipboard of the reader's hands holds. */
const clipboard = () => {
  const navigation = Reflect.get(globalThis, "navigator") as { clipboard: { readText: () => Promise<string> } };
  return navigation.clipboard.readText();
};

/** Does `method path` past the app, as another administrator's browser does, and returns the status it answered. */
const elsewhere = async (on: On, method: string, path: string) =>
  (await on.fake.serve(new Request(`${fakeOrigin}/api/v1${path}`, { method }))).status;

// The queue.

test("the queue's figures count the jobs the server holds by state, each a door to the jobs of its state", async () => {
  const app = await atAdmin(() => "/admin/jobs");
  const { data } = await app.data.api.GET("/admin/queue");
  const count = (state: string) =>
    (data?.counts ?? []).filter((c) => c.state === state).reduce((sum, c) => sum + c.count, 0);

  const figures = within(main().getByRole("list", { name: "Jobs by state" })).getAllByRole("link");
  // The figure and its state are two lines of one link, which a reader hears as one name.
  expect(figures.map((link) => [words(link), link.getAttribute("href")])).toEqual([
    [`${String(count("queued"))}Queued`, "/admin/jobs/list?state=queued"],
    [`${String(count("running"))}Running`, "/admin/jobs/list?state=running"],
    [`${String(count("retrying"))}Waiting to retry`, "/admin/jobs/list?state=retrying"],
    [`${String(count("dead"))}Failed`, "/admin/jobs/list?state=dead"],
  ]);
  expect([count("queued"), count("running"), count("retrying"), count("dead")]).toEqual([3, 1, 1, 3]);
});

test("the notice of the jobs that failed for good leads to the failed jobs, and only they are listed there", async () => {
  const app = await atAdmin(() => "/admin/jobs");
  const notice = main().getByRole("alert");
  expect(words(notice)).toStartWith("3 jobs failed for good.");

  await app.user.click(within(notice).getByRole("link", { name: "Failed jobs" }));
  await app.arrived("/admin/jobs/list?state=dead");
  await main().findByRole("table", { name: "Jobs" });
  await settled(app);

  const rows = rowsOf("Jobs");
  expect(rows).toHaveLength(3);
  for (const row of rows) expect(row).toMatch(/Failed/);
  expect(headsOf("Jobs")).toEqual(["federation.deliver", "notification.email", "media.process"]);
});

test("a queue with no job that failed for good gives no notice of them", async () => {
  await atAdmin(() => "/admin/jobs", { operations: "none" });

  expect(main().queryAllByRole("alert")).toHaveLength(0);
  expect(main().queryAllByRole("link", { name: "Failed jobs" })).toHaveLength(0);
});

test("the list of jobs asks the server for the state and the type it is filtered by, and lists what it answers", async () => {
  const on = fakeApp({ viewer: "hana" });
  const listed = on.fake.script("GET /admin/jobs");
  const app = await renderApp("/admin/jobs/list?type=federation.deliver", { on });
  await main().findByRole("table", { name: "Jobs" });
  await settled(app);

  const asked = listed.seen.at(-1)?.url.searchParams;
  expect(asked?.get("type")).toBe("federation.deliver");
  expect(asked?.get("state")).toBeNull();
  expect(headsOf("Jobs")).toEqual(["federation.deliver", "federation.deliver"]);
  expect(rowsOf("Jobs").some((row) => /Failed/.test(row))).toBe(false);

  await app.navigate("/admin/jobs/list?state=dead&type=federation.deliver");
  await main().findByRole("table", { name: "Jobs" });
  await settled(app);
  const both = listed.seen.at(-1)?.url.searchParams;
  expect([both?.get("state"), both?.get("type")]).toEqual(["dead", "federation.deliver"]);
  expect(rowsOf("Jobs")).toHaveLength(1);
});

test("the State filter of the list of jobs moves it to the state chosen", async () => {
  const app = await atAdmin(() => "/admin/jobs/list");
  const filters = within(main().getByRole("search", { name: "Filter jobs" }));

  await app.user.click(filters.getByRole("button", { name: /^State/ }));
  const states = within(await screen.findByRole("menu", { name: "State" }));
  expect(states.getAllByRole("menuitemradio").map(words)).toEqual([
    "Not finished",
    "Queued",
    "Running",
    "Waiting to retry",
    "Failed",
  ]);
  await app.user.click(states.getByRole("menuitemradio", { name: "Failed" }));

  await waitFor(() => {
    if (!app.path().includes("state=dead")) throw new Error(`test: the list is at ${app.path()}`);
  });
  await settled(app);
  const rows = rowsOf("Jobs");
  expect(rows).toHaveLength(3);
  for (const row of rows) expect(row).toMatch(/Failed/);
});

// A job.

test("a job's facts copy its ID, its ordering key, its last error and its payload whole", async () => {
  let id = "";
  const app = await atAdmin((fake) => {
    id = fake.operations.id("job:dead-deliver");
    return `/admin/jobs/${id}`;
  });
  const { data: job } = await app.data.api.GET("/admin/jobs/{id}", { params: { path: { id } } });

  for (const [name, value] of [
    ["Copy the ID", id],
    ["Copy the ordering key", job?.ordering_key ?? "none held"],
    ["Copy the error", job?.last_error ?? "none held"],
  ] as const) {
    await app.user.click(button(name));
    expect(await clipboard()).toBe(value);
  }
  await app.user.click(button("Copy the payload"));
  expect(JSON.parse(await clipboard())).toEqual(JSON.parse(job?.payload ?? "null"));

  expect(facts("The job")).toMatchObject({ State: "Failed", Attempts: "8" });
});

test("the failed attempts list each run that did not succeed, the newest first, with its outcome and its error", async () => {
  let id = "";
  const app = await atAdmin((fake) => {
    id = fake.operations.id("job:dead-deliver");
    return `/admin/jobs/${id}`;
  });
  const { data } = await app.data.api.GET("/admin/jobs/{id}/attempts", { params: { path: { id } } });

  expect(headsOf("Failed attempts")).toEqual((data?.items ?? []).map((a) => `Attempt ${String(a.attempt)}`));
  expect(headsOf("Failed attempts")).toHaveLength(8);
  // Each row's cells: its outcome first, its error last. The last run's outcome (it was not run again) is its own, and
  // the seven before it share theirs (each ran again).
  const cells = cellsOf("Failed attempts");
  const outcomes = cells.map((row) => row[0] ?? "");
  expect(outcomes.every((outcome) => outcome.startsWith("Outcome") && outcome.length > "Outcome".length)).toBe(true);
  expect(new Set(outcomes.slice(1)).size).toBe(1);
  expect(outcomes.slice(1)).not.toContain(outcomes[0]);
  expect(cells.map((row) => row.at(-1))).toEqual((data?.items ?? []).map((a) => a.error));
  expect(cells[0]?.at(-1)).toBe("deliver to gone.example: used up its attempts: 503 Service Unavailable");
});

test("a job no run of which failed has no section of failed attempts", async () => {
  await atAdmin((fake) => `/admin/jobs/${fake.operations.id("job:running")}`);

  expect(facts("The job")).toMatchObject({ State: "Running" });
  expect(main().queryAllByRole("table", { name: "Failed attempts" })).toHaveLength(0);
  expect(main().queryAllByRole("heading", { name: "Failed attempts" })).toHaveLength(0);
});

for (const key of ["running", "queued", "retrying"])
  test(`a job ${key === "retrying" ? "waiting to retry" : key} offers no Run again`, async () => {
    await atAdmin((fake) => `/admin/jobs/${fake.operations.id(`job:${key}`)}`);

    expect(main().getByLabelText("The job")).toBeDefined();
    expect(main().queryAllByRole("button", { name: "Run again" })).toHaveLength(0);
  });

test("Run again sends nothing until its question is answered, then queues the job, which the screen reads again", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.operations.id("job:dead-email");
  const retried = on.fake.script("POST /admin/jobs/{id}/retry");
  const app = await renderApp(`/admin/jobs/${id}`, { on });
  await main().findByRole("button", { name: "Run again" });
  await settled(app);

  await app.user.click(button("Run again"));
  const first = await asking("Run this job again?");
  await app.user.click(first.getByRole("button", { name: "Cancel" }));
  await answered();
  expect(retried.seen).toHaveLength(0);

  await app.user.click(button("Run again"));
  await app.user.click((await asking("Run this job again?")).getByRole("button", { name: "Run again" }));

  expect((await retried.arrived()).url.pathname).toEndWith(`/admin/jobs/${id}/retry`);
  await says("The job is queued to run again.");
  await factSays("The job", "State", "Queued");
  expect(main().queryAllByRole("button", { name: "Run again" })).toHaveLength(0);
});

test("a job run again is counted among the queued ones once the queue's screen is opened again", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.operations.id("job:dead-email");
  const app = await renderApp("/admin/jobs", { on });
  await main().findByRole("list", { name: "Jobs by state" });
  await settled(app);

  await app.navigate(`/admin/jobs/${id}`);
  await app.user.click(await main().findByRole("button", { name: "Run again" }));
  await app.user.click((await asking("Run this job again?")).getByRole("button", { name: "Run again" }));
  await says("The job is queued to run again.");
  await app.navigate("/admin/jobs");

  await waitFor(() => {
    const figures = within(main().getByRole("list", { name: "Jobs by state" }))
      .getAllByRole("link")
      .map(words);
    if (!figures.includes("4Queued") || !figures.includes("2Failed"))
      throw new Error(`test: the figures read ${figures.join(" / ")}`);
  });
  expect(words(main().getByRole("alert"))).toStartWith("2 jobs failed for good.");
});

test("a Run again the server refuses, as the job was run again meanwhile, reads the job again", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.operations.id("job:dead-email");
  const app = await renderApp(`/admin/jobs/${id}`, { on });
  await main().findByRole("button", { name: "Run again" });
  await settled(app);

  await app.user.click(button("Run again"));
  const question = await asking("Run this job again?");
  expect(await elsewhere(on, "POST", `/admin/jobs/${id}/retry`)).toBe(202);
  const read = on.fake.script("GET /admin/jobs/{id}");
  await app.user.click(question.getByRole("button", { name: "Run again" }));

  await waitFor(() => {
    if (read.seen.length === 0) throw new Error("test: the job was not read again");
  });
  await factSays("The job", "State", "Queued");
  expect(main().queryAllByRole("button", { name: "Run again" })).toHaveLength(0);
});

// Error groups.

test("the errors list the open groups, each row headed by its message and leading to its group", async () => {
  const on = fakeApp({ viewer: "hana" });
  const listed = on.fake.script("GET /admin/error_groups");
  const app = await renderApp("/admin/errors", { on });
  await main().findByRole("table", { name: "Error groups" });
  await settled(app);

  expect(listed.seen.at(-1)?.url.searchParams.get("state")).toBe("open");
  const heads = within(main().getByRole("table", { name: "Error groups" })).getAllByRole("rowheader");
  expect(heads.map(words)).toEqual([
    "runtime error: index out of range [3] with length 3",
    "Cannot read properties of undefined (reading 'media_attachments')",
  ]);
  expect(heads.map((head) => within(head).getByRole("link").getAttribute("href")?.replace(/\?.*$/, ""))).toEqual([
    `/admin/errors/${on.fake.operations.id("error:panic")}`,
    `/admin/errors/${on.fake.operations.id("error:web")}`,
  ]);
});

test("every group, of every state, is listed with its state", async () => {
  const on = fakeApp({ viewer: "hana" });
  const listed = on.fake.script("GET /admin/error_groups");
  const app = await renderApp("/admin/errors?state=any", { on });
  await main().findByRole("table", { name: "Error groups" });
  await settled(app);

  expect(listed.seen.at(-1)?.url.searchParams.get("state")).toBeNull();
  const rows = rowsOf("Error groups");
  expect(rows).toHaveLength(4);
  expect(rows.find((row) => row.startsWith("Loading chunk 812 failed."))).toMatch(/Muted/);
  expect(rows.find((row) => row.startsWith("ERROR: deadlock detected"))).toMatch(/Resolved/);
});

test("a group's facts copy its fingerprint, its request ID, its trace ID and its stack whole", async () => {
  let id = "";
  const app = await atAdmin((fake) => {
    id = fake.operations.id("error:panic");
    return `/admin/errors/${id}`;
  });
  const { data: group } = await app.data.api.GET("/admin/error_groups/{id}", { params: { path: { id } } });

  for (const [name, value] of [
    ["Copy the fingerprint", group?.fingerprint],
    ["Copy the request ID", group?.sample?.request_id],
    ["Copy the trace ID", group?.sample?.trace_id],
    ["Copy the stack", group?.sample?.stack],
  ] as const) {
    await app.user.click(button(name));
    expect(await clipboard()).toBe(value ?? "none held");
  }
  expect(facts("The error")).toMatchObject({ State: "Open", Kind: "Panic", Source: "Server" });
});

test("a group's stack is code, line for line as the server holds it", async () => {
  let id = "";
  const app = await atAdmin((fake) => {
    id = fake.operations.id("error:panic");
    return `/admin/errors/${id}`;
  });
  const { data: group } = await app.data.api.GET("/admin/error_groups/{id}", { params: { path: { id } } });

  // Each line of the stack, in its order; a line is drawn as a line of its own, so no newline is in the text.
  const code = main().getByRole("group", { name: "Stack" }).querySelector("code")?.textContent ?? "";
  const lines = (group?.sample?.stack ?? "").split("\n").map((line) => line.trim());
  expect(lines).toHaveLength(7);
  let from = 0;
  for (const line of lines) {
    const at = code.indexOf(line, from);
    expect([line, at >= from]).toEqual([line, true]);
    from = at + line.length;
  }
  expect(from).toBe(code.length);
});

test("By hour shows the twelve latest hours, and Show more the rest", async () => {
  const hour = 60 * 60 * 1000;
  const start = Math.floor(Date.now() / hour) * hour;
  const app = await atAdmin(
    (fake) => `/admin/errors/${fake.operations.id("error:panic")}`,
    {},
    (on) => {
      on.fake.script("GET /admin/error_groups/{id}", {
        change: (group) => ({
          ...group,
          hourly_counts: Array.from({ length: 20 }, (_, i) => ({
            hour: new Date(start - (19 - i) * hour).toISOString(),
            count: i + 1,
            last_version: "0.42.1",
          })),
        }),
      });
    },
  );

  // The count of each hour, the latest first: the hour of index i counted i + 1.
  const counts = () => cellsOf("By hour").map((cells) => cells[0]);
  expect(counts()).toEqual(Array.from({ length: 12 }, (_, i) => String(20 - i)));

  await app.user.click(button("Show more"));
  await waitFor(() => {
    if (counts().length !== 20) throw new Error(`test: By hour has ${String(counts().length)} rows`);
  });
  expect(counts()).toEqual(Array.from({ length: 20 }, (_, i) => String(20 - i)));
  expect(main().queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
});

test("an open group offers Resolve and Mute; Resolve resolves it, which the group and the open errors read again", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.operations.id("error:panic");
  const resolved = on.fake.script("POST /admin/error_groups/{id}/resolve");
  const app = await renderApp("/admin/errors", { on });
  await main().findByRole("table", { name: "Error groups" });
  await settled(app);

  await app.navigate(`/admin/errors/${id}`);
  await main().findByRole("button", { name: "Resolve" });
  await settled(app);
  expect(
    main()
      .queryAllByRole("button", { name: /^(Resolve|Mute|Unmute)$/ })
      .map(words),
  ).toEqual(["Resolve", "Mute"]);
  await app.user.click(button("Resolve"));

  expect((await resolved.arrived()).url.pathname).toEndWith(`/admin/error_groups/${id}/resolve`);
  await says("The error is resolved.");
  await factSays("The error", "State", "Resolved");
  expect(
    main()
      .queryAllByRole("button", { name: /^(Resolve|Mute|Unmute)$/ })
      .map(words),
  ).toEqual(["Mute"]);

  await app.user.click(button("Errors"));
  await app.arrived("/admin/errors");
  await waitFor(() => {
    const heads = within(main().getByRole("table", { name: "Error groups" }))
      .getAllByRole("rowheader")
      .map(words);
    if (heads.includes("runtime error: index out of range [3] with length 3"))
      throw new Error("test: the resolved group is still among the open ones");
  });
});

test("Mute mutes an open group and offers Unmute, which opens it again", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.operations.id("error:web");
  const muted = on.fake.script("POST /admin/error_groups/{id}/mute");
  const unmuted = on.fake.script("POST /admin/error_groups/{id}/unmute");
  const app = await renderApp(`/admin/errors/${id}`, { on });
  await main().findByRole("button", { name: "Mute" });
  await settled(app);

  await app.user.click(button("Mute"));
  expect((await muted.arrived()).url.pathname).toEndWith(`/admin/error_groups/${id}/mute`);
  await factSays("The error", "State", "Muted");
  expect(
    main()
      .queryAllByRole("button", { name: /^(Resolve|Mute|Unmute)$/ })
      .map(words),
  ).toEqual(["Unmute"]);

  await app.user.click(button("Unmute"));
  expect((await unmuted.arrived()).url.pathname).toEndWith(`/admin/error_groups/${id}/unmute`);
  await factSays("The error", "State", "Open");
  await waitFor(() => {
    const offered = main()
      .queryAllByRole("button", { name: /^(Resolve|Mute|Unmute)$/ })
      .map(words);
    if (offered.join() !== "Resolve,Mute") throw new Error(`test: the group offers ${offered.join()}`);
  });
});

test("a group muted is left out of the open errors once they are opened again", async () => {
  const on = fakeApp({ viewer: "hana" });
  const id = on.fake.operations.id("error:web");
  const app = await renderApp("/admin/errors", { on });
  await main().findByRole("table", { name: "Error groups" });
  await settled(app);

  await app.navigate(`/admin/errors/${id}`);
  await app.user.click(await main().findByRole("button", { name: "Mute" }));
  await factSays("The error", "State", "Muted");
  await app.navigate("/admin/errors");

  await waitFor(() => {
    const heads = within(main().getByRole("table", { name: "Error groups" }))
      .getAllByRole("rowheader")
      .map(words);
    if (heads.join(" / ") !== "runtime error: index out of range [3] with length 3")
      throw new Error(`test: the open errors read ${heads.join(" / ")}`);
  });
});

// The rules of the link card crawler.

/** `n` rules of domains of their own, as the server lists them, by domain. */
const manyRules = (n: number) =>
  Array.from({ length: n }, (_, i) => ({
    domain: `d${String(i).padStart(2, "0")}.example`,
    mode: "no_images" as const,
    note: "",
    updated_at: new Date(Date.UTC(2026, 0, 1)).toISOString(),
  }));

test("the rules show forty, and Show more the rest", async () => {
  const app = await atAdmin(
    () => "/admin/link-card-domains",
    {},
    (on) => {
      on.fake.script("GET /admin/link_card_domains", { change: (list) => ({ ...list, items: manyRules(45) }) });
    },
  );
  const items = () => within(main().getByRole("list", { name: "Rules" })).getAllByRole("listitem");
  expect(items()).toHaveLength(40);
  expect(words(items()[39] ?? document.body)).toStartWith("d39.example");

  await app.user.click(button("Show more"));
  await waitFor(() => {
    if (items().length !== 45) throw new Error(`test: the rules list ${String(items().length)}`);
  });
  expect(words(items()[44] ?? document.body)).toStartWith("d44.example");
});

test("a new rule reaches the server with its mode and note, and joins the rules", async () => {
  const on = fakeApp({ viewer: "hana" });
  const set = on.fake.script("PUT /admin/link_card_domains/{domain}");
  const app = await renderApp("/admin/link-card-domains/new", { on });
  await main().findByRole("textbox", { name: "Domain" });
  await settled(app);

  await app.user.type(main().getByRole("textbox", { name: "Domain" }), "slow.example");
  await app.user.click(main().getByText("Stop fetching"));
  await app.user.type(main().getByRole("textbox", { name: "Note" }), "Too slow to answer.");
  await app.user.click(button("Add a rule"));

  const sent = await set.arrived();
  expect(sent.url.pathname).toEndWith("/admin/link_card_domains/slow.example");
  expect(sent.body).toMatchObject({ mode: "no_fetch", note: "Too slow to answer." });
  await set.served();
  await app.navigate("/admin/link-card-domains");
  const items = within(await main().findByRole("list", { name: "Rules" }))
    .getAllByRole("listitem")
    .map(words);
  expect(items.some((item) => item.startsWith("slow.example Stop fetching"))).toBe(true);
});

test("a new rule for a domain that has one already is refused before it is sent, with a link to that rule", async () => {
  const on = fakeApp({ viewer: "hana" });
  const set = on.fake.script("PUT /admin/link_card_domains/{domain}");
  const app = await renderApp("/admin/link-card-domains/new", { on });
  await main().findByRole("textbox", { name: "Domain" });
  await settled(app);

  await app.user.type(main().getByRole("textbox", { name: "Domain" }), "news.example");
  await app.user.click(main().getByText("Stop fetching"));
  await app.user.click(button("Add a rule"));

  await waitFor(() => {
    if (main().getByRole("textbox", { name: "Domain" }).getAttribute("aria-invalid") !== "true")
      throw new Error("test: the domain is not marked");
  });
  const links = main()
    .getAllByRole("link")
    .filter((link) => link.getAttribute("href") === "/admin/link-card-domains/news.example");
  expect(links).toHaveLength(1);
  expect(set.seen).toHaveLength(0);
  expect(app.path()).toBe("/admin/link-card-domains/new");
});

test("a rule changed to another mode is saved with its note", async () => {
  const on = fakeApp({ viewer: "hana" });
  const set = on.fake.script("PUT /admin/link_card_domains/{domain}");
  const app = await renderApp("/admin/link-card-domains/tracker.example", { on });
  await main().findByRole("button", { name: "Save" });
  await settled(app);

  await app.user.click(main().getByText("Stop images only"));
  await app.user.click(button("Save"));

  const sent = await set.arrived();
  expect(sent.url.pathname).toEndWith("/admin/link_card_domains/tracker.example");
  expect(sent.body).toEqual({ mode: "no_images", note: "Every page of theirs redirects through a tracking service." });
  await app.navigate("/admin/link-card-domains");
  const items = within(await main().findByRole("list", { name: "Rules" }))
    .getAllByRole("listitem")
    .map(words);
  expect(items.some((item) => item.startsWith("tracker.example Stop images only"))).toBe(true);
});

test("a rule is removed only once its question is answered, edits and all, with no question about leaving them", async () => {
  const on = fakeApp({ viewer: "hana" });
  const removed = on.fake.script("DELETE /admin/link_card_domains/{domain}");
  const app = await renderApp("/admin/link-card-domains/tracker.example", { on });
  await main().findByRole("button", { name: "Remove" });
  await settled(app);
  await app.user.type(main().getByRole("textbox", { name: "Note" }), " And more.");

  await app.user.click(button("Remove"));
  const dialogs = screen.getAllByRole("alertdialog");
  expect(dialogs).toHaveLength(1);
  const question = within(dialogs[0] ?? document.body);
  await app.user.click(question.getByRole("button", { name: "Cancel" }));
  await answered();
  expect(removed.seen).toHaveLength(0);

  await app.user.click(button("Remove"));
  const again = within(screen.getByRole("alertdialog"));
  await app.user.click(again.getByRole("button", { name: "Remove" }));

  expect((await removed.arrived()).url.pathname).toEndWith("/admin/link_card_domains/tracker.example");
  await app.arrived("/admin/link-card-domains");
  expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  const items = within(await main().findByRole("list", { name: "Rules" }))
    .getAllByRole("listitem")
    .map(words);
  expect(items.filter((item) => item.startsWith("tracker.example"))).toEqual([]);
});

// A code in a language written right to left.

for (const [what, at, codes, blocks] of [
  [
    "a job",
    (fake: On["fake"]) => `/admin/jobs/${fake.operations.id("job:dead-deliver")}`,
    (fake: On["fake"]) => [fake.operations.id("job:dead-deliver"), "federation.deliver:gone.example"],
    2,
  ],
  [
    "an error group",
    (fake: On["fake"]) => `/admin/errors/${fake.operations.id("error:panic")}`,
    () => ["6f1c2a9d4b7e8f0a3c5d1e2b4a6c8d0e", "01J9Z3Y7WQ2H6N4K8M0P3R5T7V", "4bf92f3577b34da6a3ce929d0e0e4736"],
    1,
  ],
] as const)
  test(`in a language written right to left, the codes of ${what} read left to right`, async () => {
    const on = fakeApp({ viewer: "hana" });
    const app = await renderApp(at(on.fake), { on, locale: "ar-XB" });
    await main().findByRole("heading", { level: 1 });
    await settled(app);

    expect(main().getByRole("heading", { level: 1 }).closest("[dir]")?.getAttribute("dir")).toBe("rtl");
    for (const code of codes(on.fake)) {
      // The innermost element that says the code whole, which may break it into parts where it may wrap.
      const shown = main().getByText(
        (_, element) =>
          element !== null && words(element) === code && ![...element.children].some((child) => words(child) === code),
      );
      expect([code, shown.closest("[dir]")?.getAttribute("dir")]).toEqual([code, "ltr"]);
    }
    // The last error, the payload and the stack: each block of code a group of its own.
    const code = main()
      .getAllByRole("group")
      .filter((group) => group.querySelector("code") !== null);
    expect(code).toHaveLength(blocks);
    for (const block of code) expect(block.closest("[dir]")?.getAttribute("dir")).toBe("ltr");
  });

// Who each screen opens to.

for (const [permission, address, read] of [
  ["manage_jobs", "/admin/jobs", "GET /admin/queue"],
  ["manage_jobs", "/admin/jobs/list", "GET /admin/jobs"],
  ["manage_errors", "/admin/errors", "GET /admin/error_groups"],
  ["manage_errors", "/admin/ux-measurements", "GET /admin/ux_measurements"],
  ["manage_settings", "/admin/link-card-domains", "GET /admin/link_card_domains"],
] as const)
  test(`without ${permission}, ${address} opens no screen and asks the server nothing`, async () => {
    const on = fakeApp({ viewer: "hana" });
    on.fake.script("GET /accounts/verify_credentials", {
      change: (account) => ({ ...account, permissions: account.permissions.filter((p) => p !== permission) }),
    });
    const asked = on.fake.script(read);
    const app = await renderApp("/admin", { on });
    await main().findByRole("navigation", { name: "Administration screens" });
    await settled(app);

    await app.navigate(address);

    expect(await main().findByText("This address leads to no screen open to you.", { exact: false })).toBeDefined();
    await settled(app);
    expect(asked.seen).toHaveLength(0);
  });
