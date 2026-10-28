// SPDX-License-Identifier: AGPL-3.0-only
// What the admin routes share: loading their data only for a viewer whose roles have the screen's permission
// (a viewer without it sees why instead, and the server would refuse the requests), and the error of a screen
// whose permission the server refused after all (the viewer's roles changed since the account loaded).
import type { ErrorComponentProps } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { isMissingPermission, MissingPermission, type Permission } from "../../features/moderation";
import { viewerQueryOptions } from "../../features/session";
import type { RouterContext } from "../../lib/query";
import { PageHeading } from "../-layout/PageHeading";
import { RouteError } from "../-layout/RouteStates";

/** Loads the viewer's account and, when its roles grant `permission`, runs `load`. */
export async function loadWithPermission(
  { queryClient, api }: RouterContext,
  permission: Permission,
  load: () => Promise<unknown>,
): Promise<void> {
  const viewer = await queryClient.query(viewerQueryOptions(api));
  if (viewer.permissions.includes(permission)) await load();
}

type AdminErrorProps = ErrorComponentProps & { permission: Permission; heading: ReactNode };

/** The error of an admin screen: why the viewer may not use it for a 403 missing-permission, else the usual one. */
export function AdminError({ permission, heading, ...props }: AdminErrorProps) {
  if (!isMissingPermission(props.error)) return <RouteError {...props} heading={heading} />;
  return (
    <>
      <PageHeading>{heading}</PageHeading>
      <MissingPermission permission={permission} />
    </>
  );
}
