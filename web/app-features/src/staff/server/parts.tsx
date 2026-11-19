// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of roles, custom emoji, announcements and webhooks share: what a screen under an entry of the
// administration is given by its route; the notice in a screen's sections' place while what it shows loads, could not
// be read (with Try again, a finger's target), or names nothing (with the way to its list); the question before
// leaving a form with changes not saved; the mark of what cannot be undone; a value given once, with the button that
// copies it; and the way back to an entry's list.

import { BackButton, Box, Button, ConfirmDialog, CopyBlock, FeedNotice, Icon, Row, TextLink } from "@my/design-system";
import { copyText } from "@my/platform";
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { MessageId } from "../../messages.gen";
import { useStaffStrings } from "../parts";

/** What a screen under an entry of the administration is given by its route. */
export type AdminSubProps = {
  /** Goes to `href`, an address of the app, in place of the screen (back to the entry, to what a write made). */
  readonly go: (href: string) => void;
  /** Hears whether the form holds changes that are not saved, as that comes and goes; false before it leaves. */
  readonly onUnsaved?: ((unsaved: boolean) => void) | undefined;
  /** The reader is leaving with changes not saved, and is held back: the screen asks whether to leave or stay. */
  readonly leaving?: { readonly onStay: () => void; readonly onLeave: () => void } | undefined;
};

/**
 * In a screen's sections' place: that what it shows loads (`loading`), could not be read (`failed`, with Try again),
 * or that its address names nothing (`missing`, with the way to the list at `all`).
 */
export function ScreenNotice({
  status,
  retry,
  words,
  all,
}: {
  status: "loading" | "failed" | "missing";
  retry: () => void;
  words: { readonly loading: string; readonly failed: string; readonly missing?: string | undefined };
  all?: { readonly label: string; readonly href: string } | undefined;
}): ReactNode {
  const { t } = useStaffStrings();
  if (status === "missing")
    return (
      <FeedNotice alert lead action={all === undefined ? undefined : <TextLink href={all.href}>{all.label}</TextLink>}>
        {words.missing ?? words.failed}
      </FeedNotice>
    );
  if (status === "loading") return <FeedNotice lead>{words.loading}</FeedNotice>;
  return (
    <FeedNotice
      alert
      lead
      action={
        <Button size="timeline" touch onPress={retry}>
          {t("staff.retry")}
        </Button>
      }
    >
      {words.failed}
    </FeedNotice>
  );
}

/** The question before leaving a form whose changes are not saved: leave, losing them, or stay. */
export function LeaveDialog({ leaving }: { leaving: { readonly onStay: () => void; readonly onLeave: () => void } }) {
  const { t } = useStaffStrings();
  return (
    <ConfirmDialog
      title={t("staff_server.leave.title")}
      cancel={t("staff_server.leave.cancel")}
      confirm={t("staff_server.leave.confirm")}
      onCancel={leaving.onStay}
      onConfirm={leaving.onLeave}
    >
      {t("staff_server.leave.text")}
    </ConfirmDialog>
  );
}

/**
 * The mark of an action that cannot be undone, in the text's color with the glyph of caution: the one look every such
 * act of the administration has, beside the question that confirms it.
 */
export function Irreversible({ children }: { children: string }): ReactNode {
  return (
    <Row align="start" gap="entry-caption">
      <Row as="span" inline line rigid align="center">
        <Icon name="caution" place="caption" />
      </Row>
      <Box as="span" text="caption" weight="strong" breaks="phrase">
        {children}
      </Box>
    </Row>
  );
}

/** A value to pass on (a secret), named `label`, with the button that copies it, named `copy`, and what came of it. */
export function CopyValue({ label, value, copy }: { label: string; value: string; copy: string }): ReactNode {
  const { t } = useStaffStrings();
  const [copied, setCopied] = useState<boolean>();
  return (
    <CopyBlock
      label={label}
      values={[value]}
      action={
        <Button
          size="timeline"
          kind="primary"
          onPress={() => {
            void copyText(value).then(setCopied);
          }}
        >
          {copy}
        </Button>
      }
      result={copied === undefined ? undefined : t(copied ? "staff_server.copied" : "staff_server.copy_failed")}
    />
  );
}

/** The way back from a screen under an entry to the entry's list at `href`, named by the entry. */
export function BackTo({ go, href, name }: { go: (href: string) => void; href: string; name: MessageId }) {
  const { t } = useStaffStrings();
  return (
    <BackButton
      onPress={() => {
        go(href);
      }}
    >
      {t(name)}
    </BackButton>
  );
}

/**
 * Tells `onUnsaved` whether a form holds changes not on their way (`unsaved && !busy`) as that changes, and false as it
 * leaves.
 */
export function useUnsavedReport(onUnsaved: AdminSubProps["onUnsaved"], unsaved: boolean, busy: boolean): void {
  useEffect(() => {
    onUnsaved?.(unsaved && !busy);
  }, [onUnsaved, unsaved, busy]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );
}

/**
 * Whether the screen is still there: a write whose answer comes after the screen left moves nothing and sets no
 * state.
 */
export function useStillThere(): { readonly current: boolean } {
  const there = useRef(true);
  useEffect(() => {
    there.current = true;
    return () => {
      there.current = false;
    };
  }, []);
  return there;
}
