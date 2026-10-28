// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { About, instanceQueryOptions, publicDomainActionsQueryOptions } from "../features/about";
import { useT } from "../lib/i18n";
import { useInstanceName } from "../lib/instanceName";
import { InfoLayout } from "./-layout/InfoLayout";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/**
 * The instance's information page (FR-718): what the server is, its rules, the remote domains it moderates and its
 * documents, for everyone, signed in or not. The server's shell of this address carries the instance's link
 * preview (ADR-0028).
 */
export const Route = createFileRoute("/about")({
  loader: async ({ context: { queryClient, api } }) => {
    // Neither failure fails the page: the shell reads the instance too and handles its failure, and the moderated
    // domains show their own failure, or the setting that hides them, in their section. Cached data is shown at
    // once (web-client/README.md S7).
    await Promise.all([
      queryClient.query({ ...instanceQueryOptions(api), staleTime: Infinity }).catch(() => undefined),
      queryClient
        .infiniteQuery({ ...publicDomainActionsQueryOptions(api), staleTime: Infinity })
        .catch(() => undefined),
    ]);
  },
  component: AboutPage,
  errorComponent: AboutError,
});

function AboutPage() {
  return (
    <InfoLayout heading={<AboutHeading />}>
      <About />
    </InfoLayout>
  );
}

/** The page's heading, which names the instance. */
function AboutHeading() {
  const t = useT();
  const instance = useInstanceName();
  return <PageHeading>{t("client:about.title", { instance })}</PageHeading>;
}

function AboutError(props: ErrorComponentProps) {
  const t = useT();
  const instance = useInstanceName();
  return <RouteError {...props} heading={t("client:about.title", { instance })} />;
}
