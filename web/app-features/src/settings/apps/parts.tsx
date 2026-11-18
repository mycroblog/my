// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of the settings' Apps share, beside what every entry's do (../parts.tsx): their addresses; the
// words of the notice in a screen's sections' place when its address names no app of the reader's; a value of an app
// to copy, whose button names it; and the screen that shows what the server gives only once (a new app's secret, a
// token): what happened first, then each value under its own name, a secret under the plain statement that it is not
// shown again, with its Copy, and the way on at the screen's end.

import type { AuthorizedApp } from "@my/app-data";
import { Button, SettingsScreen, SettingsSection, StatusSummary, SubmitBar, useLocale } from "@my/design-system";
import { useRef, type ReactNode } from "react";
import { stringsIn } from "../../locale";
import { Kept as KeptValues, ReadingNotice, useFocusOnArrival } from "../parts";

type T = ReturnType<typeof stringsIn>["t"];

/** The address of the Apps entry, and of the screens under it. */
export const appsPath = "/settings/apps";
export const appsPaths = {
  /** Registering an app. */
  register: `${appsPath}/new`,
  /** An app the account authorized, by the ID of its client. */
  authorized: (id: string) => `${appsPath}/authorized/${encodeURIComponent(id)}`,
  /** An app the reader registered. */
  app: (id: string) => `${appsPath}/${encodeURIComponent(id)}`,
  /** Issuing a token of an app the reader registered. */
  token: (id: string) => `${appsPath}/${encodeURIComponent(id)}/token`,
} as const;

/** The name of a screen under Apps that is of no one app's name, the title of its page. */
export function useAppsTitle(screen: "register" | "token"): string {
  const { t } = stringsIn(useLocale().locale);
  return t(screen === "register" ? "settings.apps.register" : "settings.apps.token");
}

/**
 * Where an authorized app comes from, in the consent page's words: the server's own web client, the user who
 * registered it, the host of its metadata document, which the server verified, or that it registered itself.
 */
export function originOf(app: AuthorizedApp, t: T): string {
  const host = URL.canParse(app.client_id) ? new URL(app.client_id).host : app.client_id;
  const registration =
    app.registration === "user" && app.registered_by === undefined ? "user_unknown" : app.registration;
  return t("settings.apps.authorized.origin", { registration, owner: `@${app.registered_by ?? ""}`, host });
}

/** What a screen under Apps that shows one app is given by the route that places it, beside what every such screen is. */
export type AppScreenProps = {
  /** The ID of the app, from the address. */
  id: string;
};

/**
 * In the sections' place, as a screen's `notice`: that what the screen shows loads, that it could not be read, with
 * the way to try again, or that its address names no app of the reader's, with the way to the list.
 */
export function Unread({ status, retry }: { status: "loading" | "failed" | "missing"; retry: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  return (
    <ReadingNotice
      status={status}
      retry={retry}
      missing={{ said: t("settings.apps.missing"), all: t("settings.apps.missing.all"), href: appsPath }}
    />
  );
}

/** What a value to copy is, which names its button: two of them may stand on one screen. */
export type KeptWhat = "client_id" | "secret" | "access_token" | "refresh_token";

/** A value of an app that the reader copies and keeps, named `label`, under the button that names what it copies. */
export function Kept({ what, label, value }: { what: KeptWhat; label: string; value: string }) {
  const { t } = stringsIn(useLocale().locale);
  return <KeptValues label={label} values={[value]} copy={t("settings.apps.copy", { what })} />;
}

/**
 * The screen that shows what the server gives only once, in the place of the screen that asked for it: first what
 * happened (`said`), which takes the focus as the screen comes, and what it means (`about`), then each value in a section under its own name, a secret under the
 * statement that it is not shown again, right at it; a client ID, which the app's screen shows again, has none.
 * Finish ends the screen, which the reader presses once they kept what must be kept.
 */
export function Once({
  title,
  said,
  about,
  values,
  onDone,
}: {
  title: string;
  /** What happened, as a sentence without its full stop: it heads the screen. */
  said: string;
  about: string;
  values: readonly { what: KeptWhat; label: string; value: string }[];
  onDone: () => void;
}): ReactNode {
  const { t } = stringsIn(useLocale().locale);
  // It took another view's place, whose focus went with it (the form's button, the confirmation's): the sentence
  // that says what happened takes the focus, so that a screen reader reads it first.
  const happened = useRef<HTMLHeadingElement>(null);
  useFocusOnArrival(happened);
  return (
    <SettingsScreen
      sub
      title={title}
      foot={
        <SubmitBar>
          <Button kind="primary" onPress={onDone}>
            {t("settings.once.done")}
          </Button>
        </SubmitBar>
      }
    >
      <StatusSummary icon="accepted" title={said} titleRef={happened} about={about} />
      {values.map((value) => (
        <SettingsSection
          key={value.what}
          heading={value.label}
          about={value.what === "client_id" ? undefined : t("settings.apps.once.about")}
        >
          <Kept {...value} />
        </SettingsSection>
      ))}
    </SettingsScreen>
  );
}
