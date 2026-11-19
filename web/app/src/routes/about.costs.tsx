// SPDX-License-Identifier: AGPL-3.0-only

// The server's costs and income as it publishes them, at /about/costs (ADR-0067 §8, §17, §22), for everyone, signed in
// or not: the funding meter of the column beside the timeline leads here, and the about page links to it.

import { PublicFinanceScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";

export const Route = createFileRoute("/about/costs")({
  component: CostsPage,
  staticData: { scroll: "top" },
});

function CostsPage() {
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.finance_public")}</ScreenHeading>
      <PublicFinanceScreen />
    </>
  );
}
