// SPDX-License-Identifier: AGPL-3.0-only
import { Heading, Hint, RoleBadge, Stack, Switch } from "@my/ui";
import { useId } from "react";
import { useT } from "../../../lib/i18n";
import { type BadgeSwitch, useBadgeChoices } from "../hooks/useBadgeChoices";

/**
 * The badges of the user's roles on their profile (ADR-0071; the APG switch pattern): one switch per badge, named by
 * the badge itself, saved as soon as it changes, on the badges page outside any form. Without a role that has a badge,
 * the section says so.
 */
export function BadgeSettings() {
  const t = useT();
  const headingId = useId();
  const badges = useBadgeChoices();
  return (
    <Stack as="section" aria-labelledby={headingId} gap="3">
      <Heading level={2} id={headingId}>
        {t("client:settings.profile.badges")}
      </Heading>
      {badges.length === 0 ? (
        <Hint>{t("client:settings.badges.none")}</Hint>
      ) : (
        <>
          <Hint>{t("client:settings.profile.badges.description")}</Hint>
          <BadgeSwitches badges={badges} />
        </>
      )}
    </Stack>
  );
}

function BadgeSwitches({ badges }: { badges: readonly BadgeSwitch[] }) {
  const t = useT();
  return (
    <Stack as="ul" gap="3">
      {badges.map((badge) => (
        <li key={badge.roleId}>
          <Switch
            isSelected={badge.displayed}
            onChange={badge.onChange}
            {...(badge.error !== undefined && {
              isInvalid: true,
              errorMessage: t("client:settings.profile.badges.failed", { reason: badge.error }),
            })}
          >
            <RoleBadge label={badge.label} color={badge.color} />
          </Switch>
        </li>
      ))}
    </Stack>
  );
}
