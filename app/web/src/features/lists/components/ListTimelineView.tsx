// SPDX-License-Identifier: AGPL-3.0-only
// The page of one list below its heading, the list's name (web-client/screens/list-timeline.md, ADR-0122 §4): the
// heading's line goes on with the number of members and "Manage lists", then the list's timeline (features/timeline,
// the home timeline's feed with its "N new posts" button). A list without members shows, instead of the feed, the empty
// state that leads to managing them.
import { ButtonLink, Cluster, EmptyState, Hint, Link } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { listsPath } from "../../../lib/paths";
import { ListTimeline } from "../../timeline";
import { useList } from "../hooks/useLists";

export type ListTimelineViewProps = {
  /** The list's ID; the route has loaded the list and the first page of its timeline. */
  id: string;
  /** The ID of the page heading, the list's name. */
  "aria-labelledby": string;
};

export function ListTimelineView({ id, "aria-labelledby": labelledBy }: ListTimelineViewProps) {
  const t = useT();
  const list = useList(id);
  return (
    <>
      <Cluster gap="3" align="baseline" paddingX="4">
        <Hint as="p">{t("client:lists.member_count", { count: list.members_count })}</Hint>
        <Link href={listsPath} variant="standalone">
          {t("client:lists.manage")}
        </Link>
      </Cluster>
      {list.members_count === 0 ? (
        <EmptyState
          heading={t("client:lists.timeline.empty.heading")}
          description={t("client:lists.timeline.empty.description")}
          action={
            <ButtonLink href={listsPath} variant="primary">
              {t("client:lists.members.manage")}
            </ButtonLink>
          }
        />
      ) : (
        <ListTimeline id={id} aria-labelledby={labelledBy} />
      )}
    </>
  );
}
