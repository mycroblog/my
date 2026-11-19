// SPDX-License-Identifier: AGPL-3.0-only

// The server's finance for a holder of `view_finance`, at /finance, a place of the menu for them (ADR-0070 §1, §5):
// the totals of its costs and income. Without a session it asks the reader to sign in.

import { FinanceScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/finance")({
  component: FinancePage,
  // A place of the menu opens at its top, where the latest periods come first.
  staticData: { scroll: "top" },
});

function FinancePage() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.finance")}</ScreenHeading>
      {data === null ? <SignedOut returnTo="/finance" /> : <FinanceScreen />}
    </>
  );
}
