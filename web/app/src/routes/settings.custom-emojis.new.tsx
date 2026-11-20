// SPDX-License-Identifier: AGPL-3.0-only

// A request for a new custom emoji, at /settings/custom-emojis/new, a screen under the settings' Custom emoji: a
// shortcode and an image, or why the month's limit allows none. Back, in its head, goes to Custom emoji, as sending
// does. Without a session it asks the reader to sign in.

import { EmojiRequestScreen, loadCustomEmojiSettings, useEmojiRequestTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/custom-emojis/new")({
  loader: ({ context }) => screenLoad(context, loadCustomEmojiSettings),
  component: RequestScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function RequestScreen() {
  const { data } = Route.useRouteContext();
  return (
    <SettingsUnder
      entry="custom-emojis"
      title={useEmojiRequestTitle()}
      signedIn={data !== null}
      Screen={EmojiRequestScreen}
    />
  );
}
