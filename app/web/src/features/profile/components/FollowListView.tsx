// SPDX-License-Identifier: AGPL-3.0-only
// The accounts that follow an account, or that it follows: a list where people are found, so each row says who the
// account is (its name, a line of its profile text, whether it follows the viewer) and can be followed from there
// (ADR-0110 P1, web-client/screens/profile-follows.md), with a button that loads the next page. A button rather than loading at the end of the scroll keeps the end of
// the list reachable for keyboard users and the following content in reach (WCAG 2.1.1).
import { AccountItem, BackIcon, Box, Button, Cluster, EmptyState, Link, Stack, Tag, Text } from "@my/ui";
import type { ReactNode } from "react";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import type { FollowList } from "../api/accounts";
import { type FollowListState, useFollowList } from "../hooks/useFollowList";
import { isolate, useProfile } from "../hooks/useProfile";
import { useRelationship } from "../hooks/useRelationship";
import { RelationshipActions } from "./RelationshipActions";

export type FollowListViewProps = {
  /** The handle in the address: `username`, or `username@domain`. */
  acct: string;
  list: FollowList;
  /** Renders the page heading (the route's level-one heading) around its text. */
  renderHeading: (text: ReactNode) => ReactNode;
};

/** The follow list of the account whose handle is `acct`; the route loader has fetched the account and the first page. */
export function FollowListView({ acct, list, renderHeading }: FollowListViewProps) {
  const t = useT();
  const profile = useProfile(acct);
  const { items, hasMore, isLoadingMore, loadMore, loadMoreError } = useFollowList(profile.account.id, list);
  // The name is the user's text, isolated from the sentence around it (FSI … PDI, web-client/README.md Y10).
  const isolated = isolate(profile.name);

  return (
    <Stack gap="4">
      <Stack gap="1">
        {renderHeading(
          list === "followers"
            ? t("client:profile.followers.title", { name: isolated })
            : t("client:profile.following.title", { name: isolated }),
        )}
        <p>
          <Link href={profilePath(profile.account.acct)} variant="standalone" gap={1}>
            <BackIcon />
            {t("client:profile.list.back", { name: isolated })}
          </Link>
        </p>
      </Stack>
      {items.length === 0 ? (
        <EmptyState
          heading={list === "followers" ? t("client:profile.followers.empty") : t("client:profile.following.empty")}
        />
      ) : (
        <Stack as="ul" divided border="y">
          {items.map((item) => (
            <Box as="li" key={item.id}>
              <FollowListRow item={item} />
            </Box>
          ))}
        </Stack>
      )}
      {loadMoreError !== null && (
        <Text as="p" align="center">
          {t("client:profile.list.load_more_failed", { reason: loadMoreError })}
        </Text>
      )}
      {hasMore && (
        <Cluster justify="center">
          {/* A pending button keeps its focus while the page loads (React Aria Button, isPending). */}
          <Button isPending={isLoadingMore} onPress={loadMore}>
            {loadMoreError === null ? t("client:profile.list.load_more") : t("client:profile.list.retry")}
          </Button>
        </Cluster>
      )}
    </Stack>
  );
}

/**
 * One account of the list: a link to its profile, with a line of its profile text and whether it follows the viewer
 * under it, and its follow button (the profile's, without the menu) at the end. The viewer's own row and a signed-out
 * viewer's rows have no button: the profile offers editing and signing in.
 */
function FollowListRow({ item }: { item: FollowListState["items"][number] }) {
  const t = useT();
  const relationship = useRelationship(item.account);
  const followsViewer = relationship.kind === "other" && relationship.relationship.followed_by;
  const hasButton = relationship.kind === "other" || relationship.kind === "loading";
  const details =
    followsViewer || item.noteLine !== "" ? (
      <>
        {followsViewer && (
          <span>
            <Tag variant="pill">{t("client:profile.tag.follows_you")}</Tag>
          </span>
        )}
        {item.noteLine !== "" && (
          // The account's own text, in a language the app does not know (web-client/README.md Y10).
          <Text as="p" dir="auto" lang="" wrap="truncate">
            {item.noteLine}
          </Text>
        )}
      </>
    ) : undefined;
  return (
    // A right-to-left name lines up with the other rows; only its text runs right to left (web-client/16-look-states-and-wording.md G9).
    <AccountItem
      name={item.name}
      avatar={imageSources(item.account.avatar)}
      avatarSeed={item.id}
      handle={item.handle}
      href={item.href}
      layout="padded"
      details={details}
      trailing={
        hasButton && (
          <Box paddingX="4">
            <RelationshipActions
              account={item.account}
              name={item.name}
              state={relationship}
              showMenu={false}
              size="sm"
            />
          </Box>
        )
      }
    />
  );
}
