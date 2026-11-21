// SPDX-License-Identifier: AGPL-3.0-only

// A writer writes an article in its own editor (ADR-0160) against the server: the preview is the server's rendering of
// what is written, a cover and an image of the body are uploaded, and the article published reads on its page as the
// preview showed it; a published article is edited from its menu, and an edit closed is discarded. That a new article
// closed or saved is a draft of an article, which reopens and is published or scheduled, is the app's
// (web/app/src/article-drafts.dom.test.tsx): the drafts' own way to the server and back is drafts.spec.ts.

import { drawPNG } from "../src/seed/png";
import { ArticleEditor, ArticleText, Thread, type Upload } from "./screens";
import { expect, test } from "./test";

/** A small PNG to pick in the file picker. */
const png = (name: string): Upload => ({
  name,
  mimeType: "image/png",
  buffer: Buffer.from(
    drawPNG({
      width: 96,
      height: 64,
      top: [236, 196, 140],
      bottom: [120, 84, 52],
      shapes: [{ kind: "circle", x: 48, y: 36, r: 20, color: [250, 240, 220] }],
    }),
  ),
});

test("a writer writes an article with a cover, a body image and a hashtag, previews it, publishes it and reads it", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const title = "A starter in a jar";
  const first = "Seven days ago I mixed flour and water in a jar.";
  // The summary left empty is made from the start of the body's text, its heading and image left out.
  const summary = `${first} Day 1: 50 g flour, 50 g water.`;
  await signIn(writer);
  const editor = new ArticleEditor(page);
  await editor.openNew();
  await expect(editor.previewParagraph("Nothing written yet.")).toBeVisible();
  await expect(editor.images).toHaveText("0 of 20 images");

  await editor.title.fill(title);
  await editor.write(first);
  // The image goes in at the caret, in a paragraph of its own.
  const image = await editor.addImage(png("jar.png"));
  await editor.holds(new RegExp(`^${first.replaceAll(".", "\\.")}\\n\\n!\\[\\]\\(media:${image}\\)\\n*$`));

  // The body in Markdown, the image described.
  await editor.clearBody();
  await editor.write(
    [
      first,
      "",
      "# The first three days",
      "",
      "Day 1: **50 g** flour, 50 g water.",
      "",
      `![A jar of starter on the sill](media:${image})`,
    ].join("\n"),
  );
  // The preview: the title, the summary made from the start of the body, the body's heading one level under the
  // title's, its image, and the body's images counted against the server's limit.
  await expect(editor.previewHeading(2, title)).toBeVisible();
  await expect(editor.previewParagraph(summary)).toBeVisible();
  await expect(editor.previewParagraph(first)).toBeVisible();
  await expect(editor.previewHeading(2, "The first three days")).toBeVisible();
  await expect(editor.previewParagraph("Day 1: 50 g flour, 50 g water.")).toBeVisible();
  await expect(editor.preview.getByRole("img", { name: "A jar of starter on the sill", exact: true })).toBeVisible();
  await expect(editor.images).toHaveText("1 of 20 images");

  await editor.addCover(png("cover.png"));
  await editor.coverDescription.fill("A round loaf on a wooden board");
  await expect(editor.preview.getByRole("img", { name: "A round loaf on a wooden board", exact: true })).toBeVisible();
  await editor.addTag("sourdough");

  await editor.publish();
  await expect(page).toHaveURL(new RegExp(`/@${writer.username}/status/[^/]+$`));
  const text = new ArticleText(new Thread(page).post(writer.displayName, title));
  await expect(text.title(title)).toBeVisible();
  await expect(text.image("A round loaf on a wooden board")).toBeVisible();
  await expect(text.paragraph(summary)).toBeVisible();
  await expect(text.paragraph(first)).toBeVisible();
  await expect(text.headings(2)).toHaveText(["The first three days"]);
  await expect(text.paragraph("Day 1: 50 g flour, 50 g water.")).toBeVisible();
  await expect(text.image("A jar of starter on the sill")).toBeVisible();
});

test("a writer edits their article from its menu; an edit closed is discarded, one saved is the article's after a reload", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const title = "Notes on a rye loaf";
  await signIn(writer);
  const editor = new ArticleEditor(page);
  await editor.openNew();
  await editor.title.fill(title);
  await editor.write("Before the edit.");
  await editor.publish();

  const post = new Thread(page).post(writer.displayName, title);
  // An article is edited, never deleted and redrafted.
  const items = await post.more();
  await expect(items.filter({ hasText: /^Delete and redraft…$/ })).toHaveCount(0);
  await post.menu.getByRole("menuitem", { name: "Edit", exact: true }).click();
  await expect(page).toHaveURL(/\/articles\/[^/]+\/edit$/);
  await expect(editor.heading("Edit article")).toBeVisible();
  await expect(editor.title).toHaveValue(title);
  await editor.holds("Before the edit.");
  // Its visibility stays as it was published; an edit is neither a draft nor scheduled.
  await expect(editor.note(/^Public · An edit cannot change who can see it$/)).toBeVisible();
  await expect(editor.button("Drafts")).toHaveCount(0);
  await expect(editor.button("Schedule")).toHaveCount(0);
  await expect(editor.button("Publish")).toHaveCount(0);

  // Closed, the edit is discarded.
  await editor.title.fill("A title that is never saved");
  await editor.close();
  await expect(page).toHaveURL(new RegExp(`/@${writer.username}/status/[^/]+$`));
  await expect(new ArticleText(post).title(title)).toBeVisible();

  await post.menuItem("Edit");
  await expect(editor.title).toHaveValue(title);
  await editor.title.fill("Notes on a rye loaf, revised");
  await editor.clearBody();
  await editor.write("After the edit.");
  await editor.save();
  const edited = new Thread(page).post(writer.displayName, "Notes on a rye loaf, revised");
  const text = new ArticleText(edited);
  await expect(text.title("Notes on a rye loaf, revised")).toBeVisible();
  await expect(text.paragraph("After the edit.")).toHaveCount(2);
  await expect(edited.edited).toBeVisible();
  // The edit is the server's: the page read again shows it.
  await page.reload();
  await expect(text.title("Notes on a rye loaf, revised")).toBeVisible();
  await expect(text.paragraph("After the edit.")).toHaveCount(2);
  await expect(edited.edited).toBeVisible();

  // Neither the closed edit nor the saved one left a draft.
  await editor.openNew();
  const drafts = await editor.openDrafts();
  await expect(drafts.opener(title)).toHaveCount(0);
});
