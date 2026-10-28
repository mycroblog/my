// SPDX-License-Identifier: AGPL-3.0-only
// The errors of the profile routes. Their h1 is the account's name, which is not known when the account failed
// to load, so the page keeps the handle from its address instead (web-client/16-look-states-and-wording.md G16).
import { type ErrorComponentProps, useParams } from "@tanstack/react-router";
import { isolate } from "../../features/profile";
import { useT } from "../../lib/i18n";
import { RouteError } from "../-layout/RouteStates";

/** The handle of the address, `@alice` or `@alice@example.com`. */
function useHandle(): string {
  const { acct } = useParams({ strict: false });
  return `@${acct ?? ""}`;
}

/** The error of a profile page, under the handle. */
export function ProfileError(props: ErrorComponentProps) {
  // A handle reads left to right in every locale.
  return <RouteError {...props} heading={<span dir="ltr">{useHandle()}</span>} />;
}

export type FollowListErrorProps = ErrorComponentProps & { list: "followers" | "following" };

/** The error of a follow list, under the list's name with the handle in place of the account's name. */
export function FollowListError({ list, ...props }: FollowListErrorProps) {
  const t = useT();
  const name = isolate(useHandle());
  return (
    <RouteError
      {...props}
      heading={
        list === "followers"
          ? t("client:profile.followers.title", { name })
          : t("client:profile.following.title", { name })
      }
    />
  );
}
