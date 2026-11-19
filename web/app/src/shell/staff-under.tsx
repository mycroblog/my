// SPDX-License-Identifier: AGPL-3.0-only

// A screen under an entry of Moderation (an account, an appeal, a sign-up block, ADR-0058 §2), as each route of one
// renders it: under its title once its entry is open to the reader; that the address names no screen open to them
// once their permissions turn out not to open the entry; and the ask to sign in without a session. Its query, which a
// list's filters and page keep, is read and changed through the router.

import { StaffMissing, staffHref, useStaffEntry, useStaffFacts, type StaffSearch } from "@my/app-features";
import { useLocation, useRouter } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useStrings } from "../locale";
import { ScreenHeading } from "./heading";
import { SignedOut } from "./signed-out";
import { searchOfQuery } from "./staff";

/** The query of the address shown, and the way to show the same screen at another query, or another address. */
export function useStaffAddress(): {
  readonly search: StaffSearch;
  readonly onSearch: (search: StaffSearch) => void;
  readonly go: (href: string) => void;
} {
  const router = useRouter();
  const pathname = useLocation({ select: (l) => l.pathname });
  const search = searchOfQuery(useLocation({ select: (l) => l.searchStr }));
  const go = (href: string) => {
    void router.navigate({ href });
  };
  return {
    search,
    onSearch: (next) => {
      go(staffHref(pathname, next));
    },
    go,
  };
}

/**
 * The screen under the entry of Moderation at `entry`, titled `title` (its own, or a fallback while it is read), once
 * the entry is open to the reader; else that the address names no screen open to them. Only for a signed-in reader:
 * the hooks of its title and its screen read the reader's data.
 */
export function StaffOpened({ entry, title, children }: { entry: string; title: string; children: ReactNode }) {
  const t = useStrings();
  const known = useStaffFacts().known;
  const open = useStaffEntry("moderation", entry) !== undefined;
  // Nothing of the screen, and none of its reads, before the reader's permissions say it is open.
  if (!known) return <ScreenHeading>{title}</ScreenHeading>;
  if (!open)
    return (
      <>
        <ScreenHeading>{t("app.moderation")}</ScreenHeading>
        <StaffMissing area="moderation" />
      </>
    );
  return (
    <>
      <ScreenHeading>{title}</ScreenHeading>
      {children}
    </>
  );
}

/** A screen under an entry of Moderation without a session: the ask to sign in, which comes back to `returnTo`. */
export function StaffSignedOut({ returnTo }: { returnTo: string }) {
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.moderation")}</ScreenHeading>
      <SignedOut returnTo={returnTo} />
    </>
  );
}
