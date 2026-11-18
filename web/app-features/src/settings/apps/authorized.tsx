// SPDX-License-Identifier: AGPL-3.0-only

// An app the account authorized, the screen under Apps at /settings/apps/authorized/{id} (FR-003, ADR-0090 §4, §5):
// where the app comes from, in the consent page's words, and the site it names; when the reader first authorized it
// and when it last used the account; what it was granted, each scope as the consent page words it, in that page's
// groups, the heaviest first; and Revoke, after a confirmation that says what stops working.
//
// An app that is not the server's own carries the statement that this server has not verified it, as the consent page
// does: its name and its site are its own claim. Revoking the server's own web client signs the account out of every
// browser, this one included: the row and the confirmation say so, and once it is revoked the app is left as after
// signing out. Any other app revoked, the screen goes back to Apps, whose list no longer holds it.

import { useAppsRead, useAuthorizedApps, useRevokeAuthorizedApp, type AuthorizedApp } from "@my/app-data";
import {
  BackButton,
  Button,
  ModalPresence,
  SettingsList,
  SettingsListRow,
  SettingsNote,
  SettingsScreen,
  SettingsSection,
  useLocale,
} from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../../locale";
import type { SettingsSubProps } from "../entries";
import { Asking, useDates } from "../security/parts";
import { originOf, Unread, type AppScreenProps } from "./parts";
import { groupedScopes } from "./scopes";

/** The name of the authorized app of `id`, the title of its page; the list's name until the app is read. */
export function useAuthorizedAppTitle(id: string): string {
  const { t } = stringsIn(useLocale().locale);
  return useAuthorizedApps()?.find((app) => app.id === id)?.name ?? t("settings.apps.authorized");
}

export function AuthorizedAppScreen({ id, onBack, onSessionEnded }: SettingsSubProps & AppScreenProps) {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  const { day, hour } = useDates();
  const { status, retry } = useAppsRead();
  const listed = useAuthorizedApps()?.find((a) => a.id === id);
  const revoke = useRevokeAuthorizedApp();
  const [asking, setAsking] = useState(false);
  /** The app as it was when the reader asked to revoke it: the screen keeps it while it is being left. */
  const [kept, setKept] = useState<AuthorizedApp>();
  const app = listed ?? kept;
  const back = <BackButton onPress={onBack}>{t("settings.entry.apps")}</BackButton>;
  if (app === undefined)
    return (
      <SettingsScreen
        sub
        title={t("settings.apps.authorized")}
        back={back}
        notice={<Unread status={status === "read" ? "missing" : status} retry={retry} />}
      />
    );

  const own = app.first_party;
  return (
    <>
      <SettingsScreen sub title={app.name} back={back}>
        <SettingsSection heading={t("settings.apps.about")}>
          {!own && <SettingsNote>{t("settings.apps.unverified")}</SettingsNote>}
          <SettingsList label={t("settings.apps.about")}>
            <SettingsListRow icon="apps" title={originOf(app, t)} caption={app.website} />
            <SettingsListRow
              icon="time"
              title={t("settings.apps.authorized.since", { date: day(app.authorized_at) })}
              caption={
                app.last_used_at === undefined
                  ? t("settings.apps.authorized.unused")
                  : t("settings.apps.authorized.last_used", { date: hour(app.last_used_at) })
              }
            />
          </SettingsList>
        </SettingsSection>
        {groupedScopes(app.scope, t).map(({ risk, scopes }) => (
          <SettingsSection
            key={risk}
            heading={t("settings.apps.permissions", { risk })}
            count={new Intl.NumberFormat(locale).format(scopes.length)}
          >
            <SettingsList label={t("settings.apps.permissions", { risk })}>
              {scopes.map((scope) => (
                <SettingsListRow
                  key={scope.scope}
                  title={scope.text}
                  caption={scope.cautions.length === 0 ? undefined : scope.cautions.join(" · ")}
                />
              ))}
            </SettingsList>
          </SettingsSection>
        ))}
        <SettingsSection heading={t("settings.apps.revoke.heading")}>
          <SettingsList label={t("settings.apps.revoke.heading")}>
            <SettingsListRow
              title={t("settings.apps.revoke.row")}
              caption={t(own ? "settings.apps.revoke.about.web" : "settings.apps.revoke.about")}
              end={
                <Button
                  size="timeline"
                  onPress={() => {
                    setKept(app);
                    setAsking(true);
                  }}
                >
                  {t("settings.apps.revoke")}
                </Button>
              }
            />
          </SettingsList>
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>
        {asking && (
          <Asking
            title={t("settings.apps.revoke.title", { app: app.name })}
            confirm={t("settings.apps.revoke")}
            run={async () => {
              await revoke(app.id);
              // The server's own web client took this browser's session with it.
              if (own) onSessionEnded();
              else onBack();
            }}
            onClose={() => {
              setAsking(false);
            }}
          >
            {t(own ? "settings.apps.revoke.text.web" : "settings.apps.revoke.text")}
          </Asking>
        )}
      </ModalPresence>
    </>
  );
}
