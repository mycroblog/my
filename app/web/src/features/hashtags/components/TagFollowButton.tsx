// SPDX-License-Identifier: AGPL-3.0-only
// The follow button under the heading of a hashtag's page (web-client/screens/tag.md): the profile's FollowButton in
// the states of a hashtag, filled while not followed and quiet once followed, with the sentence that says what
// following does. A visitor who is not signed in sees neither.
import { FollowButton, Hint, Stack } from "@my/ui";
import { useId } from "react";
import { useT } from "../../../lib/i18n";
import { useTagFollow } from "../hooks/useTagFollow";

export type TagFollowButtonProps = {
  /** The hashtag without `#`, as the page's address names it. */
  tag: string;
};

export function TagFollowButton({ tag }: TagFollowButtonProps) {
  const t = useT();
  const follow = useTagFollow(tag);
  const descriptionId = useId();
  if (follow.kind === "hidden") return null;
  return (
    <Stack gap="2" align="start" paddingX="4">
      <FollowButton
        status={follow.status}
        label={follow.label}
        isPending={follow.isPending}
        onPress={follow.toggle}
        aria-describedby={descriptionId}
      />
      <Hint id={descriptionId}>{t("client:tag.follow.description")}</Hint>
    </Stack>
  );
}
