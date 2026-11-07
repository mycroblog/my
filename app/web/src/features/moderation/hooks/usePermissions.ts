// SPDX-License-Identifier: AGPL-3.0-only
// The permissions of the viewer's roles (CredentialAccount.permissions), which decide which admin screens the
// app shows. They only hide what the viewer cannot use: the server checks the permission of every admin
// operation itself, and answers 403 missing-permission without it.
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { shellQueryReader } from "../../../lib/query";
import { viewerQueryOptions } from "../../session";
import type { Permission } from "../api/staff";

/** The viewer's permissions; none while signed out or while the account loads. */
export function useViewerPermissions(): readonly Permission[] {
  const api = useApi();
  const status = useSessionStatus();
  const account = useQuery({ ...viewerQueryOptions(api), enabled: status === "signed-in", ...shellQueryReader });
  return status === "signed-in" ? (account.data?.permissions ?? []) : [];
}

/**
 * Whether the viewer has `permission`, on a page whose loader has loaded the viewer's account (it suspends
 * until then).
 */
export function useHasPermission(permission: Permission): boolean {
  const api = useApi();
  const account = useSuspenseQuery(viewerQueryOptions(api));
  return account.data.permissions.includes(permission);
}
