// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { instanceQueryOptions } from "../features/about";
import { SupporterListView, supporterListQueryOptions } from "../features/supporters";
import { useT } from "../lib/i18n";
import { useInstanceName } from "../lib/instanceName";
import { InfoLayout } from "./-layout/InfoLayout";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/**
 * The supporter list (ADR-0070 §13): the members of the supporter plans who chose to be listed, by plan, as far as the
 * instance setting supporter_list_visibility publishes it. The `_` keeps the page out of the layout of `/about`. The
 * server's shell of this address always asks search engines not to index or archive it.
 */
export const Route = createFileRoute("/about_/supporters")({
  loader: async ({ context: { queryClient, api } }) => {
    // Neither failure fails the page: the shell reads the instance too, and the list shows its own failure, or the
    // setting that publishes it to nobody (404) or to signed-in users only (401). Cached data is shown at once.
    await Promise.all([
      queryClient.query({ ...instanceQueryOptions(api), staleTime: Infinity }).catch(() => undefined),
      queryClient.infiniteQuery({ ...supporterListQueryOptions(api), staleTime: Infinity }).catch(() => undefined),
    ]);
  },
  component: SupportersPage,
  errorComponent: SupportersError,
});

function SupportersPage() {
  return (
    <InfoLayout heading={<SupportersHeading />}>
      <SupporterListView />
    </InfoLayout>
  );
}

/** The page's heading, which names the instance. */
function SupportersHeading() {
  const t = useT();
  const instance = useInstanceName();
  return <PageHeading>{t("client:supporters.list.title", { instance })}</PageHeading>;
}

function SupportersError(props: ErrorComponentProps) {
  const t = useT();
  const instance = useInstanceName();
  return <RouteError {...props} heading={t("client:supporters.list.title", { instance })} />;
}
