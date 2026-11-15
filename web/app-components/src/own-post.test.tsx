// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { I18nProvider } from "@my/design-system";
import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { DeleteConfirm } from "./delete-confirm";
import { EditHistory, type PostEdit } from "./edit-history";
import { Post } from "./post";
import type { PostStatus } from "./status";

const render = (node: ReactNode, locale = "en") =>
  renderToStaticMarkup(<I18nProvider locale={locale}>{node}</I18nProvider>);

/** The markup without classes, to read the structure. */
const bare = (node: ReactNode, locale?: string) => render(node, locale).replace(/ class="[^"]*"/g, "");

const none = () => undefined;

const status = (over: Partial<PostStatus> = {}): PostStatus => ({
  id: "10",
  account: { id: "1", username: "yuki", acct: "yuki", domain: "my.example", display_name: "Yuki", emojis: [] },
  content: "<p>Bread at ten on Saturday</p>",
  spoiler_text: "",
  visibility: "public",
  language: "en",
  created_at: new Date(Date.now() - 3 * 3_600_000).toISOString(),
  media_attachments: [],
  mentions: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
  ...over,
});

describe("the edited mark", () => {
  test("an edited post says so after its time, a button named for what it opens", () => {
    const html = bare(<Post status={status({ edited_at: new Date().toISOString() })} />);
    expect(html).toMatch(
      /<\/time><\/bdi><\/a><span aria-hidden="true">[^<]*<\/span><button [^>]*aria-label="Edited, show history"[^>]*>edited<\/button>/,
    );
  });

  test("another's edited post is marked too, and a post never edited is not", () => {
    const remote = status({ account: { ...status().account, id: "2" }, edited_at: new Date().toISOString() });
    expect(bare(<Post status={remote} />)).toContain('aria-label="Edited, show history"');
    expect(bare(<Post status={status()} />)).not.toContain("Edited, show history");
    expect(bare(<Post status={status()} />)).not.toContain(">edited<");
  });

  test("it reads in the reader's locale", () => {
    const html = bare(<Post status={status({ edited_at: new Date().toISOString() })} />, "ja");
    expect(html).toMatch(/<button [^>]*aria-label="編集済み、履歴を表示"[^>]*>編集済み<\/button>/);
  });
});

describe("the confirmation", () => {
  test("Delete… asks before it deletes, says what follows, and offers Cancel then Delete", () => {
    const html = bare(<DeleteConfirm layer={false} onCancel={none} onConfirm={none} />);
    expect(html).toMatch(
      /^<div role="alertdialog" aria-labelledby="([^"]+)"[^>]*><div><h2 id="\1">Delete this post\?<\/h2>/,
    );
    expect(html).toContain(
      "<p>Its boosts, favourites and reactions go with it, and other servers are asked to delete it too.",
    );
    expect(html).toMatch(/<span>Cancel<\/span><\/button><button [^>]*><span>Delete<\/span><\/button>/);
  });

  test("Delete and redraft… says the text and images become a draft", () => {
    const html = bare(<DeleteConfirm layer={false} redraft onCancel={none} onConfirm={none} />);
    expect(html).toContain(">Delete and redraft this post?</h2>");
    expect(html).toContain("Its text and images become the draft of a new post.");
    expect(html).not.toContain("votes of its poll");
    expect(html).toMatch(/<span>Cancel<\/span><\/button><button [^>]*><span>Delete and redraft<\/span><\/button>/);
  });

  test("redrafting a post with a poll says its votes are lost and its poll becomes the draft", () => {
    const html = bare(<DeleteConfirm layer={false} redraft poll onCancel={none} onConfirm={none} />);
    expect(html).toContain("the votes of its poll are lost");
    expect(html).toContain("Its text and poll become the draft of a new post.");
  });

  test("it reads in the reader's locale", () => {
    const deleting = bare(<DeleteConfirm layer={false} onCancel={none} onConfirm={none} />, "ja");
    expect(deleting).toContain(">この投稿を削除しますか？</h2>");
    expect(deleting).toMatch(/<span>キャンセル<\/span><\/button><button [^>]*><span>削除する<\/span>/);
    const redrafting = bare(<DeleteConfirm layer={false} redraft poll onCancel={none} onConfirm={none} />, "ja");
    expect(redrafting).toContain(">この投稿を削除して書き直しますか？</h2>");
    expect(redrafting).toContain("投票に入った票は消え");
  });
});

describe("the edit history", () => {
  const version = (content: string, at: string, media: PostEdit["media_attachments"] = []): PostEdit => ({
    content,
    spoiler_text: "",
    created_at: at,
    media_attachments: media,
    emojis: [],
  });
  const snow = { id: "a", type: "image", url: "https://m/snow.webp", description: "Snow on the ridge" } as const;
  const dusk = { id: "b", type: "image", url: "https://m/dusk.webp", description: "The ridge at dusk" } as const;
  // Oldest first, as the API gives them.
  const edits = [
    version("<p>Bread at nine tomorrow</p>", "2026-11-14T03:00:00Z", [snow]),
    version("<p>Bread at ten tomorrow</p>", "2026-11-14T04:00:00Z"),
    version("<p>Bread at ten on Saturday</p>", "2026-11-14T05:00:00Z", [dusk]),
  ];
  const history = (locale?: string) => bare(<EditHistory layer={false} edits={edits} onClose={none} />, locale);
  /** Each version's item of the list, in its order. */
  const items = (html: string) => html.split(/<li><div><span>/).slice(1);

  test("a dialog named Edit history, with a Close button", () => {
    const html = history();
    expect(html).toMatch(/<div role="dialog" aria-labelledby="([^"]+)"[^>]*><div><h2 id="\1">Edit history<\/h2>/);
    expect(html).toMatch(/<button [^>]*aria-label="Close"/);
    expect(html).toContain('<ol aria-label="Edit history">');
  });

  test("every version, newest first, each labelled and with its time", () => {
    const list = items(history());
    expect(list).toHaveLength(3);
    expect(list[0]).toMatch(/^Current<\/span><span><time dateTime="2026-11-14T05:00:00Z">/);
    expect(list[1]).toMatch(/^Earlier edit<\/span><span><time dateTime="2026-11-14T04:00:00Z">/);
    expect(list[2]).toMatch(/^Original<\/span><span><time dateTime="2026-11-14T03:00:00Z">/);
  });

  test("each version marks the words it removed and added against the one before, and the original none", () => {
    const [current = "", edit = "", original = ""] = items(history());
    expect(current).toMatch(/<del>[^]*?tomorrow[^]*?<\/del><ins>[^]*?on Saturday[^]*?<\/ins>/);
    expect(edit).toMatch(/<del>[^]*?nine[^]*?<\/del><ins>[^]*?ten[^]*?<\/ins>/);
    expect(original).toContain("Bread at nine tomorrow");
    expect(original).not.toContain("<ins");
    expect(original).not.toContain("<del");
  });

  test("a screen reader hears where each addition and removal starts and ends, text it does not show", () => {
    const raw = render(<EditHistory layer={false} edits={edits} onClose={none} />);
    const said = (words: string) => new RegExp(`<span class="sr-only"[^>]*> ${words} </span>`);
    expect(raw).toMatch(said("Start of addition"));
    expect(raw).toMatch(said("End of addition"));
    expect(raw).toMatch(said("Start of removal"));
    expect(raw).toMatch(said("End of removal"));
    const [current = ""] = items(history());
    expect(current).toMatch(
      /<ins><span[^>]*> Start of addition <\/span>on Saturday<span[^>]*> End of addition <\/span><\/ins>/,
    );
    expect(current).toMatch(
      /<del><span[^>]*> Start of removal <\/span>tomorrow<span[^>]*> End of removal <\/span><\/del>/,
    );
  });

  test("an image a version took away or added is marked, with what a screen reader hears", () => {
    const [current = "", edit = ""] = items(history());
    expect(edit).toMatch(/<del><span[^>]*> Removed image <\/span><img [^>]*alt="Snow on the ridge"/);
    expect(current).toMatch(/<ins[^>]*><span[^>]*> Added image <\/span><img [^>]*alt="The ridge at dusk"/);
  });

  test("it reads in the reader's locale", () => {
    const html = history("ja");
    expect(html).toContain(">編集履歴</h2>");
    const [current = "", edit = "", original = ""] = items(html);
    expect(current).toMatch(/^現在</);
    expect(edit).toMatch(/^以前の編集</);
    expect(original).toMatch(/^最初の投稿</);
    expect(current).toContain("追加ここから");
    expect(current).toContain("追加ここまで");
    expect(current).toContain("削除ここから");
    expect(current).toContain("削除ここまで");
    expect(edit).toContain("削除された画像");
  });
});
