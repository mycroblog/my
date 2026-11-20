// SPDX-License-Identifier: AGPL-3.0-only

// The reader's latest notifications, compact, for the column beside the main one (ADR-0086 §18): the newest few of
// the same list the notifications' screen reads (its cache shared), each its kind's glyph and sentence, a link to where
// it leads, when it came, and its unread mark against the read marker; following one reads it. While they load, when
// there are none, and when they failed (with a try again), one line in a caption's size says so. It never asks for Web
// Push.

import { notificationPath } from "@my/app-components";
import { useNotificationReads, useNotifications } from "@my/app-data";
import { RelativeTime, SectionLine, SectionList, SectionNote, TextButton, useLocale } from "@my/design-system";
import { stringsIn } from "../locale";
import { catalog } from "../messages.gen";
import { glyphOf, sentenceOf } from "./kinds";

const known = (id: string) => id in catalog.source;

/** The reader's `count` latest notifications; `instance` is the server's name, which the notices of the server say. */
export function LatestNotifications({ instance, count = 5 }: { instance: string; count?: number }) {
  const { t } = stringsIn(useLocale().locale);
  const { items, status, refetch } = useNotifications("all");
  const reads = useNotificationReads();
  if (status === "pending") return <SectionNote>{t("notifications.loading")}</SectionNote>;
  if (status === "error")
    return (
      <SectionNote action={<TextButton onPress={refetch}>{t("notifications.retry")}</TextButton>}>
        {t("notifications.failed")}
      </SectionNote>
    );
  if (items.length === 0) return <SectionNote>{t("notifications.empty")}</SectionNote>;
  return (
    <SectionList>
      {items.slice(0, count).map((n) => {
        const unread = reads.unread(n.id);
        return (
          <SectionLine
            key={n.id}
            href={notificationPath(n)}
            icon={glyphOf(n.type)}
            title={sentenceOf(n, t, instance, known)}
            caption={<RelativeTime date={n.created_at} />}
            unread={unread ? t("notifications.unread") : undefined}
            onPress={() => {
              reads.read(n.id);
            }}
          />
        );
      })}
    </SectionList>
  );
}
