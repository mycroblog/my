// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of the settings' Invites share (ADR-0106 §14, §15, §18): their addresses; what stands in a
// screen's sections' place until the invites are read, when they could not be, or when its address names no invite of
// the reader's; the summary that says why the reader may not create invites, in the words of the server's refusal for
// that cause (the list gives the cause by its name alone); the lines that say what an invite allows now, as the sign-up
// page takes it (one the server says is usable does not work while it takes no new account), and how often it was
// used; the invite's link, shown to copy; and its state as a section of a screen.

import {
  inviteStateNow,
  useOwnInvites,
  type InviteStateNow,
  type InvitesNotAllowedReason,
  type OwnInvite,
} from "@my/app-data";
import {
  Button,
  CopyBlock,
  FeedNotice,
  SettingsList,
  SettingsListRow,
  SettingsSection,
  StatusSummary,
  TextButton,
  TextLink,
  useLocale,
} from "@my/design-system";
import { copyText } from "@my/platform";
import { useEffect, useRef, useState } from "react";
import { stringsIn } from "../../locale";
import { useDates } from "../security/parts";

/** The address of the Invites entry, and of the screens under it. */
export const invitesPath = "/settings/invites";
export const invitesPaths = {
  /** Creating an invite. */
  create: `${invitesPath}/new`,
  /** An invite of the reader's. */
  invite: (id: string) => `${invitesPath}/${encodeURIComponent(id)}`,
} as const;

/** The name of the screen that creates an invite, the title of its page. */
export function useInviteCreateTitle(): string {
  const { t } = stringsIn(useLocale().locale);
  return t("settings.invites.create");
}

/**
 * In the sections' place, as a screen's `notice`: that the invites load, that they could not be read, with the way to
 * try again, or that the address names no invite of the reader's, with the way to the list.
 */
export function Unread({ status, retry }: { status: "loading" | "failed" | "missing"; retry: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  if (status === "missing")
    return (
      <FeedNotice alert action={<TextLink href={invitesPath}>{t("settings.invites.missing.all")}</TextLink>}>
        {t("settings.invites.missing")}
      </FeedNotice>
    );
  return status === "failed" ? (
    <FeedNotice alert action={<TextButton onPress={retry}>{t("settings.retry")}</TextButton>}>
      {t("settings.failed")}
    </FeedNotice>
  ) : (
    <FeedNotice>{t("settings.loading")}</FeedNotice>
  );
}

/**
 * At a screen's top: that the reader may not create invites now, and why (`reason`; none names no cause). `focus`, it
 * has just taken the place of the form whose button the reader pressed (the server refused the invite): its title
 * takes the focus as it comes, so that the focus does not fall to the page and a screen reader reads why.
 */
export function CannotInvite({
  reason,
  focus = false,
}: {
  reason: InvitesNotAllowedReason | undefined;
  focus?: boolean;
}) {
  const { t } = stringsIn(useLocale().locale);
  const title = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    if (focus) title.current?.focus();
  }, [focus]);
  return (
    <StatusSummary
      icon="caution"
      title={t("settings.invites.cannot")}
      titleRef={focus ? title : undefined}
      about={t("settings.invites.cannot.reason", { reason: reason ?? "other" })}
    />
  );
}

/** What is known of an invite, as `useInviteFacts` says it. */
export type InviteFacts = {
  /** What it allows a person who would sign up with it now (`inviteStateNow`). */
  now: InviteStateNow;
  /** Whether someone can sign up with it now. */
  works: boolean;
  /** What it allows now, with the hour it ends or ended where that is what the state turns on. */
  state: string;
  /** How often it was used out of how many, and, for one that does not work for now, when it ends. */
  more: readonly string[];
};

/**
 * What is known of an invite, each a short line: what it allows now (`state`), how often it was used out of how many,
 * and, for one that does not work for now (while the reader may not create invites, or while the server takes no new
 * account, when the server still says `usable` but the sign-up page takes no invite), when it ends.
 */
export function useInviteFacts(): (invite: OwnInvite) => InviteFacts {
  const { t } = stringsIn(useLocale().locale);
  const { hour } = useDates();
  const { registrationClosed } = useOwnInvites();
  return (invite) => {
    const date = hour(invite.expires_at);
    const now = inviteStateNow(invite, registrationClosed);
    return {
      now,
      works: now === "usable",
      state: t("settings.invites.state", { state: now, date }),
      more: [
        t("settings.invites.uses", { uses: invite.uses, max: invite.max_uses }),
        ...(now === "creator_ineligible" || now === "registration_closed"
          ? [t("settings.invites.ends", { date })]
          : []),
      ],
    };
  };
}

/**
 * The link of an invite that works, shown to copy, with the button that copies it, the screen's primary action, and
 * what came of copying; under the line that says what to do with it, unless the screen says that before (`about`
 * false: a summary that the invite was just created).
 */
export function InviteLink({ invite, about = true }: { invite: OwnInvite; about?: boolean }) {
  const { t } = stringsIn(useLocale().locale);
  const [copied, setCopied] = useState<boolean>();
  return (
    <SettingsSection
      heading={t("settings.invites.link")}
      about={about ? t("settings.invites.link.about", { code: invite.code }) : undefined}
    >
      <CopyBlock
        label={t("settings.invites.link")}
        values={[invite.url]}
        action={
          <Button
            kind="primary"
            size="timeline"
            onPress={() => {
              void copyText(invite.url).then(setCopied);
            }}
          >
            {t("settings.invites.copy")}
          </Button>
        }
        result={copied === undefined ? undefined : t(copied ? "settings.copy.copied" : "settings.copy.failed")}
      />
    </SettingsSection>
  );
}

/** What an invite allows now, and how often it was used, as a section of a screen. */
export function InviteStatus({ invite }: { invite: OwnInvite }) {
  const { t } = stringsIn(useLocale().locale);
  const { state, more } = useInviteFacts()(invite);
  return (
    <SettingsSection heading={t("settings.invites.status")}>
      <SettingsList label={t("settings.invites.status")}>
        <SettingsListRow icon="invite" title={state} facts={more} />
      </SettingsList>
    </SettingsSection>
  );
}
