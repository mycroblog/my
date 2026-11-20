// SPDX-License-Identifier: AGPL-3.0-only

// The h1 of a profile's screen: the account's name and handle once known, the navigation's word until then.

import { useProfileTitle } from "@my/app-features";
import { useStrings } from "../locale";
import { ScreenHeading } from "./heading";

/** The heading of the profile of `acct`, or of the reader's own without one; read by a visitor (`signedIn` false) too. */
export function ProfileHeading({ acct, signedIn = true }: { acct: string | undefined; signedIn?: boolean }) {
  const t = useStrings();
  return <ScreenHeading>{useProfileTitle(acct, signedIn) ?? t("app.profile")}</ScreenHeading>;
}
