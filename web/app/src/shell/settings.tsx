// SPDX-License-Identifier: AGPL-3.0-only

// The settings in the app's shell (ADR-0110 §5): which entry a screen of the settings is of, the address that asks
// whether to sign out, and the menu in the column beside the main one, which takes the place of what is around the
// reader while a screen of the settings is open: from the frame's `wide` the reader moves between the entries there,
// and under it goes back to the menu at /settings by the screen's head.

import type { Instance } from "@my/app-data";
import { SettingsBeside } from "@my/app-features";
import { FrameAside } from "@my/design-system";
import { useStrings } from "../locale";
import { ServerFooter } from "./around";

/**
 * The entry the screen at `pathname` is of or under (/settings/security/password is under security); undefined at the
 * menu itself and outside the settings.
 */
export const settingsEntryAt = (pathname: string): string | undefined => /^\/settings\/([^/]+)/.exec(pathname)?.[1];

/** The address that asks, over the screen of the settings at `pathname`, whether to sign out (routes/settings.tsx). */
export const signOutHref = (pathname: string): string => `${pathname}?sign_out=true`;

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
