// SPDX-License-Identifier: AGPL-3.0-only
// i18n of the web client (ADR-0009): the `client` catalogs from locales/, formatted by react-intl.
// Message IDs are "<namespace>:<key>"; their types come
// from the typed accessors that tools/i18n generates (scripts/task.sh i18n:gen-ts), so an unknown ID or a missing
// argument is a type error.
//
// The page starts in the server's locale: `my serve` writes the instance's default locale into <html lang>
// of the shell (ADR-0028). The signed-in user's own locale replaces it while the page is open (lib/locale).
import type { ReactNode } from "react";
import { createIntl, IntlProvider, useIntl } from "react-intl";
import catalogs from "virtual:locales";
import type { MessageArgs, MessageKey } from "../../../../tools/i18n/src/runtime/messages.gen";

/** The IDs of the client's messages; other namespaces are formatted by the server. */
export type ClientMessageKey = Extract<MessageKey, `client:${string}`>;

declare global {
  // react-intl's hook for typing message IDs (FormatJS docs, "TypeScript").
  // eslint-disable-next-line typescript/no-namespace -- react-intl reads the ID type from this global namespace.
  namespace FormatjsIntl {
    interface Message {
      ids: ClientMessageKey;
    }
  }
}

/** The locale every catalog is written in, and the last fallback (ADR-0009). */
const sourceLocale = "en";

/** A locale's client catalog in the FormatJS JSON catalog shape, merged from its area files (ADR-0082). */
type Catalog = Record<string, { defaultMessage: string }>;

/** A loader per locale that has a client catalog; each catalog is its own chunk of the build (vite-plugins/client-catalogs.ts). */
const catalogLoaders: ReadonlyMap<string, () => Promise<Catalog>> = new Map(
  Object.entries(catalogs).map(([locale, load]) => [locale, async () => (await load()).default]),
);

/** The locales the client has catalogs for. */
export const supportedLocales: readonly string[] = [...catalogLoaders.keys()].toSorted();

/** Pseudo-locales are formatted with the source locale's CLDR rules, as in tools/i18n and internal/i18n. */
const pseudoLocales: ReadonlySet<string> = new Set(["en-XA", "ar-XB"]);

/** Traditional Chinese never falls back to zh (parentLocales in internal/i18n). */
const parentLocales: Readonly<Record<string, string>> = {
  "zh-Hant": "",
  "zh-HK": "zh-Hant",
  "zh-MO": "zh-Hant",
  "zh-TW": "zh-Hant",
};

/** The locales `locale` falls back through, ending with the source locale: ja-JP → ja → en (ADR-0009 §2). */
export function fallbackChain(locale: string): string[] {
  const chain: string[] = [];
  let current = canonicalize(locale);
  while (current !== "") {
    chain.push(current);
    const parent = parentLocales[current];
    if (parent !== undefined) {
      current = parent;
      continue;
    }
    const cut = current.lastIndexOf("-");
    current = cut < 0 ? "" : current.slice(0, cut);
  }
  if (!chain.includes(sourceLocale)) chain.push(sourceLocale);
  return chain;
}

function canonicalize(locale: string): string {
  try {
    return Intl.getCanonicalLocales(locale)[0] ?? "";
  } catch {
    // Not a BCP 47 tag; the source locale is used.
    return "";
  }
}

/** The supported locale closest to `requested`: ja-JP → ja, fr → en. */
export function resolveLocale(requested: string): string {
  return fallbackChain(requested).find((locale) => catalogLoaders.has(locale)) ?? sourceLocale;
}

/** Languages written right to left (rtlLanguages in internal/i18n). */
const rtlLanguages: ReadonlySet<string> = new Set(["ar", "ckb", "dv", "fa", "he", "ps", "sd", "ug", "ur", "yi"]);

/** "rtl" or "ltr" for `locale`, as internal/i18n Direction computes <html dir> on the server. */
export function direction(locale: string): "ltr" | "rtl" {
  const canonical = canonicalize(locale);
  const [language = "", script = ""] = canonical.split("-");
  if (canonical === "ar-XB" || script.startsWith("Arab") || script.startsWith("Hebr")) return "rtl";
  return rtlLanguages.has(language) && !script.startsWith("Latn") ? "rtl" : "ltr";
}

/** A locale and its messages, keyed by message ID, with the fallback locales' messages filled in. */
export type I18n = { locale: string; messages: Readonly<Record<ClientMessageKey, string>> };

/** Loads the messages of the supported locale closest to `requested`. */
export async function loadI18n(requested: string): Promise<I18n> {
  const locale = resolveLocale(requested);
  const loaders = fallbackChain(locale).flatMap((l) => catalogLoaders.get(l) ?? []);
  const catalogs = await Promise.all(loaders.map((load) => load()));
  // The source locale comes last in the chain, so it is applied first and the requested locale wins.
  // The source catalog has every key (scripts/task.sh i18n:check), so the result does too.
  const messages: Record<string, string> = {};
  for (const catalog of catalogs.toReversed()) {
    for (const [key, entry] of Object.entries(catalog)) messages[`client:${key}`] = entry.defaultMessage;
  }
  return { locale, messages: messages as Record<ClientMessageKey, string> };
}

/** Sets lang and dir of <html> to the locale the page is rendered in. */
export function applyDocumentLocale(doc: Document, locale: string): void {
  doc.documentElement.lang = locale;
  doc.documentElement.dir = direction(locale);
}

export type MessagesProviderProps = {
  i18n: I18n;
  /** The IANA time zone that dates and times are formatted in; the browser's by default. */
  timeZone?: string | undefined;
  children: ReactNode;
};

/**
 * Provides react-intl. The app shell renders it together with the @my/ui I18nProvider (React Aria's
 * locale), which gets `i18n.locale` as it is: React Aria reads the direction from it, ar-XB included.
 */
export function MessagesProvider({ i18n, timeZone, children }: MessagesProviderProps) {
  return (
    <IntlProvider
      locale={formattingLocale(i18n.locale)}
      defaultLocale={sourceLocale}
      messages={i18n.messages}
      {...(timeZone === undefined ? {} : { timeZone })}
    >
      {children}
    </IntlProvider>
  );
}

/** The locale whose CLDR rules format messages of `locale`. */
function formattingLocale(locale: string): string {
  return pseudoLocales.has(locale) ? sourceLocale : locale;
}

type MessageValues<K extends ClientMessageKey> = keyof MessageArgs[K] extends never ? [] : [values: MessageArgs[K]];

/** A client message without arguments, which a value such as a route's static data can name on its own. */
export type PlainClientMessageKey = {
  [K in ClientMessageKey]: keyof MessageArgs[K] extends never ? K : never;
}[ClientMessageKey];

/** Formats a client message to a string, with its arguments type-checked: `t("client:shell.nav.home")`. */
export type Translate = <K extends ClientMessageKey>(id: K, ...values: MessageValues<K>) => string;

/** Returns `t` for the current locale. */
export function useT(): Translate {
  const intl = useIntl();
  return (id, ...[values]) => intl.formatMessage({ id }, values);
}

/** Returns the locale that formats values (numbers, language names) on the page. */
export function useLocale(): string {
  return useIntl().locale;
}

/**
 * Returns `t` for `i18n` outside React, for code that runs before or beside the providers (the API
 * client's localized errors). Components use `useT()`.
 */
export function createTranslate(i18n: I18n): Translate {
  const intl = createIntl({
    locale: formattingLocale(i18n.locale),
    defaultLocale: sourceLocale,
    messages: i18n.messages,
  });
  return (id, ...[values]) => intl.formatMessage({ id }, values);
}
