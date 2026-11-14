// SPDX-License-Identifier: AGPL-3.0-only

// The frame of a rough screen (#3047) on the catalog: the app's shell (@my/app), with the place of the screen marked in
// its navigation, around the screen's rough, under a catalog strip that names the screen's states and switches
// between them (`?state=`). The places link to the catalog's screens, the page's language kept. What the rough holds
// is the catalog's own, in screens.css; the parts it holds are the real ones.

import "./screens.css";
import { keymap, Shell } from "@my/app";
import { moveFocus } from "@my/app-components";
import { FrameHeading, NavigationProvider, ShortcutProvider, useAction, useLocale } from "@my/design-system";
import { namesCommandKey } from "@my/platform";
import { useEffect, type ReactNode } from "react";
import { stringsOf, type Strings } from "./strings";

const config = { apiOrigin: "https://my.example", instanceName: "my.example", defaultLocale: "en", clientId: "my-web" };

// The app's places, as the catalog's screens; the others stay the app's, which the catalog has not.
const screens: Readonly<Record<string, string>> = {
  "/": "/app/timeline",
  "/notifications": "/app/notifications",
  "/profile": "/app/profile",
};

const params = new URLSearchParams(location.search);

/** The catalog's address of an app's link, with the page's language. */
export const catalogHref = (href: string) => {
  const lang = params.get("lang");
  return `${screens[href] ?? href}${lang ? `?lang=${lang}` : ""}`;
};

/** The state the address asks for, else the first. */
export const stateOf = <S extends string>(states: readonly S[]): S => {
  const asked = params.get("state");
  return states.find((s) => s === asked) ?? (states[0] as S);
};

/** The reader's language among the samples': ja, else en. */
export function useLang(): { lang: "en" | "ja"; s: Strings } {
  const lang = useLocale().locale.startsWith("ja") ? "ja" : "en";
  return { lang, s: stringsOf(lang) };
}

/** The keys of the next and the previous entry, as a column has them, inside the screen's feed. */
export function Moves() {
  useAction("timeline.next", ({ repeat }) => moveFocus(document.activeElement, 1, { repeat }), {
    scope: "focus",
    label: "Next",
  });
  useAction("timeline.previous", ({ repeat }) => moveFocus(document.activeElement, -1, { repeat }), {
    scope: "focus",
    label: "Previous",
  });
  return null;
}

/** The catalog's strip of the screen's states: not a part of the app. */
function States<S extends string>({ states, current }: { states: readonly { id: S; name: string }[]; current: S }) {
  if (states.length < 2) return null;
  return (
    <nav className="sc-states" aria-label="Catalog: states">
      {states.map(({ id, name }) => {
        const next = new URLSearchParams(location.search);
        next.set("state", id);
        return (
          <a key={id} href={`?${next.toString()}`} aria-current={id === current ? "page" : undefined}>
            {name}
          </a>
        );
      })}
    </nav>
  );
}

export function ScreenPreview<S extends string>({
  place,
  title,
  states = [],
  current,
  children,
}: {
  /** The place of the navigation the screen belongs to, as the app's path; none for a post. */
  place: string;
  /** The screen's h1, for assistive technology, and the document's title. */
  title: string;
  states?: readonly { id: S; name: string }[];
  current?: S;
  children: ReactNode;
}) {
  useEffect(() => {
    document.title = title;
  }, [title]);
  return (
    <ShortcutProvider keymap={keymap} mac={namesCommandKey()}>
      <NavigationProvider
        navigate={(href) => {
          location.assign(catalogHref(href));
        }}
        useHref={catalogHref}
      >
        {current !== undefined && <States states={states} current={current} />}
        <Shell pathname={place} config={config}>
          <FrameHeading>{title}</FrameHeading>
          {children}
        </Shell>
      </NavigationProvider>
    </ShortcutProvider>
  );
}
