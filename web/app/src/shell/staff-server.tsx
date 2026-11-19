// SPDX-License-Identifier: AGPL-3.0-only

// A screen under an entry of the administration (a role at /admin/roles/{key}, a new webhook at /admin/webhooks/new),
// as each route of one renders it: under its title once its entry is open to the reader, held while it has changes
// not saved; that the address names no screen open to them once their permissions turn out not to open the entry;
// and the ask to sign in without a session. Its loader reads what the screen shows once the reader's permissions open
// it.

import { viewerQuery, type AppData, type Permission } from "@my/app-data";
import { StaffMissing, useStaffEntry, useStaffFacts, useStaffTitle, type AdminSubProps } from "@my/app-features";
import { useRouter } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useStrings } from "../locale";
import type { RouterContext } from "../router";
import { screenLoad } from "../screen-load";
import { useUnsavedGuard } from "../unsaved-guard";
import { ScreenHeading } from "./heading";
import { SignedOut } from "./signed-out";

/**
 * What the loader of a screen under an entry of the administration waits for: who the reader is, then, once their
 * permissions hold `permission`, what `load` reads.
 */
export function adminSubLoad(
  context: RouterContext,
  permission: Permission,
  load: (data: AppData) => Promise<void>,
): Promise<void> | undefined {
  return screenLoad(context, async (data) => {
    const viewer = await data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
    if (viewer?.permissions.includes(permission) === true) await load(data);
  });
}

type AdminSubPageProps = {
  /** The entry the screen is under, whose permission opens it. */
  readonly entry: string;
  /** The feature's screen, given the way to go and the guard of its unsaved changes. */
  readonly children: (sub: AdminSubProps) => ReactNode;
};

/** The heading of a screen under the entry of `entry`: the entry's name. */
function AdminSubHeading({ entry }: { entry: string }) {
  const t = useStrings();
  return <ScreenHeading>{useStaffTitle("admin", entry) ?? t("app.admin")}</ScreenHeading>;
}

/** The screen, held while it says it has changes not saved. */
function AdminSub({ entry, children }: AdminSubPageProps) {
  const router = useRouter();
  const { onUnsaved, leaving } = useUnsavedGuard();
  return (
    <>
      <AdminSubHeading entry={entry} />
      {children({
        go: (href) => {
          void router.navigate({ href });
        },
        onUnsaved,
        leaving,
      })}
    </>
  );
}

/** The screen, unless the reader's permissions turn out not to open its entry: then that the address names none. */
function AdminOpened(props: AdminSubPageProps) {
  const t = useStrings();
  const known = useStaffFacts().known;
  const open = useStaffEntry("admin", props.entry);
  if (!known) return <ScreenHeading>{t("app.admin")}</ScreenHeading>;
  if (open === undefined)
    return (
      <>
        <ScreenHeading>{t("app.admin")}</ScreenHeading>
        <StaffMissing area="admin" />
      </>
    );
  return <AdminSub {...props} />;
}

/**
 * A screen under an entry of the administration at `returnTo`, for a signed-in reader; without a session, the ask to
 * sign in.
 */
export function AdminSubPage({
  signedIn,
  returnTo,
  ...props
}: AdminSubPageProps & { readonly signedIn: boolean; readonly returnTo: string }) {
  const t = useStrings();
  if (signedIn) return <AdminOpened {...props} />;
  return (
    <>
      <ScreenHeading>{t("app.admin")}</ScreenHeading>
      <SignedOut returnTo={returnTo} />
    </>
  );
}
