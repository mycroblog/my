// SPDX-License-Identifier: AGPL-3.0-only

// The auth screens as the auth entry shows them (ADR-0090 §16), at /app/auth on the catalog's dev server: the screens
// of @my/app-features, each in each of its states, rendered from the page data of @my/e2e's fake server, one
// full page after another under a catalog label whose id names it (`auth-<screen>-<state>`, for `web:shot --section`).
// `?screen=<id>` keeps one screen's states, `&state=<id>` one state alone. The language is the page's (`?lang=en|ja`),
// the scheme the page's. No refusal takes the focus here, where many show at once; the consent's wait runs as on its
// page.

import { AuthScreen } from "@my/app-features";
import { useLocale } from "@my/design-system";
import type { CSSProperties } from "react";
import wordmarkDark from "../../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../../brand/Wordmark-Primary-Color.svg";
import { authScreens as samples, type AuthScreenStates as SampleScreen } from "@my/e2e/fake";

const params = new URLSearchParams(location.search);
const onlyScreen = params.get("screen");
const onlyState = params.get("state");

const href = (screen?: string) => {
  const next = new URLSearchParams(location.search);
  next.delete("state");
  if (screen) next.set("screen", screen);
  else next.delete("screen");
  return `?${next.toString()}`;
};

const chrome: CSSProperties = {
  background: "var(--color-surface)",
  color: "var(--color-muted-text)",
  fontFamily: "var(--font-sans)",
  fontSize: "var(--text-caption)",
};
const index: CSSProperties = {
  ...chrome,
  display: "flex",
  flexWrap: "wrap",
  gap: "0.25rem 0.75rem",
  padding: "0.75rem 1rem",
};
const label: CSSProperties = {
  ...chrome,
  borderBlockStart: "1px dashed var(--color-muted-text)",
  fontFamily: "ui-monospace, monospace",
  fontWeight: 400,
  margin: 0,
  padding: "0.25rem 1rem",
};

export function AuthPreview() {
  const { locale } = useLocale();
  const screens = samples(locale.startsWith("ja") ? "ja" : "en");
  const shown = screens
    .filter((screen) => !onlyScreen || screen.id === onlyScreen)
    .map((screen) => ({ ...screen, states: screen.states.filter((state) => !onlyState || state.id === onlyState) }));
  return (
    <>
      <nav style={index} aria-label="Auth screens">
        <a style={{ color: "inherit" }} href={href()} aria-current={onlyScreen ? undefined : "page"}>
          All
        </a>
        {screens.map((screen) => (
          <a
            key={screen.id}
            style={{ color: "inherit" }}
            href={href(screen.id)}
            aria-current={screen.id === onlyScreen ? "page" : undefined}
          >
            {screen.name} <span>{screen.states.length}</span>
          </a>
        ))}
      </nav>
      <Specimens shown={shown} />
    </>
  );
}

function Specimens({ shown }: { shown: SampleScreen[] }) {
  return shown.map((screen) =>
    screen.states.map((state) => {
      const id = `auth-${screen.id}-${state.id}`;
      return (
        <section key={id} aria-labelledby={id}>
          <h2 id={id} style={label}>
            <a
              style={{ color: "inherit" }}
              href={`?${new URLSearchParams({ ...Object.fromEntries(params), screen: screen.id, state: state.id }).toString()}`}
            >
              {screen.name} · {state.name}
            </a>
          </h2>
          <AuthScreen page={state.page} wordmark={{ light: wordmarkLight, dark: wordmarkDark }} preview />
        </section>
      );
    }),
  );
}
