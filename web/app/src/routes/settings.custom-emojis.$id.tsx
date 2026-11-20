// SPDX-License-Identifier: AGPL-3.0-only

// One of the reader's custom emoji, at /settings/custom-emojis/{id}, a screen under the settings' Custom emoji: its
// image, its place in the picker, and Delete. Back, in its head, goes to Custom emoji. Without a session it asks the
// reader to sign in.

import { loadCustomEmojiSettings, OwnEmojiScreen, useOwnEmojiTitle, type SettingsSubProps } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/custom-emojis/$id")({
  loader: ({ context }) => screenLoad(context, loadCustomEmojiSettings),
  component: EmojiPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The screen of the emoji the address names. */
function Screen(props: SettingsSubProps) {
  const { id } = Route.useParams();
  return <OwnEmojiScreen key={id} id={id} {...props} />;
}

/** The screen under the emoji's shortcode, for a signed-in reader, whose emoji alone are read. */
function Titled() {
  const { id } = Route.useParams();
  return <SettingsUnder entry="custom-emojis" title={useOwnEmojiTitle(id)} signedIn Screen={Screen} />;
}

function EmojiPage() {
  const { data } = Route.useRouteContext();
  return data === null ? <SettingsUnder entry="custom-emojis" title="" signedIn={false} Screen={Screen} /> : <Titled />;
}
