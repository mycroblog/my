// SPDX-License-Identifier: AGPL-3.0-only
// The panel of the third column from xl on the timeline pages (web-client/09-app-shell.md §9): the viewer's latest
// notifications, at most ten, as the rows of the notifications page on the panel's surface, and a link to that page.
// A notification from the stream joins it at once (api/live.ts). It is no live region: the navigation's unread badge
// says that one came, and rows read out as they come would interrupt the timeline. Showing the rows marks nothing read.
import { AsidePanel, Hint, Link, Spinner, Stack, Text } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { useNotificationsPanel } from "../hooks/useNotificationsPanel";
import { NotificationItem } from "./NotificationItem";
import { NotificationsError } from "./NotificationsError";

export function NotificationsPanel() {
  const t = useT();
  return (
    <AsidePanel heading={t("client:notifications.title")}>
      <PanelList />
      <Text as="p" size="sm">
        <Link href="/notifications" variant="standalone">
          {t("client:notifications.all")}
        </Link>
      </Text>
    </AsidePanel>
  );
}

function PanelList() {
  const t = useT();
  const list = useNotificationsPanel();
  switch (list.status) {
    case "loading":
      return <Spinner label={t("client:notifications.loading")} />;
    case "error":
      return (
        <NotificationsError headingLevel={3} message={list.message} retry={list.retry} isRetrying={list.isRetrying} />
      );
    case "loaded":
      if (list.items.length === 0) return <Hint>{t("client:notifications.empty.all")}</Hint>;
      return (
        <Stack as="ul" gap="4">
          {list.items.map((group) => (
            <li key={group.key}>
              <NotificationItem group={group} isUnread={list.isUnread(group)} surface="plain" />
            </li>
          ))}
        </Stack>
      );
  }
}
