// SPDX-License-Identifier: AGPL-3.0-only

// The modals over the page (modal.tsx), every word the caller's but the name of a confirmation's close, which is the
// design system's own where the caller gives none. The confirmation asks before what cannot be undone
// (deleting a post): a small dialog in the middle of the viewport at every width, its title the question, one or two
// sentences on what follows, and the pair of Cancel and the confirmation side by side at its end, the confirmation at
// the end side; Escape and a press outside cancel. The panel shows something to read (a post's edit history) as the
// composer does: a dialog of the main column's width over the dimmed page from `tablet`, a screen of its own on a
// phone; its head holds the title and close at its end, and its body scrolls under the head. The confirmation that
// asks the reader to authenticate again (before changing the password, deleting the account) is the confirmation's
// dialog with the proof in it: what the confirmation does where it does an act at once (deleting the account), in the
// text's color, then what the proof lets them do, the field of the proof (the password, a two-factor code) or none
// where the confirmation itself proves it (a passkey, going to an external provider), the other ways to prove it, each
// a row of its own a finger's target tall, what the server refused right before the pair, and the pair; where the
// reader has no way to prove it here, it says so and has Cancel alone. The head of every modal ends with close, after
// its title; a confirmation's close cancels, as its Cancel does.
//
// For assistive technology and the keyboard: a modal keeps the focus inside it and gives it back as it leaves, and
// the page behind is out of reach. The confirmation that asks to authenticate again is a form: the focus starts in
// the field of the proof, Enter in it confirms, what the server refused is an alert that also describes the field,
// and the focus goes back to the field for another try; while the proof is in flight nothing confirms it again.

import { Children, isValidElement, useEffect, useId, useRef, type ReactElement, type ReactNode } from "react";
import { Dialog, Heading } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { FieldContext } from "./field";
import { IconButton } from "./icon-button";
import { Box, Row, Stack } from "./layout";
import { stringsIn, useReader } from "./locale";
import { ModalLayer } from "./modal";

const confirm = tv({
  slots: {
    modal: "rounded-overlay bg-surface p-dialog font-sans text-body text-text shadow-overlay outline-hidden",
    dialog: "outline-hidden",
    // Wrapping between phrases, never inside a word of ja.
    title: "text-title [word-break:auto-phrase]",
    text: "text-muted-text [word-break:auto-phrase]",
    // What the press does, where it does an act at once: in the text's color, over the muted line of the proof.
    effect: "text-text [word-break:auto-phrase]",
    // The other ways to prove it, one under another, each a finger's target tall.
    ways: "list-none p-0",
    way: "min-h-icon-button-touch",
  },
});

/**
 * The head of a small dialog: its title, which names the dialog (the slot of React Aria's over the page, `titleId`
 * in a page's flow), and close at its end, as in every modal.
 */
function DialogHead({
  title,
  titleId,
  className,
  closeLabel,
  onClose,
  busy,
}: {
  title: string;
  /** The title's id in the flow of a page; none over the page, where React Aria ties the title. */
  titleId: string | undefined;
  className: string;
  closeLabel: string;
  onClose: () => void;
  busy: boolean;
}) {
  return (
    <Row align="start" justify="between" gap="entry-caption">
      {titleId === undefined ? (
        <Heading slot="title" className={className}>
          {title}
        </Heading>
      ) : (
        <h2 id={titleId} className={className}>
          {title}
        </h2>
      )}
      <Row as="span" inline rigid bleedEnd="icon-button-inline">
        <IconButton icon="close" label={closeLabel} onPress={onClose} isDisabled={busy} />
      </Row>
    </Row>
  );
}

export type ConfirmDialogProps = {
  /** The question (Delete this post?). */
  title: string;
  /** What follows the confirmation, in a sentence or two. */
  children: ReactNode;
  /** The close button's name, where the design system's own word (Close) does not fit; it cancels, as Cancel does. */
  closeLabel?: string | undefined;
  /** The refusal's label (Cancel). */
  cancel: string;
  /** The confirmation's label, the verb of the title (Delete). */
  confirm: string;
  onCancel: () => void;
  onConfirm: () => void;
  /** The confirmed action in flight: the confirmation spins and the dialog stays. */
  busy?: boolean | undefined;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
};

/**
 * Asks before an action that cannot be undone: the question and close, what follows, then Cancel and the
 * confirmation.
 */
export function ConfirmDialog({
  title,
  children,
  closeLabel,
  cancel,
  confirm: confirmLabel,
  onCancel,
  onConfirm,
  busy = false,
  layer = true,
}: ConfirmDialogProps) {
  const s = confirm();
  const { lang, direction } = useReader(undefined);
  const titleId = useId();
  const textId = useId();
  const body = (
    <>
      <DialogHead
        title={title}
        titleId={layer ? undefined : titleId}
        className={s.title()}
        closeLabel={closeLabel ?? stringsIn(lang).t("dialog.close")}
        onClose={onCancel}
        busy={busy}
      />
      <p id={textId} className={s.text()}>
        {children}
      </p>
      <Row wrap align="center" justify="end" gap="dialog-row">
        <Button kind="quiet" size="form" onPress={onCancel} isDisabled={busy}>
          {cancel}
        </Button>
        <Button kind="primary" size="form" busy={busy} onPress={onConfirm}>
          {confirmLabel}
        </Button>
      </Row>
    </>
  );
  // A specimen in a page stands as the modal's frame would place it: as wide as its column, a dialog's width at most.
  if (!layer)
    return (
      <Box
        anchor
        full
        max="dialog"
        role="alertdialog"
        aria-labelledby={titleId}
        aria-describedby={textId}
        className={`box-border ${s.modal()}`}
        dir={direction}
      >
        <Stack gap="dialog-gap" className={s.dialog()}>
          {body}
        </Stack>
      </Box>
    );
  return (
    // In flight, the dialog stays: Escape and a press outside cancel no more than Cancel, which is disabled.
    <ModalLayer onDismiss={busy ? () => undefined : onCancel} place="center" panelClassName={s.modal()}>
      <Stack
        gap="dialog-gap"
        as={<Dialog role="alertdialog" aria-describedby={textId} className={s.dialog()} dir={direction} />}
      >
        {body}
      </Stack>
    </ModalLayer>
  );
}

export type ReauthDialogProps = {
  /** What it asks (Confirm it's you; the act's question where the confirmation does it: Delete this account?). */
  title: string;
  /**
   * What the confirmation does once the proof holds, where it does an act at once (The account is deleted at once…),
   * in a sentence in the text's color, over `about`; none where the proof only lets the reader go on.
   */
  effect?: ReactNode;
  /** What the proof lets the reader do, in a sentence; with an `effect`, how to prove it. */
  about: ReactNode;
  /**
   * The field of the proof (a `TextField`: the password, a two-factor code), which takes the focus as the dialog
   * opens; none where the confirmation itself proves it (using a passkey, going to an external provider).
   */
  children?: ReactNode;
  /**
   * The other ways to prove it, under the field (`TextButton`s: use the password instead, continue with a provider):
   * one, or several as an array. The dialog gives each a row of its own, a finger's target tall, so that one is never
   * pressed for another.
   */
  other?: ReactNode;
  /** What the server refused, right before the pair (a `FormAlert`): read as it comes, and with the field. */
  error?: ReactNode;
  /** The close button's name; it cancels, as Cancel does. */
  closeLabel: string;
  cancel: string;
  /**
   * The confirmation's label, which says the way (Confirm, Use a passkey, Continue with GitHub) or, with an `effect`,
   * the act it does (Delete account); none where the reader has no way to prove it here, and the dialog only says so
   * (`error`): Cancel and close are then its ways out.
   */
  confirm?: string | undefined;
  onCancel: () => void;
  /** The confirmation's press, or Enter in the field of the proof; never while the proof is in flight. */
  onConfirm?: (() => void) | undefined;
  /** The proof in flight: the confirmation spins, nothing confirms again, and the dialog stays. */
  busy?: boolean | undefined;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
};

/** The field of a proof inside `root`, the first it holds. */
const proofField = (root: HTMLElement | null) => root?.querySelector<HTMLElement>("input, textarea, select") ?? null;

/**
 * Asks the reader to authenticate again before something only they may do: what it lets them do, the proof, then
 * Cancel and the confirmation. It is a form, which the confirmation sends.
 */
export function ReauthDialog({
  title,
  effect,
  about,
  children,
  other,
  error,
  closeLabel,
  cancel,
  confirm: confirmLabel,
  onCancel,
  onConfirm,
  busy = false,
  layer = true,
}: ReauthDialogProps) {
  const s = confirm();
  const { direction } = useReader(undefined);
  const titleId = useId();
  const textId = useId();
  const errorId = useId();
  const effectId = useId();
  const form = useRef<HTMLFormElement>(null);
  const refused = error !== undefined && error !== null && error !== false;
  const acts = effect !== undefined && effect !== null && effect !== false;
  // The dialog is described by what it does, then by how to prove it.
  const describedBy = acts ? `${effectId} ${textId}` : textId;
  const ways = Children.toArray(other);
  // Over the page, the focus starts in the field of the proof, where there is one: the reader came to enter it.
  useEffect(() => {
    if (layer) proofField(form.current)?.focus();
  }, [layer]);
  // A proof refused: the focus goes back to the field, for another try.
  useEffect(() => {
    if (layer && refused && !busy) proofField(form.current)?.focus();
  }, [layer, refused, busy]);
  const body = (
    <Stack
      gap="dialog-gap"
      as={
        <form
          ref={form}
          onSubmit={(e) => {
            e.preventDefault();
            if (!busy) onConfirm?.();
          }}
        />
      }
    >
      <DialogHead
        title={title}
        titleId={layer ? undefined : titleId}
        className={s.title()}
        closeLabel={closeLabel}
        onClose={onCancel}
        busy={busy}
      />
      {acts && (
        <p id={effectId} className={s.effect()}>
          {effect}
        </p>
      )}
      <p id={textId} className={s.text()}>
        {about}
      </p>
      {/* What the server refused describes the field of the proof. */}
      <FieldContext value={refused ? { describedBy: errorId } : {}}>{children}</FieldContext>
      {ways.length > 0 && (
        // Each way a row of its own, a finger's target tall: a press meant for one never lands on the next.
        <Stack as="ul" className={s.ways()}>
          {ways.map((way, i) => (
            <Row
              as="li"
              key={isValidElement(way) && way.key !== null ? way.key : i}
              align="center"
              justify="start"
              className={s.way()}
            >
              {way}
            </Row>
          ))}
        </Stack>
      )}
      {refused && <div id={errorId}>{error}</div>}
      <Row wrap align="center" justify="end" gap="dialog-row">
        <Button kind="quiet" size="form" onPress={onCancel} isDisabled={busy}>
          {cancel}
        </Button>
        {confirmLabel !== undefined && (
          <Button kind="primary" size="form" busy={busy} type="submit">
            {confirmLabel}
          </Button>
        )}
      </Row>
    </Stack>
  );
  if (!layer)
    return (
      <Box
        anchor
        full
        max="dialog"
        role="dialog"
        aria-labelledby={titleId}
        aria-describedby={describedBy}
        className={`box-border ${s.modal()} ${s.dialog()}`}
        dir={direction}
      >
        {body}
      </Box>
    );
  return (
    <ModalLayer onDismiss={busy ? () => undefined : onCancel} place="center" panelClassName={s.modal()}>
      <Dialog aria-describedby={describedBy} className={s.dialog()} dir={direction}>
        {body}
      </Dialog>
    </ModalLayer>
  );
}

const panel = tv({ slots: { title: "text-title" } });

/**
 * The frame of a panel: a column, its head over its body. Under `tablet` it is a screen of its own, as tall as the
 * viewport, its body scrolling under its head; from `tablet` a dialog of the main column's width on its own surface,
 * as tall as what it holds, the modal's overlay scrolling it. A `specimen` in a page is as tall as what it holds at
 * every width. One of the frames of a page: it reads the viewport.
 */
function PanelFrame({
  as,
  specimen,
  head,
  children,
}: {
  /** The dialog's element. */
  as: ReactElement<{ className?: unknown }>;
  specimen: boolean;
  head: ReactNode;
  children: ReactNode;
}) {
  const frame = [
    "relative box-border flex h-dvh flex-col bg-surface font-sans text-body text-text outline-hidden",
    "tablet:mx-auto tablet:h-auto tablet:w-[min(var(--shell-column),100%)] tablet:rounded-overlay",
    "tablet:shadow-overlay",
  ].join(" ");
  return (
    <Box as={as} className={specimen ? `${frame} h-auto` : frame}>
      <div className="flex items-center justify-between gap-entry-caption px-shell-edge pt-entry-line pb-entry tablet:px-dialog">
        {head}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-shell-edge pb-entry tablet:overflow-visible tablet:px-dialog tablet:pb-dialog">
        {children}
      </div>
    </Box>
  );
}

export type PanelDialogProps = {
  title: string;
  /** The close button's name. */
  closeLabel: string;
  /** Close, Escape or a press outside. */
  onClose: () => void;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
  children: ReactNode;
};

/** Something to read over the page: its title and close at the end of its head, then what it shows, which scrolls. */
export function PanelDialog({ title, closeLabel, onClose, layer = true, children }: PanelDialogProps) {
  const s = panel();
  const { direction } = useReader(undefined);
  const titleId = useId();
  const head = (
    <>
      {layer ? (
        <Heading slot="title" className={s.title()}>
          {title}
        </Heading>
      ) : (
        <h2 id={titleId} className={s.title()}>
          {title}
        </h2>
      )}
      <Row as="span" inline bleedEnd="icon-button-inline">
        <IconButton icon="close" label={closeLabel} onPress={onClose} />
      </Row>
    </>
  );
  if (!layer)
    return (
      <PanelFrame specimen head={head} as={<div role="dialog" aria-labelledby={titleId} dir={direction} />}>
        {children}
      </PanelFrame>
    );
  return (
    <ModalLayer onDismiss={onClose} backdrop="tablet" place="screen">
      <PanelFrame specimen={false} head={head} as={<Dialog dir={direction} />}>
        {children}
      </PanelFrame>
    </ModalLayer>
  );
}
