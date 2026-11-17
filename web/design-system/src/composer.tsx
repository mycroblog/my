// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the post composer (ADR-0025 §4), from the rough the decider approved (#3046): the frame, a dialog over
// the dimmed page from `tablet` and a screen of its own on a phone (where the app asks for one), whose row with the
// action stays right above the software keyboard; the text with its count, and the content warning, each around its
// editor (editor.tsx, loaded lazily); the replied or quoted post above the text; the images as rows of a thumbnail and its
// description; the poll as a raised card whose options are headed by the mark they will be answered with, its duration
// a select that opens the one menu (menu.tsx); and the notice of a discarded post
// with its Undo. Its head may hold more before close, and a line over the row may say how the post is
// sent (when it is scheduled for); with neither tools nor an action (the drafts), it has no row. The frame tells how it is left: its close button and Escape (`onClose`, meant), and a
// press on the backdrop (`onDismiss`, by accident), and neither while what was written is sent (`busy`). Every word is
// the caller's.
//
// On a phone the frame is as tall as the viewport less the keyboard: `--keyboard-inset` where the app measures it
// (Safari's visualViewport), else `env(keyboard-inset-height)` where the browser overlays the keyboard; Chrome on Android
// resizes the viewport itself (`interactive-widget=resizes-content`).

import { Suspense, useId, useRef, type ReactNode } from "react";
import { Button as AriaButton, Dialog, Heading } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { FieldError } from "./form-page";
import { IconButton } from "./icon-button";
import { Icon, type IconName } from "./icons";
import { useReader } from "./locale";
import { MenuGroup, MenuItem, MenuSurface } from "./menu";
import { ModalLayer } from "./modal";
import { PollMark } from "./poll";

const frame = tv({
  slots: {
    panel: [
      "relative box-border flex flex-col gap-entry bg-surface font-sans text-body text-text outline-hidden",
      "h-[calc(100dvh-var(--keyboard-inset,env(keyboard-inset-height,0px)))] px-shell-edge pt-entry-line",
    ],
    head: "flex items-center justify-between gap-entry-caption -me-icon-button-inline",
    title: "m-0 min-w-0 text-title",
    headEnd: "inline-flex flex-none items-center",
    body: "flex min-h-0 flex-1 flex-col gap-entry overflow-y-auto",
    end: "relative grid gap-entry-line bg-surface py-entry",
    row: "flex items-center gap-entry-caption",
    // On the narrowest phones (under 360 px) the glyphs sit closer, and the language names itself by its tag
    // (ToolText), so that the tools, the count and Post keep one line.
    tools: [
      "-ms-icon-button-inline flex min-w-0 flex-1 flex-wrap items-center",
      "max-[22.5rem]:[--icon-button-padding-inline:0.25rem]",
    ],
  },
  variants: {
    // From `tablet` a dialog of the main column's width over the dimmed page, which scrolls with what opens below it;
    // the page behind keeps its place.
    dialog: {
      true: {
        panel: [
          "tablet:mx-auto tablet:h-auto tablet:w-[min(var(--shell-column),100%)] tablet:rounded-overlay tablet:p-dialog",
          "tablet:pt-[calc(var(--dialog-padding)*0.6)] tablet:shadow-overlay",
        ],
        body: "tablet:overflow-visible",
        end: "tablet:py-0",
      },
    },
    // A specimen in a page is as tall as what it holds, with a form's room unless it fits what it holds (`fit`).
    layer: { false: { panel: "h-auto" } },
    room: { true: { panel: "min-h-[24rem]" } },
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
  /**
   * What was written is being sent or saved: close stays in its place, disabled (the action's spinner says it is sent),
   * and neither it, Escape nor the backdrop leaves.
   */
  busy?: boolean | undefined;
  /** How it shows on a phone: a screen of its own (the composer), or a dialog as from `tablet`. */
  phone?: "screen" | "dialog" | undefined;
  /** Over the page, as the app opens it, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
  /**
   * As tall as what it holds, also as a specimen, where a frame otherwise keeps a form's room (one line: the form that
   * could not be loaded). Over the page from `tablet` it is always as tall as what it holds.
   */
  fit?: boolean | undefined;
  /** What stands in the head before close (the drafts). */
  headEnd?: ReactNode;
  /** A line about how it will be sent (when it is scheduled for), right above the row that sends it. */
  note?: ReactNode;
  /** The form's error, directly before the row that sends it. */
  alert?: ReactNode;
  /** The tools at the row's start: the glyphs and what they open; with no tools and no action, there is no row. */
  tools?: ReactNode;
  /** What stands before the action: the count. */
  count?: ReactNode;
  /** The action that sends it (Post), and what stands at its end. */
  action?: ReactNode;
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
  busy = false,
  phone = "screen",
  layer = true,
  fit = false,
  headEnd,
  note,
  alert,
  tools,
  count,
  action,
  children,
}: ComposerFrameProps) {
  const s = frame({ dialog: true, layer, room: !layer && !fit });
  const leave = (how: () => void) => {
    if (!busy) how();
  };
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
          leave(onClose);
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
        <span className={s.headEnd()}>
          {headEnd}
          <IconButton
            icon="close"
            label={closeLabel}
            isDisabled={busy}
            onPress={() => {
              leave(onClose);
            }}
          />
        </span>
      </div>
      <div className={s.body()}>{children}</div>
      {(tools !== undefined || action !== undefined) && (
        <div className={s.end()}>
          {note}
          {alert}
          <div className={s.row()}>
            <div className={s.tools()}>{tools}</div>
            {count}
            {action}
          </div>
        </div>
      )}
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
      onDismiss={() => {
        leave(onDismiss);
      }}
      ownEscape
      backdrop={phone === "dialog" ? "always" : "tablet"}
      place="screen"
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
    wrap: "relative grid",
    // The button at the end of the text's first line (the emoji picker's, ADR-0091 §4), over the text's own room.
    end: "absolute top-0 end-0 z-10",
  },
});

/** The room the text takes while its editor arrives, so that the composer does not jump when it comes. */
const waiting = tv({
  base: "min-h-[calc(var(--composer-text-leading)*var(--composer-text-size))]",
  variants: {
    roomy: { true: "min-h-[calc(var(--composer-text-lines)*var(--composer-text-leading)*var(--composer-text-size))]" },
    line: { true: "h-control-form min-h-0 min-w-0 flex-1" },
  },
});

/**
 * The text of a post on the composer's surface: its editor (`children`, the `PlainTextEditor` of
 * `@my/design-system/editor` with the composer's look, loaded lazily: its room is held while it arrives), a button at
 * the end of its first line, and what is wrong with it right under it, by `errorId`, which the editor is described by.
 */
export function ComposerText({
  error,
  errorId,
  roomy = true,
  end,
  children,
}: {
  error?: string | undefined;
  errorId?: string | undefined;
  /** Three lines of room while nothing follows the text, while its editor arrives. */
  roomy?: boolean | undefined;
  /** A button at the end of the text's first line (the emoji picker's), which the editor keeps clear of (`ended`). */
  end?: ReactNode;
  children: ReactNode;
}) {
  const s = text();
  return (
    <div className={s.field()}>
      <div className={s.wrap()}>
        <Suspense fallback={<div className={waiting({ roomy })} aria-busy="true" />}>{children}</Suspense>
        {end !== undefined && <div className={s.end()}>{end}</div>}
      </div>
      {error && <FieldError id={errorId}>{error}</FieldError>}
    </div>
  );
}

const warning = tv({
  slots: {
    // The suggestions of its editor open under the band, across its width.
    // As tall as its editor's line also while it has none (an editor that could not be loaded).
    root: [
      "relative flex min-h-control-form items-center gap-disclosure-gap rounded-control bg-surface-raised",
      "ps-disclosure-start",
    ],
    glyph: "inline-flex text-muted-text",
  },
});

/**
 * The content warning: a band of the raised surface, as a post shows it, to type the warning on, with its editor
 * (`children`, the `PlainTextEditor` with the `line` look, loaded lazily: its room is held while it arrives), and what
 * is wrong with it right under it, by `errorId`, which the editor is described by.
 */
export function WarningField({
  error,
  errorId,
  children,
}: {
  error?: string | undefined;
  errorId?: string | undefined;
  children: ReactNode;
}) {
  const s = warning();
  return (
    <div className={text().field()}>
      <div className={s.root()}>
        <span className={s.glyph()}>
          <Icon name="hidden" place="caption" />
        </span>
        <Suspense fallback={<div className={waiting({ line: true })} aria-busy="true" />}>{children}</Suspense>
      </div>
      {error && <FieldError id={errorId}>{error}</FieldError>}
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
  /** What the server refused of the description, in its place of `missing`. */
  error?: string | undefined;
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
  missing: missingNote,
  error,
  removeLabel,
  onRemove,
}: AttachmentRowProps) {
  const s = attachment({ waiting: state === "uploading" || state === "processing" });
  const id = useId();
  const missing = error ? { text: error, required: true } : missingNote;
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
    input: [
      "h-control-form min-w-0 flex-1 rounded-control border-0 bg-transparent p-0 font-sans text-body text-text",
      "outline-hidden placeholder:text-muted-text focus-visible:shadow-focus",
    ],
    add: "-ms-control-timeline-inline justify-self-start",
    glyphLabel: "inline-flex items-center gap-entry-caption",
    settings: "grid justify-items-start gap-entry-line",
  },
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
  error,
}: {
  value: string;
  onChange: (value: string) => void;
  label: string;
  /** Several may be chosen: a checkbox's square, else a radio's ring. */
  multiple: boolean;
  maxLength: number;
  remove?: { label: string; onPress: () => void } | undefined;
  /** What is wrong with it, right under it. */
  error?: string | undefined;
}) {
  const s = poll();
  const errorId = useId();
  return (
    <>
      <div className={s.option()}>
        {/* How the option will be answered in the post (poll.tsx). */}
        <PollMark multiple={multiple} />
        <input
          className={s.input()}
          value={value}
          placeholder={label}
          aria-label={label}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? errorId : undefined}
          maxLength={maxLength}
          dir="auto"
          onChange={(e) => {
            onChange(e.target.value);
          }}
        />
        {remove && <IconButton icon="close" label={remove.label} onPress={remove.onPress} />}
      </div>
      {error && <FieldError id={errorId}>{error}</FieldError>}
    </>
  );
}

/** The settings of a poll under its options: how long it runs, and how it counts. */
export function PollSettings({ children }: { children: ReactNode }) {
  return <div className={poll().settings()}>{children}</div>;
}

/** A value a `ChoiceSelect` offers. */
export type Choice<T extends string> = {
  id: T;
  icon: IconName;
  label: string;
};

const select = tv({
  base: [
    "inline-flex h-control-timeline cursor-pointer items-center gap-entry-caption rounded-control border-0 bg-surface",
    "px-control-timeline-inline font-sans text-body text-text outline-hidden [&>svg]:text-muted-text",
    "data-hovered:bg-surface-hover data-pressed:bg-surface-pressed data-focus-visible:shadow-focus",
  ],
});

/**
 * A control that opens a menu of choices, as a select reads (a poll's duration): on the fields' surface, its glyph,
 * what it holds now and a chevron; the menu opens under it, the chosen one checked, and a choice closes it.
 */
export function ChoiceSelect<T extends string>({
  label,
  icon,
  value,
  items,
  chosen,
  onChoose,
}: {
  /** The name of what it chooses, which names its menu. */
  label: string;
  icon: IconName;
  /** What it holds now, as it reads alone: the control's name, which says what it chooses too ("Ends in 1 day"). */
  value: string;
  items: readonly Choice<T>[];
  chosen: T;
  onChoose: (id: T) => void;
}) {
  return (
    <MenuSurface
      label={label}
      placement="bottom start"
      trigger={
        <AriaButton className={select()}>
          <Icon name={icon} place="caption" />
          <span>{value}</span>
          <Icon name="expand" place="caption" />
        </AriaButton>
      }
    >
      <MenuGroup
        chosen={chosen}
        onChoose={(id) => {
          const item = items.find((i) => i.id === id);
          if (item) onChoose(item.id);
        }}
      >
        {items.map((item) => (
          <MenuItem key={item.id} id={item.id} icon={item.icon}>
            {item.label}
          </MenuItem>
        ))}
      </MenuGroup>
    </MenuSurface>
  );
}

/**
 * A tool of the row that names what it holds in words (the language), muted at the caption's size, the trigger of its
 * menu (`MenuSurface`); on the narrowest phones (under 360 px) it names it by `short` (EN), so that the row keeps its
 * tools and Post on one line.
 */
export function ToolText({
  label,
  lang,
  short,
  children,
}: {
  /** Its words on the narrowest phones (the language's tag). */
  short?: string | undefined;
  /** Its name for assistive technology: what it is and what it holds. */
  label: string;
  /** The language of its words. */
  lang?: string | undefined;
  children: string;
}) {
  return (
    <Button kind="quiet" size="timeline">
      {/* Its name is the label alone, read in place of its words, which may be the short tag. */}
      <span className="sr-only">{label}</span>
      <span className="text-caption font-label text-muted-text" lang={lang} aria-hidden="true">
        {short === undefined ? (
          children
        ) : (
          <>
            <span className="max-[22.5rem]:hidden">{children}</span>
            <span className="hidden max-[22.5rem]:inline">{short}</span>
          </>
        )}
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
  variants: { inline: { true: "static mx-0" }, bare: { true: "pe-control-timeline-inline" } },
});

/** The words of a notice without an action, as tall as an action, so that it keeps the height of one with. */
const noticeText = tv({ base: "flex min-h-control-timeline items-center" });

const noticeAction = tv({
  base: [
    "h-control-timeline cursor-pointer rounded-control border-0 bg-transparent px-control-timeline-inline font-sans",
    "text-body font-label text-inherit outline-hidden hover:underline",
    "focus-visible:shadow-[inset_0_0_0_var(--focus-ring-width)_var(--color-surface)]",
  ],
});

/**
 * A short notice at the bottom of the page, with its action when it has one (a discarded post, with Undo): knocked out
 * and neutral, at the start on a phone, clear of the action floating at the end. `inline` keeps it in the flow (a
 * specimen).
 */
export function UndoNotice({
  action,
  onAction,
  inline = false,
  children,
}: {
  action?: string | undefined;
  onAction?: (() => void) | undefined;
  inline?: boolean | undefined;
  children: string;
}) {
  const { direction } = useReader(undefined);
  const acts = action !== undefined && onAction !== undefined;
  return (
    <div className={notice({ inline, bare: !acts })} role="status" dir={direction}>
      <span className={acts ? undefined : noticeText()}>{children}</span>
      {acts && (
        <button type="button" className={noticeAction()} onClick={onAction}>
          {action}
        </button>
      )}
    </div>
  );
}
