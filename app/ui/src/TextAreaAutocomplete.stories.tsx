// SPDX-License-Identifier: AGPL-3.0-only
// TextAreaAutocomplete stories (ADR-0092 §3). The play functions are the completion's real-browser tests: the popover
// under the caret's line (and following it when the line moves), the whole-pixel line height of the layered text area,
// one undo step for a completion, Esc inside a Sheet, and the loading and no-results messages. The keyboard and ARIA
// details are also tested in jsdom (TextAreaAutocomplete.test.tsx).
import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useEffect, useState } from "react";
import { expect, waitFor } from "storybook/test";
import { Avatar } from "./Avatar";
import { Button } from "./Button";
import { Sheet } from "./Dialog";
import { EmojiIcon } from "./Icons";
import {
  type TextAutocompleteSuggestion,
  type TextAutocompleteWord,
  TextAreaAutocomplete,
  type TextAreaAutocompleteProps,
} from "./TextAreaAutocomplete";

type Suggestion = TextAutocompleteSuggestion & { label: string; detail: string };

// Stories may spell out English; apps pass react-intl messages instead (web-client/README.md A9).
const suggestions: Readonly<Record<string, readonly Suggestion[]>> = {
  "@": [
    { id: "alice", value: "@alice", textValue: "Alice Liddell @alice", label: "Alice Liddell", detail: "@alice" },
    {
      id: "alan",
      value: "@alan@example.org",
      textValue: "Alan @alan@example.org",
      label: "Alan",
      detail: "@alan@example.org",
    },
    {
      id: "albert",
      value: "@albert@mastodon.example",
      textValue: "アルベルト @albert@mastodon.example",
      label: "アルベルト",
      detail: "@albert@mastodon.example",
    },
  ],
  "#": [
    { id: "fediverse", value: "#fediverse", textValue: "#fediverse", label: "#fediverse", detail: "1,204 posts" },
    { id: "fedi", value: "#fedi", textValue: "#fedi", label: "#fedi", detail: "88 posts" },
  ],
  ":": [{ id: "blobcat", value: ":blobcat:", textValue: ":blobcat:", label: "🐱", detail: ":blobcat:" }],
};

/** A stand-in for the web's activeWord: a word after `@`, `#` or `:` at a word start, up to the cursor. */
function findWord(text: string, cursor: number): TextAutocompleteWord | null {
  const match = /(?:^|\s)([@#:])([\p{L}\p{N}_]*)$/u.exec(text.slice(0, cursor));
  if (match === null) return null;
  const [, trigger = "", query = ""] = match;
  return { trigger, start: cursor - query.length - 1, end: cursor, query };
}

const fixedSuggestions: TextAreaAutocompleteProps<Suggestion>["loadSuggestions"] = (word) => {
  const query = word.query.toLowerCase();
  return (suggestions[word.trigger] ?? []).filter((suggestion) => suggestion.id.startsWith(query));
};

function renderSuggestion(suggestion: Suggestion) {
  return (
    <>
      {suggestion.id === "alice" || suggestion.id === "alan" || suggestion.id === "albert" ? (
        <Avatar name={suggestion.label} size="xs" decorative />
      ) : null}
      <span dir="auto" className="truncate">
        {suggestion.label}
      </span>
      <span dir="auto" className="ms-auto truncate text-sm text-fg-muted">
        {suggestion.detail}
      </span>
    </>
  );
}

const formatRemaining = (remaining: number) =>
  remaining < 0 ? `${String(-remaining)} characters too many` : `${String(remaining)} characters left`;

/**
 * The story's args: the props with the character count's pair as two optional props, since Storybook cannot derive
 * a story's args from a union of props (TextArea's character count is one).
 */
type FieldArgs = Omit<TextAreaAutocompleteProps<Suggestion>, "characterLimit" | "formatRemaining"> & {
  characterLimit?: number;
  formatRemaining?: (remaining: number) => string;
};

function Field({ characterLimit, formatRemaining, ...props }: FieldArgs) {
  return characterLimit === undefined || formatRemaining === undefined ? (
    <TextAreaAutocomplete {...props} />
  ) : (
    <TextAreaAutocomplete {...props} characterLimit={characterLimit} formatRemaining={formatRemaining} />
  );
}

/**
 * React Aria's virtual focus tells the textarea that an option took focus with a blur event of its own (its
 * relatedTarget is the option), sent from a React effect. The play functions' user-event answers each blur of an edited
 * field with a change event wrapped in act, which React refuses while it commits ("Should not already be working");
 * no browser does that, so while these stories render, such blurs stop on the window, before user-event sees them.
 */
function IgnoreVirtualBlur({ children }: { children: ReactNode }) {
  useEffect(() => {
    const onBlur = (event: FocusEvent) => {
      if (event.relatedTarget instanceof Element && event.relatedTarget.getAttribute("role") === "option")
        event.stopImmediatePropagation();
    };
    window.addEventListener("blur", onBlur, { capture: true });
    return () => {
      window.removeEventListener("blur", onBlur, { capture: true });
    };
  }, []);
  return children;
}

const meta = {
  title: "Forms/TextAreaAutocomplete",
  component: Field,
  // The popover opens under the text area, inside the frame.
  decorators: [
    (Story) => (
      <IgnoreVirtualBlur>
        <div className="min-h-96">
          <Story />
        </div>
      </IgnoreVirtualBlur>
    ),
  ],
  args: {
    label: "Post",
    rows: 4,
    triggers: { "@": 1, "#": 1, ":": 2 },
    findWord,
    loadSuggestions: fixedSuggestions,
    renderSuggestion,
    suggestionsLabel: "Suggestions",
    formatSuggestionCount: (count) => `${String(count)} suggestions`,
    loadingLabel: "Loading suggestions…",
    emptyLabel: "No suggestions",
    queryDelay: 0,
  },
} satisfies Meta<typeof Field>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Closed: a text area like TextArea, with the mirror behind the transparent textarea. */
export const Default: Story = { args: { placeholder: "What's on your mind?" } };

/** The list under the caret's line, as wide as the text area, with the first option chosen. */
export const Suggestions: Story = {
  play: async ({ canvas, userEvent }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    // The layered text area has a whole-pixel line height, so the scrolled mirror lines up with it (#1152).
    await expect(Number.isInteger(Number.parseFloat(getComputedStyle(textbox).lineHeight))).toBe(true);
    await userEvent.click(textbox);
    await userEvent.type(textbox, "Thanks @al");
    const listbox = await canvas.findByRole("listbox", { name: "Suggestions" });
    await expect(textbox).not.toHaveAttribute("role");
    await expect(textbox).toHaveAttribute("aria-controls", listbox.id);
    await expect(canvas.getByRole("status")).toHaveTextContent("3 suggestions");
    const first = canvas.getByRole("option", { name: /Alice Liddell/ });
    await waitFor(() => expect(textbox).toHaveAttribute("aria-activedescendant", first.id));
    await expect(first).toHaveAttribute("aria-selected", "true");

    // The popover's top is the bottom of the first line, and its width the text area's.
    const popover = listbox.parentElement;
    if (popover === null) throw new Error("the listbox has no popover");
    const style = getComputedStyle(textbox);
    const lineHeight = Number.parseFloat(style.lineHeight);
    const firstLineBottom =
      textbox.getBoundingClientRect().top +
      Number.parseFloat(style.borderTopWidth) +
      Number.parseFloat(style.paddingTop) +
      lineHeight;
    await waitFor(() => expect(Math.abs(popover.getBoundingClientRect().top - firstLineBottom)).toBeLessThanOrEqual(1));
    await expect(
      Math.abs(popover.getBoundingClientRect().width - textbox.getBoundingClientRect().width),
    ).toBeLessThanOrEqual(1);
  },
};

/** The caret on a later line: the list opens under that line, and follows it when the line moves. */
export const LaterLine: Story = {
  args: {
    rows: 6,
    defaultValue: "First line\nSecond line, which is long enough to wrap in a narrow text area when it narrows\n",
  },
  play: async ({ canvas, userEvent, canvasElement }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    await userEvent.click(textbox);
    textbox.setSelectionRange(textbox.value.length, textbox.value.length);
    await userEvent.type(textbox, "#fe");
    const listbox = await canvas.findByRole("listbox", { name: "Suggestions" });
    const popover = listbox.parentElement;
    if (popover === null) throw new Error("the listbox has no popover");
    const style = getComputedStyle(textbox);
    const lineHeight = Number.parseFloat(style.lineHeight);
    const lineBottom = (line: number) =>
      textbox.getBoundingClientRect().top +
      Number.parseFloat(style.borderTopWidth) +
      Number.parseFloat(style.paddingTop) +
      line * lineHeight -
      textbox.scrollTop;
    const lineOfCaret = () => Math.round((popover.getBoundingClientRect().top - lineBottom(0)) / lineHeight);
    await waitFor(() =>
      expect(Math.abs(popover.getBoundingClientRect().top - lineBottom(lineOfCaret()))).toBeLessThanOrEqual(1),
    );
    const before = lineOfCaret();
    await expect(before).toBeGreaterThanOrEqual(3);

    // Narrowing the text area wraps the second line once more; the open list moves down with the caret's line.
    const frame = canvasElement.closest<HTMLElement>("[data-story-frame]") ?? canvasElement;
    const width = frame.style.inlineSize;
    frame.style.inlineSize = "320px";
    try {
      await waitFor(() => expect(lineOfCaret()).toBeGreaterThan(before));
      await waitFor(() =>
        expect(Math.abs(popover.getBoundingClientRect().top - lineBottom(lineOfCaret()))).toBeLessThanOrEqual(1),
      );
    } finally {
      frame.style.inlineSize = width;
    }
  },
};

// user-event keeps its own idea of a field's value, which a completion (execCommand) does not update, so each story types
// only before its one completion.

/** Enter accepts the chosen option: the word becomes the value and a space, and the caret goes after it. */
export const AcceptWithEnter: Story = {
  play: async ({ canvas, userEvent }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    await userEvent.click(textbox);
    await userEvent.type(textbox, "Hi @al");
    await canvas.findByRole("listbox");
    await userEvent.keyboard("{ArrowDown}{Enter}");
    await expect(textbox).toHaveValue("Hi @alan@example.org ");
    await expect(textbox.selectionStart).toBe("Hi @alan@example.org ".length);
    await waitFor(() => expect(canvas.queryByRole("listbox")).not.toBeInTheDocument());
  },
};

/** Tab accepts too, and focus stays in the text area; a shortcode needs two characters after its colon. */
export const AcceptWithTab: Story = {
  play: async ({ canvas, userEvent }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    await userEvent.click(textbox);
    await userEvent.type(textbox, "Nice :b");
    await expect(canvas.queryByRole("listbox")).not.toBeInTheDocument();
    await userEvent.type(textbox, "l");
    await canvas.findByRole("listbox");
    await userEvent.keyboard("{Tab}");
    await expect(textbox).toHaveValue("Nice :blobcat: ");
    await expect(textbox).toHaveFocus();
  },
};

/** Esc closes the list and keeps the text; typing on opens it again. */
export const Escape: Story = {
  play: async ({ canvas, userEvent }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    await userEvent.click(textbox);
    await userEvent.type(textbox, "#f");
    await canvas.findByRole("listbox");
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(canvas.queryByRole("listbox")).not.toBeInTheDocument());
    await expect(textbox).toHaveValue("#f");
    await userEvent.type(textbox, "e");
    await canvas.findByRole("listbox");
  },
};

/** One undo step takes a completion back and one redo step puts it again (execCommand("insertText"), ADR-0092 §3). */
export const Undo: Story = {
  play: async ({ canvas, userEvent }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    await userEvent.click(textbox);
    await userEvent.type(textbox, "Hi @al");
    await canvas.findByRole("listbox");
    await userEvent.keyboard("{Enter}");
    await expect(textbox).toHaveValue("Hi @alice ");
    // oxlint-disable-next-line typescript/no-deprecated -- the undo command that Ctrl+Z runs.
    document.execCommand("undo");
    await expect(textbox).toHaveValue("Hi @al");
    // oxlint-disable-next-line typescript/no-deprecated -- the redo command that Ctrl+Shift+Z runs.
    document.execCommand("redo");
    await expect(textbox).toHaveValue("Hi @alice ");
  },
};

/** An IME composition closes the list and its keys do nothing; the list opens again when it ends. */
export const Composition: Story = {
  play: async ({ canvas, userEvent }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    await userEvent.click(textbox);
    await userEvent.type(textbox, "#fe");
    await canvas.findByRole("listbox");
    textbox.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true, data: "" }));
    await waitFor(() => expect(canvas.queryByRole("listbox")).not.toBeInTheDocument());
    textbox.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true, isComposing: true }),
    );
    textbox.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "" }));
    await canvas.findByRole("listbox");
    // Safari's committing Enter arrives after compositionend with keyCode 229; it does not accept.
    // oxlint-disable-next-line typescript/no-deprecated -- Safari's committing Enter is told apart only by keyCode 229.
    const enter = new KeyboardEvent("keydown", { key: "Enter", keyCode: 229, bubbles: true, cancelable: true });
    textbox.dispatchEvent(enter);
    await expect(textbox).toHaveValue("#fe");
    await expect(canvas.getByRole("listbox")).toBeInTheDocument();
  },
};

/** In the compose Sheet: the list stays exposed to assistive technology, and Esc closes the list, not the Sheet. */
export const InSheet: Story = {
  decorators: [
    (Story) => (
      <div className="min-h-136">
        <Story />
      </div>
    ),
  ],
  render: function Render(args: FieldArgs) {
    const [open, setOpen] = useState(true);
    return (
      <>
        <Button
          onPress={() => {
            setOpen(true);
          }}
        >
          New post
        </Button>
        <Sheet title="New post" closeLabel="Close" isOpen={open} onOpenChange={setOpen}>
          <Field {...args} />
        </Sheet>
      </>
    );
  },
  play: async ({ canvas, userEvent }) => {
    const textbox = await canvas.findByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    await userEvent.click(textbox);
    await userEvent.type(textbox, "@a");
    const listbox = await canvas.findByRole("listbox");
    await expect(listbox.closest("[aria-hidden=true], [inert]")).toBeNull();
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(canvas.queryByRole("listbox")).not.toBeInTheDocument());
    await expect(canvas.getByRole("dialog", { name: "New post" })).toBeInTheDocument();
    // One option, so that the list fits in the frame under the sheet's text area.
    await userEvent.type(textbox, "li");
    await canvas.findByRole("option", { name: /Alice Liddell/ });
  },
};

/** A lookup slower than 300 ms says so in the popover and the status region. */
export const Loading: Story = {
  args: { loadSuggestions: () => new Promise<readonly Suggestion[]>(() => undefined) },
  play: async ({ canvas, userEvent }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    await userEvent.click(textbox);
    await userEvent.type(textbox, "@zz");
    await expect(await canvas.findByText("Loading suggestions…", { selector: "p" })).toBeInTheDocument();
    await expect(canvas.getByRole("status")).toHaveTextContent("Loading suggestions…");
  },
};

/** No suggestions: the popover says so, and the arrow keys stay with the text. */
export const NoSuggestions: Story = {
  play: async ({ canvas, userEvent }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    await userEvent.click(textbox);
    await userEvent.type(textbox, "@zz");
    await expect(await canvas.findByText("No suggestions", { selector: "p" })).toBeInTheDocument();
    await expect(canvas.getByRole("status")).toHaveTextContent("No suggestions");
    await expect(canvas.queryByRole("listbox")).not.toBeInTheDocument();
  },
};

/** With TextArea's character count and end action; the text keeps clear of the button over the mirror too. */
export const WithCountAndEndAction: Story = {
  args: {
    characterLimit: 500,
    formatRemaining,
    defaultValue:
      "Mentions like @alice, tags like #fediverse and emoji like :blobcat: wrap before the button in the corner. مرحبا بالعالم",
    endAction: (
      <Button variant="ghost" size="icon" aria-label="Add emoji">
        <EmojiIcon />
      </Button>
    ),
  },
};
