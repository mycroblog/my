// SPDX-License-Identifier: AGPL-3.0-only
// What the event pages show while their first data loads (web-client/16-look-states-and-wording.md G15): a busy region
// in the shape of what comes, hidden from assistive technology, named once by a visually hidden text.
import { FactsCardSkeleton, Skeleton, Stack, VisuallyHidden } from "@my/ui";
import { useT } from "../../../lib/i18n";

/** Placeholder cards, for as long as the first page of the list loads. */
const placeholders = ["first", "second", "third"] as const;

/** The list of events (`/events`) while its first page loads. */
export function EventListSkeleton() {
  const t = useT();
  return (
    <Stack gap="3" aria-busy="true">
      <VisuallyHidden>{t("client:events.loading")}</VisuallyHidden>
      {placeholders.map((key) => (
        <FactsCardSkeleton key={key} />
      ))}
    </Stack>
  );
}

/** The page of an event, or its editor, while the event loads. */
export function EventPageSkeleton() {
  const t = useT();
  return (
    <Stack gap="4" aria-busy="true">
      <VisuallyHidden>{t("client:events.loading")}</VisuallyHidden>
      <Skeleton shape="rect" />
      <FactsCardSkeleton />
    </Stack>
  );
}
