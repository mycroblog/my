// SPDX-License-Identifier: AGPL-3.0-only

// A post's edit history, from the versions the API gives: every version newest first with its label and its time,
// and what each changed against the one before it, in words and in images. How a deletion and an insertion are
// marked and read is the design system's (history.dom.test.tsx), and the panel's focus is too (dialog.dom.test.tsx).

import { expect, test } from "bun:test";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EditHistory, type PostEdit } from "./edit-history";
import { inLocale, type Part } from "./test/post";

const version = (content: string, at: string, media: PostEdit["media_attachments"] = []): PostEdit => ({
  content,
  spoiler_text: "",
  created_at: at,
  media_attachments: media,
  emojis: [],
});
const snow = { id: "a", type: "image", url: "https://m.example/snow.webp", description: "Snow on the ridge" } as const;
const dusk = { id: "b", type: "image", url: "https://m.example/dusk.webp", description: "The ridge at dusk" } as const;
// Oldest first, as the API gives them.
const edits = [
  version("<p>Bread at nine tomorrow</p>", "2026-11-14T03:00:00Z", [snow]),
  version("<p>Bread at ten tomorrow</p>", "2026-11-14T04:00:00Z"),
  version("<p>Bread at ten on Saturday</p>", "2026-11-14T05:00:00Z", [dusk]),
];

const words = (element: Element | undefined) => element?.textContent.replace(/\s+/g, " ").trim();

/** The words a version removed (`deletion`) or added (`insertion`): its mark of that kind that holds no image. */
const changed = (version: Part, kind: "deletion" | "insertion") =>
  version.queryAllByRole(kind).find((mark) => within(mark).queryAllByRole("img").length === 0);

/** The history of `edits`, alone in the flow of the page, for a reader of `locale`: its versions, newest first. */
function history(locale = "en", postLang?: string) {
  cleanup();
  const closes: number[] = [];
  inLocale(<EditHistory layer={false} edits={edits} postLang={postLang} onClose={() => closes.push(1)} />, locale);
  const dialog = screen.getByRole("dialog");
  const [list] = within(dialog).getAllByRole("list");
  if (list === undefined) throw new Error("test: the history has no list");
  const versions = Array.from(list.children).filter((child): child is HTMLElement => child instanceof HTMLElement);
  return { dialog, list, versions, closes };
}

test("a dialog named Edit history, with a Close button, around a list of the same name", async () => {
  const user = userEvent.setup();
  const { dialog, list, closes } = history();

  expect(screen.getByRole("dialog", { name: "Edit history" }) === dialog).toBe(true);
  expect(list.getAttribute("aria-label")).toBe("Edit history");
  await user.click(within(dialog).getByRole("button", { name: "Close" }));

  expect(closes).toHaveLength(1);
});

test("every version, newest first, each with its label and its time", () => {
  const { versions } = history();

  expect(versions).toHaveLength(3);
  expect(versions.map((item) => within(item).getByRole<HTMLTimeElement>("time").dateTime)).toEqual([
    "2026-11-14T05:00:00Z",
    "2026-11-14T04:00:00Z",
    "2026-11-14T03:00:00Z",
  ]);
  for (const [i, label] of ["Current", "Earlier edit", "Original"].entries()) {
    const item = versions[i];
    expect(item !== undefined && within(item).queryAllByText(label)).toHaveLength(1);
  }
});

test("each version marks the words it removed and added against the one before, and the original none", () => {
  const { versions } = history();
  const [current, edit, original] = versions.map((item) => within(item));
  if (current === undefined || edit === undefined || original === undefined) throw new Error("no three versions");

  expect(words(changed(current, "deletion"))).toBe("Start of removal tomorrow End of removal");
  expect(words(changed(current, "insertion"))).toBe("Start of addition on Saturday End of addition");
  expect(words(changed(edit, "deletion"))).toContain("nine");
  expect(words(changed(edit, "insertion"))).toContain("ten");
  expect(original.getByText("Bread at nine tomorrow")).not.toBeNull();
  expect([...original.queryAllByRole("deletion"), ...original.queryAllByRole("insertion")]).toHaveLength(0);
});

test("an image a version took away or added is marked, with what a screen reader hears before its description", () => {
  const { versions } = history();
  const [current, edit, original] = versions.map((item) => within(item));
  if (current === undefined || edit === undefined || original === undefined) throw new Error("no three versions");

  const removed = edit.getByRole("img", { name: "Snow on the ridge" }).closest("del");
  expect(words(removed ?? undefined)).toBe("Removed image");
  const added = current.getByRole("img", { name: "The ridge at dusk" }).closest("ins");
  expect(words(added ?? undefined)).toBe("Added image");
  // The original's image is neither.
  expect(original.getByRole("img", { name: "Snow on the ridge" }).closest("ins, del")).toBeNull();
});

test("a version's words are in the post's language", () => {
  const { versions } = history("en", "ja");
  const [current] = versions;
  if (current === undefined) throw new Error("no version");

  expect(within(current).getByText("Bread at ten", { exact: false }).closest("[lang]")?.getAttribute("lang")).toBe(
    "ja",
  );
});

test("it reads in the reader's locale", () => {
  const { dialog, versions } = history("ja");
  const [current, edit, original] = versions;
  if (current === undefined || edit === undefined || original === undefined) throw new Error("no three versions");

  expect(screen.getByRole("dialog", { name: "編集履歴" }) === dialog).toBe(true);
  expect(within(current).queryAllByText("現在")).toHaveLength(1);
  expect(within(edit).queryAllByText("以前の編集")).toHaveLength(1);
  expect(within(original).queryAllByText("最初の投稿")).toHaveLength(1);
  expect(words(changed(within(current), "insertion"))).toBe("追加ここから on Saturday 追加ここまで");
  expect(words(changed(within(current), "deletion"))).toBe("削除ここから tomorrow 削除ここまで");
  expect(words(edit)).toContain("削除された画像");
});
