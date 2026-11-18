// SPDX-License-Identifier: AGPL-3.0-only

// The settings in the app's shell (ADR-0110 §5): which entry a screen of the settings is of, the address that asks
// whether to sign out, and the menu in the column beside the main one, which takes the place of what is around the
// reader while a screen of the settings is open: from the frame's `wide` the reader moves between the entries there,
// and under it goes back to the menu at /settings by the screen's head. Also what a screen of the settings is given of
// its address, and a screen under an entry in its place, which each such route renders.

import type { Instance } from "@my/app-data";
import { SettingsBeside, settingsEntryPath, type SettingsAddress, type SettingsSubProps } from "@my/app-features";
import { FrameAside } from "@my/design-system";
import { useLocation, useRouter } from "@tanstack/react-router";
import type { ComponentType } from "react";
import { useStrings } from "../locale";
import { ServerFooter } from "./around";
import { ScreenHeading } from "./heading";
import { SignedOut } from "./signed-out";

/**
 * The entry the screen at `pathname` is of or under (/settings/security/password is under security); undefined at the
 * menu itself and outside the settings.
 */
export const settingsEntryAt = (pathname: string): string | undefined => /^\/settings\/([^/]+)/.exec(pathname)?.[1];

/** The address that asks, over the screen of the settings at `pathname`, whether to sign out (routes/settings.tsx). */
export const signOutHref = (pathname: string): string => `${pathname}?sign_out=true`;

/**
 * What a screen of the settings reads of its address and of the app, for what leaves the app and comes back (a login
 * provider's page): the fragment of the address, the way to take it off once read, which keeps the screen and replaces
 * the address in the history, and the way out of the app.
 */
export function useSettingsAddress(): SettingsAddress {
  const router = useRouter();
  const fragment = useLocation({ select: (location) => location.hash });
  return {
    fragment,
    onFragmentRead: () => {
      router.history.replace(router.state.location.pathname);
    },
    leave: router.options.context.leave,
  };
}

/**
 * A screen under the entry at `entry` (/settings/security/password), under its name: for a signed-in reader the
 * screen, whose way back and whose end lead to the entry; without a session it asks the reader to sign in.
 */
export function SettingsUnder({
  entry,
  title,
  signedIn,
  Screen,
}: {
  entry: string;
  title: string;
  signedIn: boolean;
  Screen: ComponentType<SettingsSubProps>;
}) {
  const t = useStrings();
  const router = useRouter();
  const address = useSettingsAddress();
  if (!signedIn)
    return (
      <>
        <ScreenHeading>{t("app.settings")}</ScreenHeading>
        <SignedOut />
      </>
    );
  return (
    <>
      <ScreenHeading>{title}</ScreenHeading>
      <Screen
        {...address}
        onBack={() => {
          void router.navigate({ href: settingsEntryPath(entry) });
        }}
      />
    </>
  );
}

/** The column beside a screen of the settings at `pathname`: the menu, its entry marked, and the server's pages. */
export function SettingsAside({ pathname, instance }: { pathname: string; instance: Instance | undefined }) {
  const t = useStrings();
  return (
    <FrameAside label={t("app.settings")}>
      <SettingsBeside current={settingsEntryAt(pathname)} signOutHref={signOutHref(pathname)} />
      <ServerFooter instance={instance} />
    </FrameAside>
  );
}
