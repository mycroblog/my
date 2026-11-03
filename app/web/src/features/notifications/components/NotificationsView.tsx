// SPDX-License-Identifier: AGPL-3.0-only
// The notifications page below its heading (FR-205): tabs that filter by type, kept in the URL by the
// route (web-client/README.md S3), and the notifications of the selected filter as a feed (Y6). The tabs follow
// the WAI-ARIA APG tabs pattern with automatic activation (React Aria Tabs); only the selected panel is
// mounted, and switching tabs keeps focus on the tab (the layout moves focus only when the path changes).
import {
  EmptyState,
  Feed,
  type NotificationKind,
  NotificationKindIcon,
  Stack,
  Tab,
  TabList,
  TabPanel,
  Tabs,
} from "@my/ui";
import { useRef } from "react";
import { useSessionStatus } from "../../../lib/auth/SessionProvider";
import { SignInPrompt } from "../../session";
import { PushPrompt } from "../../settings";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type NotificationFilter, notificationFilters } from "../api/notifications";
import { useNotifications } from "../hooks/useNotifications";
import { NotificationItem } from "./NotificationItem";
import { NotificationsError } from "./NotificationsError";
import { NotificationsSkeleton } from "./NotificationsSkeleton";

const filterLabels: Readonly<Record<NotificationFilter, ClientMessageKey>> = {
  all: "client:notifications.filter.all",
  mentions: "client:notifications.filter.mentions",
  follows: "client:notifications.filter.follows",
  boosts: "client:notifications.filter.boosts",
  favourites: "client:notifications.filter.favourites",
  posts: "client:notifications.filter.posts",
};

/** The kind whose icon each filter shows beside its name: the icon of the mark on the rows it keeps (P3). */
const filterKinds: Readonly<Record<NotificationFilter, NotificationKind>> = {
  all: "notice",
  mentions: "mention",
  follows: "follow",
  boosts: "boost",
  favourites: "favourite",
  posts: "status",
};

function isFilter(key: unknown): key is NotificationFilter {
  return notificationFilters.some((filter) => filter === key);
}

export type NotificationsViewProps = {
  /** The selected filter, from the URL. */
  filter: NotificationFilter;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: NotificationFilter) => void;
  /** The ID of the page heading, which names the feed. */
  headingId: string;
};

export function NotificationsView({ filter, onFilterChange, headingId }: NotificationsViewProps) {
  const t = useT();
  const signedIn = useSessionStatus() === "signed-in";
  const tabList = useRef<HTMLDivElement>(null);
  return (
    <Stack>
      {signedIn && (
        <PushPrompt
          onGone={() => {
            // The prompt's buttons go with it: the focus stays in the page, on the tab the list is filtered by.
            tabList.current?.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]')?.focus();
          }}
        />
      )}
      <Tabs
        selectedKey={filter}
        onSelectionChange={(key) => {
          if (isFilter(key)) onFilterChange(key);
        }}
      >
        <TabList ref={tabList} aria-label={t("client:notifications.filter.label")}>
          {notificationFilters.map((item) => (
            <Tab key={item} id={item} icon={<NotificationKindIcon kind={filterKinds[item]} />}>
              {t(filterLabels[item])}
            </Tab>
          ))}
        </TabList>
        {/* One panel per tab: React Aria mounts only the selected one, and each keeps the id its tab's aria-controls names. */}
        {notificationFilters.map((item) => (
          <TabPanel key={item} id={item}>
            <NotificationList filter={filter} headingId={headingId} />
          </TabPanel>
        ))}
      </Tabs>
    </Stack>
  );
}

function NotificationList({ filter, headingId }: { filter: NotificationFilter; headingId: string }) {
  const t = useT();
  const list = useNotifications(filter);
  switch (list.status) {
    case "signed-out":
      return (
        <SignInPrompt
          heading={t("client:notifications.signed_out.heading")}
          description={t("client:notifications.signed_out")}
          returnTo="/notifications"
        />
      );
    case "loading":
      return <NotificationsSkeleton />;
    case "error":
      return (
        <NotificationsError headingLevel={2} message={list.message} retry={list.retry} isRetrying={list.isRetrying} />
      );
    case "loaded":
      if (list.items.length === 0) {
        return (
          <EmptyState
            heading={t(filter === "all" ? "client:notifications.empty.all" : "client:notifications.empty.filtered")}
            description={t("client:notifications.empty.description")}
          />
        );
      }
      return (
        <Feed
          aria-labelledby={headingId}
          items={list.items}
          getKey={(group) => group.key}
          renderItem={(group) => <NotificationItem group={group} isUnread={list.isUnread(group)} />}
          isLoading={list.isLoadingMore}
          hasMore={list.hasMore}
          onLoadMore={list.loadMore}
          loadMoreLabel={t("client:notifications.load_more")}
          loadingLabel={t("client:notifications.loading")}
          estimateSize={120}
        />
      );
  }
}
