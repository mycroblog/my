// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of the settings' Security share: their addresses; the notice in the sections' place until what
// they show is read, or when it could not be; the dates of their rows, in the reader's time zone; the confirmation
// before what a row's button ends (removing a passkey, unlinking an account, signing a browser out), which asks no
// proof and says in the server's words why it did not happen (the last way to sign in cannot be removed); what a
// section says came of such a change, which takes the focus the row's button left behind; what is wrong with a
// field's value, under the field, which takes the focus; and the dialog that asks for a two-factor code.

import {
  Box,
  ConfirmDialog,
  dateFormat,
  FeedNotice,
  FormAlert,
  ReauthDialog,
  SaveState,
  SettingsResult,
  TextButton,
  TextField,
  useLocale,
  useTimeZone,
  wallTime,
} from "@my/design-system";
import { oneTimeCodeField } from "@my/platform";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { stringsIn } from "../../locale";
import { saidOf, useGone } from "../confirmation";
import type { SettingsSubProps } from "../entries";

/** The address of the Security entry, and of the screens under it. */
export const securityPath = "/settings/security";
export const securityPaths = {
  password: `${securityPath}/password`,
  twoFactor: `${securityPath}/two-factor`,
  passkey: `${securityPath}/passkey`,
} as const;

/**
 * What a screen under Security takes of what its route gives every screen under an entry: its address and the way
 * back. None of them ends the browser's own session.
 */
export type SecuritySubProps = Omit<SettingsSubProps, "onSessionEnded">;

/** The name of a screen under Security, the title of its page. */
export function useSecurityTitle(screen: keyof typeof securityPaths): string {
  const { t } = stringsIn(useLocale().locale);
  return t(
    screen === "password"
      ? "settings.security.password"
      : screen === "twoFactor"
        ? "settings.security.two_factor"
        : "settings.security.passkeys.add",
  );
}

/**
 * The `notice` of a settings screen while it has no section to show: that what it shows is being read, or that it
 * could not be, with the way to try again; none once it is read.
 */
export function useReadingNotice(status: "loading" | "failed" | "read", retry: () => void): ReactNode {
  const { t } = stringsIn(useLocale().locale);
  if (status === "read") return undefined;
  return status === "failed" ? (
    <FeedNotice alert action={<TextButton onPress={retry}>{t("settings.retry")}</TextButton>}>
      {t("settings.failed")}
    </FeedNotice>
  ) : (
    <FeedNotice>{t("settings.loading")}</FeedNotice>
  );
}

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

/**
 * What is wrong with a field's value, under the field of `fieldId` (its `words`, a `TextField`'s `error`): the
 * screen's own check, or what the server refused of that member of the request. `say` puts the words there and, once
 * they stand under the field, moves the focus into it, so that a screen reader reads the field with what is wrong and
 * the reader corrects it at once. Under a dialog that still holds the focus, `say(words, false)` waits, and `focus`
 * moves it once the dialog has left; with nothing wrong, `focus` does nothing.
 */
export function useFieldFault(fieldId: string): {
  words: string | undefined;
  say: (words: string, focus?: boolean) => void;
  clear: () => void;
  focus: () => void;
} {
  const [words, setWords] = useState<string>();
  /** Each ask to move the focus, counted: the same words said again ask again. */
  const [asked, setAsked] = useState(0);
  useEffect(() => {
    if (asked === 0) return;
    // Only a field that says something is wrong takes the focus: the words stand under it by now.
    const field = document.getElementById(fieldId);
    if (field?.getAttribute("aria-invalid") === "true") field.focus();
  }, [asked, fieldId]);
  const focus = () => {
    setAsked((n) => n + 1);
  };
  return {
    words,
    say: (said, move = true) => {
      setWords(said);
      if (move) focus();
    },
    clear: () => {
      setWords(undefined);
    },
    focus,
  };
}

/**
 * What a section says came of a change whose control went with what it changed (a row removed with its own button):
 * `line`, for the section's top, a status that holds the words of the last change done (`say`), or an alert for one
 * the server did not take (`refuse`). `settle` moves the focus to what was just said, and is called once the dialog
 * over the section is out of the document (`onLeft` of `Asking`, `left` of a confirmation): before, the dialog holds
 * the focus, and after, the button that opened it is gone and the focus would fall to the page. A screen reader then
 * reads what happened, and the keyboard stays in the section.
 */
export function useSectionResult(): {
  line: ReactNode;
  say: (words: string) => void;
  refuse: (words: string) => void;
  clear: () => void;
  settle: () => void;
} {
  const said = useRef<HTMLParagraphElement>(null);
  const [told, setTold] = useState<{ done: boolean; words: string }>();
  /** Each leaving of a dialog over the section, counted. */
  const [settled, setSettled] = useState(0);
  /** A change was said and the focus has not been moved to it yet. */
  const waits = useRef(false);
  /** The same function at every render: a dialog keeps it to call as it leaves. */
  const [settle] = useState(() => () => {
    setSettled((n) => n + 1);
  });
  useEffect(() => {
    if (settled === 0 || !waits.current) return;
    waits.current = false;
    said.current?.focus();
  }, [settled]);
  return {
    line:
      told === undefined ? null : told.done ? (
        <SettingsResult ref={said}>
          <SaveState state="saved">{told.words}</SaveState>
        </SettingsResult>
      ) : (
        <FormAlert>{told.words}</FormAlert>
      ),
    say: (words) => {
      waits.current = true;
      setTold({ done: true, words });
    },
    refuse: (words) => {
      waits.current = false;
      setTold({ done: false, words });
    },
    clear: () => {
      waits.current = false;
      setTold(undefined);
    },
    settle,
  };
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
   * focus (`settle` of the section's result), for a screen that moves it. The same function for as long as the dialog
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
 * Asks for a two-factor code before `run` (turning it off, making the recovery codes again): a code of the
 * authenticator app, or a recovery code not used yet. A code the server refuses stays in the dialog, in the server's
 * words, for another try.
 */
export function CodeDialog({
  about,
  confirm,
  run,
  onClose,
}: {
  about: string;
  confirm: string;
  run: (code: string) => Promise<unknown>;
  onClose: () => void;
}) {
  const { t } = stringsIn(useLocale().locale);
  const id = useId();
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const [said, setSaid] = useState<string>();
  return (
    <ReauthDialog
      title={t("settings.security.two_factor.code.title")}
      about={about}
      closeLabel={t("settings.confirm.close")}
      cancel={t("settings.confirm.cancel")}
      confirm={confirm}
      busy={busy}
      onCancel={onClose}
      onConfirm={() => {
        const code = typed.trim();
        if (code === "") return;
        setBusy(true);
        setSaid(undefined);
        run(code).then(onClose, (error: unknown) => {
          setBusy(false);
          setSaid(saidOf(error, t));
        });
      }}
      error={said === undefined ? undefined : <FormAlert>{said}</FormAlert>}
    >
      <TextField
        id={id}
        name="code"
        label={t("settings.security.two_factor.code")}
        autoComplete={oneTimeCodeField.autoComplete}
        autoCapitalize="none"
        required
        onChange={setTyped}
      />
    </ReauthDialog>
  );
}
