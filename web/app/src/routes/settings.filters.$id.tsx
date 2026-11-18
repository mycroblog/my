// SPDX-License-Identifier: AGPL-3.0-only

// A keyword filter's editor, at /settings/filters/new for a new one and /settings/filters/{id} for one that is there,
// the screen under the settings' entry Keyword filters (ADR-0110 §5): a form saved as a whole; back, saving and
// deleting go to the entry. Leaving it with changes that are not saved is held back, whichever way the reader leaves (a
// link, the navigation, Back, closing the tab), until they say whether to: the screen asks, here the router waits
// (unsaved-guard.ts). An address that asks about signing out over the screen is no leaving, and neither is the way
// back once the filter is saved or deleted; a session that is ending holds nothing. Without a session it asks the
// reader to sign in.

import { FilterEdit, loadFilterEdit, useFilterEditTitle } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";
import { useUnsavedGuard } from "../unsaved-guard";

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
  const { onUnsaved, leaving } = useUnsavedGuard();
  return (
    <FilterEdit
      id={id}
      onBack={() => {
        void router.navigate({ to: "/settings/$entry", params: { entry: "filters" } });
      }}
      onUnsaved={onUnsaved}
      leaving={leaving}
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
