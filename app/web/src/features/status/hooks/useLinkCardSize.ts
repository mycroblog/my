// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { shellQueryReader } from "../../../lib/query";
import { linkCardSizeKey, useStorage } from "../../../lib/storage";
import { preferencesQueryOptions } from "../api/status";

/**
 * The size of link cards in the signed-in user's preferences. Until they load, the size they had last on this browser,
 * so that a card does not change its layout on the first load. Large when nobody is signed in.
 */
export function useLinkCardSize(): "large" | "small" {
  const api = useApi();
  const status = useSessionStatus();
  const last = useStorage(linkCardSizeKey);
  const { data } = useQuery({
    ...preferencesQueryOptions(api),
    ...shellQueryReader,
    enabled: status === "signed-in",
  });
  return data?.link_card_size ?? last;
}
