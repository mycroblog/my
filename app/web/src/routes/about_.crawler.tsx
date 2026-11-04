// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { instanceQueryOptions, LinkCardCrawler } from "../features/about";
import { useT } from "../lib/i18n";
import { useInstanceName } from "../lib/instanceName";
import { InfoLayout } from "./-layout/InfoLayout";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/**
 * What the instance's link card crawler does and how a website refuses it (web-client/screens/about-crawler.md,
 * ADR-0062), for the operators of the websites it fetches, who come without an account. The `_` keeps the page out of
 * the layout of `/about`. The server's shell of this address carries the link preview (ADR-0028).
 */
export const Route = createFileRoute("/about_/crawler")({
  loader: async ({ context: { queryClient, api } }) => {
    // The failure does not fail the page: the page shows it under its heading, and the shell reads the instance too.
    // Cached data is shown at once (web-client/README.md S7).
    await queryClient.query({ ...instanceQueryOptions(api), staleTime: Infinity }).catch(() => undefined);
  },
  component: CrawlerPage,
  errorComponent: CrawlerError,
});

function CrawlerPage() {
  return (
    <InfoLayout heading={<CrawlerHeading />}>
      <LinkCardCrawler />
    </InfoLayout>
  );
}

/** The page's heading, which names the instance. */
function CrawlerHeading() {
  const t = useT();
  const instance = useInstanceName();
  return <PageHeading>{t("client:about.link_card_crawler.title", { instance })}</PageHeading>;
}

function CrawlerError(props: ErrorComponentProps) {
  const t = useT();
  const instance = useInstanceName();
  return <RouteError {...props} heading={t("client:about.link_card_crawler.title", { instance })} />;
}
