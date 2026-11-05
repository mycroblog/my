// SPDX-License-Identifier: AGPL-3.0-only
// The way to translate a post on this server for this viewer, if there is one (ADR-0148 §5, §6, §12). It is the one
// place that chooses: the card, its messages, and its states work with any Translator. A provider that may receive the
// post comes first (ADR-0148 §6), and otherwise the browser, which sends nothing anywhere and so takes every visibility
// and needs no sign-in. The operator's order of the providers stays on the server, which also decides by the post's
// effective visibility; `Status.visibility` is the visibility the author chose, so the server can still refuse a post
// whose author is limited.
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { shellQueryReader } from "../../../lib/query";
import { useViewer } from "../../session";
import type { Status } from "../api/status";
import { browserTranslationQueryOptions, hasBrowserTranslator, translateInBrowser } from "../api/browserTranslation";
import { isInLanguage, translateOnServer, type Translator } from "../api/translation";

/**
 * The translator of `post` into `target`, or null when nothing can translate it. A provider needs a signed-in reader,
 * since the server counts what it costs against the user (ADR-0148 §7), and must be allowed the post's visibility; the
 * browser needs the operator's consent, the API, and a pair of languages that it does not report `unavailable`.
 */
export function useTranslator(post: Status, target: string): Translator | null {
  const api = useApi();
  const viewer = useViewer();
  // The shell reads GET /instance on every page and handles its failures; this only reads its answer.
  const instance = useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader });
  const providers = instance.data?.translation.providers ?? [];
  const onServer =
    viewer.status === "signed-in" && providers.some((provider) => provider.visibilities.includes(post.visibility));
  const inBrowser = useQuery({
    ...browserTranslationQueryOptions(post.language, target),
    enabled:
      !onServer &&
      instance.data?.translation.browser === true &&
      hasBrowserTranslator() &&
      !isInLanguage(post.language, target),
  });
  if (onServer) return (targetLanguage) => translateOnServer(api, post.id, targetLanguage);
  if (inBrowser.data === true) {
    return (targetLanguage, onDownload) => translateInBrowser(post, targetLanguage, onDownload);
  }
  return null;
}
