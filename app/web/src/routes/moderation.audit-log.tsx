// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps, useRouter } from "@tanstack/react-router";
import { z } from "zod";
import {
  auditActions,
  AuditLog,
  type AuditLogFilter,
  auditLogMaxPages,
  auditLogQueryOptions,
  auditTargetKinds,
} from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The filter of the audit log and how many pages of it are loaded (web-client/README.md S3). `actor`: an account's ID;
 * `action` and `target_kind`: kinds of the API; `target`: one target of `target_kind`, ignored without it; `pages`:
 * the pages to read from the newest, omitted for one. Anything unknown or out of range is dropped.
 */
const searchSchema = z.object({
  actor: z.string().trim().min(1).max(64).optional().catch(undefined),
  action: z.enum(auditActions).optional().catch(undefined),
  target_kind: z.enum(auditTargetKinds).optional().catch(undefined),
  target: z.string().min(1).max(512).optional().catch(undefined),
  pages: z.int().min(2).max(auditLogMaxPages).optional().catch(undefined),
});

type Search = z.infer<typeof searchSchema>;

/** The search of the list under `filter`, from its first page. */
function searchOf(filter: AuditLogFilter): Search {
  return {
    actor: filter.actor,
    action: filter.action,
    target_kind: filter.targetKind,
    target: filter.targetKind === undefined ? undefined : filter.target,
  };
}

/** The operations of the moderators and administrators (FR-704), for view_audit. */
export const Route = createFileRoute("/moderation/audit-log")({
  staticData: { title: "client:admin.audit_log.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): { filter: AuditLogFilter; pages: number } => ({
    filter: {
      actor: search.actor,
      action: search.action,
      targetKind: search.target_kind,
      target: search.target_kind === undefined ? undefined : search.target,
    },
    pages: search.pages ?? 1,
  }),
  beforeLoad: requireSignIn,
  // The pages of the address, read one after another from the newest (TanStack Query's `pages`); pages already
  // loaded, as after loading more, are kept.
  loader: ({ context, deps: { filter, pages } }) =>
    loadWithPermission(context, "view_audit", () =>
      context.queryClient.infiniteQuery({ ...auditLogQueryOptions(context.api, filter), pages, staleTime: Infinity }),
    ),
  component: AuditLogPage,
  errorComponent: AuditLogError,
});

function AuditLogPage() {
  const t = useT();
  const router = useRouter();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.audit_log.title")}</PageHeading>
      <AuditLog
        filter={filter}
        onFilterChange={(next) => {
          void navigate({ search: searchOf(next) });
        }}
        filterHref={(next) => router.buildLocation({ to: "/moderation/audit-log", search: searchOf(next) }).href}
        onPagesLoaded={(pages) => {
          // Loading more stays on the same entry of the history and where the page is scrolled.
          void navigate({
            search: (previous) => ({ ...previous, pages: pages > 1 ? Math.min(pages, auditLogMaxPages) : undefined }),
            replace: true,
            resetScroll: false,
          });
        }}
      />
    </>
  );
}

function AuditLogError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="view_audit" heading={t("client:admin.audit_log.title")} />;
}
