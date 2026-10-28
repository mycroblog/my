// SPDX-License-Identifier: AGPL-3.0-only
import { Badge, Hint, Link, Section, Stack } from "@my/ui";
import { useId } from "react";
import { useAdminEntry } from "../../features/admin";
import { useModerationEntry } from "../../features/moderation";
import { useT } from "../../lib/i18n";

/**
 * The settings page's one way into the admin and moderation mode (ADR-0058): shown to a viewer whose roles have a
 * permission of a moderation or a server administration screen, and to nobody else. It opens the moderation page
 * while the viewer has one, and shows what awaits the moderators there (FR-712) as a badge whose words are in the
 * link's name.
 */
export function StaffModeEntry() {
  const t = useT();
  const id = useId();
  const moderation = useModerationEntry();
  const admin = useAdminEntry();
  if (!moderation.available && !admin.available) return null;
  const pending = moderation.available ? moderation.pending : 0;
  const descriptionId = `${id}-description`;
  return (
    <Section title={t("client:admin.mode.title")}>
      <Stack border="lg" paddingX="4" paddingY="3">
        <Link
          href={moderation.available ? "/moderation" : "/admin"}
          variant="standalone"
          aria-describedby={descriptionId}
          gap={2}
        >
          {t("client:admin.mode.title")}
          {pending > 0 && (
            <>
              {/* The space keeps the words apart in the link's name. */}{" "}
              <Badge count={pending} label={t("client:admin.pending_count", { count: pending })} />
            </>
          )}
        </Link>
        <Hint as="span" id={descriptionId}>
          {t("client:admin.mode.summary")}
        </Hint>
      </Stack>
    </Section>
  );
}
