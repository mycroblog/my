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
