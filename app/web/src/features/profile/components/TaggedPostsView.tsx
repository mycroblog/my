// SPDX-License-Identifier: AGPL-3.0-only
// The posts of one account that carry one hashtag (web-client/screens/profile-tagged.md), where a featured hashtag of
// its profile leads: the page heading with the links back to the profile and on to every post with the hashtag, then
// the posts as a timeline. For an account of another server they are only those this server has received (ADR-0006).
import { BackIcon, Cluster, ExternalLink, Link, Stack } from "@my/ui";
import { type ReactNode, useId } from "react";
import { useT } from "../../../lib/i18n";
import { profilePath, tagPath } from "../../../lib/paths";
import { TimelineFeed, TimelineSkeleton } from "../../timeline";
import { isolate, useProfile } from "../hooks/useProfile";

export type TaggedPostsViewProps = {
  /** The handle in the address: `username`, or `username@domain`. */
  acct: string;
  /** The hashtag, without its `#`. */
  tag: string;
  /** Renders the page heading (the route's level-one heading) around its text, with the ID that names the feed. */
  renderHeading: (text: ReactNode, id?: string) => ReactNode;
};

/** The account's posts with the hashtag; the route loader has fetched the account and the first page. */
export function TaggedPostsView({ acct, tag, renderHeading }: TaggedPostsViewProps) {
  const t = useT();
  const profile = useProfile(acct);
  const headingId = useId();
  const original = profile.original;
  const origin =
    original === null
      ? undefined
      : {
          notice: t("client:profile.remote.notice"),
          link: (
            // The profile on the account's own server leaves the app.
            <ExternalLink href={original.href} variant="standalone">
              {t("client:profile.remote.original", { host: isolate(original.host) })}
            </ExternalLink>
          ),
        };
  return (
    <Stack gap="4">
      <TaggedPostsHeading acct={acct} tag={tag} name={profile.name} id={headingId} renderHeading={renderHeading} />
      {/* Another account or hashtag is another timeline: nothing of the previous one carries over (web-client/README.md R10). */}
      <TimelineFeed
        key={`${profile.account.id}:${tag}`}
        timeline={{ kind: "tagged", id: profile.account.id, tag }}
        aria-labelledby={headingId}
        {...(origin !== undefined && { origin })}
      />
    </Stack>
  );
}

/**
 * What the page shows while the account and its first page load: the heading, which names the account by the handle
 * in the address until its name is known, and the timeline's skeleton (web-client/16-look-states-and-wording.md G15).
 */
export function TaggedPostsSkeleton({ acct, tag, renderHeading }: TaggedPostsViewProps) {
  return (
    <Stack gap="4">
      <TaggedPostsHeading acct={acct} tag={tag} name={`@${acct}`} renderHeading={renderHeading} />
      <TimelineSkeleton />
    </Stack>
  );
}

type TaggedPostsHeadingProps = TaggedPostsViewProps & { name: string; id?: string };

/** The heading, with the links back to the profile and to the hashtag's page under it. */
function TaggedPostsHeading({ acct, tag, name, id, renderHeading }: TaggedPostsHeadingProps) {
  const t = useT();
  // The name and the hashtag are the user's text, isolated from the sentence around them (FSI … PDI, web-client/README.md Y10).
  const isolatedName = isolate(name);
  const isolatedTag = isolate(tag);
  return (
    <Stack gap="1">
      {renderHeading(t("client:profile.tagged.title", { name: isolatedName, tag: isolatedTag }), id)}
      <Cluster gap="4" rowGap="1">
        <p>
          <Link href={profilePath(acct)} variant="standalone" gap="1">
            <BackIcon />
            {t("client:profile.list.back", { name: isolatedName })}
          </Link>
        </p>
        <p>
          <Link href={tagPath(tag)} variant="standalone" wrap="anywhere">
            {t("client:profile.tagged.all", { tag: isolatedTag })}
          </Link>
        </p>
      </Cluster>
    </Stack>
  );
}
