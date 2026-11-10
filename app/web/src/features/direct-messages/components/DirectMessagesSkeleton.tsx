// SPDX-License-Identifier: AGPL-3.0-only
import { NotificationRowSkeleton, Stack, VisuallyHidden } from "@my/ui";
import { useT } from "../../../lib/i18n";

/** Placeholder rows, for as long as the first page of conversations loads. */
const placeholders = ["first", "second", "third"] as const;

/**
 * What the direct messages show while their first page loads: a busy region with the shape of a few conversations
 * (an avatar, a line of names and an excerpt, as a notification's row has; hidden from assistive technology) and one
 * visually hidden text that names what is loading (web-client/16-look-states-and-wording.md G15).
 */
export function DirectMessagesSkeleton() {
  const t = useT();
  return (
    <Stack aria-busy="true">
      <VisuallyHidden>{t("client:direct_messages.loading")}</VisuallyHidden>
      {placeholders.map((key) => (
        <NotificationRowSkeleton key={key} />
      ))}
    </Stack>
  );
}
