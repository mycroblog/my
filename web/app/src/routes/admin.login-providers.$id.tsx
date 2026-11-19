// SPDX-License-Identifier: AGPL-3.0-only

// A login provider, at /admin/login-providers/{id}: its callback URL, its settings as a form saved as a whole, and
// Delete, under the entry Login providers. Leaving it with changes that are not saved is held back until the reader
// says whether to. Without a session it asks the reader to sign in.

import { LoginProviderScreen, loadLoginProviders, useLoginProviderTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubHeading, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/login-providers/$id")({
  loader: ({ context }) => adminSubLoad(context, "manage_login_providers", loadLoginProviders),
  component: LoginProviderPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function LoginProviderPage() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return (
    <AdminSubPage
      key={id}
      entry="login-providers"
      signedIn={data !== null}
      returnTo={`/admin/login-providers/${id}`}
      heading={<Heading />}
    >
      {(sub) => <LoginProviderScreen id={id} {...sub} />}
    </AdminSubPage>
  );
}

/** The heading, which names what the screen shows once it is read. */
function Heading() {
  const { id } = Route.useParams();
  return <AdminSubHeading entry="login-providers" title={useLoginProviderTitle(id)} />;
}
