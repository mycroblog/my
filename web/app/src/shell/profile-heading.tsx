// SPDX-License-Identifier: AGPL-3.0-only

// The h1 of a profile's screen: the account's name and handle once known, the navigation's word until then.

import { useProfileTitle } from "@my/app-features";
import { useStrings } from "../locale";
import { ScreenHeading } from "./heading";

/** The heading of the profile of `acct`, or of the reader's own without one. */
export function ProfileHeading({ acct }: { acct: string | undefined }) {
  const t = useStrings();
  return <ScreenHeading>{useProfileTitle(acct) ?? t("app.profile")}</ScreenHeading>;
}
