// SPDX-License-Identifier: AGPL-3.0-only

// The settings, at /settings and under it (ADR-0110 §5): the menu (settings.index.tsx) and the screens of its entries
// (settings.$entry.tsx). `?sign_out`, on any of them, asks whether to sign out, over the screen: Sign out… of the
// menu leads there, and Cancel leads back. Its loader waits for what the menu reads of the reader that the app has not
// read yet (whether they may create invites), so that the menu, at /settings and beside every screen, comes with every
// entry it has.

import { loadSettingsFacts } from "@my/app-features";
import { createFileRoute, Outlet, useCanGoBack, useRouter } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SignOutConfirm } from "../sign-out";

export const Route = createFileRoute("/settings")({
  validateSearch: (search: Record<string, unknown>): { sign_out?: true } =>
    search["sign_out"] === undefined || search["sign_out"] === false ? {} : { sign_out: true },
  loader: ({ context }) => screenLoad(context, loadSettingsFacts),
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
