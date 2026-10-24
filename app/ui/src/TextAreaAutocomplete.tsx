// SPDX-License-Identifier: AGPL-3.0-only
// A text area with trigger-based inline completion (ADR-0092 §3, B1): React Aria Components' Autocomplete around the
// kit's TextArea and a ListBox in a non-modal Popover, with the overrides #1152 found necessary. The textarea stays a
// native `textbox` (no combobox role); Autocomplete gives it aria-autocomplete="list", aria-controls and
// aria-activedescendant while the list is shown, and a status region announces the count, loading and no results.
// The overrides, all through React Aria Components' public API:
// - spellCheck, autoCorrect and enterKeyHint are given back on the TextField, whose props win over Autocomplete's;
// - the onKeyDown that Autocomplete passes through FieldInputContext is wrapped (KeyOverrides): IME keys
//   (isComposing, or keyCode 229, which Safari sends for the Enter that commits a conversion after compositionend),
//   Home / End / PageUp / PageDown and Ctrl / Cmd + Enter never reach it; Tab accepts the focused option; Escape
//   closes the list and stops there, so an enclosing Sheet stays open; every key it does not handle propagates;
// - the ListBox is rendered only while it has options, so the arrow keys move between lines otherwise;
// - Autocomplete swallows the textarea's keyup (a capture listener on document), so the caret is read on input,
//   selectionchange, select and compositionend;
// - the ListBox gives no aria-selected without a selection mode, so the option aria-activedescendant points at gets
//   aria-selected="true" on the DOM;
// - the ListBox is mounted again for each set of results with autoFocus="first", so the first option is chosen
//   whenever the list opens or its options change.
// The popover sits right under the caret's line (above it when the visual viewport has no room below), as wide as the
// text area. The caret's line comes from the mirror (TextAreaMirror), the layer the highlight (§4) also draws on.
// TextFieldAutocomplete is the same completion around a one-line TextField (the content warning and the display name,
// ADR-0092 §5): its mirror is the single-line one, and the popover opens under the whole input.
import {
  type ReactNode,
  use,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import {
  Autocomplete,
  AutocompleteStateContext,
  FieldInputContext,
  SelectableCollectionContext,
  ListBox,
  ListBoxItem,
  Popover,
  type TextFieldProps,
  useSlottedContext,
} from "react-aria-components";
import { tv } from "tailwind-variants";
import { type FieldLayers, TextAreaField, type TextAreaProps } from "./TextArea";
import { type TextHighlight, TextAreaMirror } from "./TextAreaMirror";
import { TextFieldControl, type TextFieldProps as KitTextFieldProps } from "./TextField";

/** The word being typed at the caret: from its trigger to the caret, [start, end) in UTF-16 units. */
export interface TextAutocompleteWord {
  readonly trigger: string;
  readonly start: number;
  readonly end: number;
  /** What follows the trigger up to the caret: `ali` of `@ali`. */
  readonly query: string;
}

/** A suggestion: its id, the text it inserts in place of the word, and its plain text (for the option's name). */
export interface TextAutocompleteSuggestion {
  readonly id: string;
  readonly value: string;
  readonly textValue: string;
}

/** How long typing must pause before suggestions are asked for (ADR-0092 §3). */
const suggestionQueryDelay = 200;

/** How long a lookup may take before the popover says it is loading (ADR-0092 §3). */
const suggestionLoadingDelay = 300;

/** The list's height cap (`max-h-60`), used to decide whether it fits under the caret's line. */
const listMaxHeight = 240;

const autocomplete = tv({
  slots: {
    // The box the popover is placed against: from the top of the text area to the bottom of the caret's line, or from
    // the top of that line to the bottom of the text area when it opens upwards. Its size changes whenever the line
    // moves, so React Aria's resize observer places the popover again.
    anchor: "pointer-events-none invisible absolute inset-x-0",
    status: "sr-only",
    popover: "flex w-(--trigger-width) flex-col overflow-hidden rounded-lg border border-border bg-surface-raised text-fg shadow-md outline-hidden",
    list: "max-h-60 min-h-0 overflow-y-auto overscroll-contain p-1 outline-hidden",
    option: [
      "flex min-h-8.5 cursor-default items-center gap-2 rounded-md px-2 py-1 text-base text-fg outline-hidden",
      "data-focused:bg-surface-sunken pointer-coarse:min-h-11",
    ],
    // Holds an option's content without a box of its own; it is there to find its option.
    content: "contents",
    message: "px-3 py-2 text-sm text-fg-muted",
  },
});

type FieldKeyboardEvent = Parameters<NonNullable<TextFieldProps["onKeyDown"]>>[0];

/** Replaces `word` with `value` and a space (unless whitespace follows), keeping one undo step when the browser can. */
function insertCompletion(textarea: HTMLTextAreaElement | HTMLInputElement, word: TextAutocompleteWord, value: string) {
  const next = textarea.value.charAt(word.end);
  const spaced = /\s/u.test(next);
  const insert = spaced ? value : `${value} `;
  textarea.focus();
  textarea.setSelectionRange(word.start, word.end);
  let inserted = false;
  try {
    // ADR-0092 §3: deprecated and non-standard, but the only way to put text into a textarea that its undo history keeps.
    // oxlint-disable-next-line typescript/no-deprecated -- ADR-0092 §3 relies on it for the undo history.
    inserted = document.execCommand("insertText", false, insert);
  } catch {
    inserted = false;
  }
  if (!inserted) {
    textarea.setRangeText(insert, word.start, word.end, "end");
    textarea.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertReplacementText", data: insert }));
  }
  // The caret goes after the space that follows the value, the one it added or the one that was there.
  if (spaced && next !== "\n" && next !== "\r") {
    const at = word.start + insert.length + 1;
    textarea.setSelectionRange(at, at);
  }
}

type KeyOverridesProps = {
  /** Whether the popover is shown (a list, or the loading or no-results message). */
  isOpen: boolean;
  /** Accepts the focused option; false when there is none. */
  onTab: () => boolean;
  onEscape: () => void;
  children: ReactNode;
};

/** Wraps the onKeyDown that Autocomplete passes to the text field (ADR-0092 §3). */
function KeyOverrides({ isOpen, onTab, onEscape, children }: KeyOverridesProps) {
  const context = useSlottedContext(FieldInputContext);
  if (context === null || context === undefined) return children;
  const forward = context.onKeyDown;
  const onKeyDown = (event: FieldKeyboardEvent) => {
    // React Aria stops the propagation of a key its handlers see unless they continue it. Only the keys the
    // completion takes stop here, so the form and an enclosing Sheet still get the rest (Esc while closed, Ctrl+Enter).
    // oxlint-disable-next-line typescript/no-deprecated -- Safari's committing Enter is told apart only by keyCode 229 (ADR-0092 §3).
    if (event.nativeEvent.isComposing || event.keyCode === 229) {
      event.continuePropagation();
      return;
    }
    if (!isOpen) {
      event.continuePropagation();
      return;
    }
    if (event.key === "Escape") {
      // Not continuing the propagation stops it, so an enclosing Sheet does not close.
      event.preventDefault();
      onEscape();
      return;
    }
    if (event.key === "Tab" && !event.shiftKey && !event.altKey && !event.ctrlKey && !event.metaKey) {
      if (onTab()) event.preventDefault();
      else event.continuePropagation();
      return;
    }
    const lineKey = event.key === "Home" || event.key === "End" || event.key === "PageUp" || event.key === "PageDown";
    const submit = event.key === "Enter" && (event.ctrlKey || event.metaKey);
    if (lineKey || submit || forward === undefined) {
      event.continuePropagation();
      return;
    }
    forward(event);
  };
  return <FieldInputContext value={{ ...context, onKeyDown }}>{children}</FieldInputContext>;
}

/**
 * The content of an option. It puts aria-selected="true" on its option while aria-activedescendant points at it, and
 * "false" otherwise (the APG combobox's listbox): React Aria's ListBox without a selection mode renders no
 * aria-selected, so React does not own the attribute. The options render after the ListBox mounts, so each sets its own.
 */
function SuggestionContent({ children }: { children: ReactNode }) {
  const focusedId = use(AutocompleteStateContext)?.focusedNodeId ?? null;
  const ref = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    const option = ref.current?.closest('[role="option"]');
    option?.setAttribute("aria-selected", String(option.id === focusedId));
  }, [focusedId]);
  return (
    <span ref={ref} className={autocomplete().content()}>
      {children}
    </span>
  );
}

/** The last answer of the suggestions function: for which word, and what it returned. */
type Results<T> = { key: string; start: number } & ({ status: "done"; items: readonly T[] } | { status: "failed" });

/** What the popover shows. */
type View = "closed" | "list" | "loading" | "empty";

type Placement = "bottom" | "top";

/** The completion's own props, the same for a text area and a text input. */
type CompletionProps<T extends TextAutocompleteSuggestion> = {
  /**
   * The trigger characters this field completes, each with the fewest characters its query needs before suggestions are
   * asked for, e.g. `{ "@": 1, "#": 1, ":": 2 }` (ADR-0092 §3). A word whose trigger is not here is not completed.
   */
  triggers: Readonly<Record<string, number>>;
  /** The word being typed at `cursor` in `text`, or null (the web passes `activeWord` of `lib/textTokens.ts`). */
  findWord: (text: string, cursor: number) => TextAutocompleteWord | null;
  /**
   * The suggestions for a word, at most a handful. Called after typing pauses for `queryDelay` ms; `signal` aborts when
   * the word changes before it answers. A rejection closes the list without a message (ADR-0092 §3).
   */
  loadSuggestions: (word: TextAutocompleteWord, signal: AbortSignal) => readonly T[] | Promise<readonly T[]>;
  /** The content of an option. */
  renderSuggestion: (suggestion: T) => ReactNode;
  /** The accessible name of the list of suggestions. */
  suggestionsLabel: string;
  /** The announcement of how many suggestions there are, e.g. "5 suggestions". */
  formatSuggestionCount: (count: number) => string;
  /** Shown and announced while a lookup takes longer than 300 ms. */
  loadingLabel: string;
  /** Shown and announced when there are no suggestions. */
  emptyLabel: string;
  /**
   * How long typing must pause before `loadSuggestions` is called, in ms; 0 for local data. A function gives the pause
   * of each word, so a field can ask the server after a pause and offer local data at once.
   */
  queryDelay?: number | ((word: TextAutocompleteWord) => number);
};

export type TextAreaAutocompleteProps<T extends TextAutocompleteSuggestion> = TextAreaProps & CompletionProps<T>;

export type TextFieldAutocompleteProps<T extends TextAutocompleteSuggestion> = KitTextFieldProps & CompletionProps<T>;

/** What the field takes from the completion: the value, the props that win over Autocomplete's, and the layers. */
type CompletedProps = {
  value: string;
  onChange: (next: string) => void;
  spellCheck: "true";
  autoCorrect: "on";
  enterKeyHint: "enter";
  layers: FieldLayers;
};

/** The field the completion is around: a text area or a one-line text field, with its own props. */
type CompletedField = { kind: "area"; props: TextAreaProps } | { kind: "line"; props: KitTextFieldProps };

type TextAutocompleteProps<T extends TextAutocompleteSuggestion> = CompletionProps<T> & {
  /** The field; a text input has one line, under which the popover opens. */
  field: CompletedField;
  value: string | undefined;
  defaultValue: string | undefined;
  onChange: ((value: string) => void) | undefined;
  highlights: readonly TextHighlight[] | undefined;
};

/**
 * A TextArea that completes the word being typed after a trigger character (`@`, `#`, `:`), ADR-0092 §3.
 * It takes every prop of TextArea (label, character count, end action, value / onChange) and the completion's
 * own: the triggers, how to find the word, the suggestions function, how to render an option, and its strings.
 */
export function TextAreaAutocomplete<T extends TextAutocompleteSuggestion>(props: TextAreaAutocompleteProps<T>) {
  const { completion, value, defaultValue, onChange, highlights, field } = splitProps(props);
  return (
    <TextAutocomplete
      {...completion}
      // The rest of a generic union loses its branches; the character count's props are still paired as TextAreaProps pairs them.
      field={{ kind: "area", props: field as TextAreaProps }}
      value={value}
      defaultValue={defaultValue}
      onChange={onChange}
      highlights={highlights}
    />
  );
}

/**
 * A TextField (one line) that completes the word being typed after a trigger character, as TextAreaAutocomplete does
 * (ADR-0092 §3, §5: the content warning and the display name). The popover opens under the input.
 */
export function TextFieldAutocomplete<T extends TextAutocompleteSuggestion>(props: TextFieldAutocompleteProps<T>) {
  const { completion, value, defaultValue, onChange, highlights, field } = splitProps(props);
  return (
    <TextAutocomplete
      {...completion}
      field={{ kind: "line", props: field }}
      value={value}
      defaultValue={defaultValue}
      onChange={onChange}
      highlights={highlights}
    />
  );
}

/**
 * The end action (the emoji picker's button) outside the completion's contexts. React context reaches through the
 * picker's portal, so without this its search field would take Autocomplete's input props and its menus and grid the
 * collection props meant for the list of suggestions.
 */
function isolated(endAction: ReactNode): ReactNode {
  if (endAction === undefined) return undefined;
  return (
    <AutocompleteStateContext value={null}>
      <FieldInputContext value={null}>
        <SelectableCollectionContext value={null}>{endAction}</SelectableCollectionContext>
      </FieldInputContext>
    </AutocompleteStateContext>
  );
}

/** Splits the props of a field with completion into the completion's, the value's and the field's own. */
function splitProps<T extends TextAutocompleteSuggestion, F extends { value?: string | undefined; defaultValue?: string | undefined; onChange?: ((value: string) => void) | undefined; highlights?: readonly TextHighlight[] | undefined }>({
  triggers,
  findWord,
  loadSuggestions,
  renderSuggestion,
  suggestionsLabel,
  formatSuggestionCount,
  loadingLabel,
  emptyLabel,
  queryDelay,
  value,
  defaultValue,
  onChange,
  highlights,
  ...field
}: F & CompletionProps<T>) {
  const completion: CompletionProps<T> = {
    triggers,
    findWord,
    loadSuggestions,
    renderSuggestion,
    suggestionsLabel,
    formatSuggestionCount,
    loadingLabel,
    emptyLabel,
    ...(queryDelay === undefined ? {} : { queryDelay }),
  };
  return { completion, value, defaultValue, onChange, highlights, field };
}

/** The completion around a text area or a text input (`field`). */
function TextAutocomplete<T extends TextAutocompleteSuggestion>({
  field,
  triggers,
  findWord,
  loadSuggestions,
  renderSuggestion,
  suggestionsLabel,
  formatSuggestionCount,
  loadingLabel,
  emptyLabel,
  queryDelay = suggestionQueryDelay,
  highlights,
  value,
  defaultValue,
  onChange,
}: TextAutocompleteProps<T>) {
  const singleLine = field.kind === "line";
  const [uncontrolledValue, setUncontrolledValue] = useState(defaultValue ?? "");
  const text = value ?? uncontrolledValue;
  // The caret while the selection is collapsed, else null.
  const [caret, setCaret] = useState<number | null>(null);
  const [focused, setFocused] = useState(false);
  const [composing, setComposing] = useState(false);
  // The word the user closed the list on with Esc (or a scroll closed it on), until the text changes.
  const [dismissed, setDismissed] = useState<{ text: string; start: number } | null>(null);
  const [results, setResults] = useState<Results<T> | null>(null);
  const [slowKey, setSlowKey] = useState<string | null>(null);
  const [placement, setPlacement] = useState<Placement>("bottom");
  const inputRef = useRef<HTMLTextAreaElement | HTMLInputElement>(null);
  const caretRef = useRef<HTMLSpanElement>(null);
  const anchorRef = useRef<HTMLDivElement>(null);
  const popoverRef = useRef<HTMLElement>(null);
  const listBoxRef = useRef<HTMLDivElement>(null);
  const styles = autocomplete();

  const found = caret === null || composing || !focused ? null : findWord(text, caret);
  const minimum = found === null ? undefined : triggers[found.trigger];
  const isDismissed = found !== null && dismissed !== null && dismissed.text === text && dismissed.start === found.start;
  const word = found !== null && minimum !== undefined && found.query.length >= minimum && !isDismissed ? found : null;
  const key = word === null ? null : `${word.trigger}${word.query}`;

  // The results of this word, or of an earlier state of it (the same start) while the new ones load.
  const current = word !== null && results !== null && (results.key === key || results.start === word.start) ? results : null;
  const items = current?.status === "done" ? current.items : [];
  let view: View = "closed";
  if (word !== null && current?.status !== "failed") {
    if (items.length > 0) view = "list";
    else if (slowKey === key) view = "loading";
    else if (current !== null) view = "empty";
  }
  const resultKey = view === "list" && current !== null ? `${current.key}@${String(current.start)}` : null;

  const load = useEffectEvent((word: TextAutocompleteWord, signal: AbortSignal) => loadSuggestions(word, signal));
  const wordTrigger = word?.trigger;
  const wordStart = word?.start;
  const wordEnd = word?.end;
  const wordQuery = word?.query;
  const delay = word === null ? 0 : typeof queryDelay === "number" ? queryDelay : queryDelay(word);
  useEffect(() => {
    if (key === null || wordTrigger === undefined || wordStart === undefined || wordEnd === undefined || wordQuery === undefined) return;
    const lookup: TextAutocompleteWord = { trigger: wordTrigger, start: wordStart, end: wordEnd, query: wordQuery };
    const controller = new AbortController();
    let slowTimer: ReturnType<typeof setTimeout> | undefined;
    const timer = setTimeout(() => {
      slowTimer = setTimeout(() => {
        setSlowKey(key);
      }, suggestionLoadingDelay);
      const settle = (next: Results<T>) => {
        clearTimeout(slowTimer);
        if (controller.signal.aborted) return;
        setResults(next);
        setSlowKey(null);
      };
      Promise.resolve()
        .then(() => load(lookup, controller.signal))
        .then(
          (found) => {
            settle({ key, start: wordStart, status: "done", items: found });
          },
          () => {
            settle({ key, start: wordStart, status: "failed" });
          },
        );
    }, delay);
    return () => {
      clearTimeout(timer);
      clearTimeout(slowTimer);
      controller.abort();
    };
  }, [key, wordTrigger, wordStart, wordEnd, wordQuery, delay]);

  const readCaret = () => {
    const textarea = inputRef.current;
    if (textarea === null) return;
    setCaret(textarea.selectionStart === textarea.selectionEnd ? textarea.selectionStart : null);
  };

  // A click or an arrow key moves the caret without an input event; selectionchange tells about both.
  useEffect(() => {
    const onSelectionChange = () => {
      const textarea = inputRef.current;
      if (textarea === null || textarea.ownerDocument.activeElement !== textarea) return;
      setCaret(textarea.selectionStart === textarea.selectionEnd ? textarea.selectionStart : null);
    };
    document.addEventListener("selectionchange", onSelectionChange);
    return () => {
      document.removeEventListener("selectionchange", onSelectionChange);
    };
  }, []);

  /** Puts the anchor around the caret's line and chooses the side of the line the popover opens on. */
  const placeAnchor = () => {
    const textarea = inputRef.current;
    const marker = caretRef.current;
    const anchor = anchorRef.current;
    if (textarea === null || marker === null || anchor === null) return;
    const box = textarea.getBoundingClientRect();
    const viewport = window.visualViewport;
    const viewTop = viewport?.offsetTop ?? 0;
    const viewBottom = viewTop + (viewport?.height ?? window.innerHeight);
    const needed = Math.min(popoverRef.current?.scrollHeight ?? listMaxHeight, listMaxHeight);
    if (singleLine) {
      // An input has one line, the whole box: the popover opens under it, or above it without room below.
      const side: Placement = viewBottom - box.bottom >= needed || viewBottom - box.bottom >= box.top - viewTop ? "bottom" : "top";
      anchor.style.top = "0px";
      anchor.style.height = `${String(box.height)}px`;
      setPlacement(side);
      return;
    }
    const mark = marker.getBoundingClientRect();
    const style = getComputedStyle(textarea);
    const lineHeight = Number.parseFloat(style.lineHeight) || mark.height;
    // Every line of the text area is one line height tall, so the caret's line is the one of that grid the marker sits
    // in; the grid is exact where the marker's own box (the font's ascent and descent) need not be centred in its line.
    const contentTop = box.top + Number.parseFloat(style.borderTopWidth) + Number.parseFloat(style.paddingTop) - textarea.scrollTop;
    const line = Math.max(0, Math.floor(((mark.top + mark.bottom) / 2 - contentTop) / lineHeight));
    // A caret line scrolled out of the text area is taken to be at its edge.
    const lineTop = Math.min(Math.max(contentTop + line * lineHeight, box.top), box.bottom);
    const lineBottom = Math.min(Math.max(contentTop + (line + 1) * lineHeight, box.top), box.bottom);
    const below = viewBottom - lineBottom;
    const above = lineTop - viewTop;
    const side: Placement = below >= needed || below >= above ? "bottom" : "top";
    if (side === "bottom") {
      anchor.style.top = "0px";
      anchor.style.height = `${String(lineBottom - box.top)}px`;
    } else {
      anchor.style.top = `${String(lineTop - box.top)}px`;
      anchor.style.height = `${String(box.bottom - lineTop)}px`;
    }
    setPlacement(side);
  };

  const onLayout = useEffectEvent(placeAnchor);
  useLayoutEffect(() => {
    onLayout();
  });

  // The visual viewport shrinks under a software keyboard and moves with scrolling; the side may change.
  useEffect(() => {
    const viewport = window.visualViewport;
    const onChange = () => {
      onLayout();
    };
    viewport?.addEventListener("resize", onChange);
    viewport?.addEventListener("scroll", onChange);
    window.addEventListener("scroll", onChange, { capture: true, passive: true });
    return () => {
      viewport?.removeEventListener("resize", onChange);
      viewport?.removeEventListener("scroll", onChange);
      window.removeEventListener("scroll", onChange, { capture: true });
    };
  }, []);

  const handleChange = (next: string) => {
    if (value === undefined) setUncontrolledValue(next);
    setDismissed(null);
    readCaret();
    onChange?.(next);
  };

  const dismiss = () => {
    if (word !== null) setDismissed({ text, start: word.start });
  };

  const accept = (suggestion: T) => {
    const textarea = inputRef.current;
    const cursor = textarea?.selectionStart ?? null;
    if (textarea === null || cursor === null || cursor !== textarea.selectionEnd) return;
    const at = findWord(textarea.value, cursor);
    if (at === null) return;
    insertCompletion(textarea, at, suggestion.value);
    readCaret();
  };

  const acceptFocused = () => {
    // The option with virtual focus is the one Enter would take; aria-activedescendant follows it after a delay.
    const option = listBoxRef.current?.querySelector<HTMLElement>('[role="option"][data-focused]');
    const id = option?.dataset["key"];
    const suggestion = items.find((item) => item.id === id);
    if (suggestion === undefined) return false;
    accept(suggestion);
    return true;
  };

  const status = view === "list" ? formatSuggestionCount(items.length) : view === "loading" ? loadingLabel : view === "empty" ? emptyLabel : "";

  const completedProps: CompletedProps = {
    value: text,
    onChange: handleChange,
    // Autocomplete turns these off for a search field; a post keeps spell checking, autocorrect and a return key.
    spellCheck: "true",
    autoCorrect: "on",
    enterKeyHint: "enter",
    layers: {
      inputRef,
      inputProps: {
        dir: "auto",
        onSelect: readCaret,
        onFocus: () => {
          setFocused(true);
          readCaret();
        },
        // React Aria's virtual focus sends the field a blur of its own when an option takes virtual focus;
        // focus has left only when the field is no longer the active element.
        onBlur: (event) => {
          setFocused(event.currentTarget.ownerDocument.activeElement === event.currentTarget);
        },
        onCompositionStart: () => {
          setComposing(true);
        },
        onCompositionEnd: () => {
          setComposing(false);
          readCaret();
        },
      },
      behind: (
        <TextAreaMirror
          textareaRef={inputRef}
          text={text}
          caret={caret ?? text.length}
          caretRef={caretRef}
          highlights={highlights}
          singleLine={singleLine}
          onSync={placeAnchor}
        />
      ),
      after: (
        <>
          <div ref={anchorRef} aria-hidden className={styles.anchor()} />
          <span role="status" className={styles.status()}>
            {status}
          </span>
        </>
      ),
    },
  };

  return (
    <Autocomplete inputValue={text}>
      <KeyOverrides isOpen={view !== "closed"} onTab={acceptFocused} onEscape={dismiss}>
        {field.kind === "area" ? (
          <TextAreaField {...field.props} endAction={isolated(field.props.endAction)} {...completedProps} />
        ) : (
          <TextFieldControl {...field.props} endAction={isolated(field.props.endAction)} {...completedProps} />
        )}
      </KeyOverrides>
      {view !== "closed" && (
        <Popover
          ref={popoverRef}
          isOpen
          isNonModal
          triggerRef={anchorRef}
          placement={placement}
          shouldFlip={false}
          offset={0}
          // The popover is exactly as wide as the text area and starts at its edge, so it never needs pushing inwards.
          containerPadding={0}
          onOpenChange={(isOpen) => {
            if (!isOpen) dismiss();
          }}
          className={styles.popover()}
        >
          {view === "list" ? (
            <ListBox
              key={resultKey}
              ref={listBoxRef}
              aria-label={suggestionsLabel}
              items={items}
              autoFocus="first"
              onAction={(id) => {
                const suggestion = items.find((item) => item.id === id);
                if (suggestion !== undefined) accept(suggestion);
              }}
              className={styles.list()}
            >
              {(suggestion) => (
                <ListBoxItem id={suggestion.id} textValue={suggestion.textValue} className={styles.option()}>
                  <SuggestionContent>{renderSuggestion(suggestion)}</SuggestionContent>
                </ListBoxItem>
              )}
            </ListBox>
          ) : (
            <p className={styles.message()}>{view === "loading" ? loadingLabel : emptyLabel}</p>
          )}
        </Popover>
      )}
    </Autocomplete>
  );
}
