// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import {
  ownSupporterSubscriptionQueryOptions,
  ownTipQueryOptions,
  tipAmountsQueryOptions,
  type TipReturn,
  TipSettings,
} from "../features/supporters";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { isTauriApp } from "../lib/platform";
import { SettingsHeading } from "./-layout/SettingsHeading";

/**
 * Where the payment provider sends the browser back (ADR-0162 §13, web-client/README.md S3): `tip`, the tip the
 * checkout was of, with `checkout`, completed or left. Anything else is ignored.
 */
const searchSchema = z.object({
  tip: z.string().min(1).max(64).optional().catch(undefined),
  checkout: z.enum(["success", "canceled"]).optional().catch(undefined),
});

/**
 * The user's page of the tips (ADR-0162 §13, §17, §19): a one-time tip for running the server, with nothing in
 * return, of one of the amounts the operators chose, through the provider's hosted checkout; back from it, the page
 * reads the tip until its payment is confirmed and then shows only the thanks. The `_` keeps the page out of the
 * layout of `/settings/supporter`. The Tauri app reads nothing here and says the page is not available (§3).
 */
export const Route = createFileRoute("/settings/supporter_/tip")({
  staticData: { title: "client:tip.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): { returned: TipReturn } => ({
    returned:
      search.tip !== undefined && search.checkout !== undefined
        ? { tipId: search.tip, checkout: search.checkout }
        : undefined,
  }),
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api }, deps: { returned } }) => {
    if (isTauriApp()) return;
    // A failure does not fail the page: the page shows it, or the server that takes no tips (404), in its body.
    await Promise.all([
      queryClient.query({ ...tipAmountsQueryOptions(api), staleTime: Infinity }).catch(() => undefined),
      queryClient.query({ ...ownSupporterSubscriptionQueryOptions(api), staleTime: Infinity }).catch(() => undefined),
      returned === undefined
        ? undefined
        : queryClient.query(ownTipQueryOptions(api, returned.tipId)).catch(() => undefined),
    ]);
  },
  component: TipPage,
});

function TipPage() {
  const t = useT();
  const { returned } = Route.useLoaderDeps();
  return (
    <>
      <SettingsHeading>{t("client:tip.title")}</SettingsHeading>
      <TipSettings returned={returned} />
    </>
  );
}
