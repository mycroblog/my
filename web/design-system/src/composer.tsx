// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the post composer (ADR-0025 §4), from the rough the decider approved (#3046): the frame, a dialog over
// the dimmed page from `tablet` and a screen of its own on a phone (where the app asks for one), whose row with the
// action stays right above the software keyboard; the text with its count and the part over the limit marked behind
// it; the content warning; the replied or quoted post above the text; the images as rows of a thumbnail and its
// description; the poll as a raised card whose options are headed by the mark they will be answered with; the popovers
// of choices, whose groups stand apart under their captions, one opening another as a submenu; and the notice of a
// discarded post with its Undo. The frame tells how it is left: its close button and Escape (`onClose`, meant), and a
// press on the backdrop (`onDismiss`, by accident). Every word is the caller's.
//
// On a phone the frame is as tall as the viewport less the keyboard: `--keyboard-inset` where the app measures it
// (Safari's visualViewport), else `env(keyboard-inset-height)` where the browser overlays the keyboard; Chrome on Android
// resizes the viewport itself (`interactive-widget=resizes-content`).

import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { Dialog, Heading } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { FieldError } from "./form-page";
import { IconButton } from "./icon-button";
import { Icon, type IconName } from "./icons";
import { useReader } from "./locale";
import { ModalLayer } from "./modal";

const frame = tv({
  slots: {
    overlay: "fixed inset-0 z-50 box-border grid items-start overflow-y-auto",
    rise: "box-border w-full",
    panel: [
      "relative box-border flex flex-col gap-entry bg-surface font-sans text-body text-text outline-hidden",
      "h-[calc(100dvh-var(--keyboard-inset,env(keyboard-inset-height,0px)))] px-shell-edge pt-entry-line",
    ],
    head: "flex items-center justify-between gap-entry-caption -me-icon-button-inline",
    title: "m-0 text-title",
    body: "flex min-h-0 flex-1 flex-col gap-entry overflow-y-auto",
    end: "relative grid gap-entry-line bg-surface py-entry",
    row: "flex items-center gap-entry-caption",
    tools: "-ms-icon-button-inline flex min-w-0 flex-1 flex-wrap items-center",
  },
  variants: {
    // From `tablet` a dialog of the main column's width over the dimmed page, which scrolls with what opens below it;
    // the page behind keeps its place.
    dialog: {
      true: {
        overlay: [
          "tablet:justify-items-center tablet:px-dialog-margin",
          "tablet:pt-composer-top tablet:pb-composer-bottom",
          "tablet:has-[[data-popup]]:pb-[calc(var(--composer-pop-height)+var(--composer-bottom))]",
        ],
        panel: [
          "tablet:mx-auto tablet:h-auto tablet:w-[min(var(--shell-column),100%)] tablet:rounded-overlay tablet:p-dialog",
          "tablet:pt-[calc(var(--dialog-padding)*0.6)] tablet:shadow-overlay",
        ],
        body: "tablet:overflow-visible",
        end: "tablet:py-0",
      },
    },
    // A specimen in a page is as tall as what it holds.
    layer: { false: { panel: "h-auto min-h-[24rem]" } },
  },
});

export type ComposerFrameProps = {
  /** The title of what is written: a new post, a reply, a quote. */
  title: string;
  /** The close button's name. */
  closeLabel: string;
  /** Close or Escape: the reader means to leave. */
  onClose: () => void;
  /** A press on the backdrop: the reader left by accident. */
  onDismiss: () => void;
  /** How it shows on a phone: a screen of its own (the composer), or a dialog as from `tablet`. */
  phone?: "screen" | "dialog" | undefined;
  /** Over the page, as the app opens it, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
  /** The form's error, directly before the row that sends it. */
  alert?: ReactNode;
  /** The tools at the row's start: the glyphs and what they open. */
  tools: ReactNode;
  /** What stands before the action: the count. */
  count?: ReactNode;
  /** The action that sends it (Post). */
  action: ReactNode;
  children: ReactNode;
};

/**
 * The composer's frame: the head (the title, then close at its end), what is written, and at its end the form's error
 * and the row of the tools, the count and the action. It holds the focus while it is over the page.
 */
export function ComposerFrame({
  title,
  closeLabel,
  onClose,
  onDismiss,
  phone = "screen",
  layer = true,
  alert,
  tools,
  count,
  action,
  children,
}: ComposerFrameProps) {
  const s = frame({ dialog: true, layer });
  const { direction } = useReader(undefined);
  const titleId = useId();
  const panel = (
    <div
      className={phone === "dialog" ? `${s.panel()} h-auto rounded-overlay p-dialog shadow-overlay` : s.panel()}
      dir={direction}
      onKeyDown={(e) => {
        // Escape means to leave, but not while an input method composes, where it cancels the composition.
        if (e.key === "Escape" && !e.defaultPrevented && !e.nativeEvent.isComposing) {
          e.preventDefault();
          onClose();
        }
      }}
    >
      <div className={s.head()}>
        {layer ? (
          <Heading slot="title" className={s.title()}>
            {title}
          </Heading>
        ) : (
          <h2 id={titleId} className={s.title()}>
            {title}
          </h2>
        )}
        <IconButton icon="close" label={closeLabel} onPress={onClose} />
      </div>
      <div className={s.body()}>{children}</div>
      <div className={s.end()}>
        {alert}
        <div className={s.row()}>
          <div className={s.tools()}>{tools}</div>
          {count}
          {action}
        </div>
      </div>
    </div>
  );
  if (!layer)
    return (
      <div role="dialog" aria-labelledby={titleId}>
        {panel}
      </div>
    );
  // A modal (modal.tsx) that shows on the frame it mounts in, its text focused inside the press; Escape is the
  // panel's, which tells it from a press on the backdrop.
  return (
    <ModalLayer
      onDismiss={onDismiss}
      ownEscape
      backdrop={phone === "dialog" ? "always" : "tablet"}
      overlay={s.overlay()}
      panel={s.rise()}
    >
      <Dialog className="box-border w-full outline-hidden">{panel}</Dialog>
    </ModalLayer>
  );
}

const count = tv({
  base: "text-caption text-muted-text tabular-nums",
  variants: { near: { true: "text-text" }, over: { true: "font-label text-text" } },
});

/** How many characters are left before the limit (negative past it): muted until `near` it, stronger past it. */
export function CharCount({ left, near, label }: { left: number; near: number; label: string }) {
  return (
    <span className={count({ near: left >= 0 && left <= near, over: left < 0 })}>
      <span aria-hidden="true">{left}</span>
      <span className="sr-only">{label}</span>
    </span>
  );
}

const text = tv({
  slots: {
    field: "grid gap-form-page-field-gap",
    wrap: "grid [&>*]:[grid-area:1/1]",
    both: [
      "box-border m-0 border-0 p-0 font-sans text-[length:var(--composer-text-size)]",
      "leading-[var(--composer-text-leading)] whitespace-pre-wrap [overflow-wrap:anywhere]",
    ],
    area: ["resize-none bg-transparent text-text outline-hidden [field-sizing:content] placeholder:text-muted-text"],
    mirror: "pointer-events-none text-transparent",
    over: "rounded-[var(--composer-over-radius)] bg-surface-raised-pressed text-transparent",
  },
  variants: {
    // Three lines of room while nothing follows the text; with images or a poll under it, only its own lines.
    roomy: {
      true: { both: "min-h-[calc(var(--composer-text-lines)*var(--composer-text-leading)*var(--composer-text-size))]" },
      false: { both: "min-h-[calc(var(--composer-text-leading)*var(--composer-text-size))]" },
    },
  },
});

/** The text of a post: plain on the composer's surface, the characters over the limit marked behind the text. */
export function ComposerText({
  value,
  onChange,
  label,
  placeholder,
  over,
  error,
  readOnly = false,
  roomy = true,
  autoFocus = false,
}: {
  value: string;
  onChange: (value: string) => void;
  /** The field's name, for assistive technology. */
  label: string;
  placeholder: string;
  /** How many characters at its end are over the limit. */
  over: number;
  /** What is wrong with it, right under it. */
  error?: string | undefined;
  readOnly?: boolean | undefined;
  roomy?: boolean | undefined;
  autoFocus?: boolean | undefined;
}) {
  const s = text({ roomy });
  const id = useId();
  const all = Array.from(value);
  const keep = Math.max(0, all.length - over);
  return (
    <div className={s.field()}>
      <div className={s.wrap()}>
        <div className={`${s.both()} ${s.mirror()}`} aria-hidden="true">
          {all.slice(0, keep).join("")}
          {over > 0 && <mark className={s.over()}>{all.slice(keep).join("")}</mark>}
          {"​"}
        </div>
        <textarea
          className={`${s.both()} ${s.area()}`}
          value={value}
          placeholder={placeholder}
          aria-label={label}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? `${id}-error` : undefined}
          readOnly={readOnly}
          // The composer opens to write: its text takes the focus at once.
          autoFocus={autoFocus}
          dir="auto"
          onChange={(e) => {
            onChange(e.target.value);
          }}
        />
      </div>
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </div>
  );
}

const warning = tv({
  slots: {
    root: "flex items-center gap-disclosure-gap rounded-control bg-surface-raised ps-disclosure-start",
    glyph: "inline-flex text-muted-text",
    input: [
      "h-control-form min-w-0 flex-1 border-0 bg-transparent pe-control-timeline-inline font-sans text-body",
      "text-text outline-hidden placeholder:text-muted-text",
    ],
  },
});

/** The content warning: a band of the raised surface, as a post shows it, to type the warning on. */
export function WarningField({
  value,
  onChange,
  label,
  placeholder,
  readOnly = false,
}: {
  value: string;
  onChange: (value: string) => void;
  label: string;
  placeholder: string;
  readOnly?: boolean | undefined;
}) {
  const s = warning();
  return (
    <div className={s.root()}>
      <span className={s.glyph()}>
        <Icon name="hidden" place="caption" />
      </span>
      <input
        className={s.input()}
        value={value}
        placeholder={placeholder}
        aria-label={label}
        readOnly={readOnly}
        dir="auto"
        onChange={(e) => {
          onChange(e.target.value);
        }}
      />
    </div>
  );
}

const context = tv({
  slots: {
    reply: "grid gap-entry-line text-muted-text",
    // Muted, also where what people write sets its own color.
    replyText: "line-clamp-2 overflow-hidden [&_*]:text-muted-text",
    quote: "relative [&>div>:first-child]:pe-[2rem]",
    quoteRemove: "absolute end-entry-line top-[0.375rem]",
  },
});

/** The post replied to, above the text: its byline and the start of its text, muted, cut after two lines. */
export function ComposerReply({ byline, note, children }: { byline: ReactNode; note: string; children: ReactNode }) {
  const s = context();
  return (
    <div className={s.reply()}>
      {byline}
      <div className={s.replyText()}>{children}</div>
      <span className="sr-only">{note}</span>
    </div>
  );
}

/** The quoted post, above the text, as the post will carry it (an `Embed`), with its remove at its top end. */
export function ComposerQuote({
  removeLabel,
  onRemove,
  children,
}: {
  removeLabel: string;
  onRemove: () => void;
  children: ReactNode;
}) {
  const s = context();
  return (
    <div className={s.quote()}>
      {children}
      <span className={s.quoteRemove()}>
        <IconButton icon="close" label={removeLabel} onPress={onRemove} />
      </span>
    </div>
  );
}

const attachment = tv({
  slots: {
    list: "m-0 grid list-none gap-entry p-0",
    row: "grid grid-cols-[var(--composer-thumb)_minmax(0,1fr)_auto] items-start gap-entry-avatar",
    thumb: "size-composer-thumb rounded-control bg-surface-raised object-cover",
    placeholder:
      "inline-flex size-composer-thumb items-center justify-center rounded-control bg-surface-raised text-muted-text",
    field: "grid gap-entry-line",
    label: "font-label",
    description: [
      "box-border min-h-control-form w-full resize-none rounded-control border-0 bg-surface-raised py-[0.375rem]",
      "px-control-timeline-inline font-sans text-body text-text outline-hidden [field-sizing:content]",
      "placeholder:text-muted-text focus-visible:shadow-focus",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    status: "m-0 flex flex-wrap items-center gap-entry-caption pt-entry-line text-caption text-muted-text",
  },
  variants: { waiting: { true: { thumb: "opacity-disabled" } } },
});

/** The images of a post, a row each. */
export function AttachmentList({ children }: { children: ReactNode }) {
  return <ul className={attachment().list()}>{children}</ul>;
}

export type AttachmentRowProps = {
  /** The image's preview, or none for a file that has none (a video without a poster). */
  src?: string | undefined;
  /** Uploading or processing, `failed`, or `ready` for its description. */
  state: "uploading" | "processing" | "failed" | "ready";
  /** What happens to it while it is not ready: uploading, processing, or that it failed. */
  status: string;
  /** A failed upload's retry. */
  retry?: { label: string; onPress: () => void } | undefined;
  /** The description, its field's label and the hint in it. */
  description: string;
  onDescription: (value: string) => void;
  descriptionLabel: string;
  placeholder: string;
  /** A description that is missing: a caution, or an error where the server asks for every one. */
  missing?: { text: string; required: boolean } | undefined;
  removeLabel: string;
  onRemove: () => void;
};

/** An image of a post: its thumbnail beside its description, which is asked for where it is missing; remove at the end. */
export function AttachmentRow({
  src,
  state,
  status,
  retry,
  description,
  onDescription,
  descriptionLabel,
  placeholder,
  missing,
  removeLabel,
  onRemove,
}: AttachmentRowProps) {
  const s = attachment({ waiting: state === "uploading" || state === "processing" });
  const id = useId();
  return (
    <li className={s.row()}>
      {src === undefined ? (
        <span className={s.placeholder()}>
          <Icon name="attach" place="action" />
        </span>
      ) : (
        <img className={s.thumb()} src={src} alt="" />
      )}
      <div className={s.field()}>
        {state === "ready" ? (
          <>
            <label className={s.label()} htmlFor={id}>
              {descriptionLabel}
            </label>
            <textarea
              id={id}
              className={s.description()}
              rows={2}
              value={description}
              placeholder={placeholder}
              maxLength={1500}
              aria-invalid={missing?.required ? true : undefined}
              aria-describedby={missing ? `${id}-note` : undefined}
              dir="auto"
              onChange={(e) => {
                onDescription(e.target.value);
              }}
            />
            {missing &&
              (missing.required ? (
                <FieldError id={`${id}-note`}>{missing.text}</FieldError>
              ) : (
                <p id={`${id}-note`} className={s.status()}>
                  <Icon name="caution" place="caption" />
                  {missing.text}
                </p>
              ))}
          </>
        ) : (
          <p className={s.status()} role="status">
            {state === "failed" && <Icon name="error" place="caption" />}
            <span>{status}</span>
            {retry && (
              <Button kind="secondary" size="timeline" onPress={retry.onPress}>
                {retry.label}
              </Button>
            )}
          </p>
        )}
      </div>
      <IconButton icon="close" label={removeLabel} onPress={onRemove} />
    </li>
  );
}

/** The tool that adds files: an icon button that opens the system's file picker. */
export function AttachButton({
  label,
  accept,
  isDisabled = false,
  onFiles,
}: {
  label: string;
  /** The types of file it takes, as an input's accept. */
  accept: string;
  isDisabled?: boolean | undefined;
  onFiles: (files: File[]) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <>
      <IconButton
        icon="attach"
        label={label}
        isDisabled={isDisabled}
        onPress={() => {
          input.current?.click();
        }}
      />
      <input
        ref={input}
        type="file"
        accept={accept}
        multiple
        hidden
        onChange={(e) => {
          const files = Array.from(e.target.files ?? []);
          e.target.value = "";
          if (files.length > 0) onFiles(files);
        }}
      />
    </>
  );
}

const poll = tv({
  slots: {
    card: "m-0 grid min-w-0 gap-entry-line rounded-control border-0 bg-surface-raised p-embed",
    head: "-my-[0.25rem] -me-icon-button-inline flex items-center justify-between",
    caption: "inline-flex items-center gap-entry-caption text-caption text-muted-text",
    options: "grid rounded-control bg-surface py-[0.25rem] ps-control-timeline-inline pe-[0.25rem]",
    option: "flex items-center gap-entry-caption",
    // How the option will be answered in the post: a radio's ring, or a checkbox's square for several choices.
    mark: "box-border size-form-page-check flex-none rounded-full border-[length:var(--composer-mark-line)] border-solid border-muted-text",
    input: [
      "h-control-form min-w-0 flex-1 rounded-control border-0 bg-transparent p-0 font-sans text-body text-text",
      "outline-hidden placeholder:text-muted-text focus-visible:shadow-focus",
    ],
    add: "-ms-control-timeline-inline justify-self-start",
    glyphLabel: "inline-flex items-center gap-entry-caption",
    settings: "grid justify-items-start gap-entry-line",
  },
  variants: { multiple: { true: { mark: "rounded-[3px]" } } },
});

/** The poll: a raised card under its caption and its remove; its options, then its settings. */
export function PollCard({
  title,
  removeLabel,
  onRemove,
  children,
}: {
  title: string;
  removeLabel: string;
  onRemove: () => void;
  children: ReactNode;
}) {
  const s = poll();
  return (
    <fieldset className={s.card()}>
      <legend className="sr-only">{title}</legend>
      <div className={s.head()}>
        <span className={s.caption()}>
          <Icon name="poll" place="caption" />
          {title}
        </span>
        <IconButton icon="close" label={removeLabel} onPress={onRemove} />
      </div>
      {children}
    </fieldset>
  );
}

/** The options of a poll, together on the page's surface inside the card; the last line adds one. */
export function PollOptions({
  add,
  children,
}: {
  add?: { label: string; onPress: () => void } | undefined;
  children: ReactNode;
}) {
  const s = poll();
  return (
    <div className={s.options()}>
      {children}
      {add && (
        <span className={s.add()}>
          <Button kind="quiet" size="timeline" onPress={add.onPress}>
            <span className={s.glyphLabel()}>
              <Icon name="add" place="action" />
              {add.label}
            </span>
          </Button>
        </span>
      )}
    </div>
  );
}

/** An option of a poll, headed by the mark it will be answered with; its remove at its end, where it may go. */
export function PollOption({
  value,
  onChange,
  label,
  multiple,
  maxLength,
  remove,
}: {
  value: string;
  onChange: (value: string) => void;
  label: string;
  /** Several may be chosen: a checkbox's square, else a radio's ring. */
  multiple: boolean;
  maxLength: number;
  remove?: { label: string; onPress: () => void } | undefined;
}) {
  const s = poll({ multiple });
  return (
    <div className={s.option()}>
      <span className={s.mark()} aria-hidden="true" />
      <input
        className={s.input()}
        value={value}
        placeholder={label}
        aria-label={label}
        maxLength={maxLength}
        dir="auto"
        onChange={(e) => {
          onChange(e.target.value);
        }}
      />
      {remove && <IconButton icon="close" label={remove.label} onPress={remove.onPress} />}
    </div>
  );
}

/** The settings of a poll under its options: how long it runs, and how it counts. */
export function PollSettings({ children }: { children: ReactNode }) {
  return <div className={poll().settings()}>{children}</div>;
}

const popup = tv({
  slots: {
    anchor: "relative inline-flex max-tablet:static",
    panel: [
      "absolute start-0 top-[calc(100%+0.25rem)] z-5 box-border grid w-max content-start gap-menu-group-gap",
      "max-h-[min(var(--composer-pop-height),70dvh)] max-w-[min(var(--composer-pop-width),calc(100vw-2rem))]",
      "overflow-y-auto rounded-overlay bg-surface p-menu-padding shadow-overlay",
    ],
  },
  variants: {
    // In the row above the keyboard: over the row on a phone, spanning the composer; under its trigger from `tablet`,
    // into the page below, which scrolls to it. Elsewhere (in the poll): under its trigger at every width.
    place: {
      row: {
        panel: [
          "max-tablet:inset-x-0 max-tablet:top-auto max-tablet:bottom-[calc(100%+0.25rem)] max-tablet:w-auto",
          "max-tablet:max-h-[min(var(--composer-pop-height-phone),60dvh)] max-tablet:max-w-none",
        ],
      },
      below: { anchor: "max-tablet:relative" },
    },
    end: { true: { panel: "tablet:start-auto tablet:end-0" } },
  },
});

/**
 * A popover of choices under (or, in the row on a phone, over) its trigger: a press outside it or Escape closes it.
 * `end` lines it up with the trigger's end, for a trigger at the row's end.
 */
export function ChoicePopup({
  open,
  onClose,
  trigger,
  label,
  place = "row",
  end = false,
  children,
}: {
  open: boolean;
  onClose: () => void;
  trigger: ReactNode;
  /** The popover's name. */
  label: string;
  place?: "row" | "below" | undefined;
  end?: boolean | undefined;
  children: ReactNode;
}) {
  const s = popup({ place, end });
  const anchor = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (!open) return undefined;
    const away = (e: Event) => {
      if (!(e.target instanceof Node) || !anchor.current?.contains(e.target)) onClose();
    };
    const escape = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !e.isComposing) {
        e.stopPropagation();
        e.preventDefault();
        onClose();
      }
    };
    document.addEventListener("pointerdown", away);
    document.addEventListener("keydown", escape, true);
    return () => {
      document.removeEventListener("pointerdown", away);
      document.removeEventListener("keydown", escape, true);
    };
  }, [open, onClose]);
  return (
    <span ref={anchor} className={s.anchor()}>
      {trigger}
      {open && (
        <div className={s.panel()} role="dialog" aria-label={label} data-popup="">
          {children}
        </div>
      )}
    </span>
  );
}

const choice = tv({
  slots: {
    group: "grid",
    title: "m-0 px-control-timeline-inline py-menu-padding text-caption text-muted-text",
    item: [
      "flex cursor-pointer items-start gap-menu-icon-gap rounded-control border-0 bg-transparent py-[0.375rem]",
      "px-control-timeline-inline text-start font-sans text-body text-text outline-hidden",
      "enabled:hover:bg-surface-raised focus-visible:shadow-focus-inset aria-checked:bg-surface-raised",
      "disabled:cursor-not-allowed disabled:opacity-disabled",
    ],
    glyph: "inline-flex pt-[0.125rem] text-muted-text",
    text: "grid flex-1",
    label: "[[aria-checked=true]_&]:font-label",
    about: "text-caption text-muted-text",
    opens: "self-center text-title text-muted-text rtl:-scale-x-100",
  },
  variants: {
    // A group of another kind of setting (the interactions after the visibilities): a rule above its caption.
    apart: { true: { group: "border-0 border-t border-solid border-surface-raised-hover pt-menu-group-gap" } },
    back: { true: { item: "items-center", glyph: "pt-0", label: "font-label" } },
  },
});

export type Choice<T extends string> = {
  id: T;
  icon: IconName;
  label: string;
  /** A line about it, under its label. */
  about?: string | undefined;
  disabled?: boolean | undefined;
};

/** A group of choices under its caption, the chosen one marked; a note under them says why one cannot be chosen. */
export function ChoiceGroup<T extends string>({
  title,
  items,
  chosen,
  onChoose,
  note,
  apart = false,
}: {
  title: string;
  items: readonly Choice<T>[];
  chosen: T;
  onChoose: (id: T) => void;
  note?: string | undefined;
  apart?: boolean | undefined;
}) {
  const s = choice({ apart });
  return (
    <div className={s.group()} role="radiogroup" aria-label={title}>
      <p className={s.title()}>{title}</p>
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          role="radio"
          aria-checked={item.id === chosen}
          disabled={item.disabled}
          className={s.item()}
          onClick={() => {
            onChoose(item.id);
          }}
        >
          <span className={s.glyph()}>
            <Icon name={item.icon} place="action" />
          </span>
          <span className={s.text()}>
            <span className={s.label()}>{item.label}</span>
            {item.about !== undefined && <span className={s.about()}>{item.about}</span>}
          </span>
        </button>
      ))}
      {note !== undefined && <p className={`${s.title()} max-w-[16rem]`}>{note}</p>}
    </div>
  );
}

/** A group of items of another kind under its caption, apart from the groups before it by a rule. */
export function ChoiceSection({ title, children }: { title: string; children: ReactNode }) {
  const s = choice({ apart: true });
  return (
    <div className={s.group()} role="group" aria-label={title}>
      <p className={s.title()}>{title}</p>
      {children}
    </div>
  );
}

/** An item that opens another panel of the popover (a submenu): a glyph, its label, what it holds now, and ›. */
export function ChoiceOpens({
  icon,
  label,
  now,
  onPress,
}: {
  icon: IconName;
  label: string;
  now: string;
  onPress: () => void;
}) {
  const s = choice();
  return (
    <button type="button" className={s.item()} onClick={onPress}>
      <span className={s.glyph()}>
        <Icon name={icon} place="action" />
      </span>
      <span className={s.text()}>
        <span className={s.label()}>{label}</span>
        <span className={s.about()}>{now}</span>
      </span>
      <span className={s.opens()} aria-hidden="true">
        ›
      </span>
    </button>
  );
}

/** The head of a submenu: back to the panel it opened from, under the submenu's name. */
export function ChoiceBack({ label, onPress }: { label: string; onPress: () => void }) {
  const s = choice({ back: true });
  return (
    <button type="button" className={s.item()} onClick={onPress}>
      <span className={s.glyph()}>
        <Icon name="back" place="action" />
      </span>
      <span className={s.label()}>{label}</span>
    </button>
  );
}

const select = tv({
  base: [
    "inline-flex h-control-timeline cursor-pointer items-center gap-entry-caption rounded-control border-0 bg-surface",
    "px-control-timeline-inline font-sans text-body text-text outline-hidden [&>svg]:text-muted-text",
    "hover:bg-surface-hover active:bg-surface-pressed focus-visible:shadow-focus",
  ],
});

/**
 * A control that opens a list of choices, as a select reads (a poll's duration): on the fields' surface, its glyph,
 * what it holds now and a chevron; the choices open under it.
 */
export function ChoiceSelect<T extends string>({
  label,
  icon,
  value,
  items,
  chosen,
  onChoose,
}: {
  /** The name of what it chooses. */
  label: string;
  icon: IconName;
  /** What it holds now, as it reads. */
  value: string;
  items: readonly Choice<T>[];
  chosen: T;
  onChoose: (id: T) => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <ChoicePopup
      open={open}
      onClose={() => {
        setOpen(false);
      }}
      label={label}
      place="below"
      trigger={
        <button
          type="button"
          className={select()}
          aria-haspopup="dialog"
          aria-expanded={open}
          onClick={() => {
            setOpen((o) => !o);
          }}
        >
          <Icon name={icon} place="caption" />
          <span>{value}</span>
          <Icon name="expand" place="caption" />
        </button>
      }
    >
      <ChoiceGroup
        title={label}
        items={items}
        chosen={chosen}
        onChoose={(id) => {
          onChoose(id);
          setOpen(false);
        }}
      />
    </ChoicePopup>
  );
}

/** A tool of the row that names what it holds in words (the language), muted at the caption's size. */
export function ToolText({
  label,
  lang,
  expanded,
  onPress,
  children,
}: {
  /** Its name for assistive technology: what it is and what it holds. */
  label: string;
  /** The language of its words. */
  lang?: string | undefined;
  expanded: boolean;
  onPress: () => void;
  children: string;
}) {
  return (
    <Button
      kind="quiet"
      size="timeline"
      aria-label={label}
      aria-haspopup="dialog"
      aria-expanded={expanded}
      onPress={onPress}
    >
      <span className="text-caption font-label text-muted-text" lang={lang}>
        {children}
      </span>
    </Button>
  );
}

const notice = tv({
  base: [
    "fixed inset-x-shell-edge bottom-[calc(var(--shell-bar)+var(--shell-edge)+env(safe-area-inset-bottom))] z-60",
    "box-border flex w-max max-w-[calc(100%-3*var(--shell-edge)-var(--shell-bar))] items-center gap-entry-avatar",
    "rounded-overlay bg-text py-[0.25rem] ps-control-timeline-inline pe-[0.25rem] font-sans text-body text-surface",
    "shadow-overlay tablet:inset-x-0 tablet:bottom-shell-end tablet:mx-auto tablet:max-w-[calc(100%-2*var(--shell-edge))]",
  ],
  variants: { inline: { true: "static mx-0" } },
});

const noticeAction = tv({
  base: [
    "h-control-timeline cursor-pointer rounded-control border-0 bg-transparent px-control-timeline-inline font-sans",
    "text-body font-label text-inherit outline-hidden hover:underline",
    "focus-visible:shadow-[inset_0_0_0_var(--focus-ring-width)_var(--color-surface)]",
  ],
});

/**
 * A short notice at the bottom of the page with its action (a discarded post, with Undo): knocked out and neutral, at
 * the start on a phone, clear of the action floating at the end. `inline` keeps it in the flow (a specimen).
 */
export function UndoNotice({
  action,
  onAction,
  inline = false,
  children,
}: {
  action: string;
  onAction: () => void;
  inline?: boolean | undefined;
  children: string;
}) {
  const { direction } = useReader(undefined);
  return (
    <div className={notice({ inline })} role="status" dir={direction}>
      <span>{children}</span>
      <button type="button" className={noticeAction()} onClick={onAction}>
        {action}
      </button>
    </div>
  );
}
