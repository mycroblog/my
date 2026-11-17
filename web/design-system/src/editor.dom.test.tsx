// SPDX-License-Identifier: AGPL-3.0-only

// What the plain-text editor is to assistive technology, to the keyboard and to its caller: a text box with its name
// and states, what a paste and Enter write, what its handle writes, and the suggestions it controls. No simulation
// types into it as a browser does (a key pressed in an empty box writes nothing), so the tests write by pasting;
// what a keyboard writes at the caret, and the focus the handle gives it, stay with the tests in a browser.

import { expect, test } from "bun:test";
import { ComposerFrame, SuggestionList } from "@my/design-system";
import { act, isInaccessible, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, useEffect, useState, type ReactNode, type Ref } from "react";
import {
  $plainText,
  PlainTextEditor,
  useEditor,
  type EditorHandle,
  type FieldSuggest,
  type PlainTextEditorProps,
} from "./editor";

const none = () => undefined;

/** The editor as a caller holds it: its value in state, and each value it told the caller. */
function Held({
  initial = "",
  told,
  set,
  ...props
}: Partial<PlainTextEditorProps> & {
  initial?: string;
  told?: string[];
  /** A button that sets the value from outside. */
  set?: string;
}) {
  const [value, setValue] = useState(initial);
  return (
    <>
      <PlainTextEditor
        namespace="test"
        label="Text"
        placeholder="What's on your mind?"
        look="composer"
        multiline
        {...props}
        value={value}
        onChange={(next) => {
          told?.push(next);
          setValue(next);
        }}
      />
      {set !== undefined && (
        <button
          type="button"
          onClick={() => {
            setValue(set);
          }}
        >
          Set
        </button>
      )}
    </>
  );
}

const box = (name = "Text") => screen.getByRole("textbox", { name });

/** Pastes `text` at the caret of the text box, as Ctrl+V does with plain text on the clipboard. */
async function paste(user: ReturnType<typeof userEvent.setup>, text: string) {
  await user.click(box());
  await user.paste(text);
}

test("the text box has its name, its placeholder, its description and its states as given", () => {
  render(
    <>
      <PlainTextEditor
        namespace="test"
        value="a"
        onChange={none}
        label="Text"
        placeholder="What's on your mind?"
        look="composer"
        describedBy="text-error"
        invalid
        required
        multiline
      />
      <p id="text-error">The text is too long.</p>
    </>,
  );

  const text = screen.getByRole("textbox", { name: "Text", description: "The text is too long." });
  expect(text.getAttribute("aria-placeholder")).toBe("What's on your mind?");
  expect(text.getAttribute("aria-invalid")).toBe("true");
  expect(text.getAttribute("aria-required")).toBe("true");
  expect(text.getAttribute("aria-multiline")).toBe("true");
  expect(text.isContentEditable).toBe(true);
});

test("the text box says nothing of a state not set", () => {
  render(<Held initial="a" />);

  for (const name of ["aria-describedby", "aria-invalid", "aria-required", "aria-readonly"])
    expect(box().getAttribute(name) ?? "false").toBe("false");
});

test("a read-only text box says so, is not editable, and a paste writes nothing in it", async () => {
  const user = userEvent.setup();
  const told: string[] = [];
  render(<Held initial="Quiet morning." told={told} readOnly />);

  expect(box().getAttribute("aria-readonly")).toBe("true");
  expect(box().getAttribute("contenteditable")).toBe("false");
  await paste(user, "!");

  expect(told).toEqual([]);
  expect(box().textContent).toBe("Quiet morning.");
});

test("empty, it shows its placeholder, which assistive technology gets from the text box alone", () => {
  const { rerender } = render(<Held key="empty" />);

  expect(isInaccessible(screen.getByText("What's on your mind?"))).toBe(true);
  expect(box().getAttribute("aria-placeholder")).toBe("What's on your mind?");

  rerender(<Held key="written" initial="Quiet morning." />);
  expect(screen.queryByText("What's on your mind?")).toBeNull();
});

test("it holds its value, and a value set from outside replaces what it holds", async () => {
  const user = userEvent.setup();
  render(<Held initial="Quiet morning." set="A loud evening." />);
  expect(box().textContent).toBe("Quiet morning.");

  await user.click(screen.getByRole("button", { name: "Set" }));

  await waitFor(() => {
    if (box().textContent !== "A loud evening.") throw new Error(`it holds ${box().textContent}`);
  });
});

test("a paste writes its text at the caret and tells the caller the whole text, its line breaks kept", async () => {
  const user = userEvent.setup();
  const told: string[] = [];
  render(<Held told={told} />);

  await paste(user, "first\nsecond");
  await user.paste(" line");

  expect(told).toEqual(["first\nsecond", "first\nsecond line"]);
});

test("Enter writes a line break in a text of several lines", async () => {
  const user = userEvent.setup();
  const told: string[] = [];
  render(<Held told={told} />);
  await paste(user, "first");

  await user.keyboard("{Enter}");
  await user.paste("second");

  expect(told.at(-1)).toBe("first\nsecond");
});

test("a text of one line is not multiline: Enter writes nothing, and a line break pasted in is taken out", async () => {
  const user = userEvent.setup();
  const told: string[] = [];
  render(<Held told={told} multiline={false} look="line" label="Content warning" />);
  const line = box("Content warning");
  expect(line.getAttribute("aria-multiline")).toBe("false");

  await user.click(line);
  await user.paste("Food,\nclose\r\nup");
  await user.keyboard("{Enter}");

  expect(told).toEqual(["Food,closeup"]);
});

test("Ctrl+Z takes back what was pasted", async () => {
  const user = userEvent.setup();
  const told: string[] = [];
  render(<Held initial="Quiet" told={told} />);
  await paste(user, " morning.");
  expect(told.at(-1)).toBe("Quiet morning.");

  await user.keyboard("{Control>}z{/Control}");

  expect(told.at(-1)).toBe("Quiet");
});

test("with autoFocus it takes the focus once drawn, also from a control the reader is on", async () => {
  render(
    <>
      <button type="button">Other</button>
    </>,
  );
  screen.getByRole("button", { name: "Other" }).focus();
  const { unmount } = render(<Held initial="Quiet morning." autoFocus />);

  await waitFor(() => {
    if (document.activeElement !== box()) throw new Error("the text box has no focus");
  });
  unmount();
});

test("without autoFocus it leaves the focus where it is", async () => {
  render(<button type="button">Other</button>);
  const other = screen.getByRole("button", { name: "Other" });
  other.focus();

  render(<Held initial="Quiet morning." />);
  await act(() => Promise.resolve());

  expect(document.activeElement === other).toBe(true);
});

test("drawn late with autoFocus, it takes the focus from the page", async () => {
  render(<Held initial="Quiet morning." autoFocus late />);

  await waitFor(() => {
    if (document.activeElement !== box()) throw new Error("the text box has no focus");
  });
});

test("drawn late with autoFocus, it takes the focus from the dialog that holds it", async () => {
  /** The editor, drawn a moment after what holds it, as when its chunk comes later. */
  function Later() {
    const [drawn, setDrawn] = useState(false);
    useEffect(() => {
      const timer = setTimeout(() => {
        setDrawn(true);
      }, 0);
      return () => {
        clearTimeout(timer);
      };
    }, []);
    return drawn ? <Held initial="Quiet morning." autoFocus late /> : null;
  }
  render(
    <ComposerFrame title="New post" closeLabel="Close" onClose={none} onDismiss={none}>
      <Later />
    </ComposerFrame>,
  );
  const dialog = await screen.findByRole("dialog", { name: "New post" });
  expect(dialog.contains(document.activeElement)).toBe(true);

  await waitFor(() => {
    if (document.activeElement !== within(dialog).queryByRole("textbox", { name: "Text" }))
      throw new Error("the text box has no focus");
  });
});

test("drawn late with autoFocus, it never takes the focus from a control the reader has gone to since", async () => {
  function Page() {
    const [drawn, setDrawn] = useState(false);
    return (
      <>
        <button
          type="button"
          onClick={() => {
            setDrawn(true);
          }}
        >
          Visibility
        </button>
        {drawn && <Held initial="Quiet morning." autoFocus late />}
      </>
    );
  }
  const user = userEvent.setup();
  render(<Page />);
  const control = screen.getByRole("button", { name: "Visibility" });

  await user.click(control);
  await screen.findByRole("textbox", { name: "Text" });
  await act(() => Promise.resolve());

  expect(document.activeElement === control).toBe(true);
});

test("it tells the caller where its caret is as the text changes", async () => {
  const user = userEvent.setup();
  const carets: (number | undefined)[] = [];
  render(<Held onCaret={(caret) => carets.push(caret)} />);

  await paste(user, "Quiet");
  await waitFor(() => {
    if (carets.at(-1) !== 5) throw new Error(`the caret is at ${String(carets.at(-1))}`);
  });
  await user.paste(" 😀");

  // UTF-16 code units, as the plain text counts them.
  await waitFor(() => {
    if (carets.at(-1) !== 8) throw new Error(`the caret is at ${String(carets.at(-1))}`);
  });
});

/** The editor with its handle, holding `initial`. */
function withHandle(initial: string, told: string[]) {
  const handle = createRef<EditorHandle>();
  render(<Held initial={initial} told={told} handle={handle as Ref<EditorHandle>} />);
  return () => {
    if (handle.current === null) throw new Error("test: the editor gave no handle");
    return handle.current;
  };
}

test("its handle inserts at the end where the editor had no selection, given the text before and after", async () => {
  const told: string[] = [];
  const handle = withHandle("Tea time", told);
  const given: [string, string][] = [];

  act(() => {
    handle().insert((before, after) => {
      given.push([before, after]);
      return " :teacup:";
    });
  });

  await waitFor(() => {
    if (told.at(-1) !== "Tea time :teacup:") throw new Error(`it holds ${String(told.at(-1))}`);
  });
  expect(given).toEqual([["Tea time", ""]]);
});

test("its handle inserts over the selection the editor had last", async () => {
  const user = userEvent.setup();
  const told: string[] = [];
  const handle = withHandle("", told);
  await paste(user, "Tea");
  const given: [string, string][] = [];

  act(() => {
    handle().write(0, 3, "Tea time", 3);
  });
  await waitFor(() => {
    if (told.at(-1) !== "Tea time") throw new Error(`it holds ${String(told.at(-1))}`);
  });
  act(() => {
    handle().insert(
      (before, after) => {
        given.push([before, after]);
        return " :teacup:";
      },
      { focus: false },
    );
  });

  await waitFor(() => {
    if (told.at(-1) !== "Tea :teacup: time") throw new Error(`it holds ${String(told.at(-1))}`);
  });
  expect(given).toEqual([["Tea", " time"]]);
});

test("its handle writes over a range of the plain text, as one step that Ctrl+Z takes back", async () => {
  const user = userEvent.setup();
  const told: string[] = [];
  const handle = withHandle("", told);
  await paste(user, "see @sa tomorrow");

  act(() => {
    handle().write(4, 7, "@sam@example.social ");
  });
  await waitFor(() => {
    if (told.at(-1) !== "see @sam@example.social  tomorrow") throw new Error(`it holds ${String(told.at(-1))}`);
  });
  await user.keyboard("{Control>}z{/Control}");

  expect(told.at(-1)).toBe("see @sa tomorrow");
});

test("a caller's plugin is drawn inside the editor and reaches it", async () => {
  const read: string[] = [];
  function Plugin() {
    const editor = useEditor();
    return (
      <button
        type="button"
        onClick={() => {
          read.push(editor.getEditorState().read($plainText));
        }}
      >
        Read
      </button>
    );
  }
  const user = userEvent.setup();
  render(
    <Held initial="Quiet morning.">
      <Plugin />
    </Held>,
  );

  await user.click(screen.getByRole("button", { name: "Read" }));

  expect(read).toEqual(["Quiet morning."]);
});

test("an editor that completes says so, and one that does not says nothing of it", () => {
  const { rerender } = render(<Held key="completes" initial="a" completes />);
  expect(box().getAttribute("aria-autocomplete")).toBe("list");

  rerender(<Held key="plain" initial="a" />);
  expect(box().getAttribute("aria-autocomplete") ?? "none").toBe("none");
});

/** The editor with its suggestions open on `active`, and what its keys asked of them, in order. */
function suggesting(active: number, around?: (editor: ReactNode) => ReactNode) {
  const asked: string[] = [];
  const told: string[] = [];
  const suggest: FieldSuggest = {
    at: 9,
    listId: "suggestions",
    list: (
      <SuggestionList
        id="suggestions"
        label="Suggestions"
        items={[
          { kind: "hashtag", name: "photography", uses: "1,204 posts" },
          { kind: "hashtag", name: "photo", uses: "310 posts" },
          { kind: "hashtag", name: "phone", uses: "12 posts" },
        ]}
        active={active}
        onChoose={none}
      />
    ),
    count: 3,
    active,
    announce: "3 suggestions",
    onActive: (index) => asked.push(`active ${String(index)}`),
    onAccept: (index) => asked.push(`accept ${String(index)}`),
    onClose: () => asked.push("close"),
  };
  const editor = <Held initial="Frost #ph" told={told} completes suggest={suggest} />;
  render(around === undefined ? editor : around(editor));
  return { asked, told };
}

// Where the list opens under the caret is measured, so on a simulation it stays hidden and has no role to find it
// by: the tests reach it from the text box that controls it.
test("with its suggestions open it stays a text box that controls their list, the active option its active descendant", () => {
  suggesting(1);

  const text = box();
  const list = document.getElementById(text.getAttribute("aria-controls") ?? "");
  expect(list?.getAttribute("role")).toBe("listbox");
  expect(list?.getAttribute("aria-label")).toBe("Suggestions");
  const active = document.getElementById(text.getAttribute("aria-activedescendant") ?? "");
  expect(active?.getAttribute("role")).toBe("option");
  expect(active?.getAttribute("aria-selected")).toBe("true");
  expect(Array.from(list?.children ?? []).indexOf(active as Element)).toBe(1);
});

test("a screen reader hears how many suggestions opened", () => {
  suggesting(0);

  expect(screen.getByRole("status").textContent).toBe("3 suggestions");
});

test("the arrow keys ask for the next and the previous option, and stop at either end", async () => {
  const user = userEvent.setup();
  const middle = suggesting(1);
  await user.click(box());
  await user.keyboard("{ArrowDown}{ArrowUp}");
  expect(middle.asked).toEqual(["active 2", "active 0"]);
});

test("at the first option ArrowUp stays on it", async () => {
  const user = userEvent.setup();
  const first = suggesting(0);
  await user.click(box());
  await user.keyboard("{ArrowUp}");
  expect(first.asked).toEqual(["active 0"]);
});

test("at the last option ArrowDown stays on it", async () => {
  const user = userEvent.setup();
  const last = suggesting(2);
  await user.click(box());
  await user.keyboard("{ArrowDown}");
  expect(last.asked).toEqual(["active 2"]);
});

test("Enter and Tab accept the active option, write nothing themselves, and leave the focus in the text", async () => {
  const user = userEvent.setup();
  const { asked, told } = suggesting(1, (editor) => (
    <>
      {editor}
      <button type="button">After</button>
    </>
  ));
  await user.click(box());

  await user.keyboard("{Enter}");
  await user.keyboard("{Tab}");

  expect(asked).toEqual(["accept 1", "accept 1"]);
  expect(told).toEqual([]);
  expect(document.activeElement === box()).toBe(true);
});

test("Escape closes the suggestions and is not heard by what holds the editor", async () => {
  const user = userEvent.setup();
  const heard: string[] = [];
  const { asked } = suggesting(0, (editor) => (
    <div
      onKeyDown={(event) => {
        if (!event.defaultPrevented) heard.push(event.key);
      }}
    >
      {editor}
    </div>
  ));
  await user.click(box());

  await user.keyboard("{Escape}");

  expect(asked).toEqual(["close"]);
  expect(heard).toEqual([]);
});

test("without suggestions the text box controls nothing and has no active descendant", () => {
  render(<Held initial="Frost" completes />);

  expect(box().hasAttribute("aria-controls")).toBe(false);
  expect(box().hasAttribute("aria-activedescendant")).toBe(false);
});
