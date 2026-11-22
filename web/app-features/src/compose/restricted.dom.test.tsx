// SPDX-License-Identifier: AGPL-3.0-only

// A post or an article refused for a partial restriction of the reader's account (#3216): the form's alert says the
// server's sentence and links to Account status in the settings, which a press leads to, leaving the composer. A
// refusal of another kind offers no such link. That the composer's draft is kept on the way is the app's route test
// (web/app/src/compose-restricted.dom.test.tsx).

import { expect, test } from "bun:test";
import { accountStatusPath } from "@my/app-components";
import { defaultComposeSettings } from "@my/app-data";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { RouterProvider } from "react-aria-components";
import { fakeData, pasteInto, renderFeature, type FakeData } from "../test/feature";
import { ArticleEditor } from "./article";
import { ComposerView } from "./composer";
import { emptyDraft, type Draft, type Target } from "./model";
import { useComposer, type Phase } from "./use-composer";

const none = () => undefined;

/** The server's sentence of a post refused for a restriction (server:errors.status.posting_restricted). */
const posting =
  "A moderation action keeps your account from posting for now. To see why or to appeal, open “Account status” under “Account and data” in the settings.";
/** A sentence of a reply or a quote refused for a restriction of interacting. */
const interacting = "A moderation action keeps your account from interacting with others for now.";

/** A problem of the server of `type`, saying `detail`. */
const refusal = (type: string, detail: string) =>
  ({ type: `/api/v1/problems/${type}`, title: "Forbidden", status: 403, detail }) as const;

/** sam's post, which the reader answers or quotes. */
const seeded = () =>
  fakeData({
    seed: {
      people: scenario.people,
      posts: [{ key: "loaf", author: "sam", text: "The first loaf.", visibility: "public", language: "en" }],
    },
  });

/**
 * The composer's view of `target` holding `fields`, in `phase` when given, on the server `on`, in `locale`, under the
 * app's navigation: its dialog, where a link asked to go (`went`), and how the composer was left (`left`).
 */
function composer({
  target = { kind: "new" },
  fields = {},
  phase,
  on = fakeData(),
  locale = "en",
}: { target?: Target; fields?: Partial<Draft>; phase?: Phase; on?: FakeData; locale?: string } = {}) {
  const initial = { ...emptyDraft(target, defaultComposeSettings, "k"), text: "Off to the market.", ...fields };
  const went: string[] = [];
  const left: string[] = [];
  function Composing() {
    const c = useComposer(initial, { onPosted: none, onKept: none }, phase);
    return (
      <ComposerView
        c={c}
        layer={false}
        onClose={() => left.push("close")}
        onDismiss={() => left.push("dismiss")}
      />
    );
  }
  const user = renderFeature(
    on.data,
    <RouterProvider navigate={(to) => went.push(String(to))}>
      <Composing />
    </RouterProvider>,
    locale,
  );
  return { form: within(screen.getByRole("dialog")), user, on, went, left };
}

type Drawn = ReturnType<typeof composer>;

/** The composer's alert once it says `detail`. */
const alertSaying = (form: Drawn["form"], detail: string) =>
  waitFor(() => {
    const alert = form.getAllByRole("alert").find((each) => each.textContent.includes(detail));
    if (alert === undefined) throw new Error(`test: no alert of the composer says "${detail}"`);
    return alert;
  });

/** Presses Post, and gives the alert once it says `detail`. */
async function posted({ form, user }: Drawn, detail: string) {
  await form.findAllByRole("textbox", { name: "Text" });
  await user.click(form.getByRole("button", { name: /^(Post|Reply)$/ }));
  return alertSaying(form, detail);
}

test("Account status lives at its place in the settings", () => {
  expect(accountStatusPath).toBe("/settings/account/status");
});

test("a composer refused for a restriction says the server's sentence in its alert, and links to Account status", async () => {
  const { form } = composer({ phase: { kind: "failed", reason: posting, restricted: true } });
  await form.findAllByRole("textbox", { name: "Text" });

  const alert = await alertSaying(form, posting);
  expect(alert.textContent).toMatch(/^A moderation action keeps your account from posting for now\./);
  expect(within(alert).getByRole("link", { name: "Account status" }).getAttribute("href")).toBe(accountStatusPath);
  expect(within(alert).queryAllByRole("button", { name: "Appeal" })).toHaveLength(0);
});

test("a new post the server refuses with posting-restricted: the alert says its detail, with the link", async () => {
  const on = fakeData();
  const sent = on.fake.script("POST /statuses", { fail: refusal("posting-restricted", posting) });
  const drawn = composer({ on });

  const alert = await posted(drawn, posting);

  expect(sent.seen).toHaveLength(1);
  expect(within(alert).getByRole("link", { name: "Account status" }).getAttribute("href")).toBe(accountStatusPath);
});

for (const kind of ["reply", "quote"] as const)
  test(`a ${kind} the server refuses with interaction-restricted: the alert says its detail, with the link`, async () => {
    const on = seeded();
    on.fake.script("POST /statuses", { fail: refusal("interaction-restricted", interacting) });
    const drawn = composer({ target: { kind, id: on.fake.idOf("loaf") }, on });

    const alert = await posted(drawn, interacting);

    expect(within(alert).getByRole("link", { name: "Account status" }).getAttribute("href")).toBe(accountStatusPath);
  });

test("a post refused with a 403 of no type of its own says the server's words, and offers no link to Account status", async () => {
  const words = "You may not do this.";
  const on = fakeData();
  on.fake.script("POST /statuses", { fail: { type: "about:blank", title: words, status: 403, detail: words } });
  const drawn = composer({ on });

  await drawn.form.findAllByRole("textbox", { name: "Text" });
  await drawn.user.click(drawn.form.getByRole("button", { name: "Post" }));

  await waitFor(() => {
    if (drawn.form.queryAllByRole("alert").length === 0) throw new Error("test: the composer shows no alert yet");
  });
  expect(drawn.form.queryAllByRole("link", { name: "Account status" })).toHaveLength(0);
});

test("the link is in the reader's language: ja", async () => {
  const { form } = composer({ phase: { kind: "failed", reason: posting, restricted: true }, locale: "ja" });
  await form.findAllByRole("textbox");

  const alert = await alertSaying(form, posting);
  expect(within(alert).getByRole("link", { name: "アカウントの状態" }).getAttribute("href")).toBe(accountStatusPath);
});

test("pressing Account status leads there and leaves the composer", async () => {
  const drawn = composer({ phase: { kind: "failed", reason: posting, restricted: true } });
  await drawn.form.findAllByRole("textbox", { name: "Text" });
  const alert = await alertSaying(drawn.form, posting);

  await drawn.user.click(within(alert).getByRole("link", { name: "Account status" }));

  await waitFor(() => {
    if (drawn.left.length === 0) throw new Error("test: the composer was not left");
  });
  expect(drawn.went).toEqual([accountStatusPath]);
  // Left once; that what was written is kept is the app's route test.
  expect(drawn.left).toHaveLength(1);
});

test("an article the server refuses with posting-restricted: the editor's alert says its detail, with the link", async () => {
  const on = fakeData();
  const sent = on.fake.script("POST /articles", { fail: refusal("posting-restricted", posting) });
  const user = renderFeature(
    on.data,
    <RouterProvider navigate={none}>
      <ArticleEditor opening={{ kind: "new" }} onLeave={none} onOpenDraft={none} />
    </RouterProvider>,
  );
  const body = await screen.findByRole("textbox", { name: "Body" });
  await user.type(screen.getByRole("textbox", { name: "Title" }), "Market day");
  await user.click(body);
  await pasteInto(user, body, "Flour and water.");

  await user.click(screen.getByRole("button", { name: "Publish" }));

  const alert = await waitFor(() => {
    const found = screen.queryAllByRole("alert").find((each) => each.textContent.includes(posting));
    if (found === undefined) throw new Error("test: no alert of the editor says the server's sentence");
    return found;
  });
  expect(sent.seen).toHaveLength(1);
  expect(within(alert).getByRole("link", { name: "Account status" }).getAttribute("href")).toBe(accountStatusPath);
});
