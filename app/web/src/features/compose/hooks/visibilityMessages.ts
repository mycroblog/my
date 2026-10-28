// SPDX-License-Identifier: AGPL-3.0-only
// The one definition of the visibilities that the post form, the default of the preferences and the post cards
// share (ADR-0017): their order, their names and who can see each. The icon of each comes from @my/ui's VisibilityIcon.
import type { ClientMessageKey } from "../../../lib/i18n";
import type { Visibility } from "../api/compose";

/** The visibilities from the widest to the narrowest (ADR-0017). */
export const visibilities: readonly Visibility[] = ["public", "local", "unlisted", "private", "direct"];

/** Whether `value` (a radio group's string) is a visibility. */
export function isVisibility(value: string): value is Visibility {
  return (visibilities as readonly string[]).includes(value);
}

/** The name of each visibility. */
export const visibilityLabel: Readonly<Record<Visibility, ClientMessageKey>> = {
  public: "client:compose.visibility.public",
  local: "client:compose.visibility.local",
  unlisted: "client:compose.visibility.unlisted",
  private: "client:compose.visibility.private",
  direct: "client:compose.visibility.direct",
};

/** Who can see a post of each visibility (ADR-0017). */
export const visibilityDescription: Readonly<Record<Visibility, ClientMessageKey>> = {
  public: "client:compose.visibility.public.description",
  local: "client:compose.visibility.local.description",
  unlisted: "client:compose.visibility.unlisted.description",
  private: "client:compose.visibility.private.description",
  direct: "client:compose.visibility.direct.description",
};
