// SPDX-License-Identifier: AGPL-3.0-only

// What the supporters' page shares among its parts: its strings, a day as the reader writes it, and the line under a
// list that pages.

import type { PagedList } from "@my/app-data";
import { Box, Button, dateFormat, FeedNotice, TextButton, useLocale, useTimeZone } from "@my/design-system";
import type { ReactNode } from "react";
import { stringsIn } from "../locale";

export type T = ReturnType<typeof stringsIn>["t"];

/** The strings of the supporters' page, in the reader's locale. */
export function useSupporterStrings(): T {
  return stringsIn(useLocale().locale).t;
}

/** The space a line does not break at (U+00A0), by its number: written out, it reads as any space. */
const noBreak = String.fromCharCode(0xa0);

/**
 * A day as the reader's language and time zone write it, with its year (when a period ends, when a change applies),
 * holding together: its spaces do not break.
 */
export function useDay(): (iso: string) => string {
  const { locale } = useLocale();
  const format = dateFormat(locale, useTimeZone(), { dateStyle: "medium" });
  return (iso) => format.format(new Date(iso)).replaceAll(" ", noBreak);
}

/** The button that tries again what failed. */
export function Retry({ onPress, t }: { onPress: () => void; t: T }) {
  return <TextButton onPress={onPress}>{t("supporters.retry")}</TextButton>;
}

/**
 * Under a list that pages: Show more while there is a next page, that it loads, or that it could not, with Try again
 * in the button's place, so that the focus stays on it.
 */
export function MoreOf({ list, t }: { list: PagedList<unknown>; t: T }): ReactNode {
  if (list.failedNextPage)
    return (
      <FeedNotice alert start action={<Retry onPress={list.fetchNextPage} t={t} />}>
        {t("supporters.list.failed_more")}
      </FeedNotice>
    );
  if (!list.hasNextPage) return null;
  return (
    <Box>
      <Button size="target" busy={list.isFetchingNextPage} onPress={list.fetchNextPage}>
        {t("supporters.list.more")}
      </Button>
    </Box>
  );
}
