// SPDX-License-Identifier: AGPL-3.0-only

// A document of Terms and policies, at /admin/documents/{document} (terms, privacy-policy, commerce-disclosure): its
// text in Markdown, a form saved as a whole under the entry; back goes to the entry. Leaving it with changes that are
// not saved is held back until the reader says whether to. Without a session it asks the reader to sign in.

import { AdminDocumentScreen, loadAdminDocument, useAdminDocumentTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubHeading, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/documents/$document")({
  loader: ({ context }) => adminSubLoad(context, "manage_settings", loadAdminDocument),
  component: AdminDocumentPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function AdminDocumentPage() {
  const { data } = Route.useRouteContext();
  const { document } = Route.useParams();
  return (
    <AdminSubPage
      key={document}
      entry="documents"
      signedIn={data !== null}
      returnTo={`/admin/documents/${document}`}
      heading={<Heading />}
    >
      {(sub) => <AdminDocumentScreen path={document} {...sub} />}
    </AdminSubPage>
  );
}

/** The heading, which names what the screen shows once it is read. */
function Heading() {
  const { document } = Route.useParams();
  return <AdminSubHeading entry="documents" title={useAdminDocumentTitle(document)} />;
}
