// SPDX-License-Identifier: AGPL-3.0-only

// Where the emoji picker opens, and the picker loaded when first wanted (ADR-0091 §4, §11). The picker itself
// (emoji-picker.tsx, with TanStack Virtual) is a chunk of its own, apart from the design system's entry, so that no
// screen pays for it until a reader reaches for it: `preloadEmojiPicker` fetches it from the button that opens it (its
// hover, its focus, its press) or once the page is idle (`preloadEmojiPickerWhenIdle`), and `LazyEmojiPicker` shows it,
// holding its place while it arrives. The popover sits
// beside its trigger from `tablet`, the sheet rises from the bottom on a phone; the app chooses.

import { lazy, Suspense, type ReactElement, type ReactNode, type RefObject } from "react";
import { Dialog, DialogTrigger, Heading, Popover } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import type { EmojiPickerProps } from "./emoji-picker";
import { IconButton } from "./icon-button";
import { whenIdle } from "./idle";
import { Box, Grid, Row } from "./layout";
import { useReader } from "./locale";
import { ModalLayer, ModalPresence } from "./modal";

/** The picker's module, fetched once: each call after the first gets the same promise. */
let loading: Promise<typeof import("./emoji-picker")> | undefined;
const load = () => (loading ??= import("./emoji-picker"));

/** Fetches the picker's module ahead of its first use (from the hover, the focus or the press of what opens it). */
export function preloadEmojiPicker(): void {
  load().catch(() => {
    // A failed fetch is tried again by the next.
    loading = undefined;
  });
}

/**
 * Fetches the picker's module once the page is idle, on a screen where a picker is at hand (posts, the composer), so
 * that its first opening waits only for its emoji, and with it `also` (the emoji, for a reader who opened a picker on
 * this device before); returns what cancels it.
 */
export function preloadEmojiPickerWhenIdle(also?: () => void): () => void {
  return whenIdle(() => {
    preloadEmojiPicker();
    also?.();
  });
}

const Picker = lazy(() => load().then((m) => ({ default: m.EmojiPicker })));

const overlay = tv({
  slots: {
    // The room the picker takes while its module arrives, so that what holds it does not jump.
    placeholder: "min-h-emoji-picker-height",
    popover: [
      "box-border w-emoji-picker-popover rounded-overlay bg-surface p-emoji-picker-gap shadow-overlay outline-hidden",
    ],
    dialog: "outline-hidden",
    sheet: [
      "box-border h-emoji-picker-sheet-height rounded-t-overlay bg-surface px-emoji-picker-gap pt-shell-inset",
      "pb-emoji-picker-sheet-foot font-sans text-body text-text shadow-overlay outline-hidden",
    ],
    sheetDialog: "outline-hidden",
    head: "ps-emoji-picker-heading-gap",
    title: "text-title",
  },
});

/** The emoji picker, its module fetched when first drawn (or before, by `preloadEmojiPicker`). */
export function LazyEmojiPicker(props: EmojiPickerProps) {
  return (
    <Suspense fallback={<div className={overlay().placeholder()} aria-busy="true" />}>
      <Picker {...props} />
    </Suspense>
  );
}

/** The popover's surface, open at rest (a story's), around the picker. */
export function EmojiPickerSurface({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Box className={overlay().popover()} role="dialog" aria-label={label}>
      {children}
    </Box>
  );
}

/**
 * The picker in a popover beside its trigger (an icon button), from `tablet`: a menu's surface and shadow, as wide as
 * the picker's width or the viewport less its margins. Escape and a press outside close it, and focus returns to the
 * trigger.
 */
export function EmojiPickerPopover({
  label,
  trigger,
  open,
  onOpenChange,
  placement = "bottom start",
  triggerRef,
  children,
}: {
  /** The picker's name (the trigger's label). */
  label: string;
  trigger?: ReactElement | undefined;
  open?: boolean | undefined;
  onOpenChange?: ((open: boolean) => void) | undefined;
  placement?: "bottom start" | "bottom end" | "top start" | "top end" | undefined;
  /** The element it sits beside when it has no `trigger` of its own (a story's, open at rest). */
  triggerRef?: RefObject<Element | null> | undefined;
  children: ReactNode;
}) {
  const s = overlay();
  const popover = (
    <Box
      as={
        <Popover
          placement={placement}
          offset={4}
          {...(triggerRef ? { triggerRef } : {})}
          {...(open === undefined ? {} : { isOpen: open })}
          {...(onOpenChange ? { onOpenChange } : {})}
        />
      }
      className={s.popover()}
    >
      <Dialog aria-label={label} className={s.dialog()}>
        {children}
      </Dialog>
    </Box>
  );
  if (trigger === undefined) return popover;
  return (
    <DialogTrigger {...(open === undefined ? {} : { isOpen: open })} {...(onOpenChange ? { onOpenChange } : {})}>
      {trigger}
      {popover}
    </DialogTrigger>
  );
}

/**
 * The picker in a sheet from the bottom over the dimmed page, on a phone, which it does not blur, so that what the
 * emoji goes into stays legible: titled, its close button at the end of its head, the picker filling the rest. Escape,
 * a press outside and the close button close it.
 */
export function EmojiPickerSheet({
  open,
  onClose,
  title,
  closeLabel,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  closeLabel: string;
  /** The picker, with `fill`. */
  children: ReactNode;
}) {
  const s = overlay();
  const { lang, direction } = useReader(undefined);
  return (
    <ModalPresence>
      {open && (
        <ModalLayer key="emoji-picker" onDismiss={onClose} backdrop="dim" place="sheet" panelClassName={s.sheet()}>
          <Grid
            as={<Dialog />}
            rows="head"
            grow
            shrink="block"
            gap="emoji-picker-gap"
            className={s.sheetDialog()}
            lang={lang}
            dir={direction}
          >
            <Row align="center" justify="between" gap="dialog-gap" className={s.head()}>
              <Heading slot="title" className={s.title()}>
                {title}
              </Heading>
              <IconButton icon="close" label={closeLabel} onPress={onClose} />
            </Row>
            {children}
          </Grid>
        </ModalLayer>
      )}
    </ModalPresence>
  );
}
