// SPDX-License-Identifier: AGPL-3.0-only
// The roles region of a local account's admin page (FR-720), for a viewer with manage_roles: every role of the
// instance with its permissions and whether the account's user holds it, and "Grant" or "Revoke" for the roles at
// or below the viewer's highest one; a role above it says why it has no button. Each change asks first in an
// alert dialog (ConfirmDialog); a refusal (a role above the viewer's, the last admin) stays in it with the
// reason. What a change did stays in the region's StatusMessage, and focus moves to the region's heading, since
// the pressed button gives way to the other one.
import { Button, ConfirmDialog, DescriptionItem, DescriptionList, DetailSection, EmptyState, Heading, Hint, RegionError, SectionHeader, Spinner, Stack, StatusMessage, Text } from "@my/ui";
import { useRef } from "react";
import { useT } from "../../../lib/i18n";
import { adminRoleKey } from "../api/roles";
import { type AccountRoles as AccountRolesState, type RoleChanges, type RoleRow, useAccountRoles, useRoleChanges } from "../hooks/useRoles";
import { RecordList, useFocusAfterChange } from "./parts";

/** A role's key, an identifier that reads left to right in every locale. */
function RoleKey({ value }: { value: string }) {
  return (
    <Text dir="ltr" wrap="anywhere">
      {value}
    </Text>
  );
}

/** A role's key inside a sentence, isolated left to right (U+2066 … U+2069). */
function isolatedKey(key: string): string {
  return `⁦${key}⁩`;
}

/** The roles region of local account `accountId`, whose handle (isolated) is `handle`. */
export function AccountRoles({ accountId, handle }: { accountId: string; handle: string }) {
  const t = useT();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const roles = useAccountRoles(accountId);
  const held = roles.status === "ready" ? roles.held : undefined;
  const focusHeading = useFocusAfterChange(headingRef, held);
  const changes = useRoleChanges(accountId, handle, focusHeading);
  return (
    <DetailSection title={t("client:admin.account.roles.title")} headingRef={headingRef}>
      <StatusMessage tone="success">{changes.status}</StatusMessage>
      <Hint>{t("client:admin.account.roles.description")}</Hint>
      <RoleList roles={roles} changes={changes} />
      <RoleChangeDialog changes={changes} handle={handle} />
    </DetailSection>
  );
}

function RoleList({ roles, changes }: { roles: AccountRolesState; changes: RoleChanges }) {
  const t = useT();
  switch (roles.status) {
    case "loading":
      return <Spinner size="sm" label={t("client:admin.account.roles.loading")} />;
    case "error":
      return (
        <RegionError
          heading={t("client:admin.account.roles.load_failed")}
          headingLevel={3}
          reason={roles.reason}
          retryLabel={t("client:admin.account.roles.retry")}
          onRetry={roles.retry}
          isRetrying={roles.isRetrying}
        />
      );
    case "ready":
      return (
        <RecordList
          list={{ items: roles.rows, hasMore: false, isLoadingMore: false, loadMore: () => undefined }}
          getId={(row) => row.role.id}
          empty={<EmptyState headingLevel={3} heading={t("client:admin.account.roles.empty")} />}
        >
          {(row) => <RoleItem row={row} changes={changes} />}
        </RecordList>
      );
  }
}

function RoleItem({ row, changes }: { row: RoleRow; changes: RoleChanges }) {
  const t = useT();
  const { role, held, outrankedBy, missing } = row;
  // Granting needs every permission of the role (revoking does not), so the grant button gives way to the reason.
  const cannotGrant = !held && outrankedBy === undefined && missing.length > 0;
  const key = isolatedKey(role.key);
  const pending = changes.isPending && changes.confirmation?.role.id === role.id;
  return (
    <Stack gap="3">
      <SectionHeader
        actions={
          outrankedBy === undefined &&
          !cannotGrant &&
          // Different keys: the pressed button gives way to the other one, and focus goes to the region's heading.
          (held ? (
            <Button
              key="revoke"
              size="sm"
              isPending={pending}
              aria-label={t("client:admin.account.roles.revoke_role", { role: key })}
              onPress={() => {
                changes.open(role, "revoke");
              }}
            >
              {t("client:admin.account.roles.revoke")}
            </Button>
          ) : (
            <Button
              key="grant"
              size="sm"
              isPending={pending}
              aria-label={t("client:admin.account.roles.grant_role", { role: key })}
              onPress={() => {
                changes.open(role, "grant");
              }}
            >
              {t("client:admin.account.roles.grant")}
            </Button>
          ))
        }
      >
        <Heading level={3}>
          <RoleKey value={role.key} />
        </Heading>
      </SectionHeader>
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:admin.account.roles.held")}>
          <Text {...(held && { weight: "semibold" as const })}>{held ? t("client:admin.account.roles.held.yes") : t("client:admin.account.roles.held.no")}</Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.account.roles.permissions")}>
          {role.permissions.length === 0 ? t("client:admin.account.roles.permissions.none") : <RoleKey value={role.permissions.join(", ")} />}
        </DescriptionItem>
      </DescriptionList>
      {outrankedBy !== undefined && (
        <Hint>{t("client:admin.account.roles.outranked", { role: isolatedKey(outrankedBy.key) })}</Hint>
      )}
      {cannotGrant && <Hint>{t("client:admin.account.roles.missing_permissions", { permissions: missing.join(", ") })}</Hint>}
    </Stack>
  );
}

function RoleChangeDialog({ changes, handle }: { changes: RoleChanges; handle: string }) {
  const t = useT();
  const { confirmation } = changes;
  const role = isolatedKey(confirmation?.role.key ?? "");
  const grant = confirmation?.kind !== "revoke";
  const description = grant
    ? t("client:admin.account.roles.confirm_grant.description")
    : confirmation.role.key === adminRoleKey
      ? t("client:admin.account.roles.confirm_revoke.description_admin")
      : t("client:admin.account.roles.confirm_revoke.description");
  return (
    <ConfirmDialog
      isOpen={changes.isOpen}
      onOpenChange={(isOpen) => {
        if (!isOpen) changes.cancel();
      }}
      title={grant ? t("client:admin.account.roles.confirm_grant.title", { role, handle }) : t("client:admin.account.roles.confirm_revoke.title", { role, handle })}
      description={description}
      confirmLabel={grant ? t("client:admin.account.roles.grant") : t("client:admin.account.roles.revoke")}
      cancelLabel={t("client:admin.account_actions.confirm.cancel")}
      tone={grant ? "primary" : "danger"}
      onConfirm={() => {
        changes.confirm();
      }}
      isPending={changes.isPending}
      {...(changes.error === undefined ? {} : { errorMessage: changes.error })}
    />
  );
}
