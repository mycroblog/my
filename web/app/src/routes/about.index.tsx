// SPDX-License-Identifier: AGPL-3.0-only

// The server's about page, at /about (FR-705, FR-718, FR-725, FR-810, FR-903, FR-905, FR-914), for everyone, signed in
// or not, which the footer of the shell, the landing page and the footer of a page of its own lead to: the instance,
// and the domains it moderates where it lists them to the reader; a visitor of a server that lists them to signed-in
// readers is offered to sign in, which comes back here. A file of its own (`about.index`), so that the costs, the
// supporters and the documents stay pages of their own beside it.

import { aboutPagePath } from "@my/app-components";
import { domainActionsShown, useAboutInstance, useInstanceDomainActions } from "@my/app-data";
import { AboutView, type AboutDomains } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import icon from "../../../brand/icon.svg";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";
import { signInReturningTo } from "../sign-in";

export const Route = createFileRoute("/about/")({
  component: AboutPage,
  // A page of its own opens at its top.
  staticData: { scroll: "top" },
});

function AboutPage() {
  const { data, config } = Route.useRouteContext();
  const t = useStrings();
  const { instance, retry } = useAboutInstance();
  const shown = domainActionsShown(instance, data !== null);
  const list = useInstanceDomainActions(shown);
  const domains: AboutDomains = shown
    ? { list }
    : instance?.domain_actions_visibility === "signed_in"
      ? { signIn: signInReturningTo(aboutPagePath) }
      : undefined;
  return (
    <>
      <ScreenHeading>{t("app.about", { name: instance?.name ?? config.instanceName })}</ScreenHeading>
      <AboutView instance={instance} retry={retry} domains={domains} builtInIcon={icon} signedIn={data !== null} />
    </>
  );
}
