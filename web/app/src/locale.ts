// SPDX-License-Identifier: AGPL-3.0-only

// The app's own strings (ADR-0009 §9): its catalog, web/app/locales/, compiled by @my/i18n with Lingui into
// messages.gen.ts (scripts/task.sh i18n:compile), read in the reader's locale from the I18nProvider that the entry
// places; a locale the catalog does not have reads en.

import type { Messages } from "@lingui/core";
import { catalogReader, useLocale } from "@my/design-system";
import { type MessageId, messages } from "./messages.gen";

const stringsIn = catalogReader<MessageId>(messages satisfies Readonly<Record<string, Messages>>);

/** The app's strings in the reader's locale. */
export const useStrings = () => stringsIn(useLocale().locale).t;
