// SPDX-License-Identifier: AGPL-3.0-only

// The screen at an entry's address, /<area>/<path> (ADR-0058 §2): the screen the entry declares (entries.tsx); an
// address that names no screen open to the reader says so once, with the link to the area's root.

import type { ReactNode } from "react";
import { useStaffEntries, useStaffEntry, useStaffFacts, type StaffAddress, type StaffArea } from "./entries";
import { useStaffStrings } from "./parts";
import { StaffMissing } from "./root";
import { FeedNotice } from "@my/design-system";

/** The name of the entry of `area` at `path`, the title of its screen; undefined for none open to the reader. */
export function useStaffTitle(area: StaffArea, path: string): string | undefined {
  const { t } = useStaffStrings();
  const entry = useStaffEntries(area).find((e) => e.path === path);
  return entry === undefined ? undefined : t(entry.name);
}

export type StaffEntryScreenProps = StaffAddress & { area: StaffArea; entry: string };

/** The screen of the entry of `area` at `entry`, or that the address names none open to the reader. */
export function StaffEntryScreen({ area, entry, ...address }: StaffEntryScreenProps): ReactNode {
  const { t } = useStaffStrings();
  const found = useStaffEntry(area, entry);
  if (!useStaffFacts().known) return <FeedNotice lead>{t("staff.root.loading")}</FeedNotice>;
  if (found === undefined) return <StaffMissing area={area} />;
  return <found.Screen {...address} />;
}
