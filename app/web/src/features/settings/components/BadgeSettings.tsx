// SPDX-License-Identifier: AGPL-3.0-only
import { Hint, RoleBadge, Section, Stack, Switch } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { useBadgeChoices } from "../hooks/useBadgeChoices";

/**
 * The badges of the user's roles on their profile (ADR-0071; the APG switch pattern): one switch per badge, named by
 * the badge itself, saved as soon as it changes. Without a role that has a badge, the section is not shown.
 */
export function BadgeSettings() {
  const t = useT();
  const badges = useBadgeChoices();
  if (badges.length === 0) return null;
  return (
    <Section title={t("client:settings.profile.badges")}>
      <Hint>{t("client:settings.profile.badges.description")}</Hint>
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
    </Section>
  );
}
