// SPDX-License-Identifier: AGPL-3.0-only

// A post the server refused, in the composer: the input it names stands invalid, described by what is wrong with it,
// and the form's alert says the refusal's title alone; a refusal of no input says the server's words in the alert.
// And a post the server's automated review refused (#3204, ADR-0147 §9): the alert says so in the client's own
// sentence with Appeal, which asks first and then sends the post's text to the moderators, once; where the appeal
// stands takes the alert's place, and the post is held back while the refusal stands.

import { describe, expect, test } from "bun:test";
import { defaultComposeSettings, type MediaAttachment } from "@my/app-data";
import { fakeOrigin, type FakeApi } from "@my/e2e/fake";
import { createClient, type components } from "@mycroblog/mykit";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, pasteInto, renderFeature, type FakeData } from "../test/feature";
import { ComposerView } from "./composer";
import { emptyDraft, type Draft } from "./model";
import { useComposer, type Phase } from "./use-composer";

const none = () => undefined;
const title = "This could not be done.";
const detail = "This value is not valid. Check what you entered.";

/**
 * The composer's view of `fields`, in `phase` as a story's specimen opens it, or as a reader opens it when none is
 * given, on the server `on`: its dialog.
 */
function composer(fields: Partial<Draft>, phase?: Phase, on: FakeData = fakeData()) {
  const initial = { ...emptyDraft({ kind: "new" }, defaultComposeSettings, "k"), text: "when?", ...fields };
  function Composing() {
    const c = useComposer(initial, { onPosted: none, onKept: none }, phase);
    return <ComposerView c={c} layer={false} onClose={none} onDismiss={none} />;
  }
  const user = renderFeature(on.data, <Composing />);
  return Object.assign(within(screen.getByRole("dialog")), { user, on });
}

/** The fields of the composer that are marked invalid. */
const invalid = (form: ReturnType<typeof composer>) =>
  form
    .queryAllByRole<HTMLInputElement | HTMLTextAreaElement>("textbox")
    .filter((field) => field.getAttribute("aria-invalid") === "true");

/** What describes `field`, part by part. */
const described = (field: Element) =>
  (field.getAttribute("aria-describedby") ?? "").split(" ").map((id) => document.getElementById(id)?.textContent);

const failed = (input: string): Phase => ({ kind: "failed", reason: title, field: { input, text: detail } });

test("a poll's option refused stands invalid, described by what is wrong, and the alert says the title alone", () => {
  const form = composer(
    { poll: { options: ["Sat", "Sun"], duration: 3600, multiple: false, hideTotals: false } },
    failed("option:1"),
  );

  const marked = invalid(form);
  expect(marked.map((field) => field.value)).toEqual(["Sun"]);
  expect(marked.flatMap(described)).toContain(detail);
  expect(form.getByRole("alert").textContent).toBe(title);
});

test("an image's description refused stands invalid under that image, described by what is wrong", () => {
  const ready = (key: string, description: string) => ({
    key,
    state: "ready" as const,
    media: { id: `m-${key}`, type: "image", state: "ready", url: `/${key}.png` } as unknown as MediaAttachment,
    description,
  });
  const form = composer({ attachments: [ready("a1", "a hill"), ready("a2", "a river")] }, failed("media:a2"));

  const marked = invalid(form);
  // The second image's description, after the first's.
  expect(marked.map((field) => field.value)).toEqual(["a river"]);
  expect(marked.flatMap(described)).toContain(detail);
  expect(form.getByRole("alert").textContent).toBe(title);
});

test("a refusal of no input says the server's words in the alert, and marks no input", () => {
  const form = composer({}, { kind: "failed", reason: "The post was refused." });

  expect(invalid(form)).toHaveLength(0);
  expect(form.getByRole("alert").textContent).toBe("The post was refused.");
});

test("a content warning refused stands invalid, described by what is wrong, and the alert says the title alone", async () => {
  const form = composer({ cw: "Spoilers for the season finale" }, failed("cw"));

  // The warning's editor is a chunk of its own: it is there once it came.
  const warning = await form.findByRole("textbox", { name: "Content warning" });
  expect(warning.getAttribute("aria-invalid")).toBe("true");
  expect(described(warning)).toContain(detail);
  // The option's and the descriptions' fields are not there to be marked, and the text is not.
  expect(form.getAllByRole("textbox").filter((field) => field.getAttribute("aria-invalid") === "true")).toHaveLength(1);
  expect(form.getByRole("alert").textContent).toBe(title);
});

test("the next letter typed in a refused option clears what the server said: the option is valid again, and neither the detail nor the title stands", async () => {
  const form = composer(
    { poll: { options: ["Sat", "Sun"], duration: 3600, multiple: false, hideTotals: false } },
    failed("option:1"),
  );
  const [option] = invalid(form);
  if (option === undefined) throw new Error("test: no option is refused");

  await form.user.type(option, "s");

  await waitFor(() => {
    if (invalid(form).length > 0) throw new Error("the option is still invalid");
  });
  expect(form.getByRole<HTMLInputElement>("textbox", { name: "Option 2" }).value).toBe("Suns");
  expect(form.queryAllByText(detail)).toHaveLength(0);
  expect(form.queryAllByText(title)).toHaveLength(0);
  expect(form.queryAllByRole("alert")).toHaveLength(0);
  // The text's editor is a chunk of its own: the test ends once it came.
  await form.findAllByRole("textbox", { name: "Text" });
});

describe("a post the server's automated review refused", () => {
  const decision = "01JREVIEWDECISION000000000";
  const route = "POST /hook_decisions/{id}/appeal";
  /** The server's own words of the refusal, which the composer does not repeat. */
  const serverWords =
    "This server's automated review did not accept this post. If you think this is a mistake, you can appeal.";
  const reviewed: Phase = {
    kind: "failed",
    reason: serverWords,
    appeal: { decision, deadline: "2031-03-09T12:00:00.000Z" },
  };
  const refused = "This post was not published: this server's automated review did not accept it.";
  const pending =
    "This post was not published. Your appeal was sent, and you will be notified of the result. If the moderators lift the refusal, post it again. To keep it until then, save it as a draft.";
  const upheld =
    "This post was not published. A moderator reviewed your appeal and kept the refusal. Change the post and try again.";
  const overturned = "A moderator reviewed your appeal and lifted the refusal. Post again.";

  type Form = ReturnType<typeof composer>;

  /** What the composer's alert says, before the name of the button it may end with. */
  const alertSays = (form: Form) =>
    form.queryAllByRole("alert").map((alert) => alert.textContent.replace(/\s*Appeal$/, "").trim());
  /** The buttons named Appeal the composer shows. */
  const appeals = (form: Form) => form.queryAllByRole("button", { name: "Appeal" });
  /** Whether the reader can press Post. */
  const postable = (form: Form) => {
    const post = form.getByRole<HTMLButtonElement>("button", { name: "Post" });
    return !(post.disabled || post.getAttribute("aria-disabled") === "true");
  };
  /** The confirmation of an appeal, once it is open. */
  const asking = () => screen.findByRole("alertdialog", { name: "Appeal this refusal?" });
  /** The requests about an appeal of a refusal the composer sent. */
  const asked = ({ on }: Form) => on.requests.filter((request) => request.includes("/hook_decisions/"));

  /** Presses Appeal and confirms, and returns once the confirmation closed. */
  async function appealed(form: Form): Promise<void> {
    await form.user.click(form.getByRole("button", { name: "Appeal" }));
    await form.user.click(within(await asking()).getByRole("button", { name: "Send appeal" }));
    await waitFor(() => {
      if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: the confirmation is still open");
    });
  }

  /** The composer of a refused post once it asked where its appeal stands, and its editor came. */
  async function refusedPost(fields: Partial<Draft> = {}, on: FakeData = fakeData()): Promise<Form> {
    const form = composer(fields, reviewed, on);
    await waitFor(() => {
      if (asked(form).length === 0) throw new Error("test: the composer did not ask where the appeal stands yet");
      if (on.data.queryClient.isFetching() > 0) throw new Error("test: the composer is still reading");
    });
    await form.findAllByRole("textbox", { name: "Text" });
    return form;
  }

  /** Makes the appeal of the decision on `fake` as the reader did before, from another place. */
  async function appealBefore(fake: FakeApi): Promise<void> {
    const elsewhere = createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: (r) => fake.serve(r) });
    const { error } = await elsewhere.POST("/hook_decisions/{id}/appeal", {
      params: { path: { id: decision } },
      body: { status_text: "when?" },
    });
    if (error !== undefined) throw new Error("test: the fake refused the appeal");
  }

  test("the alert says so in the composer's own sentence, not the server's, with Appeal; Post can still be pressed", async () => {
    const form = await refusedPost();

    expect(alertSays(form)).toEqual([refused]);
    expect(appeals(form)).toHaveLength(1);
    expect(form.queryAllByText(serverWords)).toHaveLength(0);
    expect(postable(form)).toBe(true);
    // Where an appeal stands was asked once, and none was made.
    expect(asked(form)).toEqual([`GET /hook_decisions/${decision}/appeal`]);
  });

  test("where the appeal stands could not be read: the alert still offers Appeal", async () => {
    const on = fakeData();
    const failing = on.fake.script("GET /hook_decisions/{id}/appeal", { fail: 500 });
    const form = await refusedPost({}, on);

    expect(failing.seen.length).toBeGreaterThan(0);
    expect(alertSays(form)).toEqual([refused]);
    expect(appeals(form)).toHaveLength(1);
  });

  test("a refusal whose time to appeal has passed: the alert says so, and offers no Appeal", async () => {
    const form = composer({}, { ...reviewed, appeal: { decision, deadline: "2024-03-09T12:00:00.000Z" } });
    await waitFor(() => {
      if (form.on.data.queryClient.isFetching() > 0) throw new Error("test: the composer is still reading");
    });
    await form.findAllByRole("textbox", { name: "Text" });

    expect(alertSays(form)).toEqual(["The time to appeal this refusal has passed. Change the post and try again."]);
    expect(appeals(form)).toHaveLength(0);
  });

  test("a refusal that names no decision to appeal offers no Appeal, and asks the server nothing of one", async () => {
    const form = composer({}, { kind: "failed", reason: "The post was refused." });
    await form.findAllByRole("textbox", { name: "Text" });

    expect(alertSays(form)).toEqual(["The post was refused."]);
    expect(appeals(form)).toHaveLength(0);
    expect(asked(form)).toEqual([]);
  });

  test("a post the server refuses so shows the alert with Appeal; one it refuses without naming a decision shows none", async () => {
    const problem = (more: object): components["schemas"]["StatusRejectedProblem"] => ({
      type: "/api/v1/problems/status-rejected",
      title: "This post was not published.",
      status: 422,
      detail: serverWords,
      ...more,
    });
    const on = fakeData();
    const named = on.fake.script("POST /statuses", {
      times: 1,
      fail: problem({ hook_decision_id: decision, appeal_deadline: "2031-03-09T12:00:00.000Z" }),
    });
    on.fake.script("POST /statuses", { fail: problem({}) });
    const form = composer({}, undefined, on);
    await form.findAllByRole("textbox", { name: "Text" });

    await form.user.click(form.getByRole("button", { name: "Post" }));

    await waitFor(() => {
      expect(alertSays(form)).toEqual([refused]);
    });
    expect(appeals(form)).toHaveLength(1);
    await named.arrived();
    named.end();

    await form.user.click(form.getByRole("button", { name: "Post" }));

    await waitFor(() => {
      expect(appeals(form)).toHaveLength(0);
    });
    expect(alertSays(form)).toHaveLength(1);
    expect(alertSays(form)).not.toEqual([refused]);
  });

  test("Appeal asks first, saying that the post's text goes to the moderators, once, and cannot be undone; Cancel sends nothing", async () => {
    const form = await refusedPost();

    await form.user.click(form.getByRole("button", { name: "Appeal" }));

    const dialog = await asking();
    expect(described(dialog)).toEqual([
      "The text of your post is sent to this server's moderators, who review it. You can appeal a refusal once. This cannot be undone.",
    ]);
    expect(
      within(dialog)
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label") ?? button.textContent),
    ).toEqual(["Close", "Cancel", "Send appeal"]);

    await form.user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: the confirmation is still open");
    });
    expect(asked(form).filter((request) => request.startsWith("POST "))).toEqual([]);
    expect(alertSays(form)).toEqual([refused]);
    expect(appeals(form)).toHaveLength(1);
  });

  test("confirmed, the appeal is sent once with the post's text; that it was sent takes the alert's place, and Post is held back", async () => {
    const form = await refusedPost({ text: "Just a recipe for sourdough." });
    const sent = form.on.fake.script(route);

    await appealed(form);

    await waitFor(() => {
      expect(alertSays(form)).toEqual([pending]);
    });
    expect(sent.seen.map((request) => [request.url.pathname.split("/").at(-2), request.body])).toEqual([
      [decision, { status_text: "Just a recipe for sourdough." }],
    ]);
    expect(appeals(form)).toHaveLength(0);
    expect(postable(form)).toBe(false);
  });

  // Where an appeal made before stands, as the server holds it when the composer opens on the refusal.
  const stands = [
    ["waits for a moderator", undefined, pending, false],
    ["was reviewed and the refusal kept", "upheld", upheld, false],
    ["was reviewed and the refusal lifted", "overturned", overturned, true],
  ] as const;
  for (const [how, state, says, posts] of stands)
    test(`an appeal that ${how}: the alert says so, with no Appeal, and Post ${posts ? "can be pressed" : "is held back"}`, async () => {
      const on = fakeData();
      await appealBefore(on.fake);
      if (state !== undefined)
        on.fake.script("GET /hook_decisions/{id}/appeal", {
          change: (made) => ({ ...made, state, decided_at: "2026-03-09T12:00:00.000Z" }),
        });

      const form = await refusedPost({}, on);

      await waitFor(() => {
        expect(alertSays(form)).toEqual([says]);
      });
      expect(appeals(form)).toHaveLength(0);
      expect(postable(form)).toBe(posts);
    });

  test("a change to the post clears the refusal: nothing of the appeal stands, and Post can be pressed again", async () => {
    const on = fakeData();
    await appealBefore(on.fake);
    const form = await refusedPost({}, on);
    await waitFor(() => {
      expect(alertSays(form)).toEqual([pending]);
    });
    expect(postable(form)).toBe(false);
    const text = form.getByRole("textbox", { name: "Text" });

    await form.user.click(text);
    await pasteInto(form.user, text, " Tomorrow?");

    await waitFor(() => {
      expect(form.queryAllByRole("alert")).toHaveLength(0);
    });
    expect(postable(form)).toBe(true);
  });

  test("a post with no text cannot be appealed: the alert says so, and nothing is sent", async () => {
    const form = await refusedPost({
      text: "",
      poll: { options: ["Sat", "Sun"], duration: 3600, multiple: false, hideTotals: false },
    });

    await appealed(form);

    await waitFor(() => {
      expect(alertSays(form)).toEqual([
        "This post has no text to send to the moderators, so it cannot be appealed. Change the post and try again.",
      ]);
    });
    expect(asked(form).filter((request) => request.startsWith("POST "))).toEqual([]);
  });

  /** A problem of the server of `type`, as it refuses an appeal. */
  const refusal = (type: string) =>
    ({ type: `/api/v1/problems/${type}`, title: "This could not be done.", status: 422 }) as const;
  // What the server answers the appeal with, and what the alert says then.
  const refusals = [
    [
      "the refusal was appealed already",
      refusal("already-appealed"),
      "You have already appealed this refusal, and a refusal can be appealed once. Wait for the notification of the result.",
    ],
    [
      "the time to appeal has passed",
      refusal("appeal-window-closed"),
      "The time to appeal this refusal has passed. Change the post and try again.",
    ],
    [
      "the refusal can no longer be appealed",
      refusal("not-appealable"),
      "This refusal can no longer be appealed. Change the post and try again.",
    ],
    ["the server fails", 500, "The appeal could not be sent. Check your connection and try again."],
    ["the server is not reached", "network", "The appeal could not be sent. Check your connection and try again."],
  ] as const;
  for (const [why, fail, says] of refusals)
    test(`an appeal the server does not take, because ${why}: the alert says so`, async () => {
      const form = await refusedPost();
      form.on.fake.script(route, { fail });

      await appealed(form);

      await waitFor(() => {
        expect(alertSays(form)).toEqual([says]);
      });
      expect(alertSays(form)).not.toContain(pending);
    });
});
