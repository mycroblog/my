// SPDX-License-Identifier: AGPL-3.0-only

// The root of an area of the staff's screens, /moderation or /admin (ADR-0058 §2): the screens of the area the
// reader's permissions open (entries.tsx), in their groups, each with how many things wait there; or, where none is
// open to them, why. It lists nothing until the server said who the reader is, so that no
// entry comes after the others painted.

import {
  FeedNotice,
  ScreenStack,
  ScreenTitle,
  SettingsGroup,
  SettingsItem,
  SettingsMenu,
  TextLink,
} from "@my/design-system";
import type { ReactNode } from "react";
import {
  staffEntryPath,
  staffGroups,
  staffRootPath,
  useStaffEntries,
  useStaffFacts,
  type StaffArea,
  type StaffEntry,
} from "./entries";
import { useStaffStrings } from "./parts";

/** The words of each area: its root's name, why none of its screens is open, and the way to its root. */
const areaNames = {
  moderation: { menu: "staff.root.moderation", none: "staff.root.none.moderation", all: "staff.missing.moderation" },
  admin: { menu: "staff.root.admin", none: "staff.root.none.admin", all: "staff.missing.admin" },
} as const;

/** The group of an entry, as any of the groups' ids, which the list of groups grows to. */
const groupOf = (entry: StaffEntry): string => entry.group;

/** The root of `area`: its screens open to the reader, or why there is none. */
export function StaffRoot({ area, rights }: { area: StaffArea; rights: boolean }): ReactNode {
  const { t } = useStaffStrings();
  const facts = useStaffFacts();
  const entries = useStaffEntries(area);
  const title = <ScreenTitle>{t("staff.root.title", { area })}</ScreenTitle>;
  if (!facts.known)
    return (
      <ScreenStack>
        {title}
        <FeedNotice lead>{t("staff.root.loading")}</FeedNotice>
      </ScreenStack>
    );
  if (entries.length === 0)
    return (
      <ScreenStack>
        {title}
        <FeedNotice lead>{t(rights ? "staff.root.none_yet" : areaNames[area].none)}</FeedNotice>
      </ScreenStack>
    );
  return (
    <ScreenStack>
      {title}
      <SettingsMenu label={t(areaNames[area].menu)}>
        {staffGroups.map((group) => {
          const own = entries.filter((e) => groupOf(e) === group.id);
          if (own.length === 0) return null;
          return (
            <SettingsGroup key={group.id} heading={t(group.heading)}>
              {own.map((entry) => {
                const count = entry.count?.of(facts);
                return (
                  <SettingsItem
                    key={entry.path}
                    icon={entry.icon}
                    label={t(entry.name)}
                    count={count}
                    countLabel={
                      count === undefined || entry.count === undefined ? undefined : t(entry.count.words, { count })
                    }
                    href={staffEntryPath(area, entry.path)}
                  />
                );
              })}
            </SettingsGroup>
          );
        })}
      </SettingsMenu>
    </ScreenStack>
  );
}

/**
 * In a screen's place, at an address of `area` that names no screen open to the reader (none, or one their
 * permissions do not open): it says so once, with the link to the area's root.
 */
export function StaffMissing({ area }: { area: StaffArea }): ReactNode {
  const { t } = useStaffStrings();
  return (
    <FeedNotice alert lead action={<TextLink href={staffRootPath(area)}>{t(areaNames[area].all)}</TextLink>}>
      {t("staff.missing")}
    </FeedNotice>
  );
}
