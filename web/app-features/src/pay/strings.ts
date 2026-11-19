// SPDX-License-Identifier: AGPL-3.0-only

// What every payment page shares: its strings, and whether what the server refused takes the focus.

import { useLocale } from "@my/design-system";
import { createContext } from "react";
import { stringsIn } from "../locale";

/** The payment pages' strings in the reader's locale. */
export const usePayStrings = () => stringsIn(useLocale().locale).t;

/**
 * Whether what the server refused takes the focus when the page shows: on the page itself, not where many pages show
 * at once (a story).
 */
export const FocusOnLoadContext = createContext(true);
