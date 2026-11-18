// SPDX-License-Identifier: AGPL-3.0-only

// Two-factor authentication, the screen under Security at /settings/security/two-factor (FR-005).
//
// - Off, it turns it on: the server's secret for an authenticator app, as a QR code and as a key to copy, asked for as
//   the screen opens (nothing is kept until the code is taken), then the code the app makes from it, and Turn on. A
//   code the server refuses says so under its field, in the server's words, and the field takes the focus; a setup
//   that expired is replaced by a new one, and the screen says so beside the button, with the field empty again and
//   the focus in it.
// - Turned on, or the recovery codes made again: the recovery codes, shown this once, with Copy and the plain statement
//   that they are not shown again; Finish goes back to Security. The view takes the place of the one before it, so
//   its title takes the focus, and what made the codes is a status.
// - On, it says since when and how many recovery codes are left (and to make new ones where few are), with the way to
//   make them again; and last, in a section of its own, apart, the way to turn it off. Each asks first for a code of
//   the app or an unused recovery code, in a dialog.

import {
  refusedMember,
  totpSetupExpired,
  useSecurityRead,
  useTotpSetup,
  useTwoFactor,
  useTwoFactorWrites,
  type TwoFactor,
} from "@my/app-data";
import {
  BackButton,
  Button,
  CopyBlock,
  FormAlert,
  ModalPresence,
  QrFrame,
  Row,
  SaveState,
  SettingsList,
  SettingsListRow,
  SettingsNote,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
  useLocale,
} from "@my/design-system";
import { copyText, oneTimeCodeField } from "@my/platform";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { stringsIn } from "../../locale";
import { saidOf } from "../confirmation";
import { CodeDialog, useDates, useFieldFault, useReadingNotice, type SecuritySubProps } from "./parts";

/** How few recovery codes are left before the screen asks to make new ones. */
const fewCodes = 3;

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

/** What the reader copies and keeps, with Copy. */
function Kept({ label, values }: { label: string; values: readonly string[] }) {
  const { t } = stringsIn(useLocale().locale);
  const { copy, result } = useCopy(values.join("\n"));
  return (
    <CopyBlock
      label={label}
      values={values}
      action={
        <Button size="timeline" onPress={copy}>
          {t("settings.security.two_factor.copy")}
        </Button>
      }
      result={result}
    />
  );
}

/** Off: the secret for the authenticator app, the code the app makes from it, and Turn on. */
function Setup({ back, onEnabled }: { back: ReactNode; onEnabled: (codes: readonly string[]) => void }) {
  const { t } = stringsIn(useLocale().locale);
  const fieldId = useId();
  const { setup, status, restart } = useTotpSetup();
  const { enable } = useTwoFactorWrites();
  const [typed, setTyped] = useState("");
  /** A new field for each setup: one that expired takes its code with it. */
  const [round, setRound] = useState(0);
  const [busy, setBusy] = useState(false);
  const fault = useFieldFault(fieldId);
  const [said, setSaid] = useState<string>();
  const reading = useReadingNotice(setup !== undefined ? "read" : status === "failed" ? "failed" : "loading", restart);

  const submit = () => {
    const code = typed.trim();
    if (setup === undefined || busy) return;
    if (code === "") {
      fault.say(t("settings.security.two_factor.code.missing"));
      return;
    }
    setBusy(true);
    fault.clear();
    setSaid(undefined);
    enable(setup.setup_token, code).then(onEnabled, (error: unknown) => {
      setBusy(false);
      if (totpSetupExpired(error)) {
        // The secret is gone with its setup: the app needs the new one, and the code of the old one is no use. The
        // field is empty again and takes the focus, for the code of the new entry.
        restart();
        setTyped("");
        setRound((n) => n + 1);
        setSaid(t("settings.security.two_factor.setup.expired"));
        return;
      }
      // What the server refused of the code stands under its field, which takes the focus.
      const refused = refusedMember(error);
      if (refused?.member === "code") fault.say(refused.said);
      else setSaid(saidOf(error, t));
    });
  };

  return (
    <form
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <SettingsScreen
        sub
        title={t("settings.security.two_factor")}
        back={back}
        notice={reading}
        foot={
          setup === undefined ? undefined : (
            <SubmitBar result={said === undefined ? undefined : <FormAlert>{said}</FormAlert>}>
              <Button kind="primary" type="submit" busy={busy}>
                {t("settings.security.two_factor.turn_on")}
              </Button>
            </SubmitBar>
          )
        }
      >
        {setup !== undefined && (
          <>
            <SettingsSection
              heading={t("settings.security.two_factor.setup")}
              about={t("settings.security.two_factor.setup.about")}
            >
              <QrFrame
                label={t("settings.security.two_factor.setup.qr")}
                size={setup.qr_code.size}
                path={setup.qr_code.path}
              />
              <Kept label={t("settings.security.two_factor.setup.key")} values={[setup.secret]} />
            </SettingsSection>
            <SettingsSection heading={t("settings.security.two_factor.verify")}>
              <TextField
                key={round}
                id={fieldId}
                name="code"
                label={t("settings.security.two_factor.code")}
                hint={t("settings.security.two_factor.code.hint")}
                error={fault.words}
                autoFocus={round > 0}
                autoComplete={oneTimeCodeField.autoComplete}
                inputMode="numeric"
                onChange={setTyped}
              />
            </SettingsSection>
          </>
        )}
      </SettingsScreen>
    </form>
  );
}

/** The recovery codes, shown this once. */
function Codes({ codes, said, onDone }: { codes: readonly string[]; said: string; onDone: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  // It took another view's place, whose focus went with it: its title takes the focus, as a screen's heading does.
  const title = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    title.current?.focus();
  }, []);
  return (
    <SettingsScreen
      sub
      title={t("settings.security.two_factor.recovery")}
      titleRef={title}
      foot={
        <SubmitBar>
          <Button kind="primary" onPress={onDone}>
            {t("settings.once.done")}
          </Button>
        </SubmitBar>
      }
    >
      <SettingsSection
        heading={t("settings.security.two_factor.recovery")}
        about={t("settings.security.two_factor.recovery.once")}
      >
        <SettingsResult>
          <SaveState state="saved">{said}</SaveState>
        </SettingsResult>
        <Kept label={t("settings.security.two_factor.recovery")} values={codes} />
      </SettingsSection>
    </SettingsScreen>
  );
}

/** On: since when, the recovery codes left, and the ways to make them again and to turn it off. */
function Manage({
  twoFactor,
  back,
  onCodes,
  onOff,
}: {
  twoFactor: TwoFactor;
  back: ReactNode;
  onCodes: (codes: readonly string[]) => void;
  onOff: () => void;
}) {
  const { t } = stringsIn(useLocale().locale);
  const { day } = useDates();
  const { disable, regenerateCodes } = useTwoFactorWrites();
  const [asking, setAsking] = useState<"codes" | "off">();
  const left = twoFactor.recovery_codes_remaining ?? 0;
  const close = () => {
    setAsking(undefined);
  };
  return (
    <>
      <SettingsScreen sub title={t("settings.security.two_factor")} back={back}>
        <SettingsSection heading={t("settings.security.two_factor.on.heading")}>
          <SettingsNote>
            {twoFactor.enrolled_at === undefined
              ? t("settings.security.two_factor.on.about")
              : t("settings.security.two_factor.on.about.since", { date: day(twoFactor.enrolled_at) })}
          </SettingsNote>
          <SettingsList label={t("settings.security.two_factor.on.heading")}>
            <SettingsListRow
              title={t("settings.security.two_factor.recovery")}
              caption={t(
                left <= fewCodes
                  ? "settings.security.two_factor.recovery.few"
                  : "settings.security.two_factor.recovery.left",
                { count: left },
              )}
              end={
                <Button
                  size="timeline"
                  onPress={() => {
                    setAsking("codes");
                  }}
                >
                  {t("settings.security.two_factor.recovery.regenerate")}
                </Button>
              }
            />
          </SettingsList>
        </SettingsSection>
        {/* What ends it stands last, in a section of its own, apart from what keeps it working. */}
        <SettingsSection
          heading={t("settings.security.two_factor.turn_off.title")}
          about={t("settings.security.two_factor.turn_off.about")}
        >
          <Row justify="start">
            <Button
              size="timeline"
              onPress={() => {
                setAsking("off");
              }}
            >
              {t("settings.security.two_factor.turn_off")}
            </Button>
          </Row>
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>
        {asking === "codes" && (
          <CodeDialog
            key="codes"
            about={t("settings.security.two_factor.recovery.regenerate.about")}
            confirm={t("settings.security.two_factor.recovery.regenerate")}
            run={(code) => regenerateCodes(code).then(onCodes)}
            onClose={close}
          />
        )}
        {asking === "off" && (
          <CodeDialog
            key="off"
            about={t("settings.security.two_factor.turn_off.confirm")}
            confirm={t("settings.security.two_factor.turn_off")}
            run={(code) => disable(code).then(onOff)}
            onClose={close}
          />
        )}
      </ModalPresence>
    </>
  );
}

export function SecurityTwoFactor({ onBack }: SecuritySubProps) {
  const { t } = stringsIn(useLocale().locale);
  const { status, retry } = useSecurityRead();
  const twoFactor = useTwoFactor();
  /** The recovery codes the server just made, shown until the reader leaves, and what made them. */
  const [made, setMade] = useState<{ codes: readonly string[]; said: string }>();
  /** Turned off here: the screen is being left for Security, and starts no new setup meanwhile. */
  const [turnedOff, setTurnedOff] = useState(false);
  const reading = useReadingNotice(status === "failed" ? "failed" : "loading", retry);
  const back = <BackButton onPress={onBack}>{t("settings.entry.security")}</BackButton>;
  if (made !== undefined) return <Codes codes={made.codes} said={made.said} onDone={onBack} />;
  if (twoFactor === undefined || turnedOff)
    return <SettingsScreen sub title={t("settings.security.two_factor")} back={back} notice={reading} />;
  if (twoFactor.enabled)
    return (
      <Manage
        twoFactor={twoFactor}
        back={back}
        onCodes={(codes) => {
          setMade({ codes, said: t("settings.security.two_factor.recovery.regenerated") });
        }}
        onOff={() => {
          setTurnedOff(true);
          onBack();
        }}
      />
    );
  return (
    <Setup
      back={back}
      onEnabled={(codes) => {
        setMade({ codes, said: t("settings.security.two_factor.enabled") });
      }}
    />
  );
}
