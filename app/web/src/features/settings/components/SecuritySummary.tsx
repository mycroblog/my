// SPDX-License-Identifier: AGPL-3.0-only
// The security of the account in one glance on the settings page (web-client/screens/settings.md §3; ADR-0110 P1, P3):
// a row per way of signing in or confirming, each a link to its page with the state as a badge in the link's name.
import {
  Box,
  CheckIcon,
  CloseIcon,
  IconLabel,
  LinkedAccountIcon,
  Link,
  PasskeyIcon,
  PasswordIcon,
  Section,
  Stack,
  StatusBadge,
  TwoFactorIcon,
  WarningIcon,
} from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import { linkedIdentitiesQueryOptions } from "../api/linkedIdentities";
import { passkeysQueryOptions } from "../api/passkeys";
import { passwordStatusQueryOptions } from "../api/password";
import { twoFactorQueryOptions } from "../api/twoFactor";

type Badge = { icon: ReactNode; label: string; tone: "success" | "warning" | "neutral" };

/**
 * The password, two-factor authentication, passkeys and linked accounts (when the server has login providers), each
 * a link to its page with its state as a `StatusBadge`. A state still loading, or that could not be read, shows no
 * badge: the page of the row says it.
 */
export function SecuritySummary() {
  const t = useT();
  const api = useApi();
  const password = useQuery(passwordStatusQueryOptions(api)).data;
  const twoFactor = useQuery(twoFactorQueryOptions(api)).data;
  const passkeys = useQuery(passkeysQueryOptions(api)).data;
  const linked = useQuery(linkedIdentitiesQueryOptions(api)).data;
  const count = (n: number): Badge =>
    n > 0
      ? { icon: <CheckIcon />, label: t("client:settings.security.count", { count: n }), tone: "success" }
      : { icon: <CloseIcon />, label: t("client:settings.security.none"), tone: "neutral" };
  return (
    <Section title={t("client:settings.security.title")} description={t("client:settings.security.description")}>
      <Stack as="ul" divided border="lg">
        <Row
          href="/settings/password"
          icon={<PasswordIcon />}
          label={t("client:settings.password.title")}
          badge={
            password === undefined
              ? undefined
              : password.has_password
                ? { icon: <CheckIcon />, label: t("client:settings.security.password.set"), tone: "success" }
                : { icon: <CloseIcon />, label: t("client:settings.security.none"), tone: "neutral" }
          }
        />
        <Row
          href="/settings/two-factor"
          icon={<TwoFactorIcon />}
          label={t("client:settings.two_factor.title")}
          badge={
            twoFactor === undefined
              ? undefined
              : twoFactor.enabled
                ? { icon: <CheckIcon />, label: t("client:settings.security.on"), tone: "success" }
                : { icon: <WarningIcon />, label: t("client:settings.security.off"), tone: "warning" }
          }
        />
        <Row
          href="/settings/passkeys"
          icon={<PasskeyIcon />}
          label={t("client:settings.passkeys.title")}
          badge={passkeys === undefined ? undefined : count(passkeys.items.length)}
        />
        {linked !== undefined && linked.providers.length > 0 && (
          <Row
            href="/settings/linked-accounts"
            icon={<LinkedAccountIcon />}
            label={t("client:settings.linked_accounts.title")}
            badge={count(linked.items.length)}
          />
        )}
      </Stack>
    </Section>
  );
}

function Row({ href, icon, label, badge }: { href: string; icon: ReactNode; label: string; badge: Badge | undefined }) {
  return (
    // A row of a list keeps its own inner space (web-client/16-look-states-and-wording.md G2).
    <Box as="li" paddingX="4" paddingY="3">
      <Link href={href} variant="standalone" gap={2}>
        <IconLabel icon={icon}>{label}</IconLabel>
        {badge !== undefined && (
          <>
            {/* The space keeps the words apart in the link's name. */}{" "}
            <StatusBadge icon={badge.icon} label={badge.label} tone={badge.tone} />
          </>
        )}
      </Link>
    </Box>
  );
}
