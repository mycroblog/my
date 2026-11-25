// SPDX-License-Identifier: AGPL-3.0-only

// The article editor as a feature (ADR-0160): what its fields tell assistive technology (the summary is described by
// its hint, then by its error), what Publish decides (what the body's count does not change refuses at once, never
// busy; with the preview of the body as it is, it decides on its count at once; pressed before that preview has come,
// it asks for this body's and decides on that; when the preview fails, the publication goes ahead), what the editor
// says of a preview or a summary the server refuses and to a reader who may not write, the appeal of an article the
// automated review refused, and the audience it offers.

import { expect, onTestFinished, test } from "bun:test";
import { articleFieldLimits, defaultArticleLimits } from "@my/app-data";
import { fakeOrigin, type FakeApi, type FakeOptions } from "@my/e2e/fake";
import { createClient, type components } from "@mycroblog/mykit";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, pasteInto, renderFeature } from "../test/feature";
import { ArticleEditor, type ArticleLeaving, type ArticleOpening } from "./article";

const none = () => undefined;

/** The editor of a new article, and its summary field. */
function editor() {
  const user = renderFeature(
    fakeData().data,
    <ArticleEditor opening={{ kind: "new" }} onLeave={none} onOpenDraft={none} />,
  );
  return { user, summary: () => screen.findByRole<HTMLTextAreaElement>("textbox", { name: "Summary" }) };
}

/** The texts of what `field` is described by, in order. */
const describedBy = (field: HTMLElement) =>
  (field.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => document.getElementById(id)?.textContent ?? `test: nothing has the ID ${id}`);

test("the summary is described by its hint", async () => {
  const { summary } = editor();
  const field = await summary();
  const texts = describedBy(field);
  expect(texts).toHaveLength(1);
  expect(texts[0]).toMatch(/\S/);
  expect(field.getAttribute("aria-invalid")).toBeNull();
  expect(screen.getByRole("textbox", { name: "Summary", description: texts[0] ?? "" })).toBe(field);
});

test("the summary is described by its hint, then by its error when it is over its limit", async () => {
  const { user, summary } = editor();
  const field = await summary();
  const [hint] = describedBy(field);
  await user.click(field);
  await user.paste("a".repeat(articleFieldLimits.summary + 1));
  const texts = describedBy(await summary());
  expect(texts).toHaveLength(2);
  expect(texts[0]).toBe(hint);
  expect(texts[1]).toMatch(/\S/);
  expect(texts[1]).not.toBe(hint);
});

// What Publish decides, and what the editor says.

const says = {
  missing: "Give the article a title and a body to publish.",
  over: "Part of the article is over its limit. Shorten it or take out images to publish.",
  refused: "This server's review did not accept the article. Change it and try again.",
};

/**
 * The article editor on a server of `options`, which `prepare` scripts before anything is read, opened for what
 * `opening` says (a new article by default); and how it was left.
 */
function opened(
  options: FakeOptions = {},
  opening: (fake: FakeApi) => ArticleOpening = () => ({ kind: "new" }),
  prepare: (fake: FakeApi) => void = () => undefined,
) {
  const made = fakeData(options);
  prepare(made.fake);
  // What the editor opens (a draft, an article, its text's editor) reaches it when it comes, as a server's answer
  // does, not inside an act() of the test: React is told so until the test ends, and the test waits for what it
  // asserts.
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  onTestFinished(() => {
    if (wrapped === undefined) delete environment.IS_REACT_ACT_ENVIRONMENT;
    else environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
  const left: ArticleLeaving["kind"][] = [];
  const user = renderFeature(
    made.data,
    <ArticleEditor opening={opening(made.fake)} onLeave={(how) => left.push(how.kind)} onOpenDraft={none} />,
  );
  return { ...made, user, left };
}

/** The editor of a new article and its parts, once its body's editor has come. */
async function writing(options: FakeOptions = {}, prepare?: (fake: FakeApi) => void) {
  const made = opened(options, undefined, prepare);
  const body = await screen.findByRole("textbox", { name: "Body" });
  return {
    ...made,
    title: screen.getByRole<HTMLInputElement>("textbox", { name: "Title" }),
    body,
    summary: screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Summary" }),
    publish: screen.getByRole("button", { name: "Publish" }),
  };
}

type Writing = Awaited<ReturnType<typeof writing>>;

/** The body of a draft or of an article opened in the editor, once the editor holds what was written. */
const writtenBody = () =>
  waitFor(() => {
    const body = screen.getByRole("textbox", { name: "Body" });
    if (body.textContent === "") throw new Error("test: the body holds nothing yet");
    return body;
  });

/** Writes `text` at the end of the body. */
async function write({ user, body }: Writing, text: string): Promise<void> {
  await user.click(body);
  await pasteInto(user, body, text);
}

/** Writes `text` in place of the whole body. */
async function rewrite({ user, body }: Writing, text: string): Promise<void> {
  await user.click(body);
  await user.keyboard("{Control>}a{/Control}{Delete}");
  await waitFor(() => {
    if (body.textContent !== "") throw new Error("test: the body is not empty yet");
  });
  await pasteInto(user, body, text);
}

/** What the editor's alerts say. */
const alerts = () => screen.queryAllByRole("alert").map((alert) => alert.textContent);

/** The share of the length limit that the limits' line says the body uses, in percent. */
const lengthUsed = () => Number(/^Length: (\d+)% of the limit$/.exec(screen.getByText(/^Length: /).textContent)?.[1]);

/** Waits until the limits' line says a share of the limit that `is` so: the preview it counts has come. */
const lengthBecomes = (is: (percent: number) => boolean) =>
  waitFor(() => {
    if (!is(lengthUsed())) throw new Error(`test: the limits say ${String(lengthUsed())}% of the limit`);
  });

const isBusy = (button: HTMLElement) => button.getAttribute("aria-busy") === "true";

/** Watches whether `button` goes busy from now on: the returned function says so. */
function busyWatch(button: HTMLElement): () => boolean {
  let went = isBusy(button);
  const watch = new MutationObserver(() => {
    if (isBusy(button)) went = true;
  });
  watch.observe(button, { attributes: true, attributeFilter: ["aria-busy"] });
  onTestFinished(() => {
    watch.disconnect();
  });
  return () => went;
}

/** Presses Publish, which refuses at once: the alert says `why`, it never went busy, nothing was sent, the editor stays. */
async function refusedAtOnce(editor: Writing, why: string): Promise<void> {
  const busy = busyWatch(editor.publish);
  const sent = editor.requests.length;
  await editor.user.click(editor.publish);
  expect(alerts()).toEqual([why]);
  expect(busy()).toBe(false);
  expect(editor.requests.slice(sent).filter((request) => request.startsWith("POST"))).toEqual([]);
  expect(editor.left).toEqual([]);
  expect(screen.getByRole("heading", { level: 1, name: "New article" })).toBeDefined();
}

/** A body of 180 bytes of HTML and more: over a limit of 60, where "Flour and water." is under it. */
const long = "flour ".repeat(30);
const short = "Flour and water.";
const limit = 60;

/** Makes the instance say a limit of `limit` bytes for an article's body; called before the editor reads it. */
function limitInstance(fake: FakeApi): void {
  fake.script("GET /instance", {
    change: (instance) => ({
      ...instance,
      limits: { ...instance.limits, articles: { ...defaultArticleLimits, max_bytes: limit } },
    }),
  });
}

/** Makes each preview count against the same limit, and returns the script of the previews. */
const limitPreviews = (fake: FakeApi) =>
  fake.script("POST /articles/preview", { change: (preview) => ({ ...preview, max_bytes: limit }) });

/** Holds the answers of the previews from now on, under the same limit, in place of the script `before`. */
function holdPreviews(fake: FakeApi, before: { end: () => void }) {
  before.end();
  return fake.script("POST /articles/preview", {
    hold: "response",
    change: (preview) => ({ ...preview, max_bytes: limit }),
  });
}

test("Publish without a title refuses at once, never busy, and the editor stays", async () => {
  const editor = await writing();
  await write(editor, short);

  await refusedAtOnce(editor, says.missing);
});

test("Publish without a body refuses at once, and the limits count nothing", async () => {
  const editor = await writing();
  await editor.user.type(editor.title, "A starter in a jar");

  await refusedAtOnce(editor, says.missing);
  expect(lengthUsed()).toBe(0);
});

test("Publish with a summary over its limit refuses at once, the summary marked invalid", async () => {
  const editor = await writing();
  await editor.user.type(editor.title, "A starter in a jar");
  await write(editor, short);
  await editor.user.click(editor.summary);
  await editor.user.paste("a".repeat(articleFieldLimits.summary + 1));
  const busy = busyWatch(editor.publish);

  await editor.user.click(editor.publish);

  expect(editor.summary.getAttribute("aria-invalid")).toBe("true");
  expect(alerts()).toHaveLength(1);
  expect(busy()).toBe(false);
  expect(editor.requests).not.toContain("POST /articles");
  expect(editor.left).toEqual([]);
});

test("a title over its limit is invalid and says by how much, and Publish refuses, the title kept", async () => {
  const editor = await writing();
  const title = "A starter in a jar. ".repeat(11).slice(0, articleFieldLimits.title + 1);
  await editor.user.click(editor.title);
  await editor.user.paste(title);
  await write(editor, short);

  expect(editor.title.getAttribute("aria-invalid")).toBe("true");
  // Under the title, what is wrong, which describes it; at the end of its line, by how much.
  expect(describedBy(editor.title).join("")).toMatch(/\S/);
  expect(screen.getByText("1 character over")).toBeDefined();

  await editor.user.click(editor.publish);
  expect(alerts()).toEqual([says.over]);
  expect(editor.requests).not.toContain("POST /articles");
  expect(editor.left).toEqual([]);
  expect(editor.title.value).toBe(title);
});

test("with the preview of a body over the length limit, the limits say so, and Publish refuses at once", async () => {
  const editor = await writing({}, limitInstance);
  limitPreviews(editor.fake);
  await editor.user.type(editor.title, "Over the limit");
  await write(editor, long);
  await lengthBecomes((percent) => percent > 100);

  await refusedAtOnce(editor, says.over);
  expect(editor.title.value).toBe("Over the limit");
});

test("a body cut under the limit and published at once, before its preview has come, is published", async () => {
  const editor = await writing({}, limitInstance);
  const previews = limitPreviews(editor.fake);
  const sent = editor.fake.script("POST /articles");
  await editor.user.type(editor.title, "Cut and published");
  await write(editor, long);
  await lengthBecomes((percent) => percent > 100);
  const held = holdPreviews(editor.fake, previews);
  await rewrite(editor, "Flour and water, cut short.");
  // The preview shown is still the long body's.
  expect(lengthUsed()).toBeGreaterThan(100);

  await editor.user.click(editor.publish);
  // Publish asks for this body's preview, busy meanwhile, and publishes on its count.
  expect(held.seen.map((asked) => asked.body.text)).toContain("Flour and water, cut short.");
  expect(isBusy(editor.publish)).toBe(true);
  expect(sent.seen).toHaveLength(0);

  held.release();
  await waitFor(() => {
    expect(editor.left).toEqual(["published"]);
  });
  expect(sent.seen.map((article) => article.body.text)).toEqual(["Flour and water, cut short."]);
});

test("a body grown past the limit and published at once, before its preview has come, is refused as over, on its own count", async () => {
  const editor = await writing({}, limitInstance);
  const previews = limitPreviews(editor.fake);
  await editor.user.type(editor.title, "Grown and published");
  await write(editor, short);
  await lengthBecomes((percent) => percent > 0 && percent <= 100);
  const held = holdPreviews(editor.fake, previews);
  await rewrite(editor, long);
  // The preview shown is still the short body's.
  expect(lengthUsed()).toBeLessThanOrEqual(100);

  await editor.user.click(editor.publish);
  expect(isBusy(editor.publish)).toBe(true);
  expect(alerts()).toEqual([]);

  held.release();
  await waitFor(() => {
    expect(alerts()).toEqual([says.over]);
  });
  // Refused, Publish is no longer busy, and the limits are this body's.
  expect(isBusy(editor.publish)).toBe(false);
  await lengthBecomes((percent) => percent > 100);
  expect(editor.requests).not.toContain("POST /articles");
  expect(editor.left).toEqual([]);
});

test("when the preview of the body fails, Publish goes ahead, and the server's refusal shows in its words", async () => {
  const editor = await writing({ articles: { preview: "fails", publish: "refused" } });
  await editor.user.type(editor.title, "Refused by the review");
  await write(editor, short);

  await editor.user.click(editor.publish);

  await waitFor(() => {
    expect(alerts()).toContain(says.refused);
  });
  expect(editor.requests).toContain("POST /articles");
  expect(isBusy(editor.publish)).toBe(false);
  expect(editor.left).toEqual([]);
});

// An article the server's automated review refused, with a decision to appeal (#3217, ADR-0147 §9).

const decision = "01JREVIEWDECISION000000000";
const reviewRefused = "This article was not published: this server's automated review did not accept it.";

/** Makes each publication refused by the review, naming `decision` to appeal; called before the editor publishes. */
function refuseByReview(fake: FakeApi): void {
  const problem: components["schemas"]["StatusRejectedProblem"] = {
    type: "/api/v1/problems/status-rejected",
    title: "This post was not published.",
    status: 422,
    detail: "This server's automated review did not accept this post. If you think this is a mistake, you can appeal.",
    hook_decision_id: decision,
    appeal_deadline: "2031-03-09T12:00:00.000Z",
  };
  fake.script("POST /articles", { fail: problem });
}

/** Whether the reader can press `button`. */
const pressable = (button: HTMLElement) =>
  !(button.hasAttribute("disabled") || button.getAttribute("aria-disabled") === "true");

/** Writes an article with a title, a Markdown body and a summary, presses Publish, and waits for the refusal. */
async function publishedRefused(editor: Writing): Promise<void> {
  await editor.user.type(editor.title, "A starter in a jar");
  await write(editor, "Flour and **water**.");
  await editor.user.click(editor.summary);
  await editor.user.paste("How to keep a starter.");
  await editor.user.click(editor.publish);
  await waitFor(() => {
    expect(alerts().map((said) => said.replace(/\s*Appeal$/, "").trim())).toEqual([reviewRefused]);
  });
}

test("an article the review refused says so with Appeal; the appeal sends its Markdown body, not its title or summary, and Publish is held back", async () => {
  const editor = await writing({}, refuseByReview);
  const sent = editor.fake.script("POST /hook_decisions/{id}/appeal");
  await publishedRefused(editor);
  const appeal = screen.getByRole("button", { name: "Appeal" });
  expect(pressable(editor.publish)).toBe(true);

  await editor.user.click(appeal);
  const dialog = await screen.findByRole("alertdialog", { name: "Appeal this refusal?" });
  await editor.user.click(within(dialog).getByRole("button", { name: "Send appeal" }));

  await waitFor(() => {
    expect(sent.seen).toHaveLength(1);
  });
  expect(sent.seen.map((request) => [request.url.pathname.split("/").at(-2), request.body])).toEqual([
    [decision, { status_text: "Flour and **water**." }],
  ]);
  await waitFor(() => {
    expect(pressable(editor.publish)).toBe(false);
  });
  expect(screen.queryAllByRole("button", { name: "Appeal" })).toHaveLength(0);
  expect(editor.left).toEqual([]);
});

test("an article whose appeal was reviewed and the refusal kept holds Publish back, with no Appeal", async () => {
  const editor = await writing({}, (fake) => {
    refuseByReview(fake);
    fake.script("GET /hook_decisions/{id}/appeal", {
      change: (made) => ({ ...made, state: "upheld" as const, decided_at: "2026-03-09T12:00:00.000Z" }),
    });
  });
  const elsewhere = createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: (r) => editor.fake.serve(r) });
  const { error } = await elsewhere.POST("/hook_decisions/{id}/appeal", {
    params: { path: { id: decision } },
    body: { status_text: "Flour and **water**." },
  });
  if (error !== undefined) throw new Error("test: the fake refused the appeal");

  await editor.user.type(editor.title, "A starter in a jar");
  await write(editor, "Flour and **water**.");
  await editor.user.click(editor.publish);

  await waitFor(() => {
    expect(pressable(editor.publish)).toBe(false);
  });
  expect(alerts()).toEqual([
    "This article was not published. A moderator reviewed your appeal and kept the refusal. Change the article and try again.",
  ]);
  expect(screen.queryAllByRole("button", { name: "Appeal" })).toHaveLength(0);
  expect(editor.left).toEqual([]);
});

test("a draft whose preview the server could not render says so in the preview, with Try again, the draft kept", async () => {
  opened({ articles: { preview: "fails" } }, (fake) => ({ kind: "draft", id: fake.idOf("yuki-article-trail") }));

  const title = await screen.findByRole<HTMLInputElement>("textbox", { name: "Title" });
  expect(title.value).toBe("A photo walk along the north ridge");
  expect((await writtenBody()).textContent).toContain("The trail starts behind the old station.");
  // Without a layout the preview renders in each of its places, beside the form and under it: the first one is read.
  const notice = await waitFor(() => {
    const [preview] = screen.getAllByRole("article");
    return within(preview as HTMLElement).getByRole("alert");
  });
  expect(notice.textContent).toMatch(/^The preview could not be shown\.\s*Try again$/);
  expect(within(notice).getByRole("button", { name: "Try again" })).toBeDefined();
  // Nothing of the body is shown as if it were rendered: its headings would be of level 3.
  expect(screen.queryAllByRole("heading", { level: 3 })).toHaveLength(0);
});

test("an edit refused for its summary comes back to Write with the summary invalid and described by what is wrong, and the next edit of the summary clears it", async () => {
  // What the fake server says of the summary it refuses (`publish: "summary"`).
  const refusal = { title: "This could not be done.", detail: "This value is not valid. Check what you entered." };
  const { user, left } = opened({ viewer: "sam", articles: { publish: "summary" } }, (fake) => ({
    kind: "edit",
    id: fake.idOf("sam-article"),
  }));
  await screen.findByRole("heading", { level: 1, name: "Edit article" });
  await writtenBody();
  const mode = within(screen.getByRole("radiogroup", { name: "Write or preview" }));
  const checked = (name: "Write" | "Preview") => mode.getByRole("radio", { name }).getAttribute("aria-checked");
  await user.click(mode.getByRole("radio", { name: "Preview" }));
  expect([checked("Write"), checked("Preview")]).toEqual(["false", "true"]);

  await user.click(screen.getByRole("button", { name: "Save" }));

  const summary = screen.getByRole("textbox", { name: "Summary" });
  await waitFor(() => {
    if (summary.getAttribute("aria-invalid") !== "true") throw new Error("test: the summary is not marked invalid");
  });
  expect([checked("Write"), checked("Preview")]).toEqual(["true", "false"]);
  expect(describedBy(summary).at(-1)).toBe(refusal.detail);
  // The alert says the problem's title alone: its detail stands under the field.
  expect(alerts()).toEqual([refusal.title]);
  expect(left).toEqual([]);

  await user.type(summary, " Shorter.");
  expect(summary.getAttribute("aria-invalid")).toBeNull();
  expect(screen.queryAllByText(refusal.detail)).toHaveLength(0);
  expect(alerts()).toEqual([]);
});

test("a reader the server keeps from writing new articles is told why, with nothing to write in or to publish", async () => {
  opened({ articles: { authoring: "permission" } });

  expect(
    await screen.findByText(
      "Your account cannot write new articles on this server. You can still edit the articles you wrote before.",
    ),
  ).toBeDefined();
  expect(screen.queryAllByRole("textbox")).toHaveLength(0);
  expect(screen.queryAllByRole("button", { name: "Publish" })).toHaveLength(0);
});

test("the audience of an article is who can see it, never Direct, and who can reply, boost and quote it, with approval", async () => {
  const { user } = await writing();

  await user.click(screen.getByRole("button", { name: /^Who can see it: / }));
  const top = () => screen.findByRole("menu", { name: "Who can see and reply" });
  expect(within(await top()).getByRole("menuitem", { name: /^Who can reply, boost and quote/ })).toBeDefined();

  await user.click(within(await top()).getByRole("menuitem", { name: /^Who can see it/ }));
  const visibilities = within(await screen.findByRole("menu", { name: "Who can see it" }));
  expect(visibilities.getAllByRole("menuitemradio")).toHaveLength(4);
  expect(visibilities.queryAllByRole("menuitemradio", { name: /^Direct/ })).toHaveLength(0);

  await user.click(screen.getByRole("button", { name: "Back" }));
  await user.click(within(await top()).getByRole("menuitem", { name: /^Who can reply, boost and quote/ }));
  await user.click(await screen.findByRole("menuitem", { name: /^Who can reply(?!,)/ }));
  await user.click(await screen.findByRole("menuitemradio", { name: "Followers" }));
  expect(await screen.findByRole("menuitemcheckbox", { name: "Allow others with your approval" })).toBeDefined();
});
