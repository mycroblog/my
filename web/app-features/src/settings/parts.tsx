// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of the settings' entries share, each once: the notice in a screen's sections' place until what it
// shows is read, when it could not be, or when its address names nothing of the reader's (every entry's); and, for
// Security, Apps and Invites, a number and the dates of a row, in the reader's language and time zone; a value to
// copy, with the button that copies it and what came of copying; the confirmation before what a row's button ends,
// which asks no proof and says in the server's words why it did not happen; the way of the focus to the field a form
// says is at fault; and the focus of a view that takes another's place, on what it says first.

import {
  Box,
  Button,
  ConfirmDialog,
  CopyBlock,
  dateFormat,
  FeedNotice,
  focusRefusedField,
  TextLink,
  useLocale,
  useTimeZone,
  wallTime,
} from "@my/design-system";
import { copyText } from "@my/platform";
import { useEffect, useState, type ReactNode, type RefObject } from "react";
import { stringsIn } from "../locale";
import { saidOf, useGone } from "./confirmation";

/** That a screen's address names nothing of the reader's, and the way to the list of what it could name. */
type Missing = {
  /** The sentence that says so. */
  said: string;
  /** The name of the link to the list. */
  all: string;
  /** The address of the list. */
  href: string;
};

/**
 * In the sections' place, as a screen's `notice` (of a `SettingsScreen`), as the screen's text reads, at its start:
 * that what the screen shows loads, that it could not be read, with Try again after it, a button whose target is a
 * finger's on a touch screen, or that its address names nothing of the reader's (`missing`, which an entry words),
 * with the way to the list. A screen that reads one thing says it by name (`words`: the account's status), as the
 * part of its entry that reads the same does; without them, the settings.
 */
export function ReadingNotice({
  status,
  retry,
  missing,
  words,
}: {
  status: "loading" | "failed" | "missing";
  retry: () => void;
  /** What `missing` says; an entry whose screens name nothing of a list gives none, and no such screen is missing. */
  missing?: Missing | undefined;
  words?: { readonly loading: string; readonly failed: string } | undefined;
}) {
  const { t } = stringsIn(useLocale().locale);
  if (status === "missing" && missing !== undefined)
    return (
      <FeedNotice alert lead action={<TextLink href={missing.href}>{missing.all}</TextLink>}>
        {missing.said}
      </FeedNotice>
    );
  return status === "failed" ? (
    <FeedNotice
      alert
      lead
      action={
        <Button size="timeline" touch onPress={retry}>
          {t("settings.retry")}
        </Button>
      }
    >
      {words?.failed ?? t("settings.failed")}
    </FeedNotice>
  ) : (
    <FeedNotice lead>{words?.loading ?? t("settings.loading")}</FeedNotice>
  );
}

/**
 * The `notice` of a settings screen while it has no section to show: that what it shows is being read, or that it
 * could not be, with the way to try again; none once it is read.
 */
export function useReadingNotice(status: "loading" | "failed" | "read", retry: () => void): ReactNode {
  return status === "read" ? undefined : <ReadingNotice status={status} retry={retry} />;
}

/** How a number reads in the reader's language. */
export const numberIn = (locale: string, n: number): string => new Intl.NumberFormat(locale).format(n);

/** The space a line does not break at (U+00A0), by its number: written out, it reads as any space. */
const noBreak = String.fromCharCode(0xa0);

/**
 * A day (`day`) and an hour of a day (`hour`, what the server keeps of a last use) as the reader's language and time
 * zone write them, short, with the year only when it is not this one. A date holds together: its spaces do not break,
 * so that the line it stands in wraps before or after it.
 */
export function useDates(): { day: (iso: string) => string; hour: (iso: string) => string } {
  const { locale } = useLocale();
  const timeZone = useTimeZone();
  const [now] = useState(() => new Date());
  const format = (iso: string, more: Intl.DateTimeFormatOptions) => {
    const date = new Date(iso);
    const year = wallTime(date, timeZone).year === wallTime(now, timeZone).year ? {} : { year: "numeric" as const };
    return dateFormat(locale, timeZone, { month: "short", day: "numeric", ...year, ...more })
      .format(date)
      .replaceAll(" ", noBreak);
  };
  return { day: (iso) => format(iso, {}), hour: (iso) => format(iso, { hour: "numeric" }) };
}

/** Copying `text`, and what came of the last copy, in words; undefined until one was asked. */
function useCopy(text: string): { copy: () => void; result: string | undefined } {
  const { t } = stringsIn(useLocale().locale);
  const [copied, setCopied] = useState<boolean>();
  return {
    copy: () => {
      void copyText(text).then(setCopied);
    },
    result: copied === undefined ? undefined : t(copied ? "settings.copy.copied" : "settings.copy.failed"),
  };
}

/**
 * What the reader copies and keeps, named `label`: its values, each on a line, the button that copies them, named
 * `copy` (it says what it copies where two stand on one screen), and what came of copying. `primary`, the button is
 * the screen's one primary action (a link to pass on).
 */
export function Kept({
  label,
  values,
  copy: name,
  primary = false,
}: {
  label: string;
  values: readonly string[];
  /** The name of the button that copies. */
  copy: string;
  primary?: boolean;
}) {
  const { copy, result } = useCopy(values.join("\n"));
  return (
    <CopyBlock
      label={label}
      values={values}
      action={
        <Button {...(primary ? { kind: "primary" as const } : {})} size="timeline" onPress={copy}>
          {name}
        </Button>
      }
      result={result}
    />
  );
}

/** What follows the leaving of a dialog whose screen moves no focus. */
const nothing = () => undefined;

/**
 * Asks before `run` ends something, with a confirmation alone: confirmed, it spins until the server answered and
 * closes; refused, it says why in place of what it explained, and the confirmation tries again.
 */
export function Asking({
  title,
  confirm,
  run,
  onClose,
  onLeft,
  children,
}: {
  title: string;
  confirm: string;
  run: () => Promise<unknown>;
  onClose: () => void;
  /**
   * The dialog is out of the document, the motion of its leaving over, however it was left: the moment to move the
   * focus (`settle` of a section's result), for a screen that moves it. The same function for as long as the dialog
   * is open.
   */
  onLeft?: (() => void) | undefined;
  children: string;
}) {
  const { t } = stringsIn(useLocale().locale);
  useGone(onLeft ?? nothing);
  const [busy, setBusy] = useState(false);
  const [said, setSaid] = useState<string>();
  return (
    <ConfirmDialog
      title={title}
      cancel={t("settings.confirm.cancel")}
      confirm={confirm}
      busy={busy}
      onCancel={onClose}
      onConfirm={() => {
        setBusy(true);
        run().then(onClose, (error: unknown) => {
          setBusy(false);
          setSaid(saidOf(error, t));
        });
      }}
    >
      {said === undefined ? (
        children
      ) : (
        <Box as="span" role="alert">
          {said}
        </Box>
      )}
    </ConfirmDialog>
  );
}

/**
 * Moves the focus to the field a form says is at fault, so that a screen reader reads the field with what is wrong
 * and the reader corrects it at once, its error in view with it, clear of the submit bar (`focusRefusedField`).
 * `within` says where the field is: the form's ref, which the screen makes and gives its form, for the first field of
 * it that is marked invalid, in the form's order; or the ID of one field's control, which takes the focus only while
 * it is marked invalid. The function returned asks, each time the form said its faults (its own check before sending,
 * or what the server refused of a field), and the focus moves once those words stand under their fields; it moves
 * again, once asked, when `within` names another field or `again` changes (new fields in the place of the ones at
 * fault). With no field at fault it moves nothing: what the server refused of the whole stands right before the
 * button, which keeps the focus.
 */
export function useFaultFocus(within: RefObject<HTMLElement | null> | string, again?: string): () => void {
  /** Each ask, counted: the same faults said again ask again. */
  const [asked, setAsked] = useState(0);
  useEffect(() => {
    if (asked === 0) return;
    const field =
      typeof within === "string"
        ? document.getElementById(within)
        : within.current?.querySelector<HTMLElement>('[aria-invalid="true"]');
    // Only a field that says something is wrong takes the focus: the words stand under it by now.
    if (field?.getAttribute("aria-invalid") !== "true") return;
    // The design system finds a field by its id; one without is focused as it is.
    if (field.id === "") field.focus();
    else focusRefusedField(field.id);
  }, [asked, within, again]);
  return () => {
    setAsked((n) => n + 1);
  };
}

/**
 * Gives the focus to `target` as the view that holds it comes: the view took another's place under the same address
 * (a form's, a confirmation's), whose focus went with it, and what it says first (its title, the sentence that says
 * what happened) is then read first. `target` is a ref the view makes and gives that element.
 */
export function useFocusOnArrival(target: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    target.current?.focus();
  }, [target]);
}
