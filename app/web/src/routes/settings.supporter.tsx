// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import {
  ownSupporterSubscriptionQueryOptions,
  SupporterSubscriptionSettings,
  type SupporterReturn,
  supporterPlansQueryOptions,
  syncOwnSubscription,
} from "../features/supporters";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { isTauriApp } from "../lib/platform";
import { SettingsHeading } from "./-layout/SettingsHeading";

/**
 * Where the payment provider sends the browser back (ADR-0072 §6, §8, web-client/README.md S3): `checkout` from the hosted
 * checkout, completed or left, and `portal` from the portal. Anything else is ignored.
 */
const searchSchema = z.object({
  checkout: z.enum(["success", "canceled"]).optional().catch(undefined),
  portal: z.enum(["returned"]).optional().catch(undefined),
});

/**
 * The user's page of the supporter plans (ADR-0072 §6, §8, §13): subscribing through the provider's hosted checkout,
 * and the user's subscription with changing, cancelling and resuming it. Back from the provider, the page asks the
 * server once to re-read the subscription. The Tauri app reads nothing here and says the page is not available (§14).
 */
export const Route = createFileRoute("/settings/supporter")({
  staticData: { title: "client:supporter.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): { returned: SupporterReturn } => ({
    returned:
      search.checkout === "success"
        ? "checkout-success"
        : search.checkout === "canceled"
          ? "checkout-canceled"
          : search.portal === "returned"
            ? "portal"
            : undefined,
  }),
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api }, deps: { returned } }) => {
    if (isTauriApp()) return;
    // A failure does not fail the page: the page shows it, or the disabled program (404), in its body.
    if (returned === "checkout-success" || returned === "portal")
      await syncOwnSubscription(api, queryClient).catch(() => undefined);
    await Promise.all([
      queryClient.query({ ...supporterPlansQueryOptions(api), staleTime: Infinity }).catch(() => undefined),
      queryClient.query({ ...ownSupporterSubscriptionQueryOptions(api), staleTime: Infinity }).catch(() => undefined),
    ]);
  },
  component: SupporterPage,
});

function SupporterPage() {
  const t = useT();
  const { returned } = Route.useLoaderDeps();
  return (
    <>
      <SettingsHeading>{t("client:supporter.title")}</SettingsHeading>
      <SupporterSubscriptionSettings returned={returned} />
    </>
  );
}
