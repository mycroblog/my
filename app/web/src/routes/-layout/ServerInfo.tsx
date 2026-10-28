// SPDX-License-Identifier: AGPL-3.0-only
import { AsidePanel, Hint, Link, Text } from "@my/ui";
import { DocumentLinks } from "../../features/documents";
import { useConfig } from "../../lib/config";
import { useT } from "../../lib/i18n";
import { aboutPath } from "../../lib/paths";
import { ServerNameLink } from "./ServerNameLink";

/**
 * In the third column on wide screens (xl): which server this is (its name links to the root address), a link to
 * its information page, and the links to its terms of service and privacy policy when it publishes them. Narrower
 * layouts leave it out; the menu and the settings have the links. Signed-out pages have no third column, and never
 * show it: their footer (`SiteFooter`) has the links.
 */
export function ServerInfo() {
  const t = useT();
  // Handles end with the account domain, whatever the instance is named.
  const { accountDomain } = useConfig();
  return (
    <AsidePanel heading={t("client:shell.about.title")}>
      <ServerNameLink size="lg" />
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
