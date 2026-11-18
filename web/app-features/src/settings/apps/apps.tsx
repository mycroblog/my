// SPDX-License-Identifier: AGPL-3.0-only

// Apps, the entry of the settings at /settings/apps (FR-003, ADR-0090 §1, §5): the apps the account authorized, and
// the apps its reader registered.
//
// - Authorized apps: each by its name, where it comes from in the consent page's words, and when it last used the
//   account, a fact a line; a row opens the app's screen (authorized.tsx), which holds what it was granted and the
//   way to revoke it. The server's own web client is one of them.
// - Your apps: each by its name, its type and the day it was registered, and that the administrators disabled it where
//   they did; a row opens the app's screen (app.tsx), and Register an app the screen that registers one
//   (register.tsx). Where the server's administrators turned registration off, which the list itself says, the
//   section says so in the button's place, and the apps registered before stay listed.

import {
  loadApps,
  useAppsRead,
  useAuthorizedApps,
  useOAuthAppRegistrationOff,
  useOAuthApps,
  type AppData,
} from "@my/app-data";
import {
  LinkButton,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsNote,
  SettingsScreen,
  SettingsSection,
  useLocale,
} from "@my/design-system";
import { stringsIn } from "../../locale";
import type { SettingsEntryProps } from "../entries";
import { numberIn, useDates } from "../parts";
import { appsPaths, originOf, Unread } from "./parts";

/** What the route's loader waits for: the two lists, from the cache when it has them. */
export async function loadAppsSettings(data: AppData): Promise<void> {
  await loadApps(data);
}

/** Authorized apps: each with where it comes from and when it last used the account, leading to its screen. */
function Authorized() {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  const { hour } = useDates();
  const apps = useAuthorizedApps() ?? [];
  return (
    <SettingsSection
      heading={t("settings.apps.authorized")}
      count={apps.length === 0 ? undefined : numberIn(locale, apps.length)}
      about={t("settings.apps.authorized.about")}
    >
      {apps.length === 0 ? (
        <SettingsEmpty>{t("settings.apps.authorized.none")}</SettingsEmpty>
      ) : (
        <SettingsList label={t("settings.apps.authorized")}>
          {apps.map((app) => (
            <SettingsListRow
              key={app.id}
              title={app.name}
              facts={[
                originOf(app, t),
                app.last_used_at === undefined
                  ? t("settings.apps.authorized.unused")
                  : t("settings.last_used", { date: hour(app.last_used_at) }),
              ]}
              href={appsPaths.authorized(app.id)}
            />
          ))}
        </SettingsList>
      )}
    </SettingsSection>
  );
}

/** Your apps: each with its type and the day it was registered, leading to its screen, and the way to register one. */
function Own() {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  const { day } = useDates();
  const apps = useOAuthApps() ?? [];
  const off = useOAuthAppRegistrationOff();
  const register = (
    <LinkButton size="timeline" href={appsPaths.register}>
      {t("settings.apps.register")}
    </LinkButton>
  );
  return (
    <SettingsSection
      heading={t("settings.apps.own")}
      count={apps.length === 0 ? undefined : numberIn(locale, apps.length)}
      about={t("settings.apps.own.about")}
      // The empty list's place is the one that adds while there is none.
      action={off || apps.length === 0 ? undefined : register}
    >
      {off && <SettingsNote>{t("settings.apps.own.off")}</SettingsNote>}
      {apps.length > 0 ? (
        <SettingsList label={t("settings.apps.own")}>
          {apps.map((app) => (
            <SettingsListRow
              key={app.id}
              title={app.name}
              facts={[
                t("settings.apps.type", { type: app.client_type }),
                t("settings.apps.own.registered", { date: day(app.created_at) }),
              ]}
              state={app.disabled ? t("settings.apps.own.disabled") : undefined}
              href={appsPaths.app(app.id)}
            />
          ))}
        </SettingsList>
      ) : (
        !off && <SettingsEmpty action={register}>{t("settings.apps.own.none")}</SettingsEmpty>
      )}
    </SettingsSection>
  );
}

/** The screen of the entry: its two lists once they are read, and until then that they load or could not be. */
export function AppsSettings({ back }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  const { status, retry } = useAppsRead();
  const read = status === "read";
  return (
    <SettingsScreen
      icon="apps"
      title={t("settings.entry.apps")}
      back={back}
      notice={read ? undefined : <Unread status={status} retry={retry} />}
    >
      {read && <Authorized />}
      {read && <Own />}
    </SettingsScreen>
  );
}
