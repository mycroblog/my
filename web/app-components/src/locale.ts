// SPDX-License-Identifier: AGPL-3.0-only

// The package's own strings (ADR-0009 §9, ADR-0025 §4): its catalog, web/app-components/locales/, compiled by @my/i18n
// with Lingui into messages.gen.ts (scripts/task.sh i18n:compile), read the way the design system reads its own. A
// component reads the reader's locale from React Aria's I18nProvider, which the app places; a locale its catalog does
// not have reads en.

import type { Messages } from "@lingui/core";
import { catalogReader } from "@my/design-system";
import { type MessageId, messages } from "./messages.gen";

/** The package's strings in `locale`, by their ids, and the locale they are in, from Lingui's compiled messages. */
export const stringsIn = catalogReader<MessageId>(messages satisfies Readonly<Record<string, Messages>>);

// Chinese is named by its script, never by a region: zh-CN and zh-TW are Simplified and Traditional Chinese, as
// ADR-0009 §2 resolves them, and a name such as "Chinese (Taiwan)" takes a political side the app does not take.
// A bare "zh" states no script and stays plain Chinese.
function byScript(tag: string): string {
  const locale = new Intl.Locale(tag);
  if (locale.language !== "zh" || (locale.script === undefined && locale.region === undefined)) return tag;
  return `zh-${locale.maximize().script}`;
}

/** The name of the language `tag` in the language `locale` (English, 日本語, 繁體中文); none for a malformed tag. */
export function languageName(tag: string, locale: string): string | undefined {
  try {
    return new Intl.DisplayNames([locale], { type: "language", languageDisplay: "dialect" }).of(byScript(tag));
  } catch {
    return undefined;
  }
}
