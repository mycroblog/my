// SPDX-License-Identifier: AGPL-3.0-only

// Asking for an author's approval in the composer (ADR-0155), on a server whose posts approve by hand (the fake's
// `approvals`): a reply says before it is sent that it will wait, and the audience's menu lets the writer choose, for
// replies, boosts and quotes each, who may at once and whether everyone else may ask. What the post then sends is the
// model's (model.test.ts), and that a reply waits on the server the E2E tests' (web/e2e/specs/approval.spec.ts).

import { expect, onTestFinished, test } from "bun:test";
import { defaultComposeSettings } from "@my/app-data";
import type { FakeOptions } from "@my/e2e/fake";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { ComposeLayer } from "./layer";
import { emptyDraft } from "./model";

const none = () => undefined;

/** A name that holds a person's name, which the app isolates (U+2068…U+2069), whole. */
const named = (before: string, person: string, after: string) => new RegExp(`^${before}⁨?${person}⁩?${after}$`);

const words = {
  en: {
    reply: "Reply",
    waits: named("Others see your reply once ", "@layla", " approves it"),
    audience: "Who can see it: Public",
    limited: "Who can see it: Public, replies, boosts or quotes limited",
    permissions: "Who can reply, boost and quote",
    kind: { reply: "Who can reply", announce: "Who can boost", quote: "Who can quote" },
    everyone: "Anyone",
    followers: "Followers",
    nobody: "Only you",
    asks: "Allow others with your approval",
    withApproval: (audience: string) => `${audience}, others with your approval`,
    back: "Back",
  },
  ja: {
    reply: "返信",
    waits: named("", "@layla", "さんが承認すると、ほかの人にも表示されます"),
    audience: "見られる人: 公開",
    limited: "見られる人: 公開、返信・ブースト・引用を制限中",
    permissions: "返信・ブースト・引用できる人",
    kind: { reply: "返信できる人", announce: "ブーストできる人", quote: "引用できる人" },
    everyone: "誰でも",
    followers: "フォロワー",
    nobody: "自分だけ",
    asks: "ほかの人も、承認すればできるようにする",
    withApproval: (audience: string) => `${audience}、ほかの人は承認すれば`,
    back: "戻る",
  },
};

type Locale = keyof typeof words;
type Kind = keyof (typeof words)["en"]["kind"];

/**
 * The composer of a reply to the post of `key`, in the flow of the page, on a server of `options` whose posts approve
 * by hand, in `locale`: its dialog, once it shows the post it replies to.
 */
async function replyTo(key: string, options: FakeOptions = {}, locale: Locale = "en") {
  const { fake, data } = fakeData({ approvals: true, ...options });
  const target = { kind: "reply" as const, id: fake.idOf(key) };
  const user = renderFeature(
    data,
    <ComposeLayer
      target={target}
      initial={emptyDraft(target, defaultComposeSettings, "k")}
      inline
      onOpen={none}
      onClose={none}
    />,
    locale,
  );
  // One wait from the first draw on: the text's editor is a chunk of its own, and the post replied to is read; the
  // composer is whole once both came.
  const dialog = await waitFor(() => {
    const dialog = within(screen.getByRole("dialog", { name: words[locale].reply }));
    dialog.getByRole("textbox");
    dialog.getByText(key === "layla-asks" ? /^Translating a poem/ : /^今朝の尾根/);
    return dialog;
  });
  return { user, dialog, w: words[locale] };
}

for (const locale of ["en", "ja"] as const)
  test(`a reply to a post whose author approves the reader's replies says, before it is sent, that others see it once the author approves it (${locale})`, async () => {
    // layla admits at once the replies of those her post mentions, and everyone else's by her approval.
    const { dialog, w } = await replyTo("layla-asks", {}, locale);

    expect(await dialog.findByText(w.waits)).toBeDefined();
  });

test("a reply to a post that admits the reader at once says nothing of an approval", async () => {
  // yuki's post admits everyone's replies.
  const { dialog } = await replyTo("yuki-ridge", { viewer: "sam" });

  expect(dialog.queryAllByText(/approv/i)).toHaveLength(0);
});

test("the audience's menu chooses, for replies, boosts and quotes each, who may at once and whether others may ask for approval; the tool says the post is limited", async () => {
  const { user, dialog, w } = await replyTo("layla-asks");
  const menu = (name: string) => within(screen.getByRole("menu", { name }));
  /** The row of `kind` in the permissions, by what it reads: its label, then who may now. */
  const row = (kind: Kind) => menu(w.permissions).getByRole("menuitem", { name: new RegExp(`^${w.kind[kind]}`) });
  const holds = (kind: Kind) => row(kind).textContent.slice(w.kind[kind].length);
  /** Opens the panel of `kind`, chooses `audience`, sets its approval (none: it must not be offered), and goes back. */
  const choose = async (kind: Kind, audience: string, approval: boolean | undefined) => {
    await user.click(row(kind));
    const panel = within(await screen.findByRole("menu", { name: w.kind[kind] }));
    await user.click(panel.getByRole("menuitemradio", { name: audience }));
    await waitFor(() => {
      if (panel.getByRole("menuitemradio", { name: audience }).getAttribute("aria-checked") !== "true")
        throw new Error(`${audience} is not chosen yet`);
    });
    expect(panel.getAllByRole("menuitemradio", { checked: true })).toHaveLength(1);
    const check = () => panel.queryAllByRole("menuitemcheckbox", { name: w.asks });
    if (approval === undefined) expect(check()).toHaveLength(0);
    else {
      expect(check()).toHaveLength(1);
      if ((check()[0]?.getAttribute("aria-checked") === "true") !== approval)
        await user.click(check()[0] as HTMLElement);
      await waitFor(() => {
        if (check()[0]?.getAttribute("aria-checked") !== String(approval))
          throw new Error("the approval is not as chosen yet");
      });
    }
    await user.click(screen.getByRole("button", { name: w.back }));
    await screen.findByRole("menu", { name: w.permissions });
  };

  await user.click(dialog.getByRole("button", { name: w.audience }));
  await user.click(await screen.findByRole("menuitem", { name: new RegExp(`^${w.permissions}`) }));
  await screen.findByRole("menu", { name: w.permissions });
  expect((["reply", "announce", "quote"] as const).map(holds)).toEqual([w.everyone, w.everyone, w.everyone]);

  await choose("announce", w.followers, true);
  await choose("quote", w.nobody, true);

  expect((["reply", "announce", "quote"] as const).map(holds)).toEqual([
    w.everyone,
    w.withApproval(w.followers),
    w.withApproval(w.nobody),
  ]);
  // The page under an open menu is hidden from a query by role; the tool is still there, renamed.
  expect(dialog.getByRole("button", { name: w.limited, hidden: true })).toBeDefined();

  // Anyone admits everyone at once, so there is nothing to ask for: the check goes, and so does the approval it held;
  // Only you without it admits no one else.
  await choose("announce", w.everyone, undefined);
  expect(holds("announce")).toBe(w.everyone);
  await choose("quote", w.nobody, false);
  expect(holds("quote")).toBe(w.nobody);
});

test("in ja, the permissions read who may and that others may ask, and the tool says the post is limited", async () => {
  const w = words.ja;
  const { data } = fakeData();
  // What the composer reads on its first draw (the instance, the reader's preferences, the custom emoji) reaches it
  // when it comes, as a server's answers do, not inside an act() of the test: React is told so until the test ends.
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  onTestFinished(() => {
    environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
  const target = { kind: "new" as const };
  /** Anyone replies, followers boost and others ask for approval, the author alone quotes. */
  const initial = {
    ...emptyDraft(target, defaultComposeSettings, "k"),
    policy: {
      reply: { audience: "everyone" as const, approval: false },
      announce: { audience: "followers" as const, approval: true },
      quote: { audience: "nobody" as const, approval: false },
    },
  };
  /**
   * The composer with the audience's menu open on `opened`, as a specimen opens it, once its text's editor came, a
   * chunk of its own; under an open menu the page is hidden from a query by role.
   */
  const open = async (opened: "permissions" | "announce") => {
    cleanup();
    renderFeature(
      data,
      <ComposeLayer target={target} initial={initial} inline specimen={{ opened }} onOpen={none} onClose={none} />,
      "ja",
    );
    await screen.findAllByRole("textbox", { hidden: true });
  };

  await open("permissions");
  const rows = within(await screen.findByRole("menu", { name: w.permissions })).getAllByRole("menuitem");
  expect(rows.map((row) => row.textContent)).toEqual([
    `${w.kind.reply}${w.everyone}`,
    `${w.kind.announce}${w.withApproval(w.followers)}`,
    `${w.kind.quote}${w.nobody}`,
  ]);
  expect(screen.getByRole("button", { name: w.limited, hidden: true })).toBeDefined();

  await open("announce");
  const panel = within(await screen.findByRole("menu", { name: w.kind.announce }));
  expect(panel.getByRole("menuitemradio", { name: w.followers }).getAttribute("aria-checked")).toBe("true");
  expect(panel.getByRole("menuitemcheckbox", { name: w.asks }).getAttribute("aria-checked")).toBe("true");
});
