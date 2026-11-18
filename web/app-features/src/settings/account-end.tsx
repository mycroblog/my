// SPDX-License-Identifier: AGPL-3.0-only

// Deactivate account and Delete account, the two screens under the settings' entry Account and data that end the
// account (FR-008, ADR-0089 §11–§17), at /settings/account/deactivate and /settings/account/delete. Each says what
// its act does before the reader acts, in sections of facts; then what to do first, the door to Export, since the
// data cannot be exported afterwards; and last, in a section of its own, the act: a button named by what it does,
// never the screen's accent, which asks the reader to confirm it's them (confirmation.tsx). Either ends every session
// of the account at once: the app then leaves as signing out leaves it, the confirmation spinning until it has.
//
// - Deactivating hides the account and what it posted, drops what reaches it meanwhile, and keeps the rest until the
//   day the server would delete it, which the screen reads and says with its time: signing in again before then
//   brings the account back. Where the server would refuse (a moderation action that stops the sign-in, a deletion
//   already scheduled, a move, a registration under review, a supporter plan that goes on, the last administrator who
//   can sign in, a reactivation in the last day), the screen says so in the act's place, with why and what to do
//   first, and offers no way to it; a refusal that only a press finds out comes as that summary, which takes the
//   focus.
// - Deleting is at once and for good; the server keeps what it holds for some days more, hidden. What it refuses
//   once the confirmation holds (the server's only administrator) is said before the button, in the server's words.

import { accountMovePath, dataExportPath } from "@my/app-components";
import {
  confirmationFields,
  deactivationRefused,
  deletionEraseDays,
  loadDeactivation,
  loadOwnAccount,
  useAccountEnd,
  useDeactivationRead,
  useOwnAccountRead,
  type AppData,
  type DeactivationPreview,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  FormAlert,
  LinkButton,
  Row,
  SettingsFacts,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  Stack,
  StatusSummary,
  useLocale,
} from "@my/design-system";
import { useRef, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";
import { accountStatusPath, useActionDate } from "./account";
import { accountDeactivatePath, accountDeletePath } from "./account-leave";
import { saidOf, useConfirmation, useProviderReturn } from "./confirmation";
import type { SettingsSessionProps, SettingsSubProps } from "./entries";
import { ReadingNotice, useFocusAfterDialog } from "./parts";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: what deactivating would do and what the account confirms with. */
export async function loadAccountDeactivateSettings(data: AppData): Promise<void> {
  await loadDeactivation(data);
}

/** What the route's loader waits for: the reader's account and what it confirms with. */
export async function loadAccountDeleteSettings(data: AppData): Promise<void> {
  await loadOwnAccount(data);
}

/** The title of Deactivate account. */
export function useAccountDeactivateTitle(): string {
  return stringsIn(useLocale().locale).t("settings.account.deactivate");
}

/** The title of Delete account. */
export function useAccountDeleteTitle(): string {
  return stringsIn(useLocale().locale).t("settings.account.delete");
}

/** The way back to the entry, in the head of either screen. */
const backTo = (onBack: () => void, t: T) => <BackButton onPress={onBack}>{t("settings.entry.account")}</BackButton>;

/** In the sections' place of either screen: that what it shows loads, or could not be read. */
function Unread({ status, retry, t }: { status: "loading" | "failed"; retry: () => void; t: T }) {
  return (
    <ReadingNotice
      words={{ loading: t("settings.account.loading"), failed: t("settings.account.failed") }}
      status={status}
      retry={retry}
    />
  );
}

/** What to do before the act: the door to Export, under why it comes first. */
function ExportFirst({ heading, about, t }: { heading: string; about: string; t: T }) {
  return (
    <SettingsSection heading={heading}>
      <SettingsList label={heading}>
        <SettingsListRow title={t("settings.account.export")} caption={about} href={dataExportPath} />
      </SettingsList>
    </SettingsSection>
  );
}

/**
 * The act, the screen's last section: what pressing does, what a login provider's return left to say (`notice`), what
 * the server refused of the last press (`said`), and the button, neutral, named by what it does.
 */
function Act({
  heading,
  about,
  notice,
  said,
  busy,
  onPress,
  children,
}: {
  heading: string;
  about: string;
  notice: ReactNode;
  said: string | undefined;
  busy: boolean;
  onPress: () => void;
  children: string;
}) {
  return (
    <SettingsSection heading={heading} about={about}>
      {notice}
      {/* The refusal's place is there from the start, with no room of its own until it says something. */}
      <Stack gap={said === undefined ? undefined : "screen-row-gap"}>
        <Box role="status">{said !== undefined && <FormAlert>{said}</FormAlert>}</Box>
        <Row justify="start">
          <Button busy={busy} onPress={onPress}>
            {children}
          </Button>
        </Row>
      </Stack>
    </SettingsSection>
  );
}

/** The session ended with the account: the confirmation stays as it is, spinning, until the app has left. */
const untilLeft = () => new Promise<never>(() => undefined);

/** Where the reader acts on what keeps the account from being deactivated, if the app has a screen for it. */
function refusalDoor(refusal: NonNullable<DeactivationPreview["refusal"]>, t: T): ReactNode {
  if (refusal === "moved")
    return (
      <LinkButton href={accountMovePath} kind="primary">
        {t("settings.account.move")}
      </LinkButton>
    );
  if (refusal === "login_restricted" || refusal === "deletion_scheduled")
    return (
      <LinkButton href={accountStatusPath} kind="primary">
        {t("settings.account.status")}
      </LinkButton>
    );
  return undefined;
}

export function AccountDeactivateScreen({
  onBack,
  onSessionEnded,
  fragment,
  onFragmentRead,
  leave,
}: SettingsSubProps & SettingsSessionProps) {
  const { t } = stringsIn(useLocale().locale);
  const date = useActionDate();
  const { status, retry, preview } = useDeactivationRead();
  const { deactivate } = useAccountEnd();
  const returned = useProviderReturn({ fragment, onFragmentRead });
  const [said, setSaid] = useState<string>();
  /** The server refused a press on this screen: the summary that takes the act's place takes the focus. */
  const [refused, setRefused] = useState(false);
  const summary = useRef<HTMLHeadingElement>(null);
  const after = useFocusAfterDialog(summary);
  const confirming = useConfirmation({
    operation: "account_deactivation",
    about: t("settings.account.deactivate.confirm"),
    returnTo: accountDeactivatePath,
    returned,
    leave,
  });

  const title = t("settings.account.deactivate");
  const back = backTo(onBack, t);
  if (preview === undefined || status !== "read")
    return (
      <SettingsScreen
        sub
        title={title}
        back={back}
        notice={<Unread status={status === "failed" ? "failed" : "loading"} retry={retry} t={t} />}
      />
    );

  const press = () => {
    setSaid(undefined);
    confirming.ask(
      async (confirmation) => {
        await deactivate(confirmationFields(confirmation));
        onSessionEnded();
        await untilLeft();
      },
      {
        failed: (error) => {
          // The server's words stand before the button. For an account that cannot be deactivated now, what
          // deactivating would do is read again by this time: where it says why, the summary takes the act's place
          // and the focus.
          setSaid(saidOf(error, t));
          if (deactivationRefused(error)) {
            after.expect();
            setRefused(true);
          }
        },
        left: after.settle,
      },
    );
  };

  const { refusal } = preview;
  if (refusal !== undefined) {
    const again = preview.available_at;
    return (
      <>
        <SettingsScreen sub title={title} back={back}>
          <StatusSummary
            icon="caution"
            title={t("settings.account.deactivate.refused")}
            titleRef={refused ? summary : undefined}
            about={t("settings.account.deactivate.refused.reason", {
              reason: refusal === "recently_reactivated" && again === undefined ? "reactivated" : refusal,
              date: again === undefined ? "" : date(again),
            })}
            action={refusalDoor(refusal, t)}
          />
        </SettingsScreen>
        {confirming.dialog}
      </>
    );
  }

  const due = date(preview.deletion_due_at);
  const effect = t("settings.account.deactivate.effect");
  const coming = t("settings.account.deactivate.after");
  return (
    <>
      <SettingsScreen sub title={title} back={back}>
        <SettingsSection heading={effect}>
          <SettingsFacts
            label={effect}
            items={[
              {
                text: t("settings.account.deactivate.effect.hidden"),
                about: [t("settings.account.deactivate.effect.hidden.about")],
              },
              { text: t("settings.account.end.signed_out"), about: [t("settings.account.end.signed_out.about")] },
              {
                text: t("settings.account.deactivate.effect.dropped"),
                about: [t("settings.account.deactivate.effect.dropped.about")],
              },
              {
                text: t("settings.account.deactivate.effect.kept"),
                about: [t("settings.account.deactivate.effect.kept.about")],
              },
            ]}
          />
        </SettingsSection>
        <SettingsSection heading={coming}>
          <SettingsFacts
            label={coming}
            items={[
              {
                text: t("settings.account.deactivate.after.reactivate", { date: due }),
                about: [t("settings.account.deactivate.after.reactivate.about")],
              },
              {
                text: t("settings.account.deactivate.after.deleted"),
                about: [t("settings.account.deactivate.after.deleted.about")],
              },
            ]}
          />
        </SettingsSection>
        <ExportFirst
          heading={t("settings.account.deactivate.first")}
          about={t("settings.account.deactivate.first.export")}
          t={t}
        />
        <Act
          heading={t("settings.account.deactivate.act")}
          about={t("settings.account.deactivate.act.about", { date: due })}
          notice={confirming.notice}
          said={said}
          busy={confirming.busy}
          onPress={press}
        >
          {t("settings.account.deactivate.submit")}
        </Act>
      </SettingsScreen>
      {confirming.dialog}
    </>
  );
}

export function AccountDeleteScreen({
  onBack,
  onSessionEnded,
  fragment,
  onFragmentRead,
  leave,
}: SettingsSubProps & SettingsSessionProps) {
  const { t } = stringsIn(useLocale().locale);
  const { status, retry, account } = useOwnAccountRead();
  const { remove } = useAccountEnd();
  const returned = useProviderReturn({ fragment, onFragmentRead });
  const [said, setSaid] = useState<string>();
  const confirming = useConfirmation({
    operation: "account_deletion",
    about: t("settings.account.delete.confirm"),
    returnTo: accountDeletePath,
    returned,
    leave,
  });

  const title = t("settings.account.delete");
  const back = backTo(onBack, t);
  if (account === undefined || status !== "read")
    return (
      <SettingsScreen
        sub
        title={title}
        back={back}
        notice={<Unread status={status === "failed" ? "failed" : "loading"} retry={retry} t={t} />}
      />
    );

  const press = () => {
    setSaid(undefined);
    confirming.ask(
      async (confirmation) => {
        await remove(confirmationFields(confirmation));
        onSessionEnded();
        await untilLeft();
      },
      {
        failed: (error) => {
          setSaid(saidOf(error, t));
        },
      },
    );
  };

  const effect = t("settings.account.delete.effect");
  return (
    <>
      <SettingsScreen sub title={title} back={back}>
        <SettingsSection heading={effect}>
          <SettingsFacts
            label={effect}
            items={[
              {
                text: t("settings.account.delete.effect.gone"),
                about: [t("settings.account.delete.effect.gone.about")],
              },
              {
                text: t("settings.account.delete.effect.final"),
                about: [t("settings.account.delete.effect.final.about")],
              },
              {
                text: t("settings.account.delete.effect.username"),
                // The username as this server registers it, which is what stays taken.
                about: [t("settings.account.delete.effect.username.about", { username: `@${account.username}` })],
              },
              { text: t("settings.account.end.signed_out"), about: [t("settings.account.end.signed_out.about")] },
              {
                text: t("settings.account.delete.effect.erased", { days: deletionEraseDays }),
                about: [t("settings.account.delete.effect.erased.about")],
              },
            ]}
          />
        </SettingsSection>
        <ExportFirst
          heading={t("settings.account.delete.first")}
          about={t("settings.account.delete.first.export")}
          t={t}
        />
        <Act
          heading={t("settings.account.delete.act")}
          about={t("settings.account.delete.act.about")}
          notice={confirming.notice}
          said={said}
          busy={confirming.busy}
          onPress={press}
        >
          {t("settings.account.delete.submit")}
        </Act>
      </SettingsScreen>
      {confirming.dialog}
    </>
  );
}
