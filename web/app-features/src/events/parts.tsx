// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of events share: their strings, the way a list asks for its next page or says it could not, and
// the line that says a list loads, failed or is empty.

import type { PagedList } from "@my/app-data";
import { Box, Button, FeedNotice, TextButton, useLocale } from "@my/design-system";
import type { ReactNode } from "react";
import { stringsIn } from "../locale";

export type T = ReturnType<typeof stringsIn>["t"];

/** The strings of the screens of events, in the reader's locale. */
export function useEventStrings(): T {
  return stringsIn(useLocale().locale).t;
}

/** The button that tries again what failed. */
export function Retry({ onPress, t }: { onPress: () => void; t: T }) {
  return <TextButton onPress={onPress}>{t("events.retry")}</TextButton>;
}

/**
 * Under a list that pages: Show more while there is a next page, that it loads, or that it could not, with Try again
 * in the button's place, so that the focus stays on it.
 */
export function MoreOf({ list, t }: { list: PagedList<unknown>; t: T }): ReactNode {
  if (list.failedNextPage)
    return (
      <FeedNotice alert start action={<Retry onPress={list.fetchNextPage} t={t} />}>
        {t("events.failed_more")}
      </FeedNotice>
    );
  if (!list.hasNextPage) return null;
  return (
    <Box>
      <Button size="timeline" touch busy={list.isFetchingNextPage} onPress={list.fetchNextPage}>
        {t("events.more")}
      </Button>
    </Box>
  );
}
