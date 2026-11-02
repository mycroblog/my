// SPDX-License-Identifier: AGPL-3.0-only
// Whether registration and sign-in are limited to the login providers now (GET /instance): the settings then stop
// offering a new password or passkey, which could not sign in; the server refuses them either way.
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { instanceQueryOptions } from "../api/me";

export function useLoginProvidersOnly(): boolean {
  const api = useApi();
  return useQuery(instanceQueryOptions(api)).data?.login_providers_only ?? false;
}
