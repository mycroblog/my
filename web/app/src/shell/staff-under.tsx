// SPDX-License-Identifier: AGPL-3.0-only

// A screen under an entry of the staff's screens (/admin/finance/costs, /admin/supporters/plans/{id}), in the app's
// shell: its title as the screen's heading, the feature's screen given its address's query and the way to another
// address; that the address names no screen open to the reader, where the entry is not open to them; and, without a
// session, the ask to sign in.

import {
  StaffMissing,
  staffHref,
  useStaffEntry,
  useStaffFacts,
  useStaffTitle,
  type StaffArea,
  type StaffUnderProps,
} from "@my/app-features";
import { useLocation, useRouter } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useStrings } from "../locale";
import { ScreenHeading } from "./heading";
import { SignedOut } from "./signed-out";
import { searchOfQuery } from "./staff";

type UnderProps = {
  area: StaffArea;
  /** The entry the screen is under (`finance`). */
  entry: string;
  /**
   * The screen's title (a thing's name, which a component of the route under the signed-in check reads); the entry's
   * name while it has none.
   */
  title?: string | undefined;
  /** The feature's screen, given its address. */
  screen: (address: StaffUnderProps) => ReactNode;
};

function Under({ area, entry, title, screen }: UnderProps) {
  const t = useStrings();
  const router = useRouter();
  const pathname = useLocation({ select: (l) => l.pathname });
  const search = searchOfQuery(useLocation({ select: (l) => l.searchStr }));
  const known = useStaffFacts().known;
  const open = useStaffEntry(area, entry) !== undefined;
  const name = useStaffTitle(area, entry);
  const areaName = t(area === "moderation" ? "app.moderation" : "app.admin");
  if (known && !open)
    return (
      <>
        <ScreenHeading>{areaName}</ScreenHeading>
        <StaffMissing area={area} />
      </>
    );
  return (
    <>
      <ScreenHeading>{title ?? name ?? areaName}</ScreenHeading>
      {screen({
        search,
        onSearch: (next) => {
          void router.navigate({ href: staffHref(pathname, next) });
        },
        go: (href) => {
          void router.navigate({ href });
        },
      })}
    </>
  );
}

/**
 * The screen under the entry of `area` at `entry` for a signed-in reader; without a session, the ask to sign in,
 * which comes back to the screen's address.
 */
export function StaffUnderPage({ signedIn, ...props }: UnderProps & { signedIn: boolean }) {
  const t = useStrings();
  const pathname = useLocation({ select: (l) => l.pathname });
  if (signedIn) return <Under {...props} />;
  return (
    <>
      <ScreenHeading>{t(props.area === "moderation" ? "app.moderation" : "app.admin")}</ScreenHeading>
      <SignedOut returnTo={pathname} />
    </>
  );
}
