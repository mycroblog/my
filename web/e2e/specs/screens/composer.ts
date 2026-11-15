// SPDX-License-Identifier: AGPL-3.0-only

// The composer, a dialog that writes a new post or a reply, keeps it as a draft on the server, or schedules it.

import { expect, type CDPSession, type Locator } from "@playwright/test";
import { Drafts } from "./drafts";
import { EmojiPicker } from "./emoji-picker";
import { answered } from "./write";

/** Waits until `box`, a text box of the editor, holds `text` as the composer sends it. */
async function holds(box: Locator, text: string | RegExp): Promise<void> {
  const value = expect.poll(() =>
    box.evaluate((root) => {
      const read = (node: Node): string => {
        if (node instanceof Text) return node.data.replaceAll("​", "");
        if (!(node instanceof Element) || node.hasAttribute("data-lexical-managed-linebreak")) return "";
        if (node instanceof HTMLBRElement) return "\n";
        if (node instanceof HTMLImageElement) return node.alt;
        return Array.from(node.childNodes, read).join("");
      };
      return Array.from(root.children, read).join("\n");
    }),
  );
  await (typeof text === "string" ? value.toBe(text) : value.toMatch(text));
}

/**
 * Where each occurrence of `token` in the text of `box` is drawn: on how many lines, and how far its first line is from
 * the box's left and right content edges. Rects that overlap in height are on one line.
 */
async function drawn(box: Locator, token: string): Promise<{ lines: number; fromLeft: number; fromRight: number }[]> {
  return box.evaluate((root, word) => {
    // The text nodes of the box in order, and where each starts in their joined text.
    const nodes: { node: Text; at: number }[] = [];
    let joined = "";
    const walker = root.ownerDocument.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    for (let n = walker.nextNode(); n !== null; n = walker.nextNode()) {
      const text = n as Text;
      nodes.push({ node: text, at: joined.length });
      joined += text.data;
    }
    const point = (offset: number, end: boolean): [Text, number] => {
      const found = nodes.find((n) => (end ? n.at < offset : n.at <= offset) && offset <= n.at + n.node.data.length);
      if (found === undefined) throw new Error(`no text at ${String(offset)}`);
      return [found.node, offset - found.at];
    };
    const style = getComputedStyle(root);
    const edges = root.getBoundingClientRect();
    const left = edges.left + parseFloat(style.borderLeftWidth) + parseFloat(style.paddingLeft);
    const right = edges.right - parseFloat(style.borderRightWidth) - parseFloat(style.paddingRight);
    const result: { lines: number; fromLeft: number; fromRight: number }[] = [];
    for (let i = joined.indexOf(word); i !== -1; i = joined.indexOf(word, i + word.length)) {
      const range = root.ownerDocument.createRange();
      range.setStart(...point(i, false));
      range.setEnd(...point(i + word.length, true));
      const rects = Array.from(range.getClientRects())
        .filter((r) => r.width > 0 && r.height > 0)
        .sort((a, b) => a.top - b.top);
      const lines: DOMRect[][] = [];
      for (const r of rects) {
        const line = lines.at(-1);
        if (line !== undefined && r.top < Math.max(...line.map((l) => l.bottom)) - 2) line.push(r);
        else lines.push([r]);
      }
      const first = lines[0] ?? [];
      result.push({
        lines: lines.length,
        fromLeft: Math.min(...first.map((r) => r.left)) - left,
        fromRight: right - Math.max(...first.map((r) => r.right)),
      });
    }
    return result;
  }, token);
}

export class Composer {
  readonly dialog: Locator;

  constructor(dialog: Locator) {
    this.dialog = dialog;
  }

  /** The text of the post; a reply starts with the mention of the person it replies to. */
  get text(): Locator {
    return this.dialog.getByRole("textbox", { name: "Text", exact: true });
  }

  /** Writes `text` after what the composer already holds. */
  async write(text: string): Promise<void> {
    await this.text.click();
    await this.text.press("ControlOrMeta+End");
    await this.text.pressSequentially(text);
  }

  /** Writes `text` in place of what the composer holds. */
  async replace(text: string): Promise<void> {
    await this.text.fill(text);
  }

  /**
   * Waits until the text holds `text` as the composer sends it: its lines joined by "\n", and a custom emoji, which
   * the editor shows as its image, as its `:shortcode:`.
   */
  async holds(text: string | RegExp): Promise<void> {
    await holds(this.text, text);
  }

  /** The button that adds the content warning's field over the text. */
  get warningButton(): Locator {
    return this.dialog.getByRole("button", { name: "Add a content warning", exact: true });
  }

  /** The content warning's field, one line, once added. */
  get warning(): Locator {
    return this.dialog.getByRole("textbox", { name: "Content warning", exact: true });
  }

  /** Adds the content warning's field with its button, and puts the caret in it. */
  async addWarning(): Promise<void> {
    await this.warningButton.click();
    await this.warning.click();
    await expect(this.warning).toBeFocused();
  }

  /** Waits until the content warning holds `text` as the composer sends it (as `holds` reads the text). */
  async warningHolds(text: string | RegExp): Promise<void> {
    await holds(this.warning, text);
  }

  /** The Post button. */
  get postButton(): Locator {
    return this.dialog.getByRole("button", { name: "Post", exact: true });
  }

  /** The count of the characters left, or over the limit, as a screen reader reads it. */
  get count(): Locator {
    return this.dialog.getByText(/^\d+ characters? (?:left|over)$/);
  }

  /**
   * Sends the post, returns once the server has answered and the composer has closed, and gives the posted status's ID
   * from the server's answer.
   */
  async postedId(): Promise<string> {
    const page = this.dialog.page();
    const response = page.waitForResponse(
      (r) => r.request().method() === "POST" && /\/statuses$/.test(new URL(r.url()).pathname),
    );
    response.catch(() => undefined);
    await this.postButton.click();
    const { id } = (await (await response).json()) as { id: string };
    await expect(this.dialog).toBeHidden();
    return id;
  }

  /**
   * How many lines each occurrence of `token` in the text is drawn on, in order: 1 for a token that never breaks inside.
   * A read of the page at one moment: assert on it with `expect.poll`.
   */
  async linesOf(token: string): Promise<number[]> {
    return (await drawn(this.text, token)).map((d) => d.lines);
  }

  /**
   * The side of the text box that the first line of each occurrence of `token` in the text keeps to: "left" for a line
   * that reads left to right, "right" for one that reads right to left. A read of the page at one moment.
   */
  async sideOf(token: string): Promise<("left" | "right")[]> {
    return (await drawn(this.text, token)).map((d) => (d.fromLeft <= d.fromRight ? "left" : "right"));
  }

  /**
   * Composes with an input method in the focused field as Japanese input does, through the browser's own composition
   * events (CDP Input.imeSetComposition): each of `steps` is the composition as it stands after a key, the caret at its
   * end. The composition stays open until `commit`.
   */
  async compose(steps: readonly string[]): Promise<void> {
    const cdp = await this.#cdp();
    for (const step of steps)
      await cdp.send("Input.imeSetComposition", { text: step, selectionStart: step.length, selectionEnd: step.length });
  }

  /** Ends the open composition with `text`, as the input method commits it (CDP Input.insertText). */
  async commit(text: string): Promise<void> {
    await (await this.#cdp()).send("Input.insertText", { text });
  }

  #session: CDPSession | undefined;

  async #cdp(): Promise<CDPSession> {
    const page = this.dialog.page();
    this.#session ??= await page.context().newCDPSession(page);
    return this.#session;
  }

  /**
   * Puts `plain`, and `html` when given, on the clipboard, and pastes it into the focused field as Ctrl+V (⌘V) does.
   */
  async paste(data: { plain: string; html?: string }): Promise<void> {
    const page = this.dialog.page();
    await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.evaluate(async ({ plain, html }) => {
      const items: Record<string, Blob> = { "text/plain": new Blob([plain], { type: "text/plain" }) };
      if (html !== undefined) items["text/html"] = new Blob([html], { type: "text/html" });
      await navigator.clipboard.write([new ClipboardItem(items)]);
    }, data);
    await page.keyboard.press("ControlOrMeta+v");
  }

  /** Copies the selection of the focused field as Ctrl+C (⌘C) does, and gives the plain text the clipboard holds. */
  async copy(): Promise<string> {
    const page = this.dialog.page();
    await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.evaluate(() => navigator.clipboard.writeText(""));
    await page.keyboard.press("ControlOrMeta+c");
    return page.evaluate(() => navigator.clipboard.readText());
  }

  /**
   * The text that each CSS Custom Highlight of the page marks in the text box, its ranges joined: the part over the
   * limit is one of them. A read of the page at one moment: assert on it with `expect.poll`.
   */
  async highlighted(): Promise<string[]> {
    return this.text.evaluate((root) =>
      Array.from(CSS.highlights.values(), (highlight) =>
        Array.from(highlight)
          .filter((r) => r instanceof Range && root.contains(r.commonAncestorContainer))
          .map(String)
          .join(""),
      ),
    );
  }

  /**
   * The suggestions for the word being typed at the caret, open under its line: "Accounts" for `@`, "Hashtags" for
   * `#`, "Emoji" for `:`. The text keeps the focus; its active option is the text's `aria-activedescendant`.
   */
  suggestions(kind: "Accounts" | "Hashtags" | "Emoji"): Locator {
    return this.dialog.page().getByRole("listbox", { name: kind, exact: true });
  }

  /**
   * The name of the text's active suggestion: the option its `aria-activedescendant` names, inside the list its
   * `aria-controls` names; undefined while it names none.
   */
  async activeSuggestion(): Promise<string | undefined> {
    return this.text.evaluate((text) => {
      const active = text.ariaActiveDescendantElement;
      const inList = (text.ariaControlsElements ?? []).some((list) => active !== null && list.contains(active));
      if (active === null || !inList) return undefined;
      return active.ariaLabel ?? active.textContent;
    });
  }

  /**
   * The live region of the dialog that announces `words` whole: the text announces how many suggestions opened ("2
   * suggestions"), or the line in their place ("Loading suggestions…", "No matching accounts").
   */
  announced(words: string | RegExp): Locator {
    const whole = typeof words === "string" ? new RegExp(`^${words.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`) : words;
    return this.dialog.getByRole("status").filter({ hasText: whole });
  }

  /** The button at the end of the text that opens the emoji picker, which writes the emoji picked at the caret. */
  get emojiButton(): Locator {
    return this.dialog.getByRole("button", { name: "Add emoji", exact: true });
  }

  /** Opens the emoji picker from the Add emoji button, and returns it. */
  async emoji(): Promise<EmojiPicker> {
    await this.emojiButton.click();
    return new EmojiPicker(this.dialog.page().getByRole("dialog", { name: "Add emoji", exact: true }));
  }

  /** Saves the edit of the reader's post, and returns once the server has answered and the composer has closed. */
  async save(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["PUT"], path: /\/statuses\/[^/]+$/ }, async () => {
      await this.dialog.getByRole("button", { name: "Save", exact: true }).click();
    });
    await expect(this.dialog).toBeHidden();
  }

  /** Sends the post, and returns once the server has answered and the composer has closed. */
  async post(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["POST"], path: /\/statuses$/ }, async () => {
      await this.dialog.getByRole("button", { name: "Post", exact: true }).click();
    });
    await expect(this.dialog).toBeHidden();
  }

  /** Closes the composer with its Close button. */
  async close(): Promise<void> {
    await this.dialog.getByRole("button", { name: "Close", exact: true }).click();
    await expect(this.dialog).toBeHidden();
  }

  /** Chooses `item` of the drafts' menu in the composer's head. */
  async draftsMenu(item: "Save as draft" | "Drafts and scheduled posts"): Promise<void> {
    await this.dialog.getByRole("button", { name: "Drafts", exact: true }).click();
    const menu = this.dialog.page().getByRole("menu", { name: "Drafts", exact: true });
    await menu.getByRole("menuitem", { name: item, exact: true }).click();
  }

  /** Keeps the post as a draft on the server, and returns once the server has answered and the composer has closed. */
  async saveAsDraft(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["POST", "PUT"], path: /\/drafts(?:\/[^/]+)?$/ }, async () => {
      await this.draftsMenu("Save as draft");
    });
    await expect(this.dialog).toBeHidden();
  }

  /**
   * Saves a draft opened from the server as a draft when another device saved a newer one since: returns once the
   * server has refused it, with the composer open and its alert shown.
   */
  async saveAsDraftRefused(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["PUT"], path: /\/drafts\/[^/]+$/ }, async () => {
      await this.draftsMenu("Save as draft");
    });
    await expect(this.alert).toBeVisible();
  }

  /** Opens the reader's drafts and scheduled posts in the composer's place. */
  async openDrafts(): Promise<Drafts> {
    await this.draftsMenu("Drafts and scheduled posts");
    return new Drafts(this.dialog.page());
  }

  /** The alert over the row: why the post was not sent, or why a draft was not saved. */
  get alert(): Locator {
    return this.dialog.getByRole("alert");
  }

  /** Opens the newer draft that another device saved, from the alert of a refused save. */
  async openNewer(): Promise<void> {
    await this.alert.getByRole("button", { name: "Open the newer one", exact: true }).click();
  }

  /** The schedule tool of the row: "Schedule" while no time is set, "Change the scheduled time" while one is. */
  scheduleTool(name: "Schedule" | "Change the scheduled time"): Locator {
    return this.dialog.getByRole("button", { name, exact: true });
  }

  /** The form of the time the post goes out, once open. */
  get scheduleForm(): Locator {
    return this.dialog.getByRole("dialog", { name: "Schedule", exact: true });
  }

  /** The field of the date and time in the schedule's form. */
  get scheduleField(): Locator {
    return this.scheduleForm.getByLabel("Date and time", { exact: true });
  }

  /**
   * Opens the schedule's form from the schedule tool and sets the time it starts with: the first whole hour an hour or
   * more from now.
   */
  async setTime(): Promise<void> {
    await this.scheduleTool("Schedule").click();
    await this.scheduleForm.getByRole("button", { name: "Set time", exact: true }).click();
    await expect(this.scheduleForm).toBeHidden();
  }

  /**
   * Moves the time in the open schedule's form a minute later, from the keyboard, and sets it: the field's segments read
   * month, day, year, hour, minute in en.
   */
  async nextMinute(): Promise<void> {
    await this.scheduleField.focus();
    for (let i = 0; i < 4; i += 1) await this.scheduleField.press("ArrowRight");
    await this.scheduleField.press("ArrowUp");
    await this.scheduleForm.getByRole("button", { name: "Set time", exact: true }).click();
    await expect(this.scheduleForm).toBeHidden();
  }

  /**
   * The line over the row while a time is set: "Scheduled for <time> (<zone>)", the zone stated while the form is
   * closed, then Change time at its end.
   */
  get timeLine(): Locator {
    return this.dialog.getByRole("paragraph").filter({ hasText: /^Scheduled for .+\s*Change time$/ });
  }

  /** Opens the schedule's form from the time line's Change time. */
  async changeTime(): Promise<void> {
    await this.timeLine.getByRole("button", { name: "Change time", exact: true }).click();
    await expect(this.scheduleForm).toBeVisible();
  }

  /**
   * Schedules the post with the row's Schedule, which takes Post's place while a time is set, and returns once the
   * server has answered and the composer has closed.
   */
  async schedule(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["PUT"], path: /\/drafts\/[^/]+\/schedule$/ }, async () => {
      await this.dialog.getByRole("button", { name: "Schedule", exact: true }).click();
    });
    await expect(this.dialog).toBeHidden();
  }

  /** Posts a draft opened from the server, and returns once the server has published it and the composer has closed. */
  async postDraft(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["POST"], path: /\/drafts\/[^/]+\/publish$/ }, async () => {
      await this.dialog.getByRole("button", { name: "Post", exact: true }).click();
    });
    await expect(this.dialog).toBeHidden();
  }
}
