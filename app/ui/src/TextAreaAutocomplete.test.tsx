// SPDX-License-Identifier: AGPL-3.0-only
// The completion's keyboard and ARIA behaviour in jsdom (ADR-0092 §3). The real-browser parts (the undo step of
// execCommand, the caret after a completion, the popover under the caret's line) are in the stories' play functions.
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Sheet } from "./Dialog";
import {
  type TextAutocompleteSuggestion,
  type TextAutocompleteWord,
  TextAreaAutocomplete,
  type TextAreaAutocompleteProps,
  TextFieldAutocomplete,
} from "./TextAreaAutocomplete";

type Account = TextAutocompleteSuggestion & { name: string };

const accounts: readonly Account[] = [
  { id: "alice", value: "@alice", textValue: "Alice @alice", name: "Alice" },
  { id: "alan", value: "@alan@example.org", textValue: "Alan @alan@example.org", name: "Alan" },
  { id: "albert", value: "@albert", textValue: "Albert @albert", name: "Albert" },
];

/** A stand-in for the web's activeWord: an ASCII word after `@`, `#` or `:` at a word start, up to the cursor. */
function findWord(text: string, cursor: number): TextAutocompleteWord | null {
  const match = /(?:^|\s)([@#:])(\w*)$/u.exec(text.slice(0, cursor));
  if (match === null) return null;
  const [, trigger = "", query = ""] = match;
  return { trigger, start: cursor - query.length - 1, end: cursor, query };
}

const loadAccounts = (word: TextAutocompleteWord) => accounts.filter((account) => account.id.startsWith(word.query.toLowerCase()));

type Props = Partial<TextAreaAutocompleteProps<Account>>;

function Field(props: Props) {
  return (
    <TextAreaAutocomplete<Account>
      label="Post"
      triggers={{ "@": 1, "#": 1 }}
      findWord={findWord}
      loadSuggestions={loadAccounts}
      renderSuggestion={(account) => account.name}
      suggestionsLabel="Suggestions"
      formatSuggestionCount={(count) => `${String(count)} suggestions`}
      loadingLabel="Loading suggestions"
      emptyLabel="No suggestions"
      queryDelay={0}
      {...props}
    />
  );
}

function setup(props: Props = {}) {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(<Field onChange={onChange} {...props} />);
  const textbox = screen.getByRole("textbox", { name: "Post" });
  return { user, onChange, textbox: textbox as HTMLTextAreaElement };
}

// React Aria's virtual focus tells the textarea that an option took focus with a blur event of its own (its
// relatedTarget is the option), sent from a React effect. user-event answers each blur of an edited field with a change
// event wrapped in act, which React refuses while it commits ("Should not already be working"); no browser has such a
// listener, so these blurs stop on the window, before user-event's listener on the document sees them.
window.addEventListener(
  "blur",
  (event) => {
    if (event.relatedTarget instanceof Element && event.relatedTarget.getAttribute("role") === "option") event.stopImmediatePropagation();
  },
  { capture: true },
);

// jsdom has neither a ResizeObserver nor document.fonts, which the mirror subscribes to in a browser.
beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  Object.defineProperty(document, "fonts", { configurable: true, value: new EventTarget() });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("TextAreaAutocomplete", () => {
  it("keeps the textarea a textbox that spell checks and has a return key", () => {
    const { textbox } = setup();
    expect(textbox.tagName).toBe("TEXTAREA");
    expect(textbox).not.toHaveAttribute("role");
    expect(textbox).toHaveAttribute("spellcheck", "true");
    expect(textbox).toHaveAttribute("autocorrect", "on");
    expect(textbox).toHaveAttribute("enterkeyhint", "enter");
    expect(textbox).toHaveAttribute("dir", "auto");
    expect(textbox).toHaveAttribute("aria-autocomplete", "list");
    expect(textbox).not.toHaveAttribute("aria-controls");
  });

  it("opens on a trigger with the first option chosen, and announces the count", async () => {
    const { user, textbox } = setup();
    await user.type(textbox, "Hi @al");
    const listbox = await screen.findByRole("listbox", { name: "Suggestions" });
    expect(textbox).toHaveAttribute("aria-controls", listbox.id);
    expect(screen.getByRole("status")).toHaveTextContent("3 suggestions");
    const first = screen.getByRole("option", { name: "Alice" });
    // React Aria delays aria-activedescendant so that it does not cut off the echo of the typed letter.
    await waitFor(() => {
      expect(textbox).toHaveAttribute("aria-activedescendant", first.id);
    });
    expect(first).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("option", { name: "Alan" })).toHaveAttribute("aria-selected", "false");
    expect(textbox).toHaveFocus();
  });

  it("waits for the fewest characters a trigger needs, and ignores other triggers", async () => {
    const loadSuggestions = vi.fn(loadAccounts);
    const { user, textbox } = setup({ loadSuggestions });
    await user.type(textbox, "@");
    await user.type(textbox, " :al");
    await user.type(textbox, " #");
    expect(loadSuggestions).not.toHaveBeenCalled();
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("moves with the arrow keys, stops at the ends and accepts with Enter", async () => {
    const { user, textbox, onChange } = setup();
    await user.type(textbox, "Hi @al");
    await screen.findByRole("listbox");
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}");
    await waitFor(() => {
      expect(textbox).toHaveAttribute("aria-activedescendant", screen.getByRole("option", { name: "Albert" }).id);
    });
    await user.keyboard("{ArrowUp}");
    await waitFor(() => {
      expect(textbox).toHaveAttribute("aria-activedescendant", screen.getByRole("option", { name: "Alan" }).id);
    });
    await user.keyboard("{Enter}");
    expect(textbox).toHaveValue("Hi @alan@example.org ");
    expect(onChange).toHaveBeenLastCalledWith("Hi @alan@example.org ");
    expect(textbox.selectionStart).toBe("Hi @alan@example.org ".length);
    await waitFor(() => {
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });
    expect(textbox).not.toHaveAttribute("aria-controls");
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
  });

  it("accepts the chosen option with Tab and keeps focus in the text area", async () => {
    const { user, textbox } = setup();
    await user.type(textbox, "@al");
    await screen.findByRole("listbox");
    await user.keyboard("{Tab}");
    expect(textbox).toHaveValue("@alice ");
    expect(textbox).toHaveFocus();
  });

  it("does not add a space when whitespace follows the word", async () => {
    const { user, textbox } = setup({ defaultValue: "@al rest" });
    await user.click(textbox);
    textbox.setSelectionRange(3, 3);
    fireEvent.select(textbox);
    await screen.findByRole("listbox");
    await user.keyboard("{Enter}");
    expect(textbox).toHaveValue("@alice rest");
    expect(textbox.selectionStart).toBe("@alice ".length);
  });

  it("accepts an option on a click", async () => {
    const { user, textbox } = setup();
    await user.type(textbox, "@alb");
    await user.click(await screen.findByRole("option", { name: "Albert" }));
    expect(textbox).toHaveValue("@albert ");
  });

  it("closes on Escape without closing the enclosing Sheet, which the next Escape closes", async () => {
    const user = userEvent.setup();
    function InSheet() {
      const [open, setOpen] = useState(true);
      return (
        <Sheet title="New post" closeLabel="Close" isOpen={open} onOpenChange={setOpen}>
          <Field />
        </Sheet>
      );
    }
    render(<InSheet />);
    const textbox = screen.getByRole("textbox", { name: "Post" });
    await user.type(textbox, "@al");
    // findByRole skips what is aria-hidden: the non-modal popover stays exposed while the modal Sheet hides the page.
    await screen.findByRole("listbox");
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });
    expect(screen.getByRole("dialog", { name: "New post" })).toBeInTheDocument();
    expect(textbox).toHaveValue("@al");
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });

  it("opens again after Escape once the text changes", async () => {
    const { user, textbox } = setup();
    await user.type(textbox, "@a");
    await screen.findByRole("listbox");
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });
    await user.type(textbox, "l");
    expect(await screen.findByRole("listbox")).toBeInTheDocument();
  });

  it("leaves Home, End, Ctrl+Enter and every key while closed to the text area and the form", async () => {
    const onKeyDown = vi.fn();
    const user = userEvent.setup();
    render(
      // The form's key handler, as a submit shortcut has.
      <form
        aria-label="Compose"
        onKeyDown={(event) => {
          onKeyDown(event.key, event.ctrlKey);
        }}
      >
        <Field />
      </form>,
    );
    const textbox = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    await user.type(textbox, "Hi");
    await user.keyboard("{Escape}");
    expect(onKeyDown).toHaveBeenCalledWith("Escape", false);
    await user.type(textbox, " @al");
    await screen.findByRole("listbox");
    onKeyDown.mockClear();
    await user.keyboard("{Control>}{Enter}{/Control}");
    expect(onKeyDown).toHaveBeenCalledWith("Enter", true);
    expect(textbox).toHaveValue("Hi @al");
    await user.keyboard("{Home}{End}{PageUp}{PageDown}");
    expect(onKeyDown).toHaveBeenCalledWith("Home", false);
    expect(onKeyDown).toHaveBeenCalledWith("End", false);
    expect(onKeyDown).toHaveBeenCalledWith("PageUp", false);
    expect(onKeyDown).toHaveBeenCalledWith("PageDown", false);
    expect(screen.getByRole("listbox")).toBeInTheDocument();
  });

  it("does nothing with keys while an IME composes, and opens after the composition ends", async () => {
    const loadSuggestions = vi.fn(loadAccounts);
    const { user, textbox } = setup({ loadSuggestions });
    await user.type(textbox, "@al");
    await screen.findByRole("listbox");

    fireEvent.compositionStart(textbox);
    await waitFor(() => {
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });
    // Chrome and Firefox mark the keys of a composition with isComposing, which user-event cannot set.
    // oxlint-disable-next-line testing-library/prefer-user-event -- user-event cannot send isComposing.
    expect(fireEvent.keyDown(textbox, { key: "Enter", isComposing: true })).toBe(true);
    fireEvent.compositionEnd(textbox);
    await screen.findByRole("listbox");

    // Safari sends the Enter that commits a conversion after compositionend, with keyCode 229.
    // oxlint-disable-next-line testing-library/prefer-user-event -- user-event cannot send keyCode 229.
    expect(fireEvent.keyDown(textbox, { key: "Enter", keyCode: 229 })).toBe(true);
    // oxlint-disable-next-line testing-library/prefer-user-event -- user-event cannot send keyCode 229.
    expect(fireEvent.keyDown(textbox, { key: "Tab", keyCode: 229 })).toBe(true);
    expect(textbox).toHaveValue("@al");
    expect(screen.getByRole("listbox")).toBeInTheDocument();
  });

  it("says when the lookup is slow and when it finds nothing", async () => {
    let answer: (found: readonly Account[]) => void = () => undefined;
    const loadSuggestions = vi.fn(
      () =>
        new Promise<readonly Account[]>((resolve) => {
          answer = resolve;
        }),
    );
    const { user, textbox } = setup({ loadSuggestions });
    await user.type(textbox, "@zz");
    expect(await screen.findByText("Loading suggestions", { selector: "p" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Loading suggestions");
    answer([]);
    expect(await screen.findByText("No suggestions", { selector: "p" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("No suggestions");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    // Without options, Escape still closes the popover.
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByText("No suggestions", { selector: "p" })).not.toBeInTheDocument();
    });
    expect(textbox).toHaveValue("@zz");
  });

  it("closes without a message when the lookup fails", async () => {
    const { user, textbox } = setup({ loadSuggestions: () => Promise.reject(new Error("429")) });
    await user.type(textbox, "@al");
    await waitFor(() => {
      expect(textbox).toHaveValue("@al");
    });
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(screen.queryByText("No suggestions")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
  });

  it("waits for typing to pause and aborts the lookup of a word that changed", async () => {
    const signals: AbortSignal[] = [];
    const loadSuggestions = vi.fn((word: TextAutocompleteWord, signal: AbortSignal) => {
      signals.push(signal);
      // `@al` never answers, so typing past it has to abort it.
      return word.query === "al" ? new Promise<readonly Account[]>(() => undefined) : loadAccounts(word);
    });
    const { user, textbox } = setup({ loadSuggestions, queryDelay: 200 });
    await user.type(textbox, "@alb");
    await screen.findByRole("option", { name: "Albert" });
    // Typed faster than the delay, the word is looked up once.
    expect(loadSuggestions).toHaveBeenCalledTimes(1);
    expect(loadSuggestions).toHaveBeenLastCalledWith({ trigger: "@", start: 0, end: 4, query: "alb" }, expect.any(AbortSignal));

    await user.type(textbox, "{Backspace}");
    await waitFor(() => {
      expect(loadSuggestions).toHaveBeenCalledTimes(2);
    });
    await user.type(textbox, "{Backspace}");
    expect(signals[1]?.aborted).toBe(true);
    // The earlier options stay while the shorter word is looked up.
    expect(screen.getByRole("option", { name: "Albert" })).toBeInTheDocument();
    expect(await screen.findByRole("option", { name: "Alan" })).toBeInTheDocument();
  });

  it("is controlled with value and onChange", async () => {
    const user = userEvent.setup();
    function Controlled() {
      const [value, setValue] = useState("");
      return <Field value={value} onChange={setValue} />;
    }
    render(<Controlled />);
    const textbox = screen.getByRole("textbox", { name: "Post" });
    await user.type(textbox, "@ali");
    await screen.findByRole("listbox");
    await user.keyboard("{Enter}");
    expect(textbox).toHaveValue("@alice ");
  });

  it("asks for each word after the pause queryDelay gives it", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const loadSuggestions = vi.fn(loadAccounts);
      const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
      render(<Field loadSuggestions={loadSuggestions} queryDelay={(word) => (word.trigger === "#" ? 0 : 1000)} />);
      const textbox = screen.getByRole("textbox", { name: "Post" });
      await user.type(textbox, "#a");
      await waitFor(() => {
        expect(loadSuggestions).toHaveBeenCalledTimes(1);
      });
      await user.type(textbox, " @al");
      vi.advanceTimersByTime(500);
      expect(loadSuggestions).toHaveBeenCalledTimes(1);
      vi.advanceTimersByTime(600);
      await waitFor(() => {
        expect(loadSuggestions).toHaveBeenCalledTimes(2);
      });
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("TextFieldAutocomplete", () => {
  it("completes the word in a one-line input, which stays a textbox", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <TextFieldAutocomplete<Account>
        label="Name"
        triggers={{ ":": 2 }}
        findWord={findWord}
        loadSuggestions={loadAccounts}
        renderSuggestion={(account) => account.name}
        suggestionsLabel="Suggestions"
        formatSuggestionCount={(count) => `${String(count)} suggestions`}
        loadingLabel="Loading suggestions"
        emptyLabel="No suggestions"
        queryDelay={0}
        onChange={onChange}
      />,
    );
    const textbox = screen.getByRole("textbox", { name: "Name" });
    expect(textbox.tagName).toBe("INPUT");
    expect(textbox).toHaveAttribute("spellcheck", "true");
    await user.type(textbox, "@al");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    await user.type(textbox, " :ali");
    const listbox = await screen.findByRole("listbox", { name: "Suggestions" });
    expect(textbox).toHaveAttribute("aria-controls", listbox.id);
    expect(textbox).toHaveAttribute("aria-autocomplete", "list");
    await user.keyboard("{Enter}");
    expect(textbox).toHaveValue("@al @alice ");
    expect(onChange).toHaveBeenLastCalledWith("@al @alice ");
  });
});
