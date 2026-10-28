// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack, Text } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  availabilityEventsQueryOptions,
  DomainName,
  InstanceDetail,
  instanceQueryOptions,
} from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/moderation/instances`. */
const paramsSchema = z.object({ domain: z.string().min(1).max(253) });

/** One remote server's health and the history of its availability (federation.md §7.3), for manage_domains. */
export const Route = createFileRoute("/moderation/instances_/$domain")({
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { domain } }) =>
    loadWithPermission(context, "manage_domains", () =>
      Promise.all([
        context.queryClient.query({ ...instanceQueryOptions(context.api, domain), staleTime: Infinity }),
        context.queryClient.infiniteQuery({
          ...availabilityEventsQueryOptions(context.api, domain),
          staleTime: Infinity,
        }),
      ]),
    ),
  component: InstancePage,
  errorComponent: InstanceError,
});

/** The page's name: the domain from its address. */
function PageName() {
  const { domain } = Route.useParams();
  return (
    <Text wrap="anywhere">
      <DomainName domain={domain} />
    </Text>
  );
}

function InstanceHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>
        <PageName />
      </PageHeading>
      <Link href="/moderation/instances" variant="standalone" size="sm">
        {t("client:admin.instance.back")}
      </Link>
    </Stack>
  );
}

function InstancePage() {
  const { domain } = Route.useParams();
  return (
    <>
      <InstanceHeading />
      {/* Another server is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <InstanceDetail key={domain} domain={domain} />
    </>
  );
}

/** The page keeps its name, the domain, when it fails (web-client/16-look-states-and-wording.md G16). */
function InstanceError(props: ErrorComponentProps) {
  return <AdminError {...props} permission="manage_domains" heading={<PageName />} />;
}
