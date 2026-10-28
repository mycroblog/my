// SPDX-License-Identifier: AGPL-3.0-only
import { AsidePanel, BrandIcon, Cluster, Hint, Link, Text } from "@my/ui";
import { DocumentLinks } from "../../features/documents";
import { useBrand } from "../../lib/brand";
import { useConfig } from "../../lib/config";
import { useT } from "../../lib/i18n";
import { aboutPath } from "../../lib/paths";
import { ServerNameLink } from "./ServerNameLink";

/**
 * In the third column on wide screens (xl): which server this is (its name links to the root address, beside its own
 * icon when it has one), a link to its information page, and the links to its terms of service and privacy policy when
 * it publishes them. Narrower layouts leave it out; the menu and the settings have the links.
 */
export function ServerInfo() {
  const t = useT();
  // Handles end with the account domain, whatever the instance is named.
  const { accountDomain } = useConfig();
  const { icon } = useBrand();
  return (
    <AsidePanel heading={t("client:shell.about.title")}>
      {icon === undefined ? (
        <ServerNameLink size="lg" />
      ) : (
        <Cluster gap="3" wrap={false} align="center">
          <BrandIcon src={icon} size="md" />
          <ServerNameLink size="lg" />
        </Cluster>
      )}
      <Hint>{t("client:shell.about.domain", { domain: accountDomain })}</Hint>
      <Text as="p" size="sm">
        <Link href={aboutPath} variant="standalone">
          {t("client:shell.about.more")}
        </Link>
      </Text>
      <DocumentLinks />
    </AsidePanel>
  );
}
