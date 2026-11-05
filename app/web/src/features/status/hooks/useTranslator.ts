// SPDX-License-Identifier: AGPL-3.0-only
// The way to translate a post on this server for this viewer, if there is one (ADR-0148 §5, §6, §12). It is the one
// place that chooses: the card, its messages, and its states work with any Translator. The operator's order of the
// providers stays on the server, which also decides by the post's effective visibility; `Status.visibility` is the
// visibility the author chose, so the server can still refuse a post whose author is limited.
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { shellQueryReader } from "../../../lib/query";
import { useViewer } from "../../session";
import type { Status } from "../api/status";
import { translateOnServer, type Translator } from "../api/translation";

/**
 * The translator of `post`, or null when nothing can translate it: no enabled provider may receive its visibility,
 * or nobody is signed in, since the server counts what a provider costs against the user (ADR-0148 §7).
 */
export function useTranslator(post: Status): Translator | null {
  const api = useApi();
  const viewer = useViewer();
  // The shell reads GET /instance on every page and handles its failures; this only reads its answer.
  const instance = useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader });
  const providers = instance.data?.translation.providers ?? [];
  if (viewer.status !== "signed-in") return null;
  if (!providers.some((provider) => provider.visibilities.includes(post.visibility))) return null;
  return (targetLanguage) => translateOnServer(api, post.id, targetLanguage);
}
