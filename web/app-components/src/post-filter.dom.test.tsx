// SPDX-License-Identifier: AGPL-3.0-only

// A post that matches one of the reader's keyword filters (ADR-0157 §6, §10): folded whole under the filters' names,
// opened by the one button the content warning's fold has, and unchanged without a match.

import { describe, expect, test } from "bun:test";
import { act, cleanup, isInaccessible, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FilterSentence } from "./post-filter";
import { Post, type PostProps } from "./post";
import type { PostFiltered } from "./status";
import { inLocale, ken, status } from "./test/post";

const match = (...titles: string[]): PostFiltered[] => titles.map((title, i) => ({ filter: { id: `f${i}`, title } }));

/** The post of `props` for a reader of `locale`, alone in the document. */
function post(props: Partial<PostProps> = {}, locale = "en") {
  cleanup();
  inLocale(<Post status={status()} onAction={() => {}} {...props} />, locale);
  return screen;
}

/** The line for `names` as its accessible name reads: a space may sit around a name's isolating element. */
const sentence = (...names: string[]) =>
  new RegExp(`^Matches filters? ${names.map((name) => `“\\s*${name}\\s*”`).join("\\s*(?:,|and)?\\s*")}$`);
const line = sentence("Spoilers");
const lineText = /^Matches filter/;

describe("a post that matches a filter", () => {
  test("is folded whole: one line names the filter, and nothing of the post is in the document", () => {
    const page = post({ status: status({ filtered: match("Spoilers") }) });

    const article = page.getByRole("article", { name: line });
    expect(within(article).getByText(lineText)).not.toBeNull();
    expect(page.queryAllByRole("link")).toHaveLength(0);
    expect(page.queryAllByText("Quiet morning.")).toHaveLength(0);
    expect(page.queryAllByText("Lena Novak")).toHaveLength(0);
    expect(page.queryAllByRole("button", { name: /^(Reply|Favourite|Boost)/ })).toHaveLength(0);
    expect(page.getAllByRole("button")).toHaveLength(1);
  });

  test("has a Show that is collapsed and described by the line", () => {
    const page = post({ status: status({ filtered: match("Spoilers") }) });

    expect(page.getByRole("button", { name: "Show", expanded: false, description: line })).not.toBeNull();
  });

  for (const key of ["{Enter}", " "])
    test(`opens with ${key === " " ? "Space" : "Enter"} on the same button, which becomes Hide and keeps the focus, and Hide folds it again`, async () => {
      const user = userEvent.setup();
      const page = post({ status: status({ filtered: match("Spoilers") }) });
      const show = page.getByRole("button", { name: "Show" });
      act(() => {
        show.focus();
      });

      await user.keyboard(key);

      const hide = page.getByRole("button", { name: "Hide", expanded: true });
      expect(hide === show).toBe(true);
      expect(document.activeElement === show).toBe(true);
      expect(page.getByRole("article", { name: "Lena Novak" })).not.toBeNull();
      expect(isInaccessible(page.getByText("Quiet morning."))).toBe(false);
      expect(page.getByRole("button", { name: /^Reply/ })).not.toBeNull();

      await user.keyboard(key);

      expect(page.getByRole("article", { name: line })).not.toBeNull();
      expect(page.queryAllByText("Quiet morning.")).toHaveLength(0);
      expect(page.getByRole("button", { name: "Show", expanded: false }) === show).toBe(true);
    });

  test("opens with a press, and is open from the start with defaultShown", async () => {
    const user = userEvent.setup();
    const page = post({ status: status({ filtered: match("Spoilers") }) });
    await user.click(page.getByRole("button", { name: "Show" }));
    expect(page.getByText("Quiet morning.")).not.toBeNull();

    const open = post({ status: status({ filtered: match("Spoilers") }), defaultShown: true });
    expect(open.getByRole("button", { name: "Hide", expanded: true })).not.toBeNull();
    expect(open.getByText("Quiet morning.")).not.toBeNull();
  });

  test("names every filter, in order, in one line, each name isolated", () => {
    const page = post({ status: status({ filtered: match("Spoilers", "Politics") }) });

    const article = page.getByRole("article", { name: sentence("Spoilers", "Politics") });
    const names = within(article)
      .getAllByText(/Spoilers|Politics/)
      .map((element) => element.textContent);
    expect(names).toEqual(["Spoilers", "Politics"]);
    expect(within(article).getByText("Spoilers").tagName).toBe("BDI");
    expect(within(article).getByText("Politics").tagName).toBe("BDI");
  });

  test("says the line in the reader's language", () => {
    const page = post({ status: status({ filtered: match("A", "B") }) }, "ja");

    expect(page.getByRole("article", { name: /^フィルタ「\s*A\s*」、「\s*B\s*」に一致$/ })).not.toBeNull();
    expect(page.getByRole("button", { name: "表示", expanded: false })).not.toBeNull();
  });

  test("keeps a content warning folded when shown, with its own Show", async () => {
    const user = userEvent.setup();
    const page = post({
      status: status({ filtered: match("Spoilers"), spoiler_text: "Food, close up" }),
    });

    await user.click(page.getByRole("button", { name: "Show" }));

    expect(page.getByText("Food, close up")).not.toBeNull();
    expect(page.queryAllByText("Quiet morning.").every((text) => isInaccessible(text))).toBe(true);
    await user.click(page.getByRole("button", { name: "Show", description: /Food, close up/, expanded: false }));
    expect(isInaccessible(page.getByText("Quiet morning."))).toBe(false);
  });

  test("is folded as a boost by the row's filters, with the boosted line inside the fold", async () => {
    const user = userEvent.setup();
    const page = post({
      status: status({ id: "20", account: ken, boost: status(), filtered: match("Spoilers") }),
    });

    expect(page.getByRole("article", { name: line })).not.toBeNull();
    expect(page.queryAllByText(/boosted$/)).toHaveLength(0);
    expect(page.queryAllByText("Quiet morning.")).toHaveLength(0);

    await user.click(page.getByRole("button", { name: "Show" }));

    expect(page.getByText(/boosted$/)).not.toBeNull();
    expect(page.getByText("Quiet morning.")).not.toBeNull();
    expect(page.getByRole("article", { name: "Lena Novak" })).not.toBeNull();
  });

  test("folds only the quoted post of a quote, and the quoting post keeps its text and actions", async () => {
    const user = userEvent.setup();
    const quoted = status({
      id: "30",
      account: ken,
      content: "<p>Small servers.</p>",
      filtered: match("Spoilers"),
    });
    const page = post({ status: status({ quote: { state: "accepted", quoted_status: quoted } }) });

    expect(page.getByText("Quiet morning.")).not.toBeNull();
    expect(page.getByRole("button", { name: /^Reply/ })).not.toBeNull();
    expect(page.queryAllByText("Small servers.")).toHaveLength(0);
    expect(page.getByText(lineText)).not.toBeNull();

    await user.click(page.getByRole("button", { name: "Show" }));

    expect(page.getByText("Small servers.")).not.toBeNull();
  });
});

describe("a post that matches no filter", () => {
  for (const [name, filtered] of [
    ["without filtered", undefined],
    ["with an empty filtered", []],
  ] as const)
    test(`is not folded ${name}`, () => {
      const page = post({ status: status({ filtered }) });

      expect(page.getByRole("article", { name: "Lena Novak" })).not.toBeNull();
      expect(page.getByText("Quiet morning.")).not.toBeNull();
      expect(page.queryAllByText(/Matches filter/)).toHaveLength(0);
      expect(page.queryAllByRole("button", { name: /^(Show|Hide)$/ })).toHaveLength(0);
    });

  test("a boost whose row has no match is not folded", () => {
    const page = post({ status: status({ id: "20", account: ken, boost: status(), filtered: [] }) });

    expect(page.getByText("Quiet morning.")).not.toBeNull();
    expect(page.queryAllByText(/Matches filter/)).toHaveLength(0);
  });
});

describe("FilterSentence", () => {
  test("says the line alone, with no button", () => {
    cleanup();
    inLocale(<FilterSentence filtered={match("Spoilers")} id="s" />);

    expect(screen.getByText(lineText).id).toBe("s");
    expect(screen.queryAllByRole("button")).toHaveLength(0);
  });
});
