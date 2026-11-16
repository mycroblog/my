// SPDX-License-Identifier: AGPL-3.0-only

// The article editor's states played on the fake server as yuki writes (#3091): an article written, its preview beside
// it on a PC and behind Write | Preview at the minimum width, a title and a body over their limits, which keep Publish
// from publishing, a preview the server could not render, an edit refused for its summary (as sam), and a reader the
// server keeps from writing new articles; in
// en and in ja, at the minimum width and on a PC.
//
// The preview is asked of the server 600 ms after the writer stops typing (useArticlePreview), and a body of 100 kB
// takes the editor a while to take in: a play waits for what the server's answer shows for as long as the screen's own
// play waits for its first answers, never for the default second, which the debounce alone nearly fills.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, screen, screenMeta } from "./app.samples";

const meta = {
  title: "Article editor states",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/articles/new" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The words of the editor the plays read, in each locale they run in. */
const words = {
  en: {
    heading: "New article",
    title: "Title",
    body: "Body",
    publish: "Publish",
    mode: "Write or preview",
    preview: "Preview",
    length: /^Length: (\d+)% of the limit$/,
    titleOver: "1 character over",
    over: "Part of the article is over its limit. Shorten it or take out images to publish.",
    failed: "The preview could not be shown.",
    tryAgain: "Try again",
    notAllowed:
      "Your account cannot write new articles on this server. You can still edit the articles you wrote before.",
  },
  ja: {
    heading: "新しい記事",
    title: "タイトル",
    body: "本文",
    publish: "公開する",
    mode: "書く・プレビュー",
    preview: "プレビュー",
    length: /^長さ: 上限の (\d+)%$/,
    titleOver: "1 文字超過",
    over: "記事の一部が上限を超えています。短くするか画像を減らすと公開できます。",
    failed: "プレビューを表示できませんでした。",
    tryAgain: "もう一度試す",
    notAllowed: "このサーバーでは、このアカウントで新しい記事を書けません。これまでに書いた記事は編集できます。",
  },
} as const;

type Lang = keyof typeof words;
type Words = (typeof words)[Lang];

const pc = { viewport: { value: "pc" } };

/** How long a play waits for what the server's preview shows: the debounce, the request and the render. */
const answered = { timeout: 5000 };

const canvasOf = (root: HTMLElement) => within(root);

/** The queries of the canvas. */
type Canvas = ReturnType<typeof canvasOf>;

/** The editor in the canvas in `lang`, once the screen shows its state. */
async function editor(context: StoryContext, lang: Lang): Promise<{ canvas: Canvas; w: Words }> {
  await screenMeta.play(context);
  const canvas = canvasOf(context.canvasElement);
  const w = words[lang];
  await canvas.findByRole("heading", { level: 1, name: w.heading });
  return { canvas, w };
}

/** The share of the body's length limit used, in percent, as the limits' line says it. */
function lengthUsed(canvas: Canvas, w: Words): number {
  const text = canvas.getByText(w.length).textContent;
  return Number(w.length.exec(text)?.[1]);
}

/** Writes a title and a Markdown body. */
async function write(context: StoryContext, lang: Lang) {
  const { canvas, w } = await editor(context, lang);
  await userEvent.type(canvas.getByRole("textbox", { name: w.title }), "A starter in a jar");
  await userEvent.click(canvas.getByRole("textbox", { name: w.body }));
  await userEvent.paste("# Day one\n\nFlour and **water**, then a lid.");
  return { canvas, w };
}

/** The preview: the title, and the server's rendering of the body, its heading a level under the title. */
async function previewed(canvas: Canvas, w: Words) {
  const preview = within(await canvas.findByRole("article"));
  await waitFor(() => expect(preview.getByRole("heading", { level: 2, name: "Day one" })).toBeVisible(), answered);
  await expect(preview.getByRole("heading", { level: 2, name: "A starter in a jar" })).toBeVisible();
  await expect(preview.getByText("water", { selector: "strong" })).toBeVisible();
  // A short body uses a share of the limit of a percent at most.
  await expect(lengthUsed(canvas, w)).toBeLessThanOrEqual(1);
}

/** Written on a PC: the preview beside what is written, both shown, with no switch between them. */
async function writePC(context: StoryContext, lang: Lang) {
  const { canvas, w } = await write(context, lang);
  await previewed(canvas, w);
  await expect(canvas.getByRole("textbox", { name: w.body })).toBeVisible();
  await expect(canvas.queryByRole("radiogroup", { name: w.mode })).toBeNull();
}

export const WritingPC: Story = {
  globals: { locale: "en", ...pc },
  play: async (context) => writePC(context, "en"),
};

export const WritingJapanesePC: Story = {
  globals: { locale: "ja", ...pc },
  play: async (context) => writePC(context, "ja"),
};

/** Written at the minimum width, then previewed from Write | Preview. */
async function previewNarrow(context: StoryContext, lang: Lang) {
  const { canvas, w } = await write(context, lang);
  const preview = within(canvas.getByRole("radiogroup", { name: w.mode })).getByRole("radio", { name: w.preview });
  await userEvent.click(preview);
  await expect(preview).toHaveAttribute("aria-checked", "true");
  await previewed(canvas, w);
}

export const PreviewMinimumWidth: Story = {
  globals: { locale: "en", ...minimumWidth },
  play: async (context) => previewNarrow(context, "en"),
};

export const PreviewJapaneseMinimumWidth: Story = {
  globals: { locale: "ja", ...minimumWidth },
  play: async (context) => previewNarrow(context, "ja"),
};

/** Publish pressed over a limit: the form's alert says why, and the editor keeps what is written. */
async function refused(canvas: Canvas, w: Words, title: string) {
  await userEvent.click(canvas.getByRole("button", { name: w.publish }));
  await waitFor(() => expect(canvas.getByRole("alert")).toHaveTextContent(w.over));
  await expect(canvas.getByRole("heading", { level: 1, name: w.heading })).toBeVisible();
  await expect(canvas.getByRole("textbox", { name: w.title })).toHaveValue(title);
}

/** A title one character over its 200: the field is invalid and says by how much, and Publish does not publish. */
async function titleOver(context: StoryContext, lang: Lang) {
  const { canvas, w } = await editor(context, lang);
  const title = canvas.getByRole("textbox", { name: w.title });
  const long = "A starter in a jar. ".repeat(11).slice(0, 201);
  await userEvent.click(title);
  await userEvent.paste(long);
  await expect(title).toHaveValue(long);
  await userEvent.click(canvas.getByRole("textbox", { name: w.body }));
  await userEvent.paste("Flour and water.");
  await waitFor(() => expect(title).toHaveAttribute("aria-invalid", "true"));
  await expect(canvas.getByText(w.titleOver, { exact: true })).toBeInTheDocument();
  await refused(canvas, w, long);
}

export const TitleOverMinimumWidth: Story = {
  globals: { locale: "en", ...minimumWidth },
  play: async (context) => titleOver(context, "en"),
};

export const TitleOverJapanesePC: Story = {
  globals: { locale: "ja", ...pc },
  play: async (context) => titleOver(context, "ja"),
};

/** A body whose HTML is over the 100,000 bytes: the limits' line says by how much, and Publish does not publish. */
async function bodyOver(context: StoryContext, lang: Lang) {
  const { canvas, w } = await editor(context, lang);
  await userEvent.type(canvas.getByRole("textbox", { name: w.title }), "A starter in a jar");
  await userEvent.click(canvas.getByRole("textbox", { name: w.body }));
  // 102,000 characters, a paragraph of 102,007 bytes of HTML.
  await userEvent.paste("flour ".repeat(17_000));
  await waitFor(() => expect(lengthUsed(canvas, w)).toBeGreaterThan(100), answered);
  await refused(canvas, w, "A starter in a jar");
}

export const BodyOverPC: Story = {
  globals: { locale: "en", ...pc },
  play: async (context) => bodyOver(context, "en"),
};

export const BodyOverJapaneseMinimumWidth: Story = {
  globals: { locale: "ja", ...minimumWidth },
  play: async (context) => bodyOver(context, "ja"),
};

/** A draft reopened whose preview the server could not render: the preview says so with Try again, the draft kept. */
async function previewFailed(context: StoryContext, lang: Lang, narrow: boolean) {
  const { canvas, w } = await editor(context, lang);
  await expect(canvas.getByRole("textbox", { name: w.title })).toHaveValue("A photo walk along the north ridge");
  // Narrow, the preview is behind Write | Preview.
  if (narrow)
    await userEvent.click(
      within(canvas.getByRole("radiogroup", { name: w.mode })).getByRole("radio", { name: w.preview }),
    );
  const preview = within(await canvas.findByRole("article"));
  // The notice, its sentence then Try again, which holds the sentence's stop (sentence.ts).
  const notice = await waitFor(() => preview.getByRole("alert"), answered);
  await expect(notice).toBeVisible();
  await expect(notice).toHaveTextContent(w.failed);
  await expect(within(notice).getByRole("button", { name: w.tryAgain })).toBeVisible();
  await expect(preview.queryByRole("heading", { level: 3 })).toBeNull();
}

const failing = screen({ at: "article-draft:yuki-article-trail", server: { articles: { preview: "fails" } } });

export const PreviewFailedPC: Story = {
  globals: { locale: "en", ...pc },
  parameters: failing,
  play: async (context) => previewFailed(context, "en", false),
};

export const PreviewFailedJapaneseMinimumWidth: Story = {
  globals: { locale: "ja", ...minimumWidth },
  parameters: failing,
  play: async (context) => previewFailed(context, "ja", true),
};

/** What the fake server says of the summary it refuses (`publish: "summary"`), in English in any locale. */
const summaryRefusal = { title: "This could not be done.", detail: "This value is not valid. Check what you entered." };

/**
 * An edit saved from the preview, at the minimum width, and refused for its summary, which the server names: Write
 * comes back with the summary invalid and described by what is wrong, the alert says the problem's title alone; the
 * next edit of the summary clears it.
 */
async function summaryRefused(context: StoryContext) {
  await screenMeta.play(context);
  const canvas = canvasOf(context.canvasElement);
  await canvas.findByRole("heading", { level: 1, name: "Edit article" });
  const mode = within(canvas.getByRole("radiogroup", { name: "Write or preview" }));
  await userEvent.click(mode.getByRole("radio", { name: "Preview" }));
  await expect(mode.getByRole("radio", { name: "Preview" })).toHaveAttribute("aria-checked", "true");

  await userEvent.click(canvas.getByRole("button", { name: "Save" }));
  const summary = await canvas.findByRole("textbox", { name: "Summary" }, answered);
  await waitFor(() => expect(summary).toHaveAttribute("aria-invalid", "true"), answered);
  await expect(mode.getByRole("radio", { name: "Write" })).toHaveAttribute("aria-checked", "true");
  await expect(summary).toBeVisible();
  await expect(summary).toHaveAccessibleDescription(new RegExp(summaryRefusal.detail.replaceAll(".", "\\.")));
  await expect(canvas.getByRole("alert")).toHaveTextContent(
    new RegExp(`^${summaryRefusal.title.replace(".", "\\.")}$`),
  );

  await userEvent.type(summary, " Shorter.");
  await waitFor(() => expect(summary).not.toHaveAttribute("aria-invalid", "true"));
  await expect(canvas.queryByText(summaryRefusal.detail)).toBeNull();
  await expect(canvas.queryByText(summaryRefusal.title)).toBeNull();
  // React Aria announces Save's busy state for 7 s through a node of the page's own, which names the button by its ID;
  // the checks of the page read the screen once it is gone.
  await waitFor(
    () => {
      if (document.querySelector('body > [data-live-announcer] [aria-labelledby], [role="img"][aria-labelledby]'))
        throw new Error("the busy state is still announced");
    },
    { timeout: 9000 },
  );
}

export const SummaryRefusedMinimumWidth: Story = {
  globals: { locale: "en", ...minimumWidth },
  parameters: screen({ at: "article-edit:sam-article", viewer: "sam", server: { articles: { publish: "summary" } } }),
  play: summaryRefused,
};

/** A reader the server keeps from writing new articles: why, and nothing to write in or to publish. */
async function notAllowed(context: StoryContext, lang: Lang) {
  const { canvas, w } = await editor(context, lang);
  await expect(canvas.getByText(w.notAllowed, { exact: true })).toBeVisible();
  await expect(canvas.queryByRole("textbox", { name: w.title })).toBeNull();
  await expect(canvas.queryByRole("textbox", { name: w.body })).toBeNull();
  await expect(canvas.queryByRole("button", { name: w.publish })).toBeNull();
}

const permission = screen({ at: "/articles/new", server: { articles: { authoring: "permission" } } });

export const NotAllowedPC: Story = {
  globals: { locale: "en", ...pc },
  parameters: permission,
  play: async (context) => notAllowed(context, "en"),
};

export const NotAllowedJapaneseMinimumWidth: Story = {
  globals: { locale: "ja", ...minimumWidth },
  parameters: permission,
  play: async (context) => notAllowed(context, "ja"),
};
