// SPDX-License-Identifier: AGPL-3.0-only
// The page of one announcement (web-client/screens/announcement.md): the heading the route renders is its title, and
// below it its state, its date, and its body in one article. Once "Mark as read" is pressed and it is read, focus moves
// to that heading, since the button goes. An announcement that does not show to the users (withdrawn, or an unknown
// address) says so, with the way to the list.
import { Cluster, InlineError, Link, RegionError, Skeleton, Stack, Text, VisuallyHidden } from "@my/ui";
import { type ReactNode, useId } from "react";
import { useT } from "../../../lib/i18n";
import { useAnnouncementPage } from "../hooks/useAnnouncementPage";
import { AnnouncementBody, AnnouncementDate, AnnouncementState } from "./AnnouncementParts";
import { AnnouncementTitle } from "./AnnouncementTitle";

/** The shape of the announcement while it loads (web-client/16-look-states-and-wording.md G15). */
export function AnnouncementPageSkeleton() {
  const t = useT();
  return (
    <Stack gap="4" aria-busy="true">
      <VisuallyHidden>{t("client:announcements.loading")}</VisuallyHidden>
      <Skeleton shape="rect" />
    </Stack>
  );
}

type AnnouncementPageViewProps = {
  id: string;
  /** The page's heading (the route's PageHeading), with the ID that the article is named by when given. */
  renderHeading: (text: ReactNode, id?: string) => ReactNode;
};

export function AnnouncementPageView({ id, renderHeading }: AnnouncementPageViewProps) {
  const t = useT();
  const page = useAnnouncementPage(id);
  const headingId = useId();
  switch (page.status) {
    case "loading":
      return (
        <>
          {renderHeading(t("client:announcements.title"))}
          <AnnouncementPageSkeleton />
        </>
      );
    case "not-found":
      return (
        <>
          {renderHeading(t("client:announcements.not_found.title"))}
          <Text as="p">{t("client:announcements.not_found.body")}</Text>
          <AllAnnouncementsLink />
        </>
      );
    case "error":
      return (
        <>
          {renderHeading(t("client:announcements.title"))}
          <RegionError
            heading={t("client:announcements.page.failed")}
            reason={page.reason}
            retryLabel={t("client:announcements.retry")}
            onRetry={page.retry}
            isRetrying={page.isRetrying}
          />
        </>
      );
    case "ready": {
      const { announcement } = page;
      return (
        <>
          {renderHeading(
            <Text wrap="anywhere">
              <AnnouncementTitle announcement={announcement} />
            </Text>,
            headingId,
          )}
          {page.markError !== undefined && (
            <InlineError alert size="inherit">
              {t("client:announcements.mark_read_failed", { reason: page.markError })}
            </InlineError>
          )}
          <Stack as="article" aria-labelledby={headingId} gap="3">
            <Cluster align="center" gap="2">
              <AnnouncementState announcement={announcement} />
              <AnnouncementDate announcement={announcement} />
            </Cluster>
            <AnnouncementBody
              announcement={announcement}
              headingId={headingId}
              isMarking={page.isMarking}
              onMarkRead={() => {
                page.markRead(() => {
                  document.getElementById(headingId)?.focus();
                });
              }}
            />
          </Stack>
          <AllAnnouncementsLink />
        </>
      );
    }
  }
}

function AllAnnouncementsLink() {
  const t = useT();
  return (
    <Text as="p">
      <Link href="/announcements" variant="standalone">
        {t("client:announcements.all")}
      </Link>
    </Text>
  );
}
