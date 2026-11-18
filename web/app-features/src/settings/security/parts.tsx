// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of the settings' Security share, beside what every entry's do (../parts.tsx): their addresses;
// how few recovery codes are few; what a section says came of a change whose control went with it (a passkey removed,
// an account unlinked, a browser signed out), which takes the focus the row's button left behind; what is wrong with
// a field's value, under the field, which takes the focus; and the dialog that asks for a two-factor code.

import { FormAlert, ReauthDialog, SaveState, SettingsResult, TextField, useLocale } from "@my/design-system";
import { oneTimeCodeField } from "@my/platform";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { stringsIn } from "../../locale";
import { saidOf } from "../confirmation";
import type { SettingsSubProps } from "../entries";
import { useFaultFocus } from "../parts";

/** The address of the Security entry, and of the screens under it. */
export const securityPath = "/settings/security";
export const securityPaths = {
  password: `${securityPath}/password`,
  twoFactor: `${securityPath}/two-factor`,
  passkey: `${securityPath}/passkey`,
} as const;

/**
 * How few recovery codes are left before Security says how many on the row of two-factor authentication, and its
 * screen asks to make new ones.
 */
export const fewCodes = 3;

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
 * What is wrong with a field's value, under the field of `fieldId` (its `words`, a `TextField`'s `error`): the
 * screen's own check, or what the server refused of that member of the request. `say` puts the words there and, once
 * they stand under the field, moves the focus into it, so that a screen reader reads the field with what is wrong and
 * the reader corrects it at once. Under a dialog that still holds the focus, `say(words, false)` waits, and `focus`
 * moves it once the dialog has left; with nothing wrong, `focus` does nothing (`useFaultFocus`, on the field's ID).
 */
export function useFieldFault(fieldId: string): {
  words: string | undefined;
  say: (words: string, focus?: boolean) => void;
  clear: () => void;
  focus: () => void;
} {
  const [words, setWords] = useState<string>();
  const focus = useFaultFocus(fieldId);
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
