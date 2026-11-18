// SPDX-License-Identifier: AGPL-3.0-only

// The settings, at /settings and under it (ADR-0110 §5): the menu (settings.index.tsx) and the screens of its entries
// (settings.$entry.tsx). `?sign_out`, on any of them, asks whether to sign out, over the screen: Sign out… of the
// menu leads there, and Cancel leads back.

import { createFileRoute, Outlet, useCanGoBack, useRouter } from "@tanstack/react-router";
import { SignOutConfirm } from "../sign-out";

export const Route = createFileRoute("/settings")({
  validateSearch: (search: Record<string, unknown>): { sign_out?: true } =>
    search["sign_out"] === undefined || search["sign_out"] === false ? {} : { sign_out: true },
  component: Settings,
  // The screens under it say their own.
  staticData: { scroll: "top" },
});

function Settings() {
  const { data } = Route.useRouteContext();
  const { sign_out: asking } = Route.useSearch();
  const router = useRouter();
  const canGoBack = useCanGoBack();
  return (
    <>
      <Outlet />
      <SignOutConfirm
        asking={data !== null && asking === true}
        onCancel={() => {
          // Back to the screen the reader asked from; an address opened with the question has none before it, and
          // becomes the address of the screen it is over.
          if (canGoBack) router.history.back();
          else router.history.replace(router.state.location.pathname);
        }}
      />
    </>
  );
}
