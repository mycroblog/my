// SPDX-License-Identifier: AGPL-3.0-only

// The features' strings (ADR-0009 §9): their catalog, web/app-features/locales/, compiled by @my/i18n with Lingui into
// messages.gen.ts (scripts/task.sh i18n:compile), read the way the design system reads its own, in the reader's
// locale from React Aria's I18nProvider, which the app places; a locale the catalog does not have reads en.

import type { Messages } from "@lingui/core";
import { catalogReader } from "@my/design-system";
import { type MessageId, messages } from "./messages.gen";

/** The features' strings in `locale`, by their ids, and the locale they are in. */
export const stringsIn = catalogReader<MessageId>(messages satisfies Readonly<Record<string, Messages>>);
