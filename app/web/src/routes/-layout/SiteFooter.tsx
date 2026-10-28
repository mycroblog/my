// SPDX-License-Identifier: AGPL-3.0-only
import { Box, Cluster, Hint, Link, Stack } from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { SourceCodeLink } from "../../features/about";
import { DocumentLinks } from "../../features/documents";
import { useApi } from "../../lib/auth/SessionProvider";
import { useT } from "../../lib/i18n";
import { aboutPath } from "../../lib/paths";
import { shellQueryReader } from "../../lib/query";

/**
 * The footer of every page of people who are not signed in (the top bar's pages, `AppShell` `footer`): the server's
 * information page and its terms of service and privacy policy (the documents it publishes), then the software the
 * server runs with its source code (AGPL-3.0 section 13). Signed in, the menu's sheet and the third column carry the
 * same links (web-client/09-app-shell.md §9).
 */
export function SiteFooter() {
  const t = useT();
  const api = useApi();
  // The shell reads GET /instance on every page and handles its failures; this only reads its answer.
  const instance = useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader });
  const sourceUrl = instance.data?.source_url;
  return (
    <Box border="top" paddingTop="4">
      <Stack gap="3">
        <Cluster gap="4" rowGap="1" align="center" textSize="sm">
          <Link href={aboutPath} variant="standalone">
            {t("client:shell.about.more")}
          </Link>
          <DocumentLinks />
        </Cluster>
        <Cluster gap="2" rowGap="1" align="center" textSize="sm">
          <Hint as="span">{t("client:landing.powered_by")}</Hint>
          {sourceUrl !== undefined && <SourceCodeLink url={sourceUrl} />}
        </Cluster>
      </Stack>
    </Box>
  );
}
