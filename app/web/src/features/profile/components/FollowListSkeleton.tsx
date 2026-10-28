// SPDX-License-Identifier: AGPL-3.0-only
import { AccountRowSkeleton, Stack, VisuallyHidden } from "@my/ui";
import type { ReactNode } from "react";
import { useT } from "../../../lib/i18n";
import type { FollowList } from "../api/accounts";
import { isolate } from "../hooks/useProfile";

/** Placeholder rows, for as long as the first page of a follow list loads. */
const placeholders = ["first", "second", "third", "fourth", "fifth"] as const;

export type FollowListSkeletonProps = {
  /** The handle in the address: `username`, or `username@domain`. */
  acct: string;
  list: FollowList;
  /** Renders the page heading (the route's level-one heading) around its text. */
  renderHeading: (text: ReactNode) => ReactNode;
};

/**
 * What a follow list shows while the account and its first page load: the page heading, which names the
 * account by the handle in the address until its name is known, and a busy region with the shape of a few
 * rows (hidden from assistive technology) named once by a visually hidden text (web-client/16-look-states-and-wording.md G15).
 */
export function FollowListSkeleton({ acct, list, renderHeading }: FollowListSkeletonProps) {
  const t = useT();
  // The handle is the user's text too, isolated from the sentence around it (FSI … PDI, web-client/README.md Y10).
  const handle = isolate(`@${acct}`);
  return (
    <Stack gap="4">
      {renderHeading(
        list === "followers"
          ? t("client:profile.followers.title", { name: handle })
          : t("client:profile.following.title", { name: handle }),
      )}
      <div aria-busy="true">
        <VisuallyHidden>{t("client:profile.list.loading")}</VisuallyHidden>
        {/* The rows are divided as the list's rows are (FollowListView). */}
        <Stack divided border="y">
          {placeholders.map((key) => (
            <AccountRowSkeleton key={key} />
          ))}
        </Stack>
      </div>
    </Stack>
  );
}
