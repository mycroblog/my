// SPDX-License-Identifier: AGPL-3.0-only

// The modals over the page (modal.tsx), every word the caller's. The confirmation asks before what cannot be undone
// (deleting a post): a small dialog in the middle of the viewport at every width, its title the question, one or two
// sentences on what follows, and the pair of Cancel and the confirmation side by side at its end, the confirmation at
// the end side; Escape and a press outside cancel. The panel shows something to read (a post's edit history) as the
// composer does: a dialog of the main column's width over the dimmed page from `tablet`, a screen of its own on a
// phone; its head holds the title and close at its end, and its body scrolls under the head. The confirmation that
// asks the reader to authenticate again (before changing the password, deleting the account) is the confirmation's
// dialog with the proof in it: what it lets them do, the field of the proof (the password, a two-factor code) or none
// where the confirmation itself proves it (a passkey, going to an external provider), another way to prove it, what
// the server refused right before the pair, and the pair.

import { useId, type ReactElement, type ReactNode } from "react";
import { Dialog, Heading } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { IconButton } from "./icon-button";
import { Box, Row, Stack } from "./layout";
import { useReader } from "./locale";
import { ModalLayer } from "./modal";

const confirm = tv({
  slots: {
    modal: "rounded-overlay bg-surface p-dialog font-sans text-body text-text shadow-overlay outline-hidden",
    dialog: "outline-hidden",
    // Wrapping between phrases, never inside a word of ja.
    title: "text-title [word-break:auto-phrase]",
    text: "text-muted-text [word-break:auto-phrase]",
  },
});

export type ConfirmDialogProps = {
  /** The question (Delete this post?). */
  title: string;
  /** What follows the confirmation, in a sentence or two. */
  children: ReactNode;
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

/** Asks before an action that cannot be undone: the question, what follows, then Cancel and the confirmation. */
export function ConfirmDialog({
  title,
  children,
  cancel,
  confirm: confirmLabel,
  onCancel,
  onConfirm,
  busy = false,
  layer = true,
}: ConfirmDialogProps) {
  const s = confirm();
  const { direction } = useReader(undefined);
  const titleId = useId();
  const textId = useId();
  const body = (
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
  /** What it asks (Confirm it's you). */
  title: string;
  /** What the proof lets the reader do, in a sentence. */
  about: ReactNode;
  /**
   * The field of the proof (a `TextField`: the password, a two-factor code); none where the confirmation itself
   * proves it (using a passkey, going to an external provider).
   */
  children?: ReactNode;
  /** Another way to prove it, under the field (a `TextButton`: use the password instead). */
  other?: ReactNode;
  /** What the server refused, right before the pair (a `FormAlert`). */
  error?: ReactNode;
  cancel: string;
  /** The confirmation's label, which says the way (Confirm, Use a passkey, Continue with GitHub). */
  confirm: string;
  onCancel: () => void;
  onConfirm: () => void;
  /** The proof in flight: the confirmation spins and the dialog stays. */
  busy?: boolean | undefined;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
};

/**
 * Asks the reader to authenticate again before something only they may do: what it lets them do, the proof, then
 * Cancel and the confirmation.
 */
export function ReauthDialog({
  title,
  about,
  children,
  other,
  error,
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
  const body = (
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
      <p id={textId} className={s.text()}>
        {about}
      </p>
      {children}
      {other !== undefined && <Row justify="start">{other}</Row>}
      {error}
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
  if (!layer)
    return (
      <Box
        anchor
        full
        max="dialog"
        role="dialog"
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
    <ModalLayer onDismiss={busy ? () => undefined : onCancel} place="center" panelClassName={s.modal()}>
      <Stack gap="dialog-gap" as={<Dialog aria-describedby={textId} className={s.dialog()} dir={direction} />}>
        {body}
      </Stack>
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
