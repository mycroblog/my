// SPDX-License-Identifier: AGPL-3.0-only
// The locale and time zone the app is displayed in, which can change while the page is open (FR-007):
// the page starts in the locale it was last displayed in on this browser, or else in the instance's locale from
// <html lang>, then follows the signed-in user's preferences. Starting from the last locale keeps the user's
// language while their preferences cannot be read (a 429 of the rate limit, say) instead of the instance's.
// It lives outside React, as an external store (react.dev: useSyncExternalStore), because the API client
// reads it for every request (Accept-Language, localized errors) and the shell renders the providers from it.
import { createContext, type ReactNode, use, useSyncExternalStore } from "react";
import {
  applyDocumentLocale,
  createTranslate,
  firstSupportedLocale,
  type I18n,
  loadI18n,
  resolveLocale,
  sourceLocale,
  type Translate,
} from "./i18n";
import { localeKey } from "./storage";

/** What the app is displayed in. Replaced, never changed, so that it can be a useSyncExternalStore snapshot. */
export type DisplaySettings = {
  i18n: I18n;
  /** The IANA time zone dates are shown in; undefined uses the browser's. */
  timeZone: string | undefined;
};

export type AppLocaleOptions = {
  /** The document whose <html lang dir> follows the locale; none in tests that only format messages. */
  doc?: Document;
  /** Whether a locale change is remembered for the next start (`startLocale`); the app's store does, tests mostly not. */
  remember?: boolean;
};

/**
 * The locale a page starts in: the one the app was last displayed in on this browser (lib/storage's localeKey,
 * which outlives the session like <html lang>; the next user's preferences replace it once they load), or else
 * the instance's locale that the server wrote into <html lang>.
 */
function startLocale(doc: Document): string {
  return localeKey.read() ?? doc.documentElement.lang;
}

/** Loads the messages of the supported locale closest to `startLocale(doc)` and sets <html lang dir> to it. */
export async function loadStartI18n(doc: Document): Promise<I18n> {
  // The app replaces <html lang>, and following the browser falls back to the instance's locale (AppLocale.browserLocale).
  doc.documentElement.dataset["instanceLocale"] = doc.documentElement.lang;
  const i18n = await loadI18n(startLocale(doc));
  applyDocumentLocale(doc, i18n.locale);
  return i18n;
}

function rememberLocale(locale: string): void {
  localeKey.write(locale);
}

export class AppLocale {
  #settings: DisplaySettings;
  #t: Translate;
  readonly #doc: Document | undefined;
  readonly #remember: boolean;
  readonly #listeners = new Set<() => void>();
  /** Counts the locale changes asked for, so that only the latest one is applied when catalogs load out of order. */
  #requests = 0;

  constructor(i18n: I18n, { doc, remember = false }: AppLocaleOptions = {}) {
    this.#settings = { i18n, timeZone: undefined };
    this.#t = createTranslate(i18n);
    this.#doc = doc;
    this.#remember = remember;
  }

  /** The current settings (the getSnapshot of useSyncExternalStore). */
  readonly getSnapshot = (): DisplaySettings => this.#settings;

  /** The locale the app is rendered in, for Accept-Language. */
  get locale(): string {
    return this.#settings.i18n.locale;
  }

  /**
   * The locale for a user who follows the browser (FR-007): the first of `languages`, the browser's in its order of
   * preference, that the app has a catalog for, or else the instance's.
   */
  browserLocale(languages: readonly string[]): string {
    const instance = this.#doc?.documentElement.dataset["instanceLocale"] || sourceLocale;
    return firstSupportedLocale(languages) ?? resolveLocale(instance);
  }

  /** Formats the client's messages in the current locale, for code outside React. */
  get t(): Translate {
    return this.#t;
  }

  /** Calls `listener` whenever the settings change (the subscribe of useSyncExternalStore). */
  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  /**
   * Displays the app in the supported locale closest to `requested` (ja-JP → ja), once its catalog has
   * loaded, and sets <html lang dir> to it. A later call wins over an earlier one still loading.
   */
  async setLocale(requested: string): Promise<void> {
    const request = ++this.#requests;
    if (resolveLocale(requested) === this.locale) {
      if (this.#remember) rememberLocale(this.locale);
      return;
    }
    const i18n = await loadI18n(requested);
    if (request !== this.#requests || i18n.locale === this.locale) return;
    if (this.#remember) rememberLocale(i18n.locale);
    if (this.#doc !== undefined) applyDocumentLocale(this.#doc, i18n.locale);
    this.#t = createTranslate(i18n);
    this.#set({ ...this.#settings, i18n });
  }

  /** Shows dates in `timeZone` (an IANA name); undefined uses the browser's. */
  setTimeZone(timeZone: string | undefined): void {
    if (timeZone === this.#settings.timeZone) return;
    this.#set({ ...this.#settings, timeZone });
  }

  #set(settings: DisplaySettings): void {
    this.#settings = settings;
    for (const listener of this.#listeners) listener();
  }
}

const AppLocaleContext = createContext<AppLocale | null>(null);

export type AppLocaleProviderProps = { locale: AppLocale; children: ReactNode };

/** Gives `useAppLocale()` and `useDisplaySettings()` to its children. */
export function AppLocaleProvider({ locale, children }: AppLocaleProviderProps) {
  return <AppLocaleContext value={locale}>{children}</AppLocaleContext>;
}

/** Returns the app's locale store, for changing the locale or the time zone. */
export function useAppLocale(): AppLocale {
  const locale = use(AppLocaleContext);
  if (locale === null) throw new Error("useAppLocale needs an <AppLocaleProvider> above it");
  return locale;
}

/** Returns what the app is displayed in, and renders again when it changes. */
export function useDisplaySettings(): DisplaySettings {
  const locale = useAppLocale();
  return useSyncExternalStore(locale.subscribe, locale.getSnapshot);
}
