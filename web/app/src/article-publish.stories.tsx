// SPDX-License-Identifier: AGPL-3.0-only

// What Publish decides in the article editor (#3091), played on the fake server as yuki writes, in en on a PC: what the
// body's count does not change (a title or a body missing, a summary over its limit) refuses at once, never busy; with
// the preview of the body as it is, Publish decides on its count at once; pressed before the preview of the body as it
// is has come (the editor asks for it 600 ms after the last key), it asks for this body's at once and decides on that,
// so a body cut under the limit and published at once is published, and one grown past it is refused; and when that
// preview fails, the publication goes ahead and the server's refusal shows in its words.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { screen, screenMeta } from "./app.samples";

const meta = {
  title: "Article publish",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/articles/new" }) },
  globals: { locale: "en", viewport: { value: "pc" } },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const words = {
  missing: "Give the article a title and a body to publish.",
  over: "Part of the article is over its limit. Shorten it or take out images to publish.",
  refused: "This server's review did not accept the article. Change it and try again.",
  length: /^Length: (\d+)% of the limit$/,
};

/** How long a play waits for what the server's preview shows: the 600 ms pause, the request and the render. */
const answered = { timeout: 5000 };

/** 102,000 characters, a paragraph of more than 100,000 bytes of HTML. */
const overLimit = "flour ".repeat(17_000);

const canvasOf = (root: HTMLElement) => within(root);

/** The queries of the canvas. */
type Canvas = ReturnType<typeof canvasOf>;

/** The editor's parts the plays use, once the screen shows its state. */
async function editor(context: StoryContext) {
  await screenMeta.play(context);
  const canvas = canvasOf(context.canvasElement);
  await canvas.findByRole("heading", { level: 1, name: "New article" });
  return {
    canvas,
    title: canvas.getByRole("textbox", { name: "Title" }),
    body: canvas.getByRole("textbox", { name: "Body" }),
    summary: canvas.getByRole("textbox", { name: "Summary" }),
    publish: canvas.getByRole("button", { name: "Publish" }),
  };
}

/** Writes `text` in place of the whole body, as a paste over all of it. */
async function replaceBody(body: HTMLElement, text: string) {
  await userEvent.click(body);
  document.getSelection()?.selectAllChildren(body);
  // The editor takes the selection from the document's selectionchange, which the browser fires after this task.
  await new Promise<void>((done) => {
    document.addEventListener(
      "selectionchange",
      () => {
        done();
      },
      { once: true },
    );
  });
  await userEvent.paste(text);
  await waitFor(() => expect(body).toHaveTextContent(text.trim()), answered);
}

/** The share of the length limit the preview shown says the body uses, in percent. */
function lengthUsed(canvas: Canvas): number {
  return Number(words.length.exec(canvas.getByText(words.length).textContent)?.[1]);
}

/** Watches whether Publish goes busy (React Aria's `data-pending`) from now on: the returned function says so. */
function busyWatch(publish: HTMLElement): () => boolean {
  let went = publish.hasAttribute("data-pending");
  new MutationObserver(() => {
    if (publish.hasAttribute("data-pending")) went = true;
  }).observe(publish, { attributes: true, attributeFilter: ["data-pending"] });
  return () => went;
}

/** Publish refused at once: its alert says `why`, it never went busy, and the editor stays. */
async function refusedAtOnce(canvas: Canvas, publish: HTMLElement, why: string) {
  const busy = busyWatch(publish);
  await userEvent.click(publish);
  await expect(canvas.getByRole("alert")).toHaveTextContent(why);
  await expect(busy()).toBe(false);
  await expect(canvas.getByRole("heading", { level: 1, name: "New article" })).toBeVisible();
}

/**
 * Waits until React Aria no longer announces Publish's busy state, which it does for 7 s through a node of the page's
 * own that names the button by its ID; the checks of the page read the screen once it is gone.
 */
async function quiet() {
  await waitFor(
    () => {
      if (document.querySelector('body > [data-live-announcer] [aria-labelledby], [role="img"][aria-labelledby]'))
        throw new Error("the busy state is still announced");
    },
    { timeout: 9000 },
  );
}

/** The article published: its page, its title its heading. */
async function published(canvas: Canvas, title: string) {
  await waitFor(() => expect(canvas.getByRole("heading", { level: 1, name: title })).toBeVisible(), answered);
  await expect(canvas.queryByRole("heading", { level: 1, name: "New article" })).toBeNull();
}

/** No title: refused at once as missing. */
export const NoTitle: Story = {
  play: async (context) => {
    const { canvas, body, publish } = await editor(context);
    await userEvent.click(body);
    await userEvent.paste("Flour and water.");
    await refusedAtOnce(canvas, publish, words.missing);
  },
};

/** An empty body: refused at once as missing, and nothing to preview. */
export const NoBody: Story = {
  play: async (context) => {
    const { canvas, title, publish } = await editor(context);
    await userEvent.type(title, "A starter in a jar");
    await refusedAtOnce(canvas, publish, words.missing);
    await expect(lengthUsed(canvas)).toBe(0);
  },
};

/** A summary over its 500 characters: refused at once, the summary marked. */
export const SummaryOver: Story = {
  play: async (context) => {
    const { canvas, title, body, summary, publish } = await editor(context);
    await userEvent.type(title, "A starter in a jar");
    await userEvent.click(body);
    await userEvent.paste("Flour and water.");
    await userEvent.click(summary);
    await userEvent.paste("A jar of flour. ".repeat(32).slice(0, 501));
    await userEvent.click(publish);
    await waitFor(() => expect(summary).toHaveAttribute("aria-invalid", "true"));
    await expect(canvas.getByRole("alert")).toBeVisible();
    await expect(publish).not.toHaveAttribute("data-pending");
    await expect(canvas.getByRole("heading", { level: 1, name: "New article" })).toBeVisible();
  },
};

/** With the preview of the body as it is, within the limits: Publish publishes. */
export const WithinTheLimits: Story = {
  play: async (context) => {
    const { canvas, title, body, publish } = await editor(context);
    await userEvent.type(title, "Within the limits");
    await userEvent.click(body);
    await userEvent.paste("Flour and water.");
    const preview = within(canvas.getByRole("article"));
    await waitFor(() => expect(preview.getAllByText("Flour and water.", { selector: "p" })).toHaveLength(2), answered);
    await userEvent.click(publish);
    await published(canvas, "Within the limits");
    await quiet();
  },
};

/** With the preview of the body as it is, over the length limit: refused at once with the over alert. */
export const OverTheLimit: Story = {
  play: async (context) => {
    const { canvas, title, body, publish } = await editor(context);
    await userEvent.type(title, "Over the limit");
    await userEvent.click(body);
    await userEvent.paste(overLimit);
    await waitFor(() => expect(lengthUsed(canvas)).toBeGreaterThan(100), answered);
    await refusedAtOnce(canvas, publish, words.over);
  },
};

/** A body over the limit, cut under it and published at once, before its preview has come: published. */
export const CutAndPublishedAtOnce: Story = {
  play: async (context) => {
    const { canvas, title, body, publish } = await editor(context);
    await userEvent.type(title, "Cut and published");
    await userEvent.click(body);
    await userEvent.paste(overLimit);
    await waitFor(() => expect(lengthUsed(canvas)).toBeGreaterThan(100), answered);
    await replaceBody(body, "Flour and water, cut short.");
    // The preview shown is still the long body's.
    await expect(lengthUsed(canvas)).toBeGreaterThan(100);
    // Publish asks for this body's preview, busy meanwhile, and publishes on its count.
    const busy = busyWatch(publish);
    await userEvent.click(publish);
    await published(canvas, "Cut and published");
    await expect(busy()).toBe(true);
    await quiet();
  },
};

/** A body within the limit, grown past it and published at once, before its preview has come: refused as over. */
export const GrownAndPublishedAtOnce: Story = {
  play: async (context) => {
    const { canvas, title, body, publish } = await editor(context);
    await userEvent.type(title, "Grown and published");
    await userEvent.click(body);
    await userEvent.paste("Flour and water.");
    await waitFor(() => expect(lengthUsed(canvas)).toBeLessThanOrEqual(1), answered);
    await replaceBody(body, overLimit);
    // The preview shown is still the short body's.
    await expect(lengthUsed(canvas)).toBeLessThanOrEqual(1);
    const busy = busyWatch(publish);
    await userEvent.click(publish);
    await waitFor(() => expect(canvas.getByRole("alert")).toHaveTextContent(words.over), answered);
    await expect(busy()).toBe(true);
    // Refused, Publish is not busy, and the problems are this body's: its own preview counts it over.
    await expect(publish).not.toHaveAttribute("data-pending");
    await waitFor(() => expect(lengthUsed(canvas)).toBeGreaterThan(100), answered);
    await expect(canvas.getByRole("heading", { level: 1, name: "New article" })).toBeVisible();
    await quiet();
  },
};

/** The preview of the body fails, and the server's review refuses the article: its words, after Publish went ahead. */
export const PreviewFailsAndServerRefuses: Story = {
  parameters: screen({ at: "/articles/new", server: { articles: { preview: "fails", publish: "refused" } } }),
  play: async (context) => {
    const { canvas, title, body, publish } = await editor(context);
    await userEvent.type(title, "Refused by the review");
    await userEvent.click(body);
    await userEvent.paste("Flour and water.");
    await userEvent.click(publish);
    await waitFor(() => expect(canvas.getByRole("alert")).toHaveTextContent(words.refused), answered);
    await expect(publish).not.toHaveAttribute("data-pending");
    await quiet();
  },
};
