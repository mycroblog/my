// SPDX-License-Identifier: AGPL-3.0-only

// An invite of the reader's, the screen under Invites at /settings/invites/{id} (ADR-0106 §18), titled by its code:
//
// - Invite link, while the invite is usable: its link, to copy, and that its code works on the sign-up page too.
// - Status: what it allows now and how often it was used out of how many.
// - Revoke invite, while it works or would work again (a reader who may not create invites now may again): after a
//   confirmation that says its link and its code stop working, for good; revoked, the screen goes back to Invites.
//
// The API reads no invite alone: the screen finds it in the list (app-data's `useOwnInviteRead`). An address that
// names no invite of the reader's says so in the sections' place, with the way to the list.

import { useOwnInviteRead, useOwnInviteWrites } from "@my/app-data";
import {
  BackButton,
  Button,
  ModalPresence,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  useLocale,
} from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../../locale";
import type { SettingsSubProps } from "../entries";
import { Asking } from "../security/parts";
import { InviteLink, InviteStatus, Unread } from "./parts";

/** What the screen of one invite is given by the route that places it, beside what every screen under an entry is. */
export type InviteScreenProps = {
  /** The ID of the invite, from the address. */
  id: string;
};

/** The code of the reader's invite of `id`, the title of its page; the list's name until the invite is read. */
export function useOwnInviteTitle(id: string): string {
  const { t } = stringsIn(useLocale().locale);
  return useOwnInviteRead(id).invite?.code ?? t("settings.invites.own");
}

export function OwnInviteScreen({ id, onBack }: SettingsSubProps & InviteScreenProps) {
  const { t } = stringsIn(useLocale().locale);
  const { invite, status, retry } = useOwnInviteRead(id);
  const { revoke } = useOwnInviteWrites();
  const [asking, setAsking] = useState(false);
  const back = <BackButton onPress={onBack}>{t("settings.entry.invites")}</BackButton>;
  if (invite === undefined)
    return (
      <SettingsScreen
        sub
        title={t("settings.invites.own")}
        back={back}
        notice={<Unread status={status === "read" ? "loading" : status} retry={retry} />}
      />
    );

  const usable = invite.state === "usable";
  return (
    <>
      <SettingsScreen sub title={invite.code} back={back}>
        {usable && <InviteLink invite={invite} />}
        <InviteStatus invite={invite} />
        {(usable || invite.state === "creator_ineligible") && (
          <SettingsSection heading={t("settings.invites.revoke.heading")}>
            <SettingsList label={t("settings.invites.revoke.heading")}>
              <SettingsListRow
                title={t("settings.invites.revoke.row")}
                caption={t("settings.invites.revoke.about")}
                end={
                  <Button
                    size="timeline"
                    onPress={() => {
                      setAsking(true);
                    }}
                  >
                    {t("settings.invites.revoke")}
                  </Button>
                }
              />
            </SettingsList>
          </SettingsSection>
        )}
      </SettingsScreen>
      <ModalPresence>
        {asking && (
          <Asking
            title={t("settings.invites.revoke.title", { code: invite.code })}
            confirm={t("settings.invites.revoke")}
            run={() => revoke(invite.id).then(onBack)}
            onClose={() => {
              setAsking(false);
            }}
          >
            {t("settings.invites.revoke.about")}
          </Asking>
        )}
      </ModalPresence>
    </>
  );
}
