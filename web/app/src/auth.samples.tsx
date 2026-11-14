// SPDX-License-Identifier: AGPL-3.0-only

// The auth screens as the auth entry shows them (ADR-0090 §16), for their stories: a screen of @my/app-features in one
// of its states, rendered from the page data of @my/e2e's fake server, a full page. A story per state, a file per
// screen (auth-*.stories.tsx); the language is the toolbar's locale (ja, else en). No refusal takes the focus here; the
// consent's wait runs as on its page.

import { AuthScreen } from "@my/app-features";
import { useLocale } from "@my/design-system";
import { authScreens } from "@my/e2e/fake";
import type { StoryObj } from "@storybook/react-vite";
import wordmarkDark from "../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../brand/Wordmark-Primary-Color.svg";

/** One state of one auth screen, by their ids in @my/e2e's authScreens. */
function AuthState({ screen, state }: { screen: string; state: string }) {
  const { locale } = useLocale();
  const page = authScreens(locale.startsWith("ja") ? "ja" : "en")
    .find((s) => s.id === screen)
    ?.states.find((s) => s.id === state)?.page;
  if (page === undefined) throw new Error(`no state ${state} of the auth screen ${screen}`);
  return <AuthScreen page={page} wordmark={{ light: wordmarkLight, dark: wordmarkDark }} preview />;
}

/**
 * What every auth screen's story file shares: a full page, which places its own shortcuts. Tagged `themes`, the story
 * tests run it in dark too.
 */
export const authMeta = { tags: ["themes"], parameters: { screen: true, layout: "fullscreen" } };

/** The story of a state of an auth screen. */
export const authStory = (screen: string, state: string): StoryObj => ({
  render: () => <AuthState screen={screen} state={state} />,
});
