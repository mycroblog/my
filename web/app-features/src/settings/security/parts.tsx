// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of the settings' Security share: their addresses; the notice in the sections' place until what
// they show is read, or when it could not be; the dates of their rows, in the reader's time zone; the confirmation
// before what a row's button ends (removing a passkey, unlinking an account, signing a browser out), which asks no
// proof and says in the server's words why it did not happen (the last way to sign in cannot be removed); and the
// dialog that asks for a two-factor code.

import {
  Box,
  ConfirmDialog,
  dateFormat,
  FeedNotice,
  FormAlert,
  ReauthDialog,
  TextButton,
  TextField,
  useLocale,
  useTimeZone,
  wallTime,
} from "@my/design-system";
import { oneTimeCodeField } from "@my/platform";
import { useId, useState } from "react";
import { stringsIn } from "../../locale";
import { saidOf } from "../confirmation";

/** The address of the Security entry, and of the screens under it. */
export const securityPath = "/settings/security";
export const securityPaths = {
  password: `${securityPath}/password`,
  twoFactor: `${securityPath}/two-factor`,
  passkey: `${securityPath}/passkey`,
} as const;

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

/** In the sections' place: that what the screen shows loads, or that it could not be read, with the way to try again. */
export function Reading({ status, retry }: { status: "loading" | "failed"; retry: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  return status === "failed" ? (
    <FeedNotice alert action={<TextButton onPress={retry}>{t("settings.retry")}</TextButton>}>
      {t("settings.failed")}
    </FeedNotice>
  ) : (
    <FeedNotice>{t("settings.loading")}</FeedNotice>
  );
}

/**
 * A day (`day`) and an hour of a day (`hour`, what the server keeps of a last use) as the reader's language and time
 * zone write them, short, with the year only when it is not this one.
 */
export function useDates(): { day: (iso: string) => string; hour: (iso: string) => string } {
  const { locale } = useLocale();
  const timeZone = useTimeZone();
  const [now] = useState(() => new Date());
  const format = (iso: string, more: Intl.DateTimeFormatOptions) => {
    const date = new Date(iso);
    const year = wallTime(date, timeZone).year === wallTime(now, timeZone).year ? {} : { year: "numeric" as const };
    return dateFormat(locale, timeZone, { month: "short", day: "numeric", ...year, ...more }).format(date);
  };
  return { day: (iso) => format(iso, {}), hour: (iso) => format(iso, { hour: "numeric" }) };
}

/**
 * Asks before `run` ends something, with a confirmation alone: confirmed, it spins until the server answered and
 * closes; refused, it says why in place of what it explained, and the confirmation tries again.
 */
export function Asking({
  title,
  confirm,
  run,
  onClose,
  children,
}: {
  title: string;
  confirm: string;
  run: () => Promise<unknown>;
  onClose: () => void;
  children: string;
}) {
  const { t } = stringsIn(useLocale().locale);
  const [busy, setBusy] = useState(false);
  const [said, setSaid] = useState<string>();
  return (
    <ConfirmDialog
      title={title}
      cancel={t("settings.security.confirm.cancel")}
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
      closeLabel={t("settings.security.confirm.close")}
      cancel={t("settings.security.confirm.cancel")}
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
