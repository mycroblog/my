// SPDX-License-Identifier: AGPL-3.0-only

// A keyword filter's editor, at /settings/filters/new for a new one and /settings/filters/{id} for one that is there,
// the screen under the settings' entry Keyword filters (ADR-0110 §5): a form saved as a whole; back, saving and
// deleting go to the entry. Leaving it with changes that are not saved is held back, whichever way the reader leaves (a
// link, the navigation, Back, closing the tab), until they say whether to: the screen asks, here the router waits. An
// address that asks about signing out over the screen is no leaving, and neither is the way back once the filter is
// saved or deleted. Without a session it asks the reader to sign in.

import { FilterEdit, loadFilterEdit, useFilterEditTitle } from "@my/app-features";
import { createFileRoute, useBlocker, useRouter } from "@tanstack/react-router";
import { useCallback, useRef } from "react";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

/** The filter of the address's last part; undefined for a new one. */
const filterOf = (id: string) => (id === "new" ? undefined : id);

export const Route = createFileRoute("/settings/filters/$id")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadFilterEdit(data, filterOf(params.id))),
  component: FilterEditScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The form, for a signed-in reader, and the router held while it has changes that are not saved. */
function Edit({ id }: { id: string | undefined }) {
  const router = useRouter();
  // Read as the router asks, so that the way back right after a save is not held by a change the save just made.
  const unsaved = useRef(false);
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) => unsaved.current && current.pathname !== next.pathname,
    enableBeforeUnload: () => unsaved.current,
    withResolver: true,
  });
  const onUnsaved = useCallback((now: boolean) => {
    unsaved.current = now;
  }, []);
  return (
    <FilterEdit
      id={id}
      onBack={() => {
        void router.navigate({ to: "/settings/$entry", params: { entry: "filters" } });
      }}
      onUnsaved={onUnsaved}
      leaving={blocker.status === "blocked" ? { onStay: blocker.reset, onLeave: blocker.proceed } : undefined}
    />
  );
}

function FilterEditScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const filter = filterOf(id);
  const title = useFilterEditTitle(filter);
  return (
    <>
      <ScreenHeading>{title}</ScreenHeading>
      {data === null ? <SignedOut /> : <Edit key={id} id={filter} />}
    </>
  );
}
