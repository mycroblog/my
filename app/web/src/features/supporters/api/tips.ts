// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the tips as users see them (web-client/README.md S1, ADR-0162 §6, §7, §13, §17): the
// amounts that a tip may be (GET /instance/tip_amounts, a 404 while the server takes no tips), starting a tip at the
// payment provider's hosted checkout (POST /supporter/tips), and the state of the user's tip when the checkout returns
// (GET /supporter/tips/{id}, which makes the server read an open tip's checkout again). A tip is a one-time payment
// to the server's operation with nothing in return; nothing here keeps a list of the user's tips.
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";

type Schemas = components["schemas"];

export type Money = Schemas["Money"];
export type SupporterTip = Schemas["SupporterTip"];
export type SupporterTipCheckout = Schemas["SupporterTipCheckout"];

/** The path of the user's page of the tips, where the provider's checkout returns too (`startSupporterTip`). */
export const tipPath = "/settings/supporter/tip";

/** The problem types of a start of a tip that the screen tells apart (api/spec paths/supporter_tips.yaml). */
export const tipProblems = {
  ineligible: "/api/v1/problems/supporter-ineligible",
  amount: "/api/v1/problems/supporter-tip-amount",
  pending: "/api/v1/problems/supporter-tip-pending",
  closing: "/api/v1/problems/service-closing",
} as const;

/** The amounts that a tip may be, smallest first, in one currency; a 404 while the server takes no tips. */
export function tipAmountsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance/tip_amounts");
}

/** A tip of the user (GET /supporter/tips/{id}); another user's tip and an unknown one are a 404. */
export function ownTipQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/supporter/tips/{id}", { params: { path: { id } } });
}

/** Starts a tip of one of the amounts at the provider's hosted checkout (POST /supporter/tips), as the options of useMutation. */
export function startTipMutation(api: Api) {
  return {
    mutationKey: ["post", "/supporter/tips"] as const,
    mutationFn: async (amount: Money): Promise<SupporterTipCheckout> => {
      const { data, error } = await api.fetch.POST("/supporter/tips", { body: { amount } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}
